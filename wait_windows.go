//go:build windows

package main

import (
	"golang.org/x/sys/windows"
	"os"
	"strconv"
	"strings"
)

func waitForPrevious() {
	for _, arg := range os.Args {
		if strings.HasPrefix(arg, "--wait-pid=") {
			pid, e := strconv.ParseUint(strings.TrimPrefix(arg, "--wait-pid="), 10, 32)
			if e == nil {
				h, e := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
				if e == nil {
					windows.WaitForSingleObject(h, 10000)
					windows.CloseHandle(h)
				}
			}
		}
	}
}
