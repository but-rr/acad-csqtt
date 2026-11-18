package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// String fields keep existing portable profiles compatible without conversion.
type Profile struct {
	Server        string `json:"server"`
	Password      string `json:"password"`
	Hashes        string `json:"hashes"`
	Proxy         string `json:"proxy"`
	ProxyUser     string `json:"proxy_user"`
	ProxyPassword string `json:"proxy_password"`
	SocksPort     string `json:"socks_port"`
	HTTPPort      string `json:"http_port"`
	Workers       string `json:"workers"`
	Turn          string `json:"turn"`
	Obfs          string `json:"obfs"`
	SRTP          string `json:"srtp"`
	Generation    string `json:"generation"`
	Salt          string `json:"salt"`
	InternetMode  string `json:"internet_mode"`
	SystemProxy   string `json:"system_proxy"`
	Tun           string `json:"tun"`
}

func DefaultProfile() Profile {
	return Profile{Proxy: "10.0.0.1:3128", SocksPort: "1080", HTTPPort: "8080", Workers: "6", Obfs: "unified", SRTP: "auto", Generation: "0", InternetMode: "proxy", SystemProxy: "0", Tun: "0"}
}

var separators = regexp.MustCompile(`[\s,;]+`)

func (p Profile) NormalizedHashes() ([]string, error) {
	result := []string{}
	seen := map[string]bool{}
	for _, token := range separators.Split(strings.TrimSpace(p.Hashes), -1) {
		token = strings.Trim(token, "<>\"'")
		if i := strings.Index(token, "/call/join/"); i >= 0 {
			token = token[i+11:]
		} else if strings.HasPrefix(token, "http://") || strings.HasPrefix(token, "https://") {
			return nil, errors.New("Ссылка VK должна содержать /call/join/.")
		}
		if i := strings.IndexAny(token, "?#/"); i >= 0 {
			token = token[:i]
		}
		if token != "" && !seen[token] {
			seen[token] = true
			result = append(result, token)
		}
	}
	return result, nil
}

