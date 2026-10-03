// Package control concentra o estado do show e traduz as ações da interface
// em comandos para o engine.
package control

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"

	"luzes/internal/engine"
	"luzes/internal/fixture"
	"luzes/internal/qlc"
)

// Teclas fixas dos atalhos da interface web (não podem ser usadas por botões).
const (
	KeyBlackout   = ' '
	KeyStopStrobe = 'b'
)

// Button é um botão do console já resolvido para uma Function.
type Button struct {
	ID       int // índice em Console.Buttons
	Caption  string
	Color    string // cor do botão no QLC+ ("#rrggbb"), opcional
	Group    string
	Layer    engine.Layer
	Key      byte // atalho de teclado na interface web; 0 se não tem
	Function int  // ID da Function no workspace do QLC+
	Name     string
	Src      engine.Source
}

// Label é como o botão aparece nos eventos.
func (b *Button) Label() string {
	return b.Group + " | " + b.Caption
}

// Console é o conjunto de botões e parâmetros do show.
type Console struct {
	Groups  []string
	Buttons []*Button
	Limits  struct{ Flash, Strobe time.Duration }
	// Workspace é o workspace do QLC+ de onde vêm as Functions.
	Workspace *qlc.Workspace
	// Skipped lista Functions do workspace que não puderam ser carregadas.
	Skipped []string
}

type consoleFile struct {
	Workspace string `yaml:"workspace"`
	Limits    struct {
		Flash  time.Duration `yaml:"flash"`
		Strobe time.Duration `yaml:"strobe"`
	} `yaml:"limits"`
	Groups []struct {
		Name    string `yaml:"name"`
		Layer   string `yaml:"layer"`
		Buttons []struct {
			Caption  string `yaml:"caption"`
			Function int    `yaml:"function"`
			Key      string `yaml:"key"`
			Color    string `yaml:"color"`
		} `yaml:"buttons"`
	} `yaml:"groups"`
}

var layers = map[string]engine.Layer{
	"base": engine.Base, "strobe": engine.Strobe, "flash": engine.Flash,
}

// LoadConsole lê o console e o workspace do QLC+ que ele referencia
// (caminho relativo ao arquivo do console).
func LoadConsole(path string) (*Console, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cf consoleFile
	if err := yaml.Unmarshal(data, &cf); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	ws, err := qlc.Load(filepath.Join(filepath.Dir(path), cf.Workspace))
	if err != nil {
		return nil, err
	}
	c, err := build(&cf, ws)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return c, nil
}

func build(cf *consoleFile, ws *qlc.Workspace) (*Console, error) {
	c := &Console{
		Workspace: ws,
		Skipped:   ws.Skipped,
	}
	c.Limits.Flash, c.Limits.Strobe = cf.Limits.Flash, cf.Limits.Strobe
	if c.Limits.Flash <= 0 || c.Limits.Strobe <= 0 {
		return nil, fmt.Errorf("limits.flash e limits.strobe são obrigatórios")
	}

	reserved := map[byte]bool{KeyBlackout: true, KeyStopStrobe: true}
	byKey := map[byte]*Button{}
	for _, g := range cf.Groups {
		layer, ok := layers[g.Layer]
		if !ok {
			return nil, fmt.Errorf("grupo %q: layer %q inválida (base, strobe ou flash)", g.Name, g.Layer)
		}
		c.Groups = append(c.Groups, g.Name)
		for _, b := range g.Buttons {
			name, src, ok := ws.Function(b.Function)
			if !ok {
				return nil, fmt.Errorf("botão %q: function %d não existe ou não é suportada", b.Caption, b.Function)
			}
			btn := &Button{ID: len(c.Buttons), Caption: b.Caption, Color: b.Color, Group: g.Name, Layer: layer, Function: b.Function, Name: name, Src: src}
			switch {
			case b.Key == "":
			case len(b.Key) != 1 || b.Key[0] <= ' ' || b.Key[0] > '~':
				return nil, fmt.Errorf("botão %q: tecla %q inválida (use uma letra ou símbolo ASCII)", b.Caption, b.Key)
			case reserved[b.Key[0]]:
				return nil, fmt.Errorf("botão %q: tecla %q é reservada", b.Caption, b.Key)
			case byKey[b.Key[0]] != nil:
				return nil, fmt.Errorf("botão %q: tecla %q já usada por %q", b.Caption, b.Key, byKey[b.Key[0]].Caption)
			default:
				btn.Key = b.Key[0]
				byKey[btn.Key] = btn
			}
			c.Buttons = append(c.Buttons, btn)
		}
	}
	return c, nil
}

// NewRig liga as posições do patch aos fixtures do workspace do QLC+ (o
// fixture de ID 0 é a primeira posição, e assim por diante).
func NewRig(fixtures []*fixture.Fixture, c *Console, patchPath string) (*fixture.Rig, error) {
	ws := c.Workspace.Fixtures
	if len(ws) != len(fixtures) {
		return nil, fmt.Errorf("o workspace do QLC+ tem %d fixtures e o patch tem %d", len(ws), len(fixtures))
	}
	logical := make([]int, len(ws))
	for i, f := range ws {
		if n := len(fixtures[i].Profile.Channels); n != f.Channels {
			return nil, fmt.Errorf("posição %q tem %d canais no perfil, mas %q no QLC+ tem %d", fixtures[i].Name, n, f.Name, f.Channels)
		}
		logical[i] = f.Address
	}
	return fixture.NewRig(fixtures, logical, patchPath)
}
