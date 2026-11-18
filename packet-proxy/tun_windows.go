//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/windows"
	"golang.zx2c4.com/wireguard/tun"
)

const (
	csqttTunName = "CSQTT"
	tunMTU       = 1280
)

type tunBypassRoute struct {
	IP             string
	Gateway        string
	InterfaceIndex int
	AddRoute       bool
}

type tunRecoveryState struct {
	InterfaceName       string
	InterfaceIndex      int
	ProxyIP             string
	AddProxyRoute       bool
	ProxyInterfaceIndex int
	BypassRoutes        []tunBypassRoute
	DNSIPs              []string
	HelperPID           uint32
}

type routeInfo struct {
	InterfaceIndex int
	Gateway        string
	PrefixLength   int
	Metric         int
}

func runTunMode(ctx context.Context, core, ip, dns, bypassIP string, ready func() error) error {
	if err := requireElevated(); err != nil {
		return err
	}
	if err := recoverTunStateOnlyIfOrphaned(); err != nil {
		return fmt.Errorf("не удалось очистить предыдущую TUN-сессию: %w", err)
	}
	if err := loopback(core); err != nil {
		return err
	}
	localIP, err := netip.ParseAddr(ip)
	if err != nil || !localIP.Is4() {
		return fmt.Errorf("--ip должен быть IPv4-адресом из TUNCONF")
	}
	bypassIPs, err := parseBypassIPs(bypassIP)
	if err != nil {
		return err
	}
	dnsServers, err := parseDNSServers(dns)
	if err != nil {
		return err
	}

	device, err := tun.CreateTUN(csqttTunName, tunMTU)
	if err != nil {
		return fmt.Errorf("не удалось создать Wintun-адаптер %s: %w", csqttTunName, err)
	}
	defer device.Close()

	ifName, err := device.Name()
	if err != nil {
		return fmt.Errorf("не удалось получить имя Wintun: %w", err)
	}
	iface, err := waitForInterface(ifName, 10*time.Second)
	if err != nil {
		return err
	}

	// Preserve physical routes to every TURN endpoint (or the selected HTTP proxy)
	// before installing the split-default routes through Wintun.
	bypassRoutes := make([]tunBypassRoute, 0, len(bypassIPs))
	for _, address := range bypassIPs {
		physicalRoute, routeErr := findBestIPv4Route(address)
		if routeErr != nil {
			return fmt.Errorf("не удалось определить физический маршрут к серверу %s: %w", address, routeErr)
		}
		bypassRoutes = append(bypassRoutes, tunBypassRoute{
			IP: address.String(), Gateway: physicalRoute.Gateway,
			InterfaceIndex: physicalRoute.InterfaceIndex,
			AddRoute:       physicalRoute.PrefixLength < 32,
		})
	}
	state := tunRecoveryState{
		InterfaceName:  ifName,
		InterfaceIndex: iface.Index,
		BypassRoutes:   bypassRoutes,
		DNSIPs:         dnsServers,
		HelperPID:      uint32(os.Getpid()),
	}

	stateFile, err := writeTunRecoveryState(state)
	if err != nil {
		return err
	}
	cleanupNeeded := true
	defer func() {
		if cleanupNeeded {
			if err := cleanupTunState(state, stateFile); err != nil {
				fmt.Fprintln(os.Stderr, "TUN cleanup warning:", err)
			}
		}
	}()

	if err := runNetsh("interface", "ipv4", "set", "address",
		"name="+ifName, "source=static", "address="+localIP.String(),
		"mask=255.255.255.255", "gateway=none", "store=active"); err != nil {
		return fmt.Errorf("не удалось назначить TUN IP: %w", err)
	}
	if err := setTunDNS(ifName, dnsServers); err != nil {
		return fmt.Errorf("не удалось настроить DNS TUN: %w", err)
	}
	if err := runNetsh("interface", "ipv4", "set", "interface", "interface="+ifName, "metric=1"); err != nil {
		return err
	}
	// CSQTT currently carries IPv4. Capture and discard IPv6 instead of bypassing the VPN.
	if err := runNetsh("interface", "ipv6", "add", "address", "interface="+ifName, "address=fd73:6373:7174::2", "store=active"); err != nil {
		return err
	}
	for _, prefix := range []string{"::/1", "8000::/1"} {
		if err := runNetsh("interface", "ipv6", "add", "route", prefix, ifName, "metric=1", "store=active"); err != nil {
			return err
		}
	}
	if err := runNetsh("interface", "ipv6", "set", "interface", "interface="+ifName, "metric=1", "store=active"); err != nil {
		return err
	}

	for _, route := range bypassRoutes {
		if !route.AddRoute {
			continue
		}
		gateway := route.Gateway
		if gateway == "" {
			gateway = "0.0.0.0"
		}
		if err := runRoute("ADD", route.IP, "MASK", "255.255.255.255",
			gateway, "METRIC", "1", "IF", strconv.Itoa(route.InterfaceIndex)); err != nil {
			return fmt.Errorf("не удалось закрепить физический маршрут к серверу %s вне TUN: %w", route.IP, err)
		}
	}

	if err := runRoute("ADD", "0.0.0.0", "MASK", "128.0.0.0",
		"0.0.0.0", "METRIC", "1", "IF", strconv.Itoa(iface.Index)); err != nil {
		return fmt.Errorf("не удалось добавить первый системный маршрут TUN: %w", err)
	}
	if err := runRoute("ADD", "128.0.0.0", "MASK", "128.0.0.0",
		"0.0.0.0", "METRIC", "1", "IF", strconv.Itoa(iface.Index)); err != nil {
		return fmt.Errorf("не удалось добавить второй системный маршрут TUN: %w", err)
	}

	// Pin the DNS resolver to Wintun as well: a pre-existing more-specific
	// physical LAN route must not make Windows send resolver traffic outside TUN.
	for _, server := range dnsServers {
		if bypassIPContains(bypassIPs, server) {
			continue
		}
		if err := runRoute("ADD", server, "MASK", "255.255.255.255",
			"0.0.0.0", "METRIC", "1", "IF", strconv.Itoa(iface.Index)); err != nil {
			return fmt.Errorf("не удалось направить DNS %s в TUN: %w", server, err)
		}
	}

	coreAddress, err := net.ResolveUDPAddr("udp4", core)
	if err != nil {
		return fmt.Errorf("неверный адрес локального CSQTT-порта: %w", err)
	}
	wire, err := net.DialUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)}, coreAddress)
	if err != nil {
		return fmt.Errorf("не удалось подключиться к локальному CSQTT-порту: %w", err)
	}
	defer wire.Close()

	errCh := make(chan error, 2)
	go func() {
		buffer := make([]byte, 65535)
		buffers := [][]byte{buffer}
		sizes := make([]int, 1)
		for {
			count, readErr := device.Read(buffers, sizes, 0)
			if readErr != nil {
				errCh <- fmt.Errorf("чтение Wintun: %w", readErr)
				return
			}
			for i := 0; i < count; i++ {
				if sizes[i] == 0 || buffers[i][0]>>4 != 4 {
					continue
				}
				if _, writeErr := wire.Write(buffers[i][:sizes[i]]); writeErr != nil {
					errCh <- fmt.Errorf("передача IP-пакета в CSQTT: %w", writeErr)
					return
				}
			}
		}
	}()
	go func() {
		buffer := make([]byte, 65535)
		for {
			count, readErr := wire.Read(buffer)
			if readErr != nil {
				if ctx.Err() != nil {
					return
				}
				if errors.Is(readErr, windows.WSAECONNRESET) {
					time.Sleep(100 * time.Millisecond)
					continue
				}
				errCh <- fmt.Errorf("приём IP-пакета от CSQTT: %w", readErr)
				return
			}
			if _, writeErr := device.Write([][]byte{buffer[:count]}, 0); writeErr != nil {
				errCh <- fmt.Errorf("передача IP-пакета в Wintun: %w", writeErr)
				return
			}
		}
	}()

	fmt.Printf("TUN_READY interface=%s ip=%s dns=%s\n", ifName, localIP, strings.Join(dnsServers, ","))
	if err := ready(); err != nil {
		return err
	}
	for {
		select {
		case address := <-bypassUpdates:
			if bypassIPContains(bypassIPs, address.String()) {
				continue
			}
			physical, err := findBestIPv4Route(address)
			if err != nil {
				return err
			}
			route := tunBypassRoute{IP: address.String(), Gateway: physical.Gateway, InterfaceIndex: physical.InterfaceIndex, AddRoute: physical.PrefixLength < 32}
			state.BypassRoutes = append(state.BypassRoutes, route)
			if _, err := writeTunRecoveryState(state); err != nil {
				return err
			}
			if route.AddRoute {
				if err := runRoute("ADD", route.IP, "MASK", "255.255.255.255", route.Gateway, "METRIC", "1", "IF", strconv.Itoa(route.InterfaceIndex)); err != nil {
					return err
				}
			}
			bypassIPs = append(bypassIPs, address)
		case <-ctx.Done():
			fmt.Println("TUN_STOPPED")
			cleanupNeeded = false
			if err := cleanupTunState(state, stateFile); err != nil {
				return fmt.Errorf("ошибка восстановления сетевых настроек: %w", err)
			}
			return nil
		case err := <-errCh:
			return err
		}
	}
}

