// Package sound gerencia a saída de som do show: escolhe a saída do sistema
// (Bluetooth, P2, alto-falante), conecta a caixa Bluetooth e guarda as
// configurações (atraso, volume) entre execuções.
//
// Linux usa pw-dump/wpctl (PipeWire) e bluetoothctl (BlueZ); macOS usa
// SwitchAudioSource e blueutil (brew install switchaudio-osx blueutil). No
// Windows, a saída e a caixa ficam nas Configurações do sistema (windows.go).
package sound

import (
	"context"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Output é uma saída de som do sistema.
type Output struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Kind    string `json:"kind"` // bluetooth | wired (P2) | builtin | hdmi | other
	Default bool   `json:"default"`
}

// Device é um aparelho Bluetooth (caixa, fone...).
type Device struct {
	Address   string `json:"address"`
	Name      string `json:"name"`
	Paired    bool   `json:"paired"`
	Connected bool   `json:"connected"`
}

// Backend fala com o sistema operacional.
type Backend interface {
	// Missing lista ferramentas necessárias que não foram encontradas.
	Missing() []string
	Outputs(ctx context.Context) ([]Output, error)
	SetOutput(ctx context.Context, id string) error
	Devices(ctx context.Context) ([]Device, error)
	// Scan procura aparelhos por d e devolve os encontrados (pareados ou não).
	Scan(ctx context.Context, d time.Duration) ([]Device, error)
	// Pair pareia, marca como confiável e conecta.
	Pair(ctx context.Context, addr string) error
	Connect(ctx context.Context, addr string) error
	Disconnect(ctx context.Context, addr string) error
	Forget(ctx context.Context, addr string) error
}

// runner executa um comando e devolve stdout+stderr (trocável nos testes).
type runner func(ctx context.Context, name string, args ...string) ([]byte, error)

func execRun(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

// lookPath acha um executável no PATH ou nos lugares comuns do Homebrew
// (o programa pode ser aberto sem o PATH do terminal).
func lookPath(name string) (string, bool) {
	if p, err := exec.LookPath(name); err == nil {
		return p, true
	}
	for _, dir := range []string{"/opt/homebrew/bin", "/usr/local/bin"} {
		p := filepath.Join(dir, name)
		if _, err := exec.LookPath(p); err == nil {
			return p, true
		}
	}
	return name, false
}

var ansi = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]|\r|\x01|\x02`)

func clean(b []byte) string { return ansi.ReplaceAllString(string(b), "") }

// kindOf classifica uma saída pelo nome (fallback quando o sistema não diz).
func kindOf(name string) string {
	n := strings.ToLower(name)
	switch {
	case strings.Contains(n, "hdmi") || strings.Contains(n, "displayport") || strings.Contains(n, "display"):
		return "hdmi"
	case strings.Contains(n, "fone") || strings.Contains(n, "headphone") || strings.Contains(n, "analog") || strings.Contains(n, "line out"):
		return "wired"
	case strings.Contains(n, "speaker") || strings.Contains(n, "alto-falante"):
		return "builtin"
	}
	return "other"
}
