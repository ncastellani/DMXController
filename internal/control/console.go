// Package control concentra o estado do show e traduz as ações da interface
// em comandos para o engine.
package control

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/ncastellani/DMXController/internal/engine"
	"github.com/ncastellani/DMXController/internal/fixture"
	"github.com/ncastellani/DMXController/internal/library"
)

// Teclas fixas dos atalhos da interface web (não podem ser usadas por botões).
const (
	KeyBlackout   = ' '
	KeyStopStrobe = 'b'
)

// Button é um botão do console já resolvido para um preset da biblioteca.
type Button struct {
	ID      int // índice em Console.Buttons
	Caption string
	Color   string // cor do botão na interface ("#rrggbb"), opcional
	Group   string
	Layer   engine.Layer
	Key     byte   // atalho de teclado na interface web; 0 se não tem
	Preset  string // id do preset na biblioteca ("pista:club")
	Name    string // nome de exibição do preset ("PISTA | CLUB")
	Src     engine.Source
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
	// Library é a biblioteca de onde vêm os presets.
	Library *library.Library
}

type consoleFile struct {
	Library string `yaml:"library"`
	Limits  struct {
		Flash  time.Duration `yaml:"flash"`
		Strobe time.Duration `yaml:"strobe"`
	} `yaml:"limits"`
	Groups []struct {
		Name    string `yaml:"name"`
		Layer   string `yaml:"layer"`
		Buttons []struct {
			Caption string `yaml:"caption"`
			Preset  string `yaml:"preset"`
			Key     string `yaml:"key"`
			Color   string `yaml:"color"`
		} `yaml:"buttons"`
	} `yaml:"groups"`
}

var layers = map[string]engine.Layer{
	"base": engine.Base, "strobe": engine.Strobe, "flash": engine.Flash,
}

// LoadConsole lê o console e a biblioteca que ele referencia (caminho
// relativo ao arquivo do console). positions são os canhões com endereço
// lógico, onde os presets são desenhados.
func LoadConsole(path string, positions []*fixture.Fixture) (*Console, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cf consoleFile
	if err := yaml.Unmarshal(data, &cf); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if cf.Library == "" {
		return nil, fmt.Errorf("%s: falta library (o arquivo da biblioteca de presets)", path)
	}
	lib, err := library.Load(filepath.Join(filepath.Dir(path), cf.Library))
	if err != nil {
		return nil, err
	}
	c, err := build(&cf, lib, positions)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return c, nil
}

func build(cf *consoleFile, lib *library.Library, positions []*fixture.Fixture) (*Console, error) {
	c := &Console{Library: lib}
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
			name, src, ok := lib.Preset(b.Preset, positions)
			if !ok {
				return nil, fmt.Errorf("botão %q: preset %q não existe na biblioteca", b.Caption, b.Preset)
			}
			btn := &Button{ID: len(c.Buttons), Caption: b.Caption, Color: b.Color, Group: g.Name, Layer: layer, Preset: lib.Canonical(b.Preset), Name: name, Src: src}
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

// NewRig monta o rig: no quadro lógico (onde os presets são desenhados), as
// posições do patch ocupam blocos seguidos a partir do canal 1 (esquerda em
// 1, centro em 8, direita em 15, com 7 canais cada).
func NewRig(fixtures []*fixture.Fixture, patchPath string) (*fixture.Rig, error) {
	logical := make([]int, len(fixtures))
	next := 1
	for i, f := range fixtures {
		logical[i] = next
		next += len(f.Profile.Channels)
	}
	return fixture.NewRig(fixtures, logical, patchPath)
}

// Positions devolve os canhões do rig com o endereço lógico.
func Positions(rig *fixture.Rig) []*fixture.Fixture {
	var out []*fixture.Fixture
	for i := range rig.Fixtures {
		out = append(out, rig.Logical(i))
	}
	return out
}
