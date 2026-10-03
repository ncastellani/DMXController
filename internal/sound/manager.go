package sound

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// Settings são as preferências de som, guardadas em config/som.yaml.
type Settings struct {
	LatencyMs int     `yaml:"latency_ms"` // atraso da saída compensado na luz
	Volume    float64 `yaml:"volume"`     // volume do player (0..1)
	Output    string  `yaml:"output"`     // saída preferida (id do sistema)
	Speaker   string  `yaml:"speaker"`    // caixa Bluetooth a manter conectada
}

// Player é o que o Manager ajusta no player de áudio.
type Player interface {
	SetLatency(time.Duration)
	SetVolume(float64)
}

// State é o retrato para a interface.
type State struct {
	Platform string   `json:"platform"`
	Missing  []string `json:"missing,omitempty"` // ferramentas a instalar
	Outputs  []Output `json:"outputs"`
	Devices  []Device `json:"devices"`
	Speaker  string   `json:"speaker"` // endereço da caixa preferida
	Busy     string   `json:"busy,omitempty"`
	Warning  string   `json:"warning,omitempty"` // ex.: caixa caiu
	Error    string   `json:"error,omitempty"`   // erro da última ação
}

const (
	pollEvery      = 3 * time.Second
	reconnectEvery = 10 * time.Second
	scanTime       = 12 * time.Second
)

// Manager mantém o estado do som e executa as ações pedidas pela interface.
type Manager struct {
	b      Backend
	path   string
	player Player // nil sem saída de som
	notice func(string, ...any)

	mu       sync.Mutex
	set      Settings
	dirty    bool
	st       State
	lastTry  time.Time
	onChange []func()
}

// New escolhe o backend do sistema operacional.
func New() Backend {
	if runtime.GOOS == "darwin" {
		return NewDarwin()
	}
	return NewLinux()
}