func Address(value string) (string, int, error) {
	host, port, e := net.SplitHostPort(strings.TrimSpace(value))
	if e != nil {
		return "", 0, errors.New("Укажите адрес в формате IP:порт или домен:порт.")
	}
	number, e := strconv.Atoi(port)
	if e != nil || number < 1 || number > 65535 || host == "" || strings.ContainsAny(host, " /@?#\r\n\t") {
		return "", 0, errors.New("Неверный адрес или порт.")
	}
	return host, number, nil
}
func (p Profile) Validate() ([]string, error) {
	if _, _, e := Address(p.Server); e != nil {
		return nil, fmt.Errorf("Сервер CSQTT: %w", e)
	}
	if p.Password == "" {
		return nil, errors.New("Введите пароль сервера CSQTT.")
	}
	if p.InternetMode != "proxy" && p.InternetMode != "direct" {
		return nil, errors.New("Неверный режим подключения.")
	}
	if (p.Tun != "0" && p.Tun != "1") || (p.SystemProxy != "0" && p.SystemProxy != "1") {
		return nil, errors.New("Неверный режим туннеля.")
	}
	if p.InternetMode == "proxy" {
		host, _, e := Address(p.Proxy)
		if e != nil || net.ParseIP(host) == nil || net.ParseIP(host).To4() == nil {
			return nil, errors.New("Корпоративный прокси: нужен IPv4:порт.")
		}
		if p.ProxyUser != "" && p.ProxyPassword == "" {
			return nil, errors.New("Введите пароль корпоративного прокси.")
		}
		if strings.Contains(p.ProxyUser, ":") || strings.ContainsAny(p.ProxyUser+p.ProxyPassword, "\r\n") {
			return nil, errors.New("Недопустимые символы в авторизации прокси.")
		}
	}
	hashes, e := p.NormalizedHashes()
	if e != nil {
		return nil, e
	}
	if len(hashes) < 1 || len(hashes) > 6 {
		return nil, errors.New("Нужно от 1 до 6 хешей VK или ссылок звонков.")
	}
	if strings.ContainsAny(p.Turn, ":/ \r\n\t") {
		return nil, errors.New("TURN: IPv4 или домен без порта и схемы.")
	}
	socks, e := strconv.Atoi(p.SocksPort)
	httpPort, e2 := strconv.Atoi(p.HTTPPort)
	if e != nil || e2 != nil || socks < 1024 || socks > 65535 || httpPort < 1024 || httpPort > 65535 || socks == httpPort {
		return nil, errors.New("Локальные порты: разные числа от 1024 до 65535.")
	}
	workers, e := strconv.Atoi(p.Workers)
	if e != nil || workers < 1 || workers > 100 {
		return nil, errors.New("Потоки: от 1 до 100.")
	}
	if _, e := strconv.ParseUint(p.Generation, 10, 64); e != nil {
		return nil, errors.New("Поколение: целое неотрицательное число.")
	}
	if p.Obfs != "unified" && p.Obfs != "audio" && p.Obfs != "video" {
		return nil, errors.New("Неверная обфускация.")
	}
	if p.SRTP != "auto" && p.SRTP != "wrap" && p.SRTP != "dtls" {
		return nil, errors.New("Неверный режим SRTP.")
	}
	return hashes, nil
}
func LoadProfile(directory string) (Profile, error) {
	p := DefaultProfile()
	data, e := os.ReadFile(filepath.Join(directory, "profile.json"))
	if os.IsNotExist(e) {
		data, e = legacyProfile()
		if os.IsNotExist(e) {
			return p, nil
		}
	}
	if e != nil {
		return p, e
	}
	e = json.Unmarshal([]byte(strings.TrimPrefix(string(data), "\ufeff")), &p)
	if e != nil {
		return DefaultProfile(), errors.New("Не удалось прочитать profile.json. Исходный файл сохранён.")
	}
	return p, nil
}
func SaveProfile(directory string, p Profile, remember bool) error {
	path := filepath.Join(directory, "profile.json")
	if !remember {
		e := os.Remove(path)
		if e != nil && !os.IsNotExist(e) {
			return e
		}
		return forgetLegacy()
	}
	data, e := json.MarshalIndent(p, "", "  ")
	if e != nil {
		return e
	}
	temp := path + ".tmp"
	defer os.Remove(temp)
	if e = os.WriteFile(temp, append(data, '\n'), 0600); e != nil {
		return e
	}
	return os.Rename(temp, path)
}
func CoreCommand(directory string, p Profile, port int) ([]string, []string, error) {
	hashes, e := p.Validate()
	if e != nil {
		return nil, nil, e
	}
	args := []string{filepath.Join(directory, "bin/csqtt-core.exe"), "--peer", strings.TrimSpace(p.Server), "--listen", fmt.Sprintf("127.0.0.1:%d", port), "--vk", strings.Join(hashes, ","), "--workers", p.Workers, "--exact-workers", "--turn-transport", "tcp", "--port", "443", "--obfs", p.Obfs, "--srtp-mode", p.SRTP, "--gen", p.Generation, "--salt", p.Salt}
	if strings.TrimSpace(p.Turn) != "" {
		args = append(args, "--turn", strings.TrimSpace(p.Turn))
	}
	env := []string{}
	for _, item := range os.Environ() {
		key := strings.ToUpper(strings.SplitN(item, "=", 2)[0])
		switch key {
		case "HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY", "CSQTT_HTTP_PROXY", "CSQTT_PROXY_USER", "CSQTT_PROXY_PASSWORD", "CSQTT_PASSWORD", "CSQTT_EVENTS":
			continue
		}
		env = append(env, item)
	}
	env = append(env, "CSQTT_PASSWORD="+p.Password, "CSQTT_EVENTS=1")
	if p.InternetMode == "proxy" {
		env = append(env, "CSQTT_HTTP_PROXY="+strings.TrimSpace(p.Proxy), "CSQTT_PROXY_USER="+p.ProxyUser, "CSQTT_PROXY_PASSWORD="+p.ProxyPassword)
	}
	return args, env, nil
}
