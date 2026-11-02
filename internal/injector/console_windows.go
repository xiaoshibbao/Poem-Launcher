//go:build windows

package injector

import (
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"strconv"
	"strings"
	"time"
	"unsafe"
)

// ConsoleRecorderMain runs only in an isolated helper process.
func ConsoleRecorderMain() bool {
	if len(os.Args) != 4 || os.Args[1] != "--poem-console-recorder" {
		return false
	}
	pid, err := strconv.ParseUint(os.Args[2], 10, 32)
	if err != nil {
		return true
	}
	process, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return true
	}
	defer windows.CloseHandle(process)
	file, err := os.OpenFile(os.Args[3], os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return true
	}
	defer file.Close()
	k := windows.NewLazySystemDLL("kernel32.dll")
	free := k.NewProc("FreeConsole")
	attach := k.NewProc("AttachConsole")
	info := k.NewProc("GetConsoleScreenBufferInfo")
	read := k.NewProc("ReadConsoleOutputCharacterW")
	free.Call()
	defer free.Call()
	var console *os.File
	defer func() {
		if console != nil {
			console.Close()
		}
	}()
	lastFailure := ""
	fmt.Fprintf(file, "[%s] recorder started pid=%d\n", time.Now().Format(time.RFC3339), pid)
	attached := false
	last := ""
	type coord struct{ X, Y int16 }
	type rect struct{ Left, Top, Right, Bottom int16 }
	type screen struct {
		Size, Cursor coord
		Attributes   uint16
		Window       rect
		Maximum      coord
	}
	for {
		state, err := windows.WaitForSingleObject(process, 0)
		if err != nil || state == windows.WAIT_OBJECT_0 {
			break
		}
		if !attached {
			ok, _, attachErr := attach.Call(uintptr(pid))
			attached = ok != 0
			if !attached && attachErr.Error() != lastFailure {
				lastFailure = attachErr.Error()
				fmt.Fprintf(file, "AttachConsole: %v\n", attachErr)
			}
		}
		if attached {
			if console == nil {
				console, err = os.OpenFile("CONOUT$", os.O_RDWR, 0)
				if err != nil {
					if err.Error() != lastFailure {
						lastFailure = err.Error()
						fmt.Fprintf(file, "Open console: %v\n", err)
					}
					time.Sleep(500 * time.Millisecond)
					continue
				}
			}
			handle := console.Fd()
			var s screen
			ok, _, _ := info.Call(handle, uintptr(unsafe.Pointer(&s)))
			if ok != 0 && s.Size.X > 0 && s.Size.Y > 0 {
				start := int(s.Cursor.Y) - 160
				if start < 0 {
					start = 0
				}
				rows := int(s.Cursor.Y) - start + 1
				count := rows * int(s.Size.X)
				if count > 0 && count <= 100000 {
					buf := make([]uint16, count)
					var n uint32
					pos := uint32(uint16(start)) << 16
					ok, _, _ := read.Call(handle, uintptr(unsafe.Pointer(&buf[0])), uintptr(count), uintptr(pos), uintptr(unsafe.Pointer(&n)))
					if ok != 0 && int(n) <= len(buf) {
						var lines []string
						for off := 0; off < int(n); off += int(s.Size.X) {
							end := off + int(s.Size.X)
							if end > int(n) {
								end = int(n)
							}
							line := strings.TrimRight(windows.UTF16ToString(buf[off:end]), " ")
							if line != "" {
								lines = append(lines, line)
							}
						}
						text := strings.Join(lines, "\n")
						if text != "" && text != last {
							fmt.Fprintf(file, "\n[%s] console snapshot pid=%d\n%s\n", time.Now().Format(time.RFC3339), pid, text)
							last = text
							file.Sync()
							if stat, err := file.Stat(); err == nil && stat.Size() >= 16*1024*1024 {
								return true
							}
						}
					}
				}
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return true
}
