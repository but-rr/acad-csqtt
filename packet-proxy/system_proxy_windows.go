//go:build windows

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const internetSettings = `Software\Microsoft\Windows\CurrentVersion\Internet Settings`

var internetSetOption = windows.NewLazySystemDLL("wininet.dll").NewProc("InternetSetOptionW")
var internetQueryOption = windows.NewLazySystemDLL("wininet.dll").NewProc("InternetQueryOptionW")

type proxyValue struct {
	Exists bool
	Kind   uint32
	Text   string
	Number uint32
}
type proxySnapshot struct {
	PID    uint32
	Server string
	Flags  uint32
	Values map[string]proxyValue
}
type proxyController struct {
	sync.Mutex
	address        string
	enabled, ready bool
	snapshot       *proxySnapshot
}
type perConnectionOption struct {
	Option uint32
	Value  uintptr
}
type perConnectionList struct {
	Size         uint32
	Connection   *uint16
	Count, Error uint32
	Options      *perConnectionOption
}

func hiddenCommand(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	return cmd
}

func newProxyController(address string, enabled bool) *proxyController {
	return &proxyController{address: address, enabled: enabled}
}

func connectionFlags(flags *uint32, query bool) error {
	option := perConnectionOption{Option: 1, Value: uintptr(*flags)}
	list := perConnectionList{Count: 1, Options: &option}
	list.Size = uint32(unsafe.Sizeof(list))
	size := list.Size
	var ok uintptr
	var err error
	if query {
		ok, _, err = internetQueryOption.Call(0, 75, uintptr(unsafe.Pointer(&list)), uintptr(unsafe.Pointer(&size)))
	} else {
		ok, _, err = internetSetOption.Call(0, 75, uintptr(unsafe.Pointer(&list)), uintptr(size))
	}
	if ok == 0 {
		return fmt.Errorf("Windows proxy options: %w", err)
	}
	*flags = uint32(option.Value)
	return nil
}

func notifyProxy() {
	internetSetOption.Call(0, 39, 0, 0) // INTERNET_OPTION_SETTINGS_CHANGED
	internetSetOption.Call(0, 37, 0, 0) // INTERNET_OPTION_REFRESH
}

func proxyStatePath() string {
	return filepath.Join(os.Getenv("LOCALAPPDATA"), "CSQTT", "system-proxy-state.json")
}

func recoverProxyState() error {
	data, err := os.ReadFile(proxyStatePath())
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var old proxySnapshot
	if err = json.Unmarshal(data, &old); err != nil {
		return err
	}
	if processIsRunning(old.PID) {
		return nil
	}
	return restoreSnapshot(&old)
}

func restoreSnapshot(snapshot *proxySnapshot) error {
	key, err := registry.OpenKey(registry.CURRENT_USER, internetSettings, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	current, _, _ := key.GetStringValue("ProxyServer")
	// Respect changes made by the user or another VPN since CSQTT applied its proxy.
	if current != snapshot.Server {
		return os.Remove(proxyStatePath())
	}
	flags := snapshot.Flags
	if err := connectionFlags(&flags, false); err != nil {
		return err
	}
	for name, value := range snapshot.Values {
		if !value.Exists {
			err = key.DeleteValue(name)
			if err == registry.ErrNotExist {
				err = nil
			}
		} else if value.Kind == registry.DWORD {
			err = key.SetDWordValue(name, value.Number)
		} else {
			err = key.SetStringValue(name, value.Text)
		}
		if err != nil {
			return err
		}
	}
	notifyProxy()
	if err := os.Remove(proxyStatePath()); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (p *proxyController) setReady() error {
	p.Lock()
	defer p.Unlock()
	p.ready = true
	if err := recoverProxyState(); err != nil {
		return err
	}
	return p.applyLocked()
}

func (p *proxyController) setEnabled(enabled bool) error {
	p.Lock()
	defer p.Unlock()
	p.enabled = enabled
	return p.applyLocked()
}

func (p *proxyController) applyLocked() error {
	if !p.ready {
		return nil
	}
	if !p.enabled {
		if p.snapshot != nil {
			if err := restoreSnapshot(p.snapshot); err != nil {
				return err
			}
			p.snapshot = nil
		}
		fmt.Println("SYSTEM_PROXY_OFF")
		return nil
	}
	if p.snapshot != nil {
		return nil
	}
	if err := loopback(p.address); err != nil {
		return err
	}
	if data, err := os.ReadFile(proxyStatePath()); err == nil {
		var old proxySnapshot
		if err = json.Unmarshal(data, &old); err != nil {
			return err
		}
		if processIsRunning(old.PID) {
			return fmt.Errorf("system proxy belongs to another CSQTT session")
		}
		if err = restoreSnapshot(&old); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	key, err := registry.OpenKey(registry.CURRENT_USER, internetSettings, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	snapshot := &proxySnapshot{PID: uint32(os.Getpid()), Server: p.address, Values: map[string]proxyValue{}}
	if err := connectionFlags(&snapshot.Flags, true); err != nil {
		return err
	}
	for _, name := range []string{"ProxyEnable", "ProxyServer", "ProxyOverride", "AutoConfigURL", "AutoDetect"} {
		value := proxyValue{}
		text, kind, e := key.GetStringValue(name)
		if e == nil {
			value = proxyValue{Exists: true, Kind: kind, Text: text}
		} else if number, kind, e := key.GetIntegerValue(name); e == nil {
			value = proxyValue{Exists: true, Kind: kind, Number: uint32(number)}
		}
		snapshot.Values[name] = value
	}
	path := proxyStatePath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, _ := json.Marshal(snapshot)
	if err := os.WriteFile(path+".tmp", data, 0600); err != nil {
		return err
	}
	if err := os.Rename(path+".tmp", path); err != nil {
		return err
	}
	p.snapshot = snapshot
	// Write the owner first, allowing rollback even when a later operation fails.
	if err := key.SetStringValue("ProxyServer", p.address); err != nil {
		return err
	}
	if err := key.SetStringValue("ProxyOverride", "<local>;127.*;[::1];localhost"); err != nil {
		return err
	}
	if err := key.SetDWordValue("ProxyEnable", 1); err != nil {
		return err
	}
	if err := key.DeleteValue("AutoConfigURL"); err != nil && err != registry.ErrNotExist {
		return err
	}
	flags := uint32(3) // DIRECT | PROXY; temporarily disable PAC and auto-detection.
	if err := connectionFlags(&flags, false); err != nil {
		return err
	}
	notifyProxy()
	fmt.Println("SYSTEM_PROXY_ON")
	return nil
}

func (p *proxyController) restore() {
	p.Lock()
	defer p.Unlock()
	if p.snapshot != nil {
		if err := restoreSnapshot(p.snapshot); err != nil {
			fmt.Println("SYSTEM_PROXY_ERROR", err)
		} else {
			p.snapshot = nil
		}
	}
}
