//go:build windows

package console

import (
	"syscall"
	"unsafe"
)

var procSetConsoleTitle = syscall.NewLazyDLL("kernel32.dll").NewProc("SetConsoleTitleW")

func SetTitle(title string) {
	p, err := syscall.UTF16PtrFromString(title)
	if err != nil {
		return
	}
	procSetConsoleTitle.Call(uintptr(unsafe.Pointer(p)))
}
