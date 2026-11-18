package client

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const repository = "but-rr/acad-csqtt"

type Update struct {
	Version string `json:"version"`
	URL     string `json:"url"`
}

func transport(p Profile, local bool) (*http.Transport, error) {
	t := &http.Transport{DialContext: (&net.Dialer{Timeout: 8 * time.Second}).DialContext, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}, TLSHandshakeTimeout: 8 * time.Second, ResponseHeaderTimeout: 10 * time.Second}
	if local {
		t.Proxy = http.ProxyURL(&url.URL{Scheme: "http", Host: "127.0.0.1:" + p.HTTPPort})
	} else if p.InternetMode == "proxy" {
		host, _, e := Address(p.Proxy)
		if e != nil || net.ParseIP(host) == nil {
			return nil, errors.New("Укажите IP и порт корпоративного прокси.")
		}
		proxy := &url.URL{Scheme: "http", Host: p.Proxy}
		if p.ProxyUser != "" {
			proxy.User = url.UserPassword(p.ProxyUser, p.ProxyPassword)
		}
		t.Proxy = http.ProxyURL(proxy)
	}
	return t, nil
}
func CheckProxy(ctx context.Context, p Profile) error {
	if _, _, e := Address(p.Proxy); e != nil {
		return e
	}
	conn, e := (&net.Dialer{Timeout: 8 * time.Second}).DialContext(ctx, "tcp", p.Proxy)
	if e != nil {
		return errors.New("Прокси недоступен: проверьте адрес и сеть.")
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(8 * time.Second))
	target := p.Turn
	if target == "" {
		target = "dns.google"
	}
	target = net.JoinHostPort(target, "443")
	auth := ""
	if p.ProxyUser != "" {
		if strings.ContainsAny(p.ProxyUser+p.ProxyPassword, "\r\n") {
			return errors.New("Неверная авторизация.")
		}
		auth = "Proxy-Authorization: Basic " + base64.StdEncoding.EncodeToString([]byte(p.ProxyUser+":"+p.ProxyPassword)) + "\r\n"
	}
	if _, e = fmt.Fprintf(conn, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n%s\r\n", target, target, auth); e != nil {
		return e
	}
	response, e := http.ReadResponse(bufio.NewReader(io.LimitReader(conn, 32768)), &http.Request{Method: "CONNECT"})
	if e != nil {
		return errors.New("Прокси вернул неверный ответ.")
	}
	if response.StatusCode != 200 {
		return fmt.Errorf("HTTP %d: CONNECT отклонён прокси.", response.StatusCode)
	}
	return nil
}
func CheckInternet(ctx context.Context, p Profile) (string, error) {
	t, e := transport(p, true)
	if e != nil {
		return "", e
	}
	defer t.CloseIdleConnections()
	req, _ := http.NewRequestWithContext(ctx, "GET", "https://api.ipify.org", nil)
	response, e := (&http.Client{Transport: t, Timeout: 25 * time.Second}).Do(req)
	if e != nil {
		return "", errors.New("Интернет пока не подтверждён. Проверьте параметры и журнал.")
	}
	defer response.Body.Close()
	data, e := io.ReadAll(io.LimitReader(response.Body, 128))
	if e != nil || response.StatusCode != 200 || net.ParseIP(strings.TrimSpace(string(data))) == nil {
		return "", errors.New("Не удалось проверить внешний IP.")
	}
	return strings.TrimSpace(string(data)), nil
}

var versionPattern = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)$`)

func newer(candidate, current string) bool {
	a, b := versionPattern.FindStringSubmatch(candidate), versionPattern.FindStringSubmatch(current)
	if a == nil || b == nil {
		return false
	}
	for i := 1; i <= 3; i++ {
		x, _ := strconv.Atoi(a[i])
		y, _ := strconv.Atoi(b[i])
		if x != y {
			return x > y
		}
	}
	return false
}
func ParseUpdate(data []byte, current string) (*Update, error) {
	var release struct {
		Tag        string `json:"tag_name"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
		Assets     []struct {
			Name  string `json:"name"`
			State string `json:"state"`
			URL   string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if e := json.Unmarshal(data, &release); e != nil {
		return nil, e
	}
	if release.Draft || release.Prerelease || !newer(release.Tag, current) || !strings.HasPrefix(release.Tag, "v") {
		return nil, nil
	}
	file := "CSQTT-" + release.Tag + "-Win10-x64.zip"
	page := "https://github.com/" + repository + "/releases/tag/" + release.Tag
	for _, a := range release.Assets {
		if a.Name == file && a.State == "uploaded" && a.URL == "https://github.com/"+repository+"/releases/download/"+release.Tag+"/"+file {
			return &Update{strings.TrimPrefix(release.Tag, "v"), page}, nil
		}
	}
	return nil, nil
}
func FetchUpdate(ctx context.Context, p Profile, local bool, current string) (*Update, error) {
	t, e := transport(p, local)
	if e != nil {
		return nil, e
	}
	defer t.CloseIdleConnections()
	req, _ := http.NewRequestWithContext(ctx, "GET", "https://api.github.com/repos/"+repository+"/releases/latest", nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "CSQTT/"+current)
	response, e := (&http.Client{Transport: t, Timeout: 12 * time.Second}).Do(req)
	if e != nil {
		return nil, errors.New("Не удалось проверить обновления. Проверьте доступ к GitHub.")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub HTTP %d", response.StatusCode)
	}
	data, e := io.ReadAll(io.LimitReader(response.Body, 262145))
	if e != nil || len(data) > 262144 {
		return nil, errors.New("Неверный ответ GitHub.")
	}
	return ParseUpdate(data, current)
}
