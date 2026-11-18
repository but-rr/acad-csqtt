package client

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

type LogLine struct {
	Time    string `json:"time"`
	Message string `json:"message"`
	Level   string `json:"level"`
}
type State struct {
	Status         string    `json:"status"`
	Message        string    `json:"message"`
	Active         int       `json:"active"`
	Upload         uint64    `json:"upload"`
	Download       uint64    `json:"download"`
	IP             string    `json:"ip"`
	Ready          bool      `json:"ready"`
	Logs           []LogLine `json:"logs"`
	Revision       uint64    `json:"revision"`
	Update         *Update   `json:"update"`
	UpdateChecking bool      `json:"updateChecking"`
	UpdateMessage  string    `json:"updateMessage"`
}
type process struct {
	cmd   *exec.Cmd
	stdin io.WriteCloser
	done  chan struct{}
	write sync.Mutex
}

func (p *process) send(line string) {
	p.write.Lock()
	defer p.write.Unlock()
	io.WriteString(p.stdin, line+"\n")
}

type Manager struct {
	mu                  sync.Mutex
	Directory, Version  string
	state               State
	core, helper        *process
	session             uint64
	profile             Profile
	port                int
	configIP, configDNS string
	ready               bool
	bypass              []string
	secrets             []string
	ctx                 context.Context
	cancel              context.CancelFunc
	stopDone            chan struct{}
}

func NewManager(directory, version string) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{Directory: directory, Version: version, state: State{Status: "offline", Message: "Готов к подключению", Logs: []LogLine{}}, ctx: ctx, cancel: cancel}
}
func (m *Manager) Snapshot() State {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.state
	s.Logs = append([]LogLine{}, s.Logs...)
	return s
}
func (m *Manager) set(status, message string) {
	m.state.Status = status
	m.state.Message = message
	m.state.Revision++
}

var secretPattern = regexp.MustCompile(`(?i)(access_token|client_secret|password|secret)=([^\s&]+)`)

