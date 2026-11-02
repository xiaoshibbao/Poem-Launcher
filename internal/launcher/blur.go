//go:build windows

package launcher

import (
	"encoding/binary"
	"fmt"
	"golang.org/x/sys/windows"
	"time"
	"unsafe"
)

type blurControl struct {
	handle windows.Handle
	view   uintptr
	fps    int
	refs   int
	stop   chan struct{}
	done   chan struct{}
}

func newBlurControl(fps int) (*blurControl, error) {
	return newBlurControlNamed(fps, "SRUnlocker")
}
func newBlurControlNamed(fps int, mappingName string) (*blurControl, error) {
	name, _ := windows.UTF16PtrFromString(mappingName)
	handle, err := windows.CreateFileMapping(windows.InvalidHandle, nil, windows.PAGE_READWRITE, 0, 4096, name)
	if err != nil {
		if handle != 0 {
			windows.CloseHandle(handle)
		}
		if err == windows.ERROR_ALREADY_EXISTS {
			return nil, fmt.Errorf("去虚化控制已被其他启动器占用，请先关闭占用配置的启动器及其游戏")
		}
		return nil, err
	}
	view, err := windows.MapViewOfFile(handle, windows.FILE_MAP_WRITE, 0, 0, 4096)
	if err != nil {
		windows.CloseHandle(handle)
		return nil, err
	}
	control := &blurControl{handle: handle, view: view, fps: fps, stop: make(chan struct{}), done: make(chan struct{})}
	data := unsafe.Slice((*byte)(unsafe.Pointer(view)), 8)
	binary.LittleEndian.PutUint32(data[:4], uint32(fps))
	binary.LittleEndian.PutUint32(data[4:], 0) // FOV delta stays at the game's default.
	// Match the original launcher's continuous configuration publication.
	go func() {
		defer close(control.done)
		ticker := time.NewTicker(16 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-control.stop:
				return
			case <-ticker.C:
				binary.LittleEndian.PutUint32(data[:4], uint32(fps))
				binary.LittleEndian.PutUint32(data[4:], 0)
			}
		}
	}()
	return control, nil
}
func (c *blurControl) close() {
	if c == nil {
		return
	}
	if c.stop != nil {
		close(c.stop)
		<-c.done
		c.stop = nil
	}
	if c.view != 0 {
		windows.UnmapViewOfFile(c.view)
		c.view = 0
	}
	if c.handle != 0 {
		windows.CloseHandle(c.handle)
		c.handle = 0
	}
}