func requireElevated() error {
	if !windows.GetCurrentProcessToken().IsElevated() {
		return errors.New("для режима TUN необходим запуск от имени администратора")
	}
	return nil
}

func parseBypassIPs(value string) ([]netip.Addr, error) {
	seen := make(map[netip.Addr]bool)
	var result []netip.Addr
	for _, item := range strings.Split(value, ",") {
		address, err := netip.ParseAddr(strings.TrimSpace(item))
		if err != nil || !address.Is4() {
			continue
		}
		if !seen[address] {
			seen[address] = true
			result = append(result, address)
		}
	}
	if len(result) == 0 {
		return nil, errors.New("--bypass-ip must contain at least one TURN server or HTTP proxy IPv4 address")
	}
	return result, nil
}

func bypassIPContains(addresses []netip.Addr, value string) bool {
	address, err := netip.ParseAddr(value)
	if err != nil {
		return false
	}
	for _, candidate := range addresses {
		if candidate == address {
			return true
		}
	}
	return false
}

func parseDNSServers(value string) ([]string, error) {
	seen := map[string]bool{}
	var result []string
	for _, item := range strings.Split(value, ",") {
		address, err := netip.ParseAddr(strings.TrimSpace(item))
		if err == nil && address.Is4() && !seen[address.String()] {
			seen[address.String()] = true
			result = append(result, address.String())
		}
	}
	if len(result) == 0 {
		return nil, errors.New("TUNCONF не содержит корректный DNS IPv4")
	}
	return result, nil
}

