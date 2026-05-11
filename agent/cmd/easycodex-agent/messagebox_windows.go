//go:build windows

package main

import (
	"fmt"
	"syscall"
	"unsafe"
)

var (
	user32           = syscall.NewLazyDLL("user32.dll")
	procMessageBoxW  = user32.NewProc("MessageBoxW")
)

func showStartupError(title string, err error) {
	message := title
	if err != nil {
		message = fmt.Sprintf("%s\r\n\r\n%s", title, err.Error())
	}
	titlePtr, _ := syscall.UTF16PtrFromString("EasyCodex")
	messagePtr, _ := syscall.UTF16PtrFromString(message)
	const mbIconError = 0x00000010
	const mbSetForeground = 0x00010000
	_, _, _ = procMessageBoxW.Call(0, uintptr(unsafe.Pointer(messagePtr)), uintptr(unsafe.Pointer(titlePtr)), uintptr(mbIconError|mbSetForeground))
}
