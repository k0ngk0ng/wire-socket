//go:build windows

package wireguard

import (
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

var (
	kernel32        = syscall.NewLazyDLL("kernel32.dll")
	setDllDirectory = kernel32.NewProc("SetDllDirectoryW")
)

func init() {
	// Set DLL directory to executable directory so wintun.dll can be found
	exePath, err := os.Executable()
	if err != nil {
		return
	}
	exeDir := filepath.Dir(exePath)

	// Convert to UTF-16 for Windows API
	dirPtr, err := syscall.UTF16PtrFromString(exeDir)
	if err != nil {
		return
	}

	// Call SetDllDirectoryW to add executable directory to DLL search path
	setDllDirectory.Call(uintptr(unsafe.Pointer(dirPtr)))
}