func waitForInterface(name string, timeout time.Duration) (*net.Interface, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if iface, err := net.InterfaceByName(name); err == nil {
			return iface, nil
		}
		time.Sleep(150 * time.Millisecond)
	}
	return nil, fmt.Errorf("Windows не зарегистрировала интерфейс %q за %s", name, timeout)
}

func setTunDNS(name string, servers []string) error {
	args := []string{"interface", "ipv4", "set", "dnsservers",
		"name=" + name, "source=static", "address=" + servers[0], "validate=no"}
	if err := runNetsh(args...); err != nil {
		return err
	}
	for index, server := range servers[1:] {
		if err := runNetsh("interface", "ipv4", "add", "dnsservers",
			"name="+name, "address="+server, "index="+strconv.Itoa(index+2), "validate=no"); err != nil {
			return err
		}
	}
	return nil
}

func findBestIPv4Route(target netip.Addr) (routeInfo, error) {
	output, err := hiddenCommand("route.exe", "print", "-4").CombinedOutput()
	if err != nil {
		return routeInfo{}, fmt.Errorf("route print: %w: %s", err, strings.TrimSpace(string(output)))
	}
	indexByAddress := map[string]int{}
	interfaces, _ := net.Interfaces()
	for _, iface := range interfaces {
		if iface.Name == csqttTunName {
			continue
		}
		addresses, _ := iface.Addrs()
		for _, raw := range addresses {
			prefix, parseErr := netip.ParsePrefix(raw.String())
			if parseErr == nil && prefix.Addr().Is4() {
				indexByAddress[prefix.Addr().String()] = iface.Index
			}
		}
	}

	best := routeInfo{PrefixLength: -1, Metric: int(^uint(0) >> 1)}
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		destination, e1 := netip.ParseAddr(fields[0])
		mask, e2 := netip.ParseAddr(fields[1])
		interfaceIP, e3 := netip.ParseAddr(fields[3])
		metric, e4 := strconv.Atoi(fields[4])
		if e1 != nil || e2 != nil || e3 != nil || e4 != nil ||
			!destination.Is4() || !mask.Is4() || !interfaceIP.Is4() {
			continue
		}
		maskBytes := mask.As4()
		prefixLength, bitCount := net.IPv4Mask(maskBytes[0], maskBytes[1], maskBytes[2], maskBytes[3]).Size()
		if bitCount != 32 {
			continue
		}
		prefix := netip.PrefixFrom(destination, prefixLength)
		if !prefix.Contains(target) {
			continue
		}
		ifIndex := indexByAddress[interfaceIP.String()]
		if ifIndex == 0 {
			continue
		}
		gateway := "0.0.0.0"
		if address, parseErr := netip.ParseAddr(fields[2]); parseErr == nil && address.Is4() {
			gateway = address.String()
		}
		if prefixLength > best.PrefixLength ||
			(prefixLength == best.PrefixLength && metric < best.Metric) {
			best = routeInfo{InterfaceIndex: ifIndex, Gateway: gateway, PrefixLength: prefixLength, Metric: metric}
		}
	}
	if best.InterfaceIndex == 0 {
		return routeInfo{}, fmt.Errorf("не найден подходящий физический маршрут к %s", target)
	}
	return best, nil
}

