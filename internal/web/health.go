package web

import "sync"

// Health guarda o estado do hardware para avisar na interface.
type Health struct {
	mu       sync.Mutex
	dmxOK    bool
	dmxErr   string
	audioErr string
	warnings []string
}

// Warn adiciona um aviso fixo (ex.: programa rodando como root).
func (h *Health) Warn(text string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.warnings = append(h.warnings, text)
}

// SetDMX registra se a interface DMX está conectada.
func (h *Health) SetDMX(ok bool, err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.dmxOK, h.dmxErr = ok, errText(err)
}

// SetAudio registra o erro da saída de som (nil = ok).
func (h *Health) SetAudio(err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.audioErr = errText(err)
}

type healthMsg struct {
	DMX      bool     `json:"dmx"`
	DMXError string   `json:"dmxError,omitempty"`
	Audio    bool     `json:"audio"`
	AudioErr string   `json:"audioError,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

func (h *Health) snapshot() healthMsg {
	if h == nil {
		return healthMsg{DMX: true, Audio: true}
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return healthMsg{DMX: h.dmxOK, DMXError: h.dmxErr, Audio: h.audioErr == "", AudioErr: h.audioErr, Warnings: h.warnings}
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