func (m *Manager) log(line string) {
	for _, secret := range m.secrets {
		if secret != "" {
			line = strings.ReplaceAll(line, secret, "[скрыто]")
		}
	}
	line = secretPattern.ReplaceAllString(line, "$1=[скрыто]")
	level := "info"
	lower := strings.ToLower(line)
	if strings.Contains(lower, "ошиб") || strings.Contains(lower, "failed") || strings.Contains(lower, "fatal") {
		level = "error"
	}
	m.state.Logs = append(m.state.Logs, LogLine{time.Now().Format("15:04:05"), line, level})
	if len(m.state.Logs) > 400 {
		copy(m.state.Logs, m.state.Logs[len(m.state.Logs)-400:])
		m.state.Logs = m.state.Logs[:400]
	}
	m.state.Revision++
}
func (m *Manager) launch(args, env []string, kind string, session uint64) (*process, error) {
	cmd := exec.Command(args[0], args[1:]...)
	hidden(cmd)
	cmd.Env = env
	stdin, e := cmd.StdinPipe()
	if e != nil {
		return nil, e
	}
	reader, writer := io.Pipe()
	cmd.Stdout = writer
	cmd.Stderr = writer
	if e = cmd.Start(); e != nil {
		stdin.Close()
		reader.Close()
		writer.Close()
		return nil, e
	}
	p := &process{cmd: cmd, stdin: stdin, done: make(chan struct{})}
	go func() {
		scanner := bufio.NewScanner(reader)
		scanner.Buffer(make([]byte, 4096), 1024*1024)
		for scanner.Scan() {
			m.consume(session, kind, scanner.Text())
		}
		reader.Close()
	}()
	go func() {
		e := cmd.Wait()
		writer.Close()
		stdin.Close()
		close(p.done)
		m.mu.Lock()
		current := m.session == session && m.state.Status != "stopping"
		if current {
			m.log(fmt.Sprintf("%s завершился: %v", kind, e))
		}
		m.mu.Unlock()
		if current {
			m.Stop()
		}
	}()
	return p, nil
}
func (m *Manager) Start(p Profile) error {
	hashes, e := p.Validate()
	if e != nil {
		return e
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.core != nil || m.state.Status == "stopping" {
		return errors.New("Дождитесь завершения текущего подключения.")
	}
	for _, name := range []string{"csqtt-core.exe", "packet-proxy.exe"} {
		if _, e := os.Stat(filepath.Join(m.Directory, "bin", name)); e != nil {
			return fmt.Errorf("Не найден bin/%s. Распакуйте архив целиком.", name)
		}
	}
	for _, port := range []string{p.SocksPort, p.HTTPPort} {
		l, e := net.Listen("tcp4", "127.0.0.1:"+port)
		if e != nil {
			return fmt.Errorf("Локальный порт %s занят.", port)
		}
		l.Close()
	}
	udp, e := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if e != nil {
		return e
	}
	m.port = udp.LocalAddr().(*net.UDPAddr).Port
	udp.Close()
	args, env, e := CoreCommand(m.Directory, p, m.port)
	if e != nil {
		return e
	}
	m.session++
	m.profile = p
	m.ready = false
	m.configIP = ""
	m.configDNS = ""
	m.bypass = nil
	m.secrets = []string{p.Password, p.ProxyPassword, p.Salt}
	for _, h := range hashes {
		m.secrets = append(m.secrets, h)
		if len(h) >= 8 {
			m.secrets = append(m.secrets, h[:8])
		}
	}
	sort.Slice(m.secrets, func(i, j int) bool { return len(m.secrets[i]) > len(m.secrets[j]) })
	m.state.Ready = false
	m.state.IP = ""
	m.state.Active = 0
	m.state.Upload = 0
	m.state.Download = 0
	m.core, e = m.launch(args, env, "core", m.session)
	if e != nil {
		return e
	}
	m.set("connecting", "Получаю конфигурацию сервера…")
	m.log("Подключение CSQTT · TURN TCP / 443")
	return nil
}
func (m *Manager) helperLocked() {
	if m.helper != nil || !m.ready || m.configIP == "" || m.core == nil || m.state.Status == "stopping" {
		return
	}
	p := m.profile
	args := []string{filepath.Join(m.Directory, "bin/packet-proxy.exe"), "--core", fmt.Sprintf("127.0.0.1:%d", m.port), "--ip", m.configIP, "--dns", m.configDNS, "--socks", "127.0.0.1:" + p.SocksPort, "--http", "127.0.0.1:" + p.HTTPPort, "--parent-pid", fmt.Sprint(os.Getpid()), "--system-proxy=" + fmt.Sprint(p.SystemProxy == "1")}
	if p.Tun == "1" {
		bypass := m.bypass
		if p.InternetMode == "proxy" {
			host, _, _ := Address(p.Proxy)
			bypass = []string{host}
		}
		if len(bypass) == 0 {
			return
		}
		args = append(args, "--mode", "tun", "--bypass-ip", strings.Join(bypass, ","))
	}
	helper, e := m.launch(args, os.Environ(), "helper", m.session)
	if e != nil {
		m.log("Ошибка запуска локального прокси.")
		go m.Stop()
		return
	}
	m.helper = helper
}
func (m *Manager) consume(session uint64, kind, line string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if session != m.session {
		return
	}
	if kind == "core" && strings.HasPrefix(line, "TUN_BYPASS:") {
		ip := strings.TrimSpace(strings.TrimPrefix(line, "TUN_BYPASS:"))
		if net.ParseIP(ip) == nil {
			return
		}
		found := false
		for _, value := range m.bypass {
			if value == ip {
				found = true
			}
		}
		if !found {
			m.bypass = append(m.bypass, ip)
			if m.helper != nil && m.profile.Tun == "1" && m.profile.InternetMode == "direct" {
				helper := m.helper
				go helper.send("BYPASS " + ip)
			}
		}
		m.helperLocked()
		return
	}
	if strings.HasPrefix(line, "__CSQTT_EVENT__|") {
		parts := strings.SplitN(line, "|", 3)
		if len(parts) != 3 {
			return
		}
		var payload struct {
			Config  string `json:"config"`
			Active  int    `json:"active"`
			Up      uint64 `json:"bytes_up"`
			Down    uint64 `json:"bytes_down"`
			Message string `json:"message"`
		}
		if json.Unmarshal([]byte(parts[2]), &payload) != nil {
			return
		}
		switch parts[1] {
		case "CONFIG":
			fields := strings.Split(payload.Config, ":")
			if len(fields) < 3 || fields[0] != "TUNCONF" || net.ParseIP(fields[1]) == nil {
				m.log("Ошибка конфигурации сервера.")
				go m.Stop()
				return
			}
			if m.configIP != "" && (m.configIP != fields[1] || m.configDNS != fields[2]) {
				m.log("Сервер изменил IP/DNS. Переподключитесь.")
				go m.Stop()
				return
			}
			m.configIP, m.configDNS = fields[1], fields[2]
		case "READY":
			m.ready = true
		case "STATS":
			m.state.Active = payload.Active
			m.state.Upload = payload.Up
			m.state.Download = payload.Down
			m.state.Revision++
		case "ACTIVE_ZERO", "SERVER_RESTART", "NETWORK_SUSPECT":
			m.set("recovering", "Восстанавливаю связь с сервером…")
		case "ERROR":
			m.log(payload.Message)
		}
		m.helperLocked()
		return
	}
	if kind == "helper" && strings.HasPrefix(line, "PROXY_READY") {
		m.state.Ready = true
		m.set("checking", "Проверяю доступ в интернет…")
		go m.CheckInternet()
	}
	if strings.HasPrefix(line, "SYSTEM_PROXY_ERROR") {
		m.state.Message = "Не удалось изменить системный прокси. Откройте журнал."
	}
	m.log(line)
}
func (m *Manager) Stop() {
	m.mu.Lock()
	if m.stopDone != nil {
		m.mu.Unlock()
		return
	}
	if m.core == nil && m.helper == nil {
		m.mu.Unlock()
		return
	}
	done := make(chan struct{})
	m.stopDone = done
	m.session++
	core, helper, tun := m.core, m.helper, m.profile.Tun == "1"
	m.core = nil
	m.helper = nil
	m.state.Ready = false
	m.set("stopping", "Восстанавливаю настройки сети…")
	m.mu.Unlock()
	go func() {
		defer close(done)
		for _, p := range []*process{helper, core} {
			if p == nil {
				continue
			}
			go p.send("STOP")
		}
		for _, p := range []*process{helper, core} {
			if p == nil {
				continue
			}
			timeout := 3 * time.Second
			if p == helper && tun {
				timeout = 20 * time.Second
			}
			select {
			case <-p.done:
			case <-time.After(timeout):
				p.cmd.Process.Kill()
				<-p.done
			}
		}
		m.Recover(tun)
		m.mu.Lock()
		m.state.Active = 0
		m.state.Ready = false
		m.set("offline", "Отключено")
		m.stopDone = nil
		m.mu.Unlock()
	}()
}
func (m *Manager) Close() {
	m.cancel()
	m.Stop()
	m.mu.Lock()
	done := m.stopDone
	m.mu.Unlock()
	if done != nil {
		<-done
	}
}
func (m *Manager) Recover(tun bool) {
	flags := []string{"--recover-proxy"}
	if tun {
		flags = append(flags, "--recover-tun")
	}
	for _, flag := range flags {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		cmd := exec.CommandContext(ctx, filepath.Join(m.Directory, "bin/packet-proxy.exe"), flag)
		hidden(cmd)
		cmd.Run()
		cancel()
	}
}
func (m *Manager) SetSystemProxy(enabled bool) {
	m.mu.Lock()
	helper := m.helper
	m.mu.Unlock()
	if helper != nil {
		command := "SYSTEM_PROXY_OFF"
		if enabled {
			command = "SYSTEM_PROXY_ON"
		}
		go helper.send(command)
	}
}
func (m *Manager) CheckInternet() {
	m.mu.Lock()
	if !m.state.Ready {
		m.mu.Unlock()
		return
	}
	p, session := m.profile, m.session
	m.mu.Unlock()
	ip, e := CheckInternet(m.ctx, p)
	m.mu.Lock()
	if session == m.session {
		if e != nil {
			m.set("unconfirmed", e.Error())
		} else {
			m.state.IP = ip
			m.set("online", "Интернет через CSQTT работает")
		}
	}
	m.mu.Unlock()
	if e == nil {
		m.CheckUpdates(p)
	}
}
func (m *Manager) CheckUpdates(profile Profile) {
	m.mu.Lock()
	if m.state.UpdateChecking {
		m.mu.Unlock()
		return
	}
	m.state.UpdateChecking = true
	local := m.state.Ready
	if local {
		profile = m.profile
	}
	m.state.Revision++
	m.mu.Unlock()
	go func() {
		update, e := FetchUpdate(m.ctx, profile, local, m.Version)
		m.mu.Lock()
		defer m.mu.Unlock()
		m.state.UpdateChecking = false
		m.state.Revision++
		if e != nil {
			m.state.UpdateMessage = e.Error()
		} else if update != nil {
			m.state.Update = update
			m.state.UpdateMessage = "Доступна версия v" + update.Version
		} else {
			m.state.UpdateMessage = "Установлена актуальная версия"
		}
	}()
}
