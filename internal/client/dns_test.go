package client

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDNSSelectionAndValidation(t *testing.T) {
	for _, test := range []struct {
		mode, primary, secondary, want string
		invalid                        bool
	}{
		{"server", "invalid", "ignored", "9.9.9.9", false},
		{"google", "", "", "8.8.8.8,8.8.4.4", false},
		{"cloudflare", "", "", "1.1.1.1,1.0.0.1", false},
		{"custom", " 8.8.8.8 ", "", "8.8.8.8", false},
		{"custom", "10.0.0.53", "10.0.0.54", "10.0.0.53,10.0.0.54", false},
		{"custom", "8.8.8.8", "8.8.8.8", "8.8.8.8", false},
		{"custom", "", "", "", true},
		{"custom", "dns.google", "", "", true},
		{"custom", "8.8.8.8:53", "", "", true},
		{"custom", "https://dns.google", "", "", true},
		{"custom", "::1", "", "", true},
		{"custom", "127.0.0.1", "", "", true},
		{"custom", "0.0.0.0", "", "", true},
		{"custom", "224.0.0.1", "", "", true},
		{"custom", "255.255.255.255", "", "", true},
		{"custom", "8.8.8.8", "invalid", "", true},
		{"unknown", "", "", "", true},
	} {
		p := valid()
		p.DNSMode, p.DNSPrimary, p.DNSSecondary = test.mode, test.primary, test.secondary
		got, e := p.ResolverDNS("9.9.9.9")
		if test.invalid {
			if e == nil {
				t.Errorf("accepted invalid DNS: %+v", test)
			}
			if _, e = p.Validate(); e == nil {
				t.Errorf("connection accepted invalid DNS: %+v", test)
			}
			continue
		}
		if e != nil || got != test.want {
			t.Errorf("%+v: got %q, %v", test, got, e)
		}
	}
}

func TestDNSProfileMigrationAndPortability(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	dir := t.TempDir()
	path := filepath.Join(dir, "profile.json")
	if e := os.WriteFile(path, []byte(`{"server":"203.0.113.2:443","workers":"12"}`), 0600); e != nil {
		t.Fatal(e)
	}
	p, e := LoadProfile(dir)
	if e != nil || p.DNSMode != "server" || p.Workers != "12" {
		t.Fatalf("old profile changed behavior: %+v %v", p, e)
	}
	p.DNSMode, p.DNSPrimary, p.DNSSecondary = "custom", "8.8.8.8", "8.8.4.4"
	if e = SaveProfile(dir, p, true); e != nil {
		t.Fatal(e)
	}
	data, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	copyDir := t.TempDir()
	if e = os.WriteFile(filepath.Join(copyDir, "profile.json"), data, 0600); e != nil {
		t.Fatal(e)
	}
	loaded, e := LoadProfile(copyDir)
	if e != nil || loaded != p {
		t.Fatalf("DNS not portable: %+v %v", loaded, e)
	}
}

func TestDNSIsPassedToBothHelperModesWithoutChangingServerConfig(t *testing.T) {
	for _, tun := range []string{"0", "1"} {
		m := NewManager(t.TempDir(), "1.4.2")
		m.profile = valid()
		m.profile.Tun = tun
		m.profile.DNSMode = "custom"
		m.profile.DNSPrimary = "8.8.8.8"
		m.profile.DNSSecondary = "8.8.4.4"
		m.port = 19090
		m.consume(0, "core", `__CSQTT_EVENT__|CONFIG|{"config":"TUNCONF:10.23.0.2:9.9.9.9"}`)
		args, e := m.helperCommand()
		if e != nil {
			t.Fatal(e)
		}
		if !strings.Contains(strings.Join(args, " "), "--dns 8.8.8.8,8.8.4.4") {
			t.Fatalf("mode TUN=%s uses wrong DNS: %v", tun, args)
		}
		m.consume(0, "core", `__CSQTT_EVENT__|CONFIG|{"config":"TUNCONF:10.23.0.2:9.9.9.9"}`)
		if m.configDNS != "9.9.9.9" || len(m.Snapshot().Logs) != 0 {
			t.Fatal("override corrupted server CONFIG comparison")
		}
		m.profile.DNSMode = "server"
		args, e = m.helperCommand()
		if e != nil || !strings.Contains(strings.Join(args, " "), "--dns 9.9.9.9") {
			t.Fatal("automatic DNS lost")
		}
		m.Close()
	}
}
