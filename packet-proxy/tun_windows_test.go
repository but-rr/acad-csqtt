//go:build windows

package main

import (
	"os"
	"reflect"
	"testing"

	"golang.org/x/sys/windows/registry"
)

func TestBypassAndDNSValidation(t *testing.T) {
	addresses, err := parseBypassIPs("203.0.113.1,203.0.113.1,203.0.113.2,invalid,::1")
	if err != nil || len(addresses) != 2 {
		t.Fatalf("invalid bypass parsing: %v %v", addresses, err)
	}
	if _, err := parseBypassIPs("::1,invalid"); err == nil {
		t.Fatal("an empty IPv4 bypass must be rejected")
	}
	servers, err := parseDNSServers("1.1.1.1,1.1.1.1,8.8.8.8,invalid")
	if err != nil || len(servers) != 2 {
		t.Fatalf("invalid DNS parsing: %v %v", servers, err)
	}
}

// Explicitly opt in: verify real WinINet changes and restore them before returning.
func TestSystemProxyRestoreIntegration(t *testing.T) {
	if os.Getenv("CSQTT_SYSTEM_PROXY_TEST") != "1" {
		t.Skip("set CSQTT_SYSTEM_PROXY_TEST=1 for the reversible Windows settings check")
	}
	t.Setenv("LOCALAPPDATA", t.TempDir())
	p := newProxyController("127.0.0.1:18080", true)
	defer p.restore()
	if err := p.setReady(); err != nil {
		t.Fatal(err)
	}
	original := p.snapshot
	if original == nil {
		t.Fatal("missing backup")
	}
	key, err := registry.OpenKey(registry.CURRENT_USER, internetSettings, registry.QUERY_VALUE)
	if err != nil {
		t.Fatal(err)
	}
	defer key.Close()
	server, _, _ := key.GetStringValue("ProxyServer")
	enabled, _, _ := key.GetIntegerValue("ProxyEnable")
	if server != "127.0.0.1:18080" || enabled != 1 {
		t.Fatal("Windows proxy not applied")
	}
	if err := p.setEnabled(false); err != nil {
		t.Fatal(err)
	}
	for name, want := range original.Values {
		got := proxyValue{}
		if text, kind, e := key.GetStringValue(name); e == nil {
			got = proxyValue{Exists: true, Kind: kind, Text: text}
		} else if number, kind, e := key.GetIntegerValue(name); e == nil {
			got = proxyValue{Exists: true, Kind: kind, Number: uint32(number)}
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%s not restored: %+v != %+v", name, got, want)
		}
	}
}
