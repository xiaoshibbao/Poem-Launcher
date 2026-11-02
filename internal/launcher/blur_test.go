//go:build windows

package launcher

import (
	"encoding/binary"
	"fmt"
	"testing"
	"time"
	"unsafe"
)

func TestBlurMappingProtocolAndExclusiveOwnership(t *testing.T) {
	name := fmt.Sprintf("Poem-test-%d", time.Now().UnixNano())
	c, err := newBlurControlNamed(144, name)
	if err != nil {
		t.Fatal(err)
	}
	defer c.close()
	b := unsafe.Slice((*byte)(unsafe.Pointer(c.view)), 8)
	if binary.LittleEndian.Uint32(b[:4]) != 144 || binary.LittleEndian.Uint32(b[4:]) != 0 {
		t.Fatal("wrong original module ABI")
	}
	if another, err := newBlurControlNamed(240, name); err == nil {
		another.close()
		t.Fatal("mapping ownership conflict ignored")
	}
	if binary.LittleEndian.Uint32(b[:4]) != 144 {
		t.Fatal("existing mapping overwritten")
	}
	c.close()
	next, err := newBlurControlNamed(240, name)
	if err != nil {
		t.Fatal(err)
	}
	next.close()
}