// NewManager carrega as preferências de path (se existir) e as aplica ao
// player. player pode ser nil.
func NewManager(b Backend, path string, player Player, notice func(string, ...any)) (*Manager, error) {
	m := &Manager{b: b, path: path, player: player, notice: notice}
	m.set = Settings{Volume: 1}
	if data, err := os.ReadFile(path); err == nil {
		if err := yaml.Unmarshal(data, &m.set); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	m.st = State{Platform: runtime.GOOS, Missing: b.Missing(), Speaker: m.set.Speaker}
	if player != nil {
		player.SetLatency(time.Duration(m.set.LatencyMs) * time.Millisecond)
		player.SetVolume(m.set.Volume)
	}
	return m, nil
}

// OnChange registra quem deve ser avisado quando o estado muda.
func (m *Manager) OnChange(f func()) {
	m.mu.Lock()
	m.onChange = append(m.onChange, f)
	m.mu.Unlock()
}

func (m *Manager) changed() {
	m.mu.Lock()
	fs := m.onChange
	m.mu.Unlock()
	for _, f := range fs {
		f()
	}
}

// State devolve o estado atual.
func (m *Manager) State() State {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.st
}

// Settings devolve as preferências atuais.
func (m *Manager) Settings() Settings {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.set
}

// SetLatency ajusta e guarda o atraso da saída.
func (m *Manager) SetLatency(ms int) {
	ms = min(max(ms, 0), 1000)
	if m.player != nil {
		m.player.SetLatency(time.Duration(ms) * time.Millisecond)
	}
	m.mu.Lock()
	m.set.LatencyMs, m.dirty = ms, true
	m.mu.Unlock()
}

// SetVolume ajusta e guarda o volume do player.
func (m *Manager) SetVolume(v float64) {
	v = min(max(v, 0), 1)
	if m.player != nil {
		m.player.SetVolume(v)
	}
	m.mu.Lock()
	m.set.Volume, m.dirty = v, true
	m.mu.Unlock()
}

// Run atualiza o estado periodicamente, reconecta a caixa preferida e grava
// as preferências alteradas, até ctx acabar.
func (m *Manager) Run(ctx context.Context) {
	if len(m.State().Missing) > 0 {
		m.notice("som: instale %v para controlar saídas e Bluetooth pela tela", m.State().Missing)
	}
	m.refresh(ctx)
	if out := m.Settings().Output; out != "" {
		m.applyOutput(ctx, out)
	}
	save := time.NewTicker(time.Second)
	defer save.Stop()
	for i := 0; ; i++ {
		select {
		case <-ctx.Done():
			m.save()
			return
		case <-save.C:
		}
		m.save()
		if i%int(pollEvery/time.Second) == 0 {
			m.refresh(ctx)
			m.keepSpeaker(ctx)
		}
	}
}

func (m *Manager) save() {
	m.mu.Lock()
	if !m.dirty {
		m.mu.Unlock()
		return
	}
	set := m.set
	m.dirty = false
	m.mu.Unlock()
	data, err := yaml.Marshal(&set)
	if err != nil {
		return
	}
	header := []byte("# Preferências de som (gravadas pela interface web).\n")
	tmp := m.path + ".tmp"
	if err := os.WriteFile(tmp, append(header, data...), 0o644); err == nil {
		err = os.Rename(tmp, m.path)
		if err != nil {
			m.notice("som: não consegui gravar %s: %v", filepath.Base(m.path), err)
		}
	}
}

// refresh relê saídas e aparelhos e avisa se a caixa preferida caiu/voltou.
func (m *Manager) refresh(ctx context.Context) {
	if len(m.State().Missing) > 0 {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	outs, errO := m.b.Outputs(ctx)
	devs, errD := m.b.Devices(ctx)

	m.mu.Lock()
	before := m.st
	if errO == nil {
		m.st.Outputs = outs
	}
	if errD == nil {
		m.st.Devices = devs
	}
	m.st.Warning = ""
	var name string
	var wasUp, isUp bool
	if sp := m.set.Speaker; sp != "" {
		name = sp
		for _, d := range before.Devices {
			if d.Address == sp {
				wasUp = d.Connected
			}
		}
		for _, d := range m.st.Devices {
			if d.Address == sp {
				name, isUp = d.Name, d.Connected
			}
		}
		if !isUp {
			m.st.Warning = fmt.Sprintf("Caixa %s desconectada — tentando reconectar", name)
		}
	}
	m.mu.Unlock()

	if wasUp && !isUp {
		m.notice("⚠ caixa %s desconectou", name)
	} else if !wasUp && isUp && len(before.Devices) > 0 {
		m.notice("caixa %s conectada", name)
		if out := m.Settings().Output; out != "" {
			m.applyOutput(ctx, out) // ao voltar, o sistema pode ter trocado a saída
		}
	}
	m.changed()
}

// keepSpeaker tenta reconectar a caixa preferida, se ela caiu.
func (m *Manager) keepSpeaker(ctx context.Context) {
	m.mu.Lock()
	sp, busy := m.set.Speaker, m.st.Busy
	up := false
	for _, d := range m.st.Devices {
		if d.Address == sp {
			up = d.Connected
		}
	}
	if sp == "" || up || busy != "" || time.Since(m.lastTry) < reconnectEvery {
		m.mu.Unlock()
		return
	}
	m.lastTry = time.Now()
	m.mu.Unlock()
	cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if m.b.Connect(cctx, sp) == nil {
		m.refresh(ctx)
	}
}

func (m *Manager) applyOutput(ctx context.Context, id string) {
	for _, o := range m.State().Outputs {
		if o.ID == id && !o.Default {
			if err := m.b.SetOutput(ctx, id); err == nil {
				m.notice("saída de som: %s", o.Name)
			}
		}
	}
}

// Do executa uma ação da interface em segundo plano. action: output, scan,
// pair, connect, disconnect, forget.
func (m *Manager) Do(ctx context.Context, action, arg string) error {
	if miss := m.State().Missing; len(miss) > 0 {
		return fmt.Errorf("instale %v para usar esta função", miss)
	}
	labels := map[string]string{
		"output": "trocando a saída", "scan": "procurando caixas",
		"pair": "pareando", "connect": "conectando", "disconnect": "desconectando", "forget": "esquecendo",
	}
	label, ok := labels[action]
	if !ok {
		return fmt.Errorf("ação %q desconhecida", action)
	}
	m.mu.Lock()
	if m.st.Busy != "" {
		busy := m.st.Busy
		m.mu.Unlock()
		return fmt.Errorf("aguarde: %s", busy)
	}
	m.st.Busy, m.st.Error = label+"…", ""
	m.mu.Unlock()
	m.changed()

	go func() {
		cctx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
		var err error
		switch action {
		case "output":
			if err = m.b.SetOutput(cctx, arg); err == nil {
				m.mu.Lock()
				m.set.Output, m.dirty = arg, true
				// Saída Bluetooth escolhida: aquela caixa passa a ser mantida conectada.
				for _, o := range m.st.Outputs {
					for _, d := range m.st.Devices {
						if o.ID == arg && o.Kind == "bluetooth" && o.Name == d.Name {
							m.set.Speaker, m.st.Speaker = d.Address, d.Address
						}
					}
				}
				m.mu.Unlock()
			}
		case "scan":
			var devs []Device
			if devs, err = m.b.Scan(cctx, scanTime); err == nil {
				m.mu.Lock()
				m.st.Devices = devs
				m.mu.Unlock()
			}
		case "pair", "connect":
			if action == "pair" {
				err = m.b.Pair(cctx, arg)
			} else {
				err = m.b.Connect(cctx, arg)
			}
			if err == nil { // a caixa conectada vira a preferida
				m.mu.Lock()
				m.set.Speaker, m.st.Speaker, m.dirty = arg, arg, true
				m.mu.Unlock()
			}
		case "disconnect", "forget":
			m.mu.Lock() // desconectar de propósito: não reconectar sozinho
			if m.set.Speaker == arg {
				m.set.Speaker, m.st.Speaker, m.dirty = "", "", true
			}
			m.mu.Unlock()
			if action == "disconnect" {
				err = m.b.Disconnect(cctx, arg)
			} else {
				err = m.b.Forget(cctx, arg)
			}
		}
		m.mu.Lock()
		m.st.Busy = ""
		if err != nil {
			m.st.Error = err.Error()
		}
		m.mu.Unlock()
		if err != nil {
			m.notice("som: %v", err)
		} else {
			m.notice("som: %s ok", label)
		}
		if action != "scan" {
			m.refresh(ctx)
		}
		m.changed()
	}()
	return nil
}
