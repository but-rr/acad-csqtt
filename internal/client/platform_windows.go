//go:build windows

package client

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

func hidden(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
}
func IsAdmin() bool { return windows.GetCurrentProcessToken().IsElevated() }
func Elevate(executable, directory string) error {
	proc := windows.NewLazySystemDLL("shell32.dll").NewProc("ShellExecuteW")
	verb, _ := windows.UTF16PtrFromString("runas")
	exe, _ := windows.UTF16PtrFromString(executable)
	args, _ := windows.UTF16PtrFromString("--connect --wait-pid=" + strconv.Itoa(os.Getpid()))
	dir, _ := windows.UTF16PtrFromString(directory)
	result, _, _ := proc.Call(0, uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(exe)), uintptr(unsafe.Pointer(args)), uintptr(unsafe.Pointer(dir)), 1)
	if result <= 32 {
		return errors.New("Запуск TUN отменён. Нужны права администратора.")
	}
	return nil
}
func legacyProfile() ([]byte, error) {
	for _, name := range []string{"CSQTT", "CSQTT-Kolenka"} {
		data, e := os.ReadFile(filepath.Join(os.Getenv("LOCALAPPDATA"), name, "profile.dat"))
		if os.IsNotExist(e) {
			continue
		}
		if e != nil {
			return nil, e
		}
		if len(data) == 0 {
			return nil, errors.New("Пустой файл старого профиля.")
		}
		source := windows.DataBlob{Size: uint32(len(data)), Data: &data[0]}
		var out windows.DataBlob
		if e = windows.CryptUnprotectData(&source, nil, nil, 0, nil, 1, &out); e != nil {
			return nil, e
		}
		result := append([]byte(nil), unsafe.Slice(out.Data, out.Size)...)
		windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
		return result, nil
	}
	return nil, os.ErrNotExist
}

func forgetLegacy() error {
	for _, name := range []string{"CSQTT", "CSQTT-Kolenka"} {
		e := os.Remove(filepath.Join(os.Getenv("LOCALAPPDATA"), name, "profile.dat"))
		if e != nil && !os.IsNotExist(e) {
			return e
		}
	}
	return nil
}
