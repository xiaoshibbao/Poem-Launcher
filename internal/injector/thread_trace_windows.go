//go:build windows

package injector

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

type tracedNode struct {
	address, next, handle uint64
	id                    uint32
	state                 uint32
}

type memoryReader func(uint64, []byte) error

func readThreadNodes(read memoryReader, head uint64) ([]tracedNode, error) {
	var nodes []tracedNode
	seen := map[uint64]bool{}
	for head != 0 {
		if seen[head] || len(nodes) >= 512 {
			return nodes, fmt.Errorf("thread list cycle/limit at 0x%x", head)
		}
		seen[head] = true
		b := make([]byte, 0xc0)
		if err := read(head, b); err != nil {
			return nodes, fmt.Errorf("node 0x%x unreadable: %w", head, err)
		}
		n := tracedNode{address: head, state: binary.LittleEndian.Uint32(b[0x38:]), next: binary.LittleEndian.Uint64(b[0x58:]), handle: binary.LittleEndian.Uint64(b[0x60:]), id: uint32(binary.LittleEndian.Uint64(b[0xb8:]))}
		if n.state & ^uint32(0x1fb) != 0 || n.id == 0 || n.id == 0xffffffff {
			return nodes, fmt.Errorf("node 0x%x invalid state=0x%x tid=%d next=0x%x", head, n.state, n.id, n.next)
		}
		nodes = append(nodes, n)
		head = n.next
	}
	return nodes, nil
}

func processModules(pid uint32) ([]windows.ModuleEntry32, error) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPMODULE|windows.TH32CS_SNAPMODULE32, pid)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(snap)
	entry := windows.ModuleEntry32{Size: uint32(windows.SizeofModuleEntry32)}
	err = windows.Module32First(snap, &entry)
	var mods []windows.ModuleEntry32
	for err == nil {
		mods = append(mods, entry)
		err = windows.Module32Next(snap, &entry)
	}
	if err != windows.ERROR_NO_MORE_FILES {
		return nil, err
	}
	return mods, nil
}

