// Package fixture converte o estado abstrato de cada canhão (cor, intensidade)
// em canais DMX, a partir de perfis e de um patch configurados em YAML.
package fixture

import (
	"fmt"
	"os"
	"sort"

	"gopkg.in/yaml.v3"

	"github.com/ncastellani/DMXController/internal/dmx"
)

// Role é a função de um canal dentro do perfil.
type Role string

const (
	Dimmer Role = "dimmer"
	Red    Role = "red"
	Green  Role = "green"
	Blue   Role = "blue"
	White  Role = "white"
	Mode   Role = "mode"  // seleção de função/macro interna do fixture
	Speed  Role = "speed" // velocidade da função interna
	Empty  Role = "empty" // canal sem uso
)

var knownRoles = map[Role]bool{
	Dimmer: true, Red: true, Green: true, Blue: true, White: true,
	Mode: true, Speed: true, Empty: true,
}

// Profile descreve o mapa de canais de um modelo de fixture.
type Profile struct {
	Name     string         `yaml:"name"`
	Channels []Role         `yaml:"channels"`
	Fixed    map[Role]uint8 `yaml:"fixed"` // valores constantes (ex.: mode)
}

// Color é uma cor RGB.
type Color struct{ R, G, B uint8 }

// State é o estado abstrato de um fixture.
type State struct {
	Color     Color
	Intensity float64 // 0..1
}

// Fixture é um canhão patcheado num endereço.
type Fixture struct {
	Name      string
	Address   int // canal DMX inicial (1..512)
	ProfileID string
	Profile   *Profile
}

// Render escreve no quadro os canais deste fixture para o estado s.
func (f *Fixture) Render(frame *dmx.Frame, s State) {
	intensity := clamp01(s.Intensity)
	hasDimmer := false
	for _, r := range f.Profile.Channels {
		if r == Dimmer {
			hasDimmer = true
		}
	}
	// Sem canal de dimmer, a intensidade é aplicada direto nas cores.
	scale := func(v uint8) byte {
		if hasDimmer {
			return v
		}
		return byte(float64(v)*intensity + 0.5)
	}
	for i, role := range f.Profile.Channels {
		var v byte
		switch role {
		case Dimmer:
			v = byte(intensity*255 + 0.5)
		case Red:
			v = scale(s.Color.R)
		case Green:
			v = scale(s.Color.G)
		case Blue:
			v = scale(s.Color.B)
		default:
			v = f.Profile.Fixed[role]
		}
		frame.Set(f.Address+i, v)
	}
}

// Look estima, a partir do quadro, a cor que o fixture está emitindo e se
// está em strobo. É uma aproximação para o simulador: o strobo é inferido do
// canal de função na faixa 1..50 com velocidade > 0 (MM-T114).
func (f *Fixture) Look(frame *dmx.Frame) (c Color, strobe bool) {
	dim := 1.0
	var mode, speed byte
	for i, role := range f.Profile.Channels {
		v := frame[f.Address+i-1]
		switch role {
		case Dimmer:
			dim = float64(v) / 255
		case Red:
			c.R = v
		case Green:
			c.G = v
		case Blue:
			c.B = v
		case Mode:
			mode = v
		case Speed:
			speed = v
		}
	}
	c = Color{
		R: byte(float64(c.R)*dim + 0.5),
		G: byte(float64(c.G)*dim + 0.5),
		B: byte(float64(c.B)*dim + 0.5),
	}
	return c, mode >= 1 && mode <= 50 && speed > 0
}

func clamp01(x float64) float64 {
	switch {
	case x < 0:
		return 0
	case x > 1:
		return 1
	}
	return x
}

type profilesFile struct {
	Profiles map[string]*Profile `yaml:"profiles"`
}

type patchFile struct {
	Fixtures []struct {
		Name    string `yaml:"name"`
		Profile string `yaml:"profile"`
		Address int    `yaml:"address"`
	} `yaml:"fixtures"`
}

// Load lê os perfis e o patch e devolve os fixtures validados.
func Load(profilesPath, patchPath string) ([]*Fixture, error) {
	var pf profilesFile
	if err := readYAML(profilesPath, &pf); err != nil {
		return nil, err
	}
	var patch patchFile
	if err := readYAML(patchPath, &patch); err != nil {
		return nil, err
	}

	for id, p := range pf.Profiles {
		if len(p.Channels) == 0 {
			return nil, fmt.Errorf("perfil %q sem canais", id)
		}
		for _, r := range p.Channels {
			if !knownRoles[r] {
				return nil, fmt.Errorf("perfil %q: função de canal desconhecida %q", id, r)
			}
		}
	}

	var fixtures []*Fixture
	for _, e := range patch.Fixtures {
		p, ok := pf.Profiles[e.Profile]
		if !ok {
			return nil, fmt.Errorf("fixture %q: perfil %q não existe", e.Name, e.Profile)
		}
		fixtures = append(fixtures, &Fixture{Name: e.Name, Address: e.Address, ProfileID: e.Profile, Profile: p})
	}
	if err := validate(fixtures); err != nil {
		return nil, err
	}
	return fixtures, nil
}

// validate checa se os endereços cabem no universo e não se sobrepõem.
func validate(fixtures []*Fixture) error {
	sorted := append([]*Fixture(nil), fixtures...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Address < sorted[j].Address })
	for i, f := range sorted {
		end := f.Address + len(f.Profile.Channels) - 1
		if f.Address < 1 || end > dmx.Universe {
			return fmt.Errorf("fixture %q: canais %d..%d fora do universo", f.Name, f.Address, end)
		}
		if i > 0 {
			prev := sorted[i-1]
			if prevEnd := prev.Address + len(prev.Profile.Channels) - 1; f.Address <= prevEnd {
				return fmt.Errorf("fixtures %q e %q se sobrepõem (canal %d)", prev.Name, f.Name, f.Address)
			}
		}
	}
	return nil
}

func readYAML(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal(data, v); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}
