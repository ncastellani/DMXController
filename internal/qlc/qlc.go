// Package qlc lê Scenes e Sequences de um workspace do QLC+ (.qxw) e as
// reproduz como quadros DMX.
//
// O que importa do formato (conferido no código-fonte do QLC+):
//   - A pasta de uma Function é o atributo Path ("SEQUÊNCIAS/BASES TÉCNICAS").
//   - Uma Sequence não guarda DMX sozinha: aponta para uma Scene base
//     (BoundScene), e cada <Step> traz só os valores que a sobrescrevem. No
//     workspace do Amós cada Sequence tem a sua Scene base zerada
//     ("SEQ | ... [base]"), e os passos gravam os 21 canais explícitos.
//   - Texto do <Step>: "fixtureID:canal,valor,canal,valor:fixtureID:...", em
//     ordem crescente de fixture e canal.
//   - SpeedModes Duration="Common": todos os passos usam o Duration de
//     <Speed> (o Hold do passo é ignorado); "PerStep" usa o de cada passo.
//     FadeIn="PerStep" dá fade por passo (CHILL e CINEMA).
//   - Cores de widget da Virtual Console são inteiros QRgb, não "#rrggbb".
//
// O projeto do QLC+ é do Amós e é editado no próprio QLC+; o pacote original
// (com o HANDOVER.md e os CSVs exportados) fica fora do repositório.
package qlc

import (
	"encoding/xml"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ncastellani/DMXController/internal/dmx"
	"github.com/ncastellani/DMXController/internal/engine"
)

// Value é o valor de um canal DMX absoluto (1..512).
type Value struct {
	Channel int
	Value   byte
}

// Scene é um conjunto fixo de valores.
type Scene struct {
	ID         int
	Name, Path string
	Values     []Value
}

// Render escreve os valores da Scene no quadro.
func (s *Scene) Render(_ time.Duration, f *dmx.Frame) {
	for _, v := range s.Values {
		f.Set(v.Channel, v.Value)
	}
}

// Step é um passo de uma Sequence. Duration inclui o FadeIn.
type Step struct {
	FadeIn   time.Duration
	Duration time.Duration
	Values   []Value
}

// Sequence é uma lista de passos tocada em loop, para frente.
type Sequence struct {
	ID         int
	Name, Path string
	Steps      []Step
	Cycle      time.Duration
}

// Render escreve no quadro o estado da Sequence após elapsed desde o início.
// O fade de um passo parte dos valores do passo anterior (no primeiro passo,
// do último, como no loop).
func (s *Sequence) Render(elapsed time.Duration, f *dmx.Frame) {
	if s.Cycle <= 0 {
		return
	}
	t := elapsed % s.Cycle
	i := 0
	for t >= s.Steps[i].Duration {
		t -= s.Steps[i].Duration
		i++
	}
	cur := s.Steps[i]
	if cur.FadeIn <= 0 || t >= cur.FadeIn {
		for _, v := range cur.Values {
			f.Set(v.Channel, v.Value)
		}
		return
	}
	from := *f
	for _, v := range s.Steps[(i+len(s.Steps)-1)%len(s.Steps)].Values {
		from.Set(v.Channel, v.Value)
	}
	frac := float64(t) / float64(cur.FadeIn)
	for _, v := range cur.Values {
		a := float64(from[v.Channel-1])
		f.Set(v.Channel, byte(a+(float64(v.Value)-a)*frac+0.5))
	}
}

// Fixture é um fixture do workspace (universo 1).
type Fixture struct {
	ID       int
	Name     string
	Address  int // canal DMX inicial (1..512)
	Channels int
}

// Workspace contém as Functions suportadas de um .qxw.
type Workspace struct {
	Fixtures []Fixture // em ordem de ID

	Scenes    map[int]*Scene
	Sequences map[int]*Sequence
	// Skipped lista Functions ignoradas por usarem recursos não suportados.
	Skipped []string
}

// Function devolve a Scene ou Sequence com o ID dado.
func (w *Workspace) Function(id int) (name string, src engine.Source, ok bool) {
	if s, ok := w.Scenes[id]; ok {
		return s.Name, s, true
	}
	if s, ok := w.Sequences[id]; ok {
		return s.Name, s, true
	}
	return "", nil, false
}

type xmlWorkspace struct {
	Engine struct {
		Fixtures  []xmlFixture  `xml:"Fixture"`
		Functions []xmlFunction `xml:"Function"`
	} `xml:"Engine"`
}

type xmlFixture struct {
	ID       int    `xml:"ID"`
	Name     string `xml:"Name"`
	Universe int    `xml:"Universe"`
	Address  int    `xml:"Address"` // base 0
	Channels int    `xml:"Channels"`
}

type xmlFunction struct {
	ID    int    `xml:"ID,attr"`
	Type  string `xml:"Type,attr"`
	Name  string `xml:"Name,attr"`
	Path  string `xml:"Path,attr"`
	Speed struct {
		FadeIn   uint64 `xml:"FadeIn,attr"`
		Duration uint64 `xml:"Duration,attr"`
	} `xml:"Speed"`
	Direction  string `xml:"Direction"`
	RunOrder   string `xml:"RunOrder"`
	SpeedModes struct {
		FadeIn   string `xml:"FadeIn,attr"`
		Duration string `xml:"Duration,attr"`
	} `xml:"SpeedModes"`
	FixtureVals []struct {
		ID   int    `xml:"ID,attr"`
		Text string `xml:",chardata"`
	} `xml:"FixtureVal"`
	Steps []struct {
		FadeIn uint64 `xml:"FadeIn,attr"`
		Hold   uint64 `xml:"Hold,attr"`
		Text   string `xml:",chardata"`
	} `xml:"Step"`
}