// StartProxyThreadTrace only reads the supported proxy's thread list. It does
// not suspend threads, install hooks, modify memory or change the game state.
func StartProxyThreadTrace(p *Process, path string) (func(), error) {
	var handle windows.Handle
	if err := windows.DuplicateHandle(windows.CurrentProcess(), p.handle, windows.CurrentProcess(), &handle, 0, false, windows.DUPLICATE_SAME_ACCESS); err != nil {
		return nil, err
	}
	failed := true
	defer func() {
		if failed {
			windows.CloseHandle(handle)
		}
	}()
	mods, err := processModules(p.pid)
	if err != nil {
		return nil, err
	}
	var base uint64
	for _, m := range mods {
		if strings.HasPrefix(strings.ToLower(windows.UTF16ToString(m.Module[:])), "poem.proxy") {
			base = uint64(m.ModBaseAddr)
			break
		}
	}
	if base == 0 {
		return nil, fmt.Errorf("proxy module not found")
	}
	read := func(address uint64, b []byte) error {
		var got uintptr
		err := windows.ReadProcessMemory(handle, uintptr(address), &b[0], uintptr(len(b)), &got)
		if err != nil {
			return err
		}
		if got != uintptr(len(b)) {
			return fmt.Errorf("short read: %d", got)
		}
		return nil
	}
	code := make([]byte, 7)
	if err = read(base+0xe3140, code); err != nil {
		return nil, err
	}
	if !bytes.Equal(code, []byte{0x48, 0x8b, 0x05, 0xd9, 0xff, 0x10, 0x00}) {
		return nil, fmt.Errorf("unsupported proxy runtime layout")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	stop, done := make(chan struct{}), make(chan struct{})
	var once sync.Once
	go func() {
		defer close(done)
		defer f.Close()
		defer windows.CloseHandle(handle)
		type tracked struct {
			n      tracedNode
			handle windows.Handle
			start  string
			exited bool
		}
		cache := map[uint64]*tracked{}
		defer func() {
			for _, v := range cache {
				if v.handle != 0 {
					windows.CloseHandle(v.handle)
				}
			}
		}()
		log := func(format string, args ...any) {
			fmt.Fprintf(f, "%s pid=%d %s\n", time.Now().Format(time.RFC3339Nano), p.pid, fmt.Sprintf(format, args...))
		}
		log("trace start proxyBase=0x%x launcherPid=%d", base, os.Getpid())
		query := windows.NewLazySystemDLL("ntdll.dll").NewProc("NtQueryInformationThread")
		tick := time.NewTicker(250 * time.Millisecond)
		defer tick.Stop()
		lastError := ""
		for {
			select {
			case <-stop:
				return
			case <-tick.C:
			}
			if wait, _ := windows.WaitForSingleObject(handle, 0); wait == windows.WAIT_OBJECT_0 {
				log("process exited")
				return
			}
			if stat, e := f.Stat(); e == nil && stat.Size() > 16<<20 {
				log("trace size limit reached")
				return
			}
			b := make([]byte, 8)
			if read(base+0x1f3120, b) != nil {
				continue
			}
			ri := binary.LittleEndian.Uint64(b)
			if ri == 0 {
				continue
			}
			if read(ri, b) != nil {
				continue
			}
			store := binary.LittleEndian.Uint64(b)
			if store == 0 {
				continue
			}
			if read(store, b) != nil {
				continue
			}
			head := binary.LittleEndian.Uint64(b)
			nodes, scanErr := readThreadNodes(read, head)
			present := map[uint64]bool{}
			for _, n := range nodes {
				present[n.address] = true
				old := cache[n.address]
				if old != nil && old.n.id != n.id {
					log("node reused address=0x%x oldTid=%d newTid=%d", n.address, old.n.id, n.id)
					if old.handle != 0 {
						windows.CloseHandle(old.handle)
					}
					delete(cache, n.address)
					old = nil
				}
				if old == nil {
					old = &tracked{n: n}
					cache[n.address] = old
					if n.handle != 0 && n.handle != ^uint64(0) {
						_ = windows.DuplicateHandle(handle, windows.Handle(n.handle), windows.CurrentProcess(), &old.handle, 0, false, windows.DUPLICATE_SAME_ACCESS)
					}
					if old.handle != 0 {
						var start uintptr
						status, _, _ := query.Call(uintptr(old.handle), 9, uintptr(unsafe.Pointer(&start)), unsafe.Sizeof(start), 0)
						if status == 0 {
							old.start = fmt.Sprintf("0x%x", start)
							for _, m := range mods {
								if start >= m.ModBaseAddr && start < m.ModBaseAddr+uintptr(m.ModBaseSize) {
									old.start = fmt.Sprintf("%s+0x%x", windows.UTF16ToString(m.Module[:]), start-m.ModBaseAddr)
									break
								}
							}
						}
					}
					log("registered node=0x%x tid=%d state=0x%x entry=%s next=0x%x", n.address, n.id, n.state, old.start, n.next)
				}
				if old.handle != 0 && !old.exited {
					var code uint32
					if traceThreadExitCode(old.handle, &code) == nil && code != 259 {
						old.exited = true
						log("exited thread remains listed node=0x%x tid=%d code=0x%x entry=%s", n.address, n.id, code, old.start)
					}
				}
			}
			if scanErr != nil {
				if scanErr.Error() != lastError {
					log("list observation: %v", scanErr)
					lastError = scanErr.Error()
					for address, v := range cache {
						if !present[address] {
							var code uint32
							e := traceThreadExitCode(v.handle, &code)
							log("previous node not reached node=0x%x tid=%d entry=%s threadCode=0x%x queryError=%v", address, v.n.id, v.start, code, e)
						}
					}
				}
			} else {
				if lastError != "" {
					log("list readable again")
					lastError = ""
				}
				for address, v := range cache {
					if !present[address] {
						log("unregistered node=0x%x tid=%d entry=%s", address, v.n.id, v.start)
						if v.handle != 0 {
							windows.CloseHandle(v.handle)
						}
						delete(cache, address)
					}
				}
			}
		}
	}()
	failed = false
	return func() { once.Do(func() { close(stop); <-done }) }, nil
}

func traceThreadExitCode(handle windows.Handle, code *uint32) error {
	if handle == 0 {
		return windows.ERROR_INVALID_HANDLE
	}
	ok, _, err := procGetExitCodeThread.Call(uintptr(handle), uintptr(unsafe.Pointer(code)))
	if ok == 0 {
		return err
	}
	return nil
}
