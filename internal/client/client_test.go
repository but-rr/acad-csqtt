package client

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func valid() Profile {
	p := DefaultProfile()
	p.Server = "203.0.113.2:443"
	p.Password = "test-only-secret"
	p.Hashes = "https://vk.com/call/join/testhash123456?extra=1"
	return p
}
func TestPortableProfileAndCommandCompatibility(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	p := valid()
	p.ProxyUser = "test-user"
	p.ProxyPassword = "test-proxy-secret"
	p.Tun = "1"
	p.SystemProxy = "1"
	p.Salt = "test-salt"
	directory := t.TempDir()
	if e := SaveProfile(directory, p, true); e != nil {
		t.Fatal(e)
	}
	loaded, e := LoadProfile(directory)
	if e != nil || p != loaded {
		t.Fatalf("profile roundtrip: %v", e)
	}
	copyDir := t.TempDir()
	data, _ := os.ReadFile(filepath.Join(directory, "profile.json"))
	os.WriteFile(filepath.Join(copyDir, "profile.json"), data, 0600)
	if copied, e := LoadProfile(copyDir); e != nil || copied != p {
		t.Fatal("profile is not portable")
	}
	args, env, e := CoreCommand(directory, p, 19091)
	if e != nil {
		t.Fatal(e)
	}
	command := strings.Join(args, " ")
	if strings.Contains(command, p.Password) || strings.Contains(command, p.ProxyPassword) {
		t.Fatal("password in command")
	}
	if !strings.Contains(command, "--turn-transport tcp --port 443") {
		t.Fatal(command)
	}
	if !strings.Contains(strings.Join(env, "\n"), "CSQTT_PROXY_PASSWORD="+p.ProxyPassword) {
		t.Fatal("missing proxy auth")
	}
	t.Setenv("CSQTT_HTTP_PROXY", "stale")
	p.InternetMode = "direct"
	p.Proxy = ""
	args, env, e = CoreCommand(directory, p, 19091)
	if e != nil {
		t.Fatal(e)
	}
	for _, v := range env {
		if strings.HasPrefix(v, "CSQTT_HTTP_PROXY=") || strings.HasPrefix(v, "CSQTT_PROXY_PASSWORD=") {
			t.Fatal("direct mode inherited proxy")
		}
	}
	if e := SaveProfile(directory, p, false); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(filepath.Join(directory, "profile.json")); !os.IsNotExist(e) {
		t.Fatal("profile not removed")
	}
}
func TestUpdateOpensReleasePageAndRejectsUntrustedAssets(t *testing.T) {
	tag := "v1.4.2"
	asset := map[string]string{"name": "CSQTT-" + tag + "-Win10-x64.zip", "state": "uploaded", "browser_download_url": "https://github.com/" + repository + "/releases/download/" + tag + "/CSQTT-" + tag + "-Win10-x64.zip"}
	data, _ := json.Marshal(map[string]interface{}{"tag_name": tag, "assets": []interface{}{asset}})
	u, e := ParseUpdate(data, "1.4.1")
	if e != nil || u == nil || u.URL != "https://github.com/"+repository+"/releases/tag/"+tag {
		t.Fatal("update must open the release page")
	}
	asset["browser_download_url"] = "https://example.com/bad.zip"
	data, _ = json.Marshal(map[string]interface{}{"tag_name": tag, "assets": []interface{}{asset}})
	if u, _ := ParseUpdate(data, "1.4.1"); u != nil {
		t.Fatal("untrusted download accepted")
	}
	if newer("1.4.0", "1.4.1") || !newer("1.10.0", "1.9.0") {
		t.Fatal("version ordering")
	}
}
func TestAuthenticatedConnect(t *testing.T) {
	l, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer l.Close()
	received := make(chan string, 1)
	go func() {
		c, e := l.Accept()
		if e != nil {
			return
		}
		defer c.Close()
		c.SetDeadline(time.Now().Add(3 * time.Second))
		var b [8192]byte
		n, _ := c.Read(b[:])
		received <- string(b[:n])
		fmt.Fprint(c, "HTTP/1.1 200 Connection Established\r\n\r\n")
	}()
	p := valid()
	p.Proxy = l.Addr().String()
	p.ProxyUser = "test-user"
	p.ProxyPassword = "test-secret"
	if e := CheckProxy(context.Background(), p); e != nil {
		t.Fatal(e)
	}
	request := <-received
	if !strings.Contains(request, "CONNECT dns.google:443") || !strings.Contains(request, base64.StdEncoding.EncodeToString([]byte("test-user:test-secret"))) {
		t.Fatal("CONNECT authorization missing")
	}
}
func TestEventFloodIsBoundedAndMasksSecrets(t *testing.T) {
	m := NewManager(t.TempDir(), "1.4.1")
	defer m.Close()
	m.session = 1
	m.secrets = []string{"sensitive-value"}
	start := time.Now()
	for i := 0; i < 10000; i++ {
		m.consume(1, "core", fmt.Sprintf("failed %d sensitive-value password=hidden", i))
	}
	s := m.Snapshot()
	if len(s.Logs) > 400 {
		t.Fatal("unbounded journal")
	}
	for _, line := range s.Logs {
		if strings.Contains(line.Message, "sensitive-value") || strings.Contains(line.Message, "password=hidden") {
			t.Fatal("secret leak")
		}
	}
	if time.Since(start) > 3*time.Second {
		t.Fatal("event handling stalled")
	}
	m.consume(1, "core", "__CSQTT_EVENT__|STATS|{\"active\":6,\"bytes_up\":123,\"bytes_down\":456}")
	s = m.Snapshot()
	if s.Active != 6 || s.Upload != 123 || s.Download != 456 {
		t.Fatal("stats event lost")
	}
}
func TestBuiltCoreRemainsCompatibleWithController(t *testing.T) {
	root, e := filepath.Abs("../..")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(filepath.Join(root, "bin/csqtt-core.exe")); e != nil {
		t.Skip("core not built")
	}
	l, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer l.Close()
	seen := make(chan string, 1)
	go func() {
		c, e := l.Accept()
		if e != nil {
			return
		}
		defer c.Close()
		c.SetDeadline(time.Now().Add(5 * time.Second))
		b := make([]byte, 8192)
		n, _ := c.Read(b)
		seen <- string(b[:n])
		io.WriteString(c, "HTTP/1.1 407 Proxy Authentication Required\r\nContent-Length: 0\r\n\r\n")
	}()
	p := valid()
	p.Proxy = l.Addr().String()
	m := NewManager(root, "1.4.1")
	defer m.Close()
	if e := m.Start(p); e != nil {
		t.Fatal(e)
	}
	select {
	case request := <-seen:
		if !strings.Contains(strings.ToLower(request), "connect api.vk.me:443") {
			t.Fatal("unexpected core request")
		}
	case <-time.After(8 * time.Second):
		t.Fatal("core did not use explicit CONNECT")
	}
	m.Stop()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if m.Snapshot().Status == "offline" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("disconnect stalled")
}