// Load lê um workspace do QLC+.
func Load(path string) (*Workspace, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	w, err := parse(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return w, nil
}

func parse(data []byte) (*Workspace, error) {
	var x xmlWorkspace
	if err := xml.Unmarshal(data, &x); err != nil {
		return nil, err
	}

	fixtures := map[int]xmlFixture{}
	for _, f := range x.Engine.Fixtures {
		if f.Universe != 0 {
			continue // só um universo de saída
		}
		fixtures[f.ID] = f
	}
	// abs converte (fixture, canal base 0) em canal DMX absoluto.
	abs := func(fixtureID, ch int) (int, error) {
		f, ok := fixtures[fixtureID]
		if !ok {
			return 0, fmt.Errorf("fixture %d não existe no universo 1", fixtureID)
		}
		if ch < 0 || ch >= f.Channels {
			return 0, fmt.Errorf("fixture %d não tem canal %d", fixtureID, ch)
		}
		return f.Address + ch + 1, nil
	}

	w := &Workspace{Scenes: map[int]*Scene{}, Sequences: map[int]*Sequence{}}
	for _, f := range fixtures {
		w.Fixtures = append(w.Fixtures, Fixture{ID: f.ID, Name: f.Name, Address: f.Address + 1, Channels: f.Channels})
	}
	sort.Slice(w.Fixtures, func(i, j int) bool { return w.Fixtures[i].ID < w.Fixtures[j].ID })
	for _, fn := range x.Engine.Functions {
		switch fn.Type {
		case "Scene":
			s := &Scene{ID: fn.ID, Name: fn.Name, Path: fn.Path}
			for _, fv := range fn.FixtureVals {
				vals, err := parsePairs(fv.ID, fv.Text, abs)
				if err != nil {
					return nil, fmt.Errorf("scene %q: %w", fn.Name, err)
				}
				s.Values = append(s.Values, vals...)
			}
			w.Scenes[fn.ID] = s
		case "Sequence":
			seq, err := parseSequence(fn, abs)
			if err != nil {
				return nil, fmt.Errorf("sequence %q: %w", fn.Name, err)
			}
			if seq == nil {
				w.Skipped = append(w.Skipped, fmt.Sprintf("%s (direção %s, ordem %s)", fn.Name, fn.Direction, fn.RunOrder))
				continue
			}
			w.Sequences[fn.ID] = seq
		default:
			w.Skipped = append(w.Skipped, fmt.Sprintf("%s (tipo %s)", fn.Name, fn.Type))
		}
	}
	return w, nil
}

// parseSequence devolve nil se a Sequence usa recursos não suportados.
func parseSequence(fn xmlFunction, abs func(int, int) (int, error)) (*Sequence, error) {
	if fn.Direction != "Forward" || fn.RunOrder != "Loop" {
		return nil, nil
	}
	seq := &Sequence{ID: fn.ID, Name: fn.Name, Path: fn.Path}
	for i, st := range fn.Steps {
		var step Step
		switch fn.SpeedModes.FadeIn {
		case "PerStep":
			step.FadeIn = ms(st.FadeIn)
		case "Common":
			step.FadeIn = ms(fn.Speed.FadeIn)
		}
		// "Default" usa o fade da Scene vinculada, que é sempre 0 aqui.
		if fn.SpeedModes.Duration == "PerStep" {
			step.Duration = ms(st.FadeIn + st.Hold)
		} else {
			step.Duration = ms(fn.Speed.Duration)
		}
		if step.FadeIn > step.Duration {
			step.FadeIn = step.Duration
		}

		// Texto: "fixture:canal,valor,...:fixture:canal,valor,..."
		parts := strings.Split(strings.TrimSpace(st.Text), ":")
		if len(parts)%2 != 0 {
			return nil, fmt.Errorf("passo %d: formato inválido", i)
		}
		for j := 0; j < len(parts); j += 2 {
			fid, err := strconv.Atoi(parts[j])
			if err != nil {
				return nil, fmt.Errorf("passo %d: fixture %q inválido", i, parts[j])
			}
			vals, err := parsePairs(fid, parts[j+1], abs)
			if err != nil {
				return nil, fmt.Errorf("passo %d: %w", i, err)
			}
			step.Values = append(step.Values, vals...)
		}
		seq.Steps = append(seq.Steps, step)
		seq.Cycle += step.Duration
	}
	return seq, nil
}

// parsePairs lê "canal,valor,canal,valor" de um fixture.
func parsePairs(fixtureID int, text string, abs func(int, int) (int, error)) ([]Value, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, nil
	}
	f := strings.Split(text, ",")
	if len(f)%2 != 0 {
		return nil, fmt.Errorf("fixture %d: número ímpar de valores", fixtureID)
	}
	var vals []Value
	for i := 0; i < len(f); i += 2 {
		ch, err1 := strconv.Atoi(f[i])
		v, err2 := strconv.Atoi(f[i+1])
		if err1 != nil || err2 != nil || v < 0 || v > 255 {
			return nil, fmt.Errorf("fixture %d: par inválido %q,%q", fixtureID, f[i], f[i+1])
		}
		c, err := abs(fixtureID, ch)
		if err != nil {
			return nil, err
		}
		vals = append(vals, Value{Channel: c, Value: byte(v)})
	}
	return vals, nil
}

func ms(v uint64) time.Duration { return time.Duration(v) * time.Millisecond }