func runRoute(args ...string) error {
	output, err := hiddenCommand("route.exe", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("route %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}

func runNetsh(args ...string) error {
	output, err := hiddenCommand("netsh.exe", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("netsh %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}

func tunStatePath() (string, error) {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		var err error
		base, err = os.UserConfigDir()
		if err != nil {
			return "", err
		}
	}
	directory := filepath.Join(base, "CSQTT")
	if err := os.MkdirAll(directory, 0700); err != nil {
		return "", err
	}
	return filepath.Join(directory, "tun-state.json"), nil
}

func writeTunRecoveryState(state tunRecoveryState) (string, error) {
	path, err := tunStatePath()
	if err != nil {
		return "", err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return "", err
	}
	temp := path + ".tmp"
	if err := os.WriteFile(temp, data, 0600); err != nil {
		return "", err
	}
	if err := os.Rename(temp, path); err != nil {
		_ = os.Remove(temp)
		return "", err
	}
	return path, nil
}

func recoverTunState() (bool, error) {
	path, err := tunStatePath()
	if err != nil {
		return false, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var state tunRecoveryState
	if err := json.Unmarshal(data, &state); err != nil {
		return false, fmt.Errorf("повреждён файл восстановления TUN: %w", err)
	}
	if state.HelperPID != 0 && processIsRunning(state.HelperPID) {
		return false, nil
	}
	if err := cleanupTunState(state, path); err != nil {
		return false, err
	}
	return true, nil
}

func recoverTunStateOnlyIfOrphaned() error {
	recovered, err := recoverTunState()
	if err != nil || recovered {
		return err
	}
	path, pathErr := tunStatePath()
	if pathErr != nil {
		return pathErr
	}
	data, readErr := os.ReadFile(path)
	if errors.Is(readErr, os.ErrNotExist) {
		return nil
	}
	if readErr != nil {
		return readErr
	}
	var state tunRecoveryState
	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}
	if state.HelperPID != 0 && processIsRunning(state.HelperPID) {
		return errors.New("обнаружен другой активный TUN-сеанс CSQTT")
	}
	return nil
}

func processIsRunning(pid uint32) bool {
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, pid)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(handle)
	status, err := windows.WaitForSingleObject(handle, 0)
	return err == nil && status == uint32(windows.WAIT_TIMEOUT)
}

func cleanupTunState(state tunRecoveryState, path string) error {
	var problems []string
	if state.InterfaceIndex > 0 {
		_ = runRoute("DELETE", "0.0.0.0", "MASK", "128.0.0.0", "IF", strconv.Itoa(state.InterfaceIndex))
		_ = runRoute("DELETE", "128.0.0.0", "MASK", "128.0.0.0", "IF", strconv.Itoa(state.InterfaceIndex))
	}
	if state.InterfaceIndex > 0 {
		for _, server := range state.DNSIPs {
			_ = runRoute("DELETE", server, "MASK", "255.255.255.255", "IF", strconv.Itoa(state.InterfaceIndex))
		}
	}
	if len(state.BypassRoutes) > 0 {
		for _, route := range state.BypassRoutes {
			if route.AddRoute && route.InterfaceIndex > 0 {
				_ = runRoute("DELETE", route.IP, "MASK", "255.255.255.255", "IF", strconv.Itoa(route.InterfaceIndex))
			}
		}
	} else if state.AddProxyRoute && state.ProxyIP != "" && state.ProxyInterfaceIndex > 0 {
		// Backward-compatible cleanup for an older recovery state file.
		_ = runRoute("DELETE", state.ProxyIP, "MASK", "255.255.255.255", "IF", strconv.Itoa(state.ProxyInterfaceIndex))
	}
	if iface, err := net.InterfaceByName(state.InterfaceName); err == nil && iface.Index == state.InterfaceIndex {
		for _, prefix := range []string{"::/1", "8000::/1"} {
			_ = runNetsh("interface", "ipv6", "delete", "route", prefix, state.InterfaceName, "store=active")
		}
		_ = runNetsh("interface", "ipv6", "delete", "address", state.InterfaceName, "fd73:6373:7174::2", "store=active")
		if err := runNetsh("interface", "ipv4", "set", "dnsservers",
			"name="+state.InterfaceName, "source=dhcp"); err != nil {
			problems = append(problems, err.Error())
		}
		if err := runNetsh("interface", "ipv4", "set", "address",
			"name="+state.InterfaceName, "source=dhcp"); err != nil {
			problems = append(problems, err.Error())
		}
	}
	if len(problems) == 0 {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			problems = append(problems, err.Error())
		}
	}
	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}

func watchParentProcess(pid uint32, cancel context.CancelFunc) {
	if pid == 0 {
		return
	}
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, pid)
	if err != nil {
		cancel()
		return
	}
	go func() {
		defer windows.CloseHandle(handle)
		_, _ = windows.WaitForSingleObject(handle, windows.INFINITE)
		cancel()
	}()
}
