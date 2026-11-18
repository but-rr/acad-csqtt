// Build-time packaging; not part of the desktop executable.
package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func check(e error) {
	if e != nil {
		panic(e)
	}
}
func digest(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }
func wintun() {
	const expected = "07c256185d6ee3652e09fa55c0b673e2624b565e02c4b9091c79ca7d2f24ef51"
	path := "build/wintun-0.14.1.zip"
	data, _ := os.ReadFile(path)
	if digest(data) != expected {
		var e error
		for attempt := 0; attempt < 4; attempt++ {
			var r *http.Response
			r, e = (&http.Client{Timeout: 25 * time.Second}).Get("https://www.wintun.net/builds/wintun-0.14.1.zip")
			if e == nil {
				data, e = io.ReadAll(io.LimitReader(r.Body, 4*1024*1024))
				r.Body.Close()
				if e == nil && digest(data) == expected {
					break
				}
				e = fmt.Errorf("Wintun SHA256 mismatch")
			}
			time.Sleep(time.Second)
		}
		check(e)
		check(os.MkdirAll("build", 0755))
		check(os.WriteFile(path, data, 0600))
	}
	z, e := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	check(e)
	for _, f := range z.File {
		var target string
		switch f.Name {
		case "wintun/bin/amd64/wintun.dll":
			target = "bin/wintun.dll"
		case "wintun/LICENSE.txt":
			target = "build/wintun-license.txt"
		default:
			continue
		}
		r, e := f.Open()
		check(e)
		b, e := io.ReadAll(r)
		r.Close()
		check(e)
		check(os.MkdirAll(filepath.Dir(target), 0755))
		check(os.WriteFile(target, b, 0644))
	}
	fmt.Println("Verified official signed Wintun 0.14.1 AMD64")
}
func notices(destination string) {
	root, e := os.Getwd()
	check(e)
	var out strings.Builder
	include := func(path, label string) {
		b, e := os.ReadFile(path)
		check(e)
		fmt.Fprintf(&out, "\n%s\n%s\n%s\n\n%s\n", strings.Repeat("=", 72), label, strings.Repeat("=", 72), b)
	}
	scan := func(directory, label string) {
		files, _ := os.ReadDir(directory)
		for _, f := range files {
			u := strings.ToUpper(f.Name())
			if !f.IsDir() && (strings.HasPrefix(u, "LICENSE") || strings.HasPrefix(u, "LICENCE") || strings.HasPrefix(u, "COPYING") || strings.HasPrefix(u, "NOTICE") || strings.HasPrefix(u, "COPYRIGHT")) {
				include(filepath.Join(directory, f.Name()), label+"/"+f.Name())
			}
		}
	}
	for _, f := range []string{"LICENSE", "NOTICE.md", "assets/fonts/OFL.txt", "build/wintun-license.txt"} {
		include(f, f)
	}
	for _, d := range []string{"core", "core/rust-client/vendor/primp", "core/rust-client/vendor/netstack-smoltcp"} {
		scan(d, d)
	}
	cargo := os.Getenv("CARGO_HOME")
	if cargo == "" {
		home, _ := os.UserHomeDir()
		cargo = filepath.Join(home, ".cargo")
	}
	crates, _ := filepath.Glob(filepath.Join(cargo, "registry/src/*/*"))
	for _, d := range crates {
		scan(d, "Rust/"+filepath.Base(d))
	}
	for _, d := range []string{root, filepath.Join(root, "packet-proxy")} {
		cmd := exec.Command("go", "list", "-m", "-json", "all")
		cmd.Dir = d
		b, e := cmd.Output()
		check(e)
		decoder := json.NewDecoder(bytes.NewReader(b))
		for {
			var m struct{ Dir, Path, Version string }
			e = decoder.Decode(&m)
			if e == io.EOF {
				break
			}
			check(e)
			if m.Dir != "" {
				scan(m.Dir, "Go/"+m.Path+"/"+m.Version)
			}
		}
	}
	check(filepath.WalkDir("frontend/node_modules", func(path string, f os.DirEntry, e error) error {
		if e == nil && !f.IsDir() {
			u := strings.ToUpper(f.Name())
			if strings.HasPrefix(u, "LICENSE") || strings.HasPrefix(u, "COPYING") {
				include(path, path)
			}
		}
		return nil
	}))
	check(os.MkdirAll(filepath.Join(destination, "_internal"), 0755))
	check(os.WriteFile(filepath.Join(destination, "_internal/THIRD_PARTY_NOTICES.txt"), []byte(out.String()), 0644))
}
func archive(directory, name string) {
	f, e := os.Create(name)
	check(e)
	z := zip.NewWriter(f)
	check(filepath.WalkDir(directory, func(path string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.IsDir() {
			return nil
		}
		relative, e := filepath.Rel(directory, path)
		if e != nil {
			return e
		}
		if strings.EqualFold(d.Name(), "profile.json") || strings.EqualFold(d.Name(), "profile.tmp") {
			return fmt.Errorf("Profiles must never be packaged")
		}
		w, e := z.Create(filepath.ToSlash(relative))
		if e != nil {
			return e
		}
		r, e := os.Open(path)
		if e != nil {
			return e
		}
		defer r.Close()
		_, e = io.Copy(w, r)
		return e
	}))
	check(z.Close())
	check(f.Close())
	r, e := zip.OpenReader(name)
	check(e)
	names := map[string]bool{}
	for _, f := range r.File {
		names[f.Name] = true
	}
	r.Close()
	for _, name := range []string{"CSQTT.exe", "bin/csqtt-core.exe", "bin/packet-proxy.exe", "bin/wintun.dll", "bin/VCRUNTIME140.dll", "_internal/THIRD_PARTY_NOTICES.txt"} {
		if !names[name] {
			panic("Missing " + name)
		}
	}
	b, e := os.ReadFile(name)
	check(e)
	check(os.WriteFile(name+".sha256", []byte(digest(b)+"  "+filepath.Base(name)+"\r\n"), 0644))
	fmt.Println("Built", name)
}
func main() {
	switch os.Args[1] {
	case "wintun":
		wintun()
	case "notices":
		notices(os.Args[2])
	case "zip":
		archive(os.Args[2], os.Args[3])
	default:
		panic("Unknown packaging command")
	}
}
