package main

import (
	"encoding/binary"
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

func main() {
	if len(os.Args) != 2 {
		panic("DLL path required")
	}
	dll, err := syscall.LoadDLL(os.Args[1])
	if err != nil {
		panic(err)
	}
	defer dll.Release()
	if os.Getenv("POEM_CHECK_GUARD_UNWIND") == "1" {
		nt := syscall.NewLazyDLL("ntdll.dll")
		var base uintptr
		function, _, _ := nt.NewProc("RtlLookupFunctionEntry").Call(uintptr(dll.Handle)+0x6850, uintptr(unsafe.Pointer(&base)), 0)
		if function == 0 {
			panic("DllMain unwind entry missing")
		}
		for _, pcOffset := range []uintptr{5, 20} {
			var stack [256]byte
			var context [1232]byte
			originalSP := uintptr(unsafe.Pointer(&stack[128]))
			const returnAddress = uint64(0x12345678)
			binary.LittleEndian.PutUint64(stack[128:], returnAddress)
			sp := originalSP
			if pcOffset == 20 {
				sp -= 56
			}
			binary.LittleEndian.PutUint64(context[152:], uint64(sp))
			binary.LittleEndian.PutUint64(context[248:], uint64(uintptr(dll.Handle)+0x6850+pcOffset))
			var handlerData, frame uintptr
			nt.NewProc("RtlVirtualUnwind").Call(0, base, uintptr(dll.Handle)+0x6850+pcOffset, function, uintptr(unsafe.Pointer(&context[0])), uintptr(unsafe.Pointer(&handlerData)), uintptr(unsafe.Pointer(&frame)), 0)
			if binary.LittleEndian.Uint64(context[248:]) != returnAddress || binary.LittleEndian.Uint64(context[152:]) != uint64(originalSP+8) {
				panic(fmt.Sprintf("unwind failed at offset %d", pcOffset))
			}
		}
		fmt.Println("guardAndManagedUnwind=ok")
	}

	if os.Getenv("POEM_NAOT_OBSERVE") == "1" {
		for _, path := range []string{os.Getenv("POEM_EXTRA_GAME"), os.Getenv("POEM_EXTRA_VISUAL")} {
			if path != "" {
				extra, err := syscall.LoadDLL(path)
				if err != nil {
					panic(err)
				}
				defer extra.Release()
			}
		}
	}
	k := syscall.NewLazyDLL("kernel32.dll")
	page, _, e := k.NewProc("VirtualAlloc").Call(0, 4096, 0x3000, 0x40)
	if page == 0 {
		panic(e)
	}
	defer k.NewProc("VirtualFree").Call(page, 0, 0x8000)
	mem := unsafe.Slice((*byte)(unsafe.Pointer(page)), 4096)
	// An entirely native callback increments a native counter. No Go callback
	// or Go-managed worker is forcibly terminated during the original comparison.
	callback := []byte{0x48, 0xb8}
	callback = binary.LittleEndian.AppendUint64(callback, uint64(page+0x300))
	callback = append(callback, 0xf0, 0xff, 0x00, 0xc3)
	copy(mem[0x100:], callback)
	index, _, e := k.NewProc("FlsAlloc").Call(page + 0x100)
	if uint32(index) == 0xffffffff {
		panic(e)
	}
	defer k.NewProc("FlsFree").Call(index)
	if os.Getenv("POEM_FLS_OBSERVE") == "1" {
		get := k.NewProc("FlsGetValue")
		if err := get.Find(); err != nil {
			panic(err)
		}
		callback = []byte{0x48, 0xb8}
		callback = binary.LittleEndian.AppendUint64(callback, uint64(page+0x300))
		callback = append(callback, 0xf0, 0xff, 0x00, 0x48, 0x89, 0x48, 0x08, 0x48, 0x83, 0xec, 0x28, 0xb9)
		callback = binary.LittleEndian.AppendUint32(callback, uint32(index))
		callback = append(callback, 0x48, 0xb8)
		callback = binary.LittleEndian.AppendUint64(callback, uint64(get.Addr()))
		callback = append(callback, 0xff, 0xd0, 0x48, 0xba)
		callback = binary.LittleEndian.AppendUint64(callback, uint64(page+0x310))
		callback = append(callback, 0x48, 0x89, 0x02, 0x48, 0x83, 0xc4, 0x28, 0xc3)
		copy(mem[0x100:], callback)
	}

	flsset := k.NewProc("FlsSetValue")
	if err := flsset.Find(); err != nil {
		panic(err)
	}
	entry := []byte{0x48, 0x83, 0xec, 0x28, 0xb9}
	entry = binary.LittleEndian.AppendUint32(entry, uint32(index))
	entry = append(entry, 0xba, 1, 0, 0, 0, 0x48, 0xb8)
	entry = binary.LittleEndian.AppendUint64(entry, uint64(flsset.Addr()))
	entry = append(entry, 0xff, 0xd0, 0x48, 0xb8)
	entry = binary.LittleEndian.AppendUint64(entry, uint64(dll.Handle)+0x8d70)
	entry = append(entry, 0xff, 0xd0, 0x31, 0xc0, 0x48, 0x83, 0xc4, 0x28, 0xc3)

	if os.Getenv("POEM_NAOT_OBSERVE") == "1" {
		init, err := dll.FindProc("Initialize")
		if err != nil {
			panic(err)
		}
		entry = []byte{0x48, 0x83, 0xec, 0x28, 0x48, 0xb8}
		entry = binary.LittleEndian.AppendUint64(entry, uint64(init.Addr()))
		entry = append(entry, 0xff, 0xd0, 0x31, 0xc0, 0x48, 0x83, 0xc4, 0x28, 0xc3)
	}
	copy(mem, entry)
	if os.Getenv("POEM_DLLMAIN_OBSERVE") == "1" {
		proc, err := dll.FindProc("DllMain")
		if err != nil {
			panic(err)
		}
		index := *(*uint32)(unsafe.Pointer(uintptr(dll.Handle) + 0x1f955c))
		entry = []byte{0x48, 0x83, 0xec, 0x28}
		capture := func(address uintptr) {
			entry = append(entry, 0x65, 0x48, 0x8b, 0x04, 0x25, 0x58, 0, 0, 0, 0xb9)
			entry = binary.LittleEndian.AppendUint32(entry, index)
			entry = append(entry, 0x48, 0x8b, 0x04, 0xc8, 0x8b, 0x40, 0x68, 0x48, 0xa3)
			entry = binary.LittleEndian.AppendUint64(entry, uint64(address))
		}
		capture(page + 0x320)
		for _, reason := range []byte{2, 3} {
			entry = append(entry, 0x31, 0xc9, 0xba, reason, 0, 0, 0, 0x45, 0x31, 0xc0, 0x48, 0xb8)
			entry = binary.LittleEndian.AppendUint64(entry, uint64(proc.Addr()))
			entry = append(entry, 0xff, 0xd0, 0x48, 0xa3)
			entry = binary.LittleEndian.AppendUint64(entry, uint64(page+0x340+uintptr(reason)*8))
		}
		capture(page + 0x328)
		entry = append(entry, 0x31, 0xc0, 0x48, 0x83, 0xc4, 0x28, 0xc3)
		copy(mem, entry)
	}
	k.NewProc("FlushInstructionCache").Call(^uintptr(0), page, 4096)
	var nativeTid uint32
	thread, _, e := k.NewProc("CreateThread").Call(0, 0, page, 0, 0, uintptr(unsafe.Pointer(&nativeTid)))
	if thread == 0 {
		panic(e)
	}
	defer syscall.CloseHandle(syscall.Handle(thread))
	result, _, _ := k.NewProc("WaitForSingleObject").Call(thread, 15000)
	if result != 0 {
		panic(fmt.Sprintf("native worker wait: %x", result))
	}

	if os.Getenv("POEM_NAOT_OBSERVE") == "1" {
		readq := func(address uint64) uint64 {
			var result uint64
			var read uintptr
			ok, _, err := k.NewProc("ReadProcessMemory").Call(^uintptr(0), uintptr(address), uintptr(unsafe.Pointer(&result)), 8, uintptr(unsafe.Pointer(&read)))
			if ok == 0 || read != 8 {
				panic(fmt.Sprintf("read %x: %v", address, err))
			}
			return result
		}
		instance := readq(uint64(dll.Handle) + 0x1f3120)
		store := readq(instance)
		head := readq(store)
		listed := false
		count := 0
		seen := map[uint64]bool{}
		for head != 0 && !seen[head] && count < 512 {
			seen[head] = true
			count++
			if readq(head+0xb8) == uint64(nativeTid) {
				listed = true
			}
			head = readq(head + 0x58)
		}
		fmt.Printf("nativeTid=%d exitedThreadStillListed=%t listNodes=%d\n", nativeTid, listed, count)
	}
	var code uint32
	if os.Getenv("POEM_DLLMAIN_OBSERVE") == "1" {
		fmt.Printf("threadStateBefore=%x threadStateAfter=%x attachResult=%d detachResult=%d\n", binary.LittleEndian.Uint64(mem[0x320:]), binary.LittleEndian.Uint64(mem[0x328:]), binary.LittleEndian.Uint64(mem[0x350:]), binary.LittleEndian.Uint64(mem[0x358:]))
	}
	k.NewProc("GetExitCodeThread").Call(thread, uintptr(unsafe.Pointer(&code)))
	fmt.Printf("exitCode=%d cleanupCallbacks=%d\n", code, binary.LittleEndian.Uint32(mem[0x300:]))
	if os.Getenv("POEM_FLS_OBSERVE") == "1" {
		fmt.Printf("callbackArgument=%d FlsGetValueInsideCallback=%d\n", binary.LittleEndian.Uint64(mem[0x308:]), binary.LittleEndian.Uint64(mem[0x310:]))
	}
}
