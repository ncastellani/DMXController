// Package show toca a setlist: carrega as timelines (TIMELINE.md), toca
// o áudio e faz a luz seguir a posição da música.
package show

import (
	"cmp"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ncastellani/DMXController/internal/dmx"
	"github.com/ncastellani/DMXController/internal/engine"
	"github.com/ncastellani/DMXController/internal/fixture"
	"github.com/ncastellani/DMXController/internal/qlc"
)

// Timeline é o conteúdo de musicas/<faixa>.json.
type Timeline struct {
	Version int `json:"version"`
	Song    struct {
		Title    string  `json:"title"`
		File     string  `json:"file"`
		Duration float64 `json:"duration"`
	} `json:"song"`
	Generated   json.RawMessage `json:"generated,omitempty"`
	Personality json.RawMessage `json:"personality,omitempty"`
	AI          *struct {
		Genero    string  `json:"genero"`
		Clima     string  `json:"clima"`
		Descricao string  `json:"descricao"`
		BPM       float64 `json:"bpm_percebido"`
	} `json:"ai,omitempty"`
	Analysis Analysis `json:"analysis"`
	Lyrics   []Lyric  `json:"lyrics,omitempty"` // letra transcrita da versão cantada
	Cues     []Cue    `json:"cues"`
}

// Lyric é uma frase cantada, de T a End (segundos da música).
type Lyric struct {
	T    float64 `json:"t"`
	End  float64 `json:"end"`
	Text string  `json:"text"`
}

// Analysis é a grade da música (librosa).
type Analysis struct {
	Duration  float64   `json:"duration"`
	BPM       float64   `json:"bpm"`
	Beats     []float64 `json:"beats"`
	Downbeats []float64 `json:"downbeats"`
	Energy    struct {
		Hop    float64   `json:"hop"`
		Values []float64 `json:"values"`
	} `json:"energy"`
	Sections []Section `json:"sections"`
}

// Section é uma parte da música.
type Section struct {
	Start  float64 `json:"start"`
	End    float64 `json:"end"`
	Bars   int     `json:"bars"`
	Energy float64 `json:"energy"`
	Group  string  `json:"group"`
	Name   string  `json:"name"`
}

// Cue é uma ação da luz num instante da música.
type Cue struct {
	T        float64 `json:"t"`
	Layer    string  `json:"layer"` // base | flash
	Kind     string  `json:"kind"`  // preset | blackout
	Preset   string  `json:"preset,omitempty"`
	Name     string  `json:"name,omitempty"`
	Rate     float64 `json:"rate,omitempty"`
	Duration float64 `json:"duration,omitempty"`
	Note     string  `json:"note,omitempty"`
}

// LoadTimeline lê uma timeline.
func LoadTimeline(path string) (*Timeline, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var t Timeline
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if t.Version != 1 {
		return nil, fmt.Errorf("%s: versão %d não suportada", path, t.Version)
	}
	SortCues(t.Cues) // arquivos gravados com a ordem antiga
	return &t, nil
}

// SortCues ordena as trocas pelo tempo. No mesmo instante, a fritação vem
// antes da luz de base: ela acontece primeiro (dura poucas batidas) e a base
// fica valendo depois. Não muda a luz (o Program separa as camadas), só a
// ordem em que as trocas aparecem nas listas.
func SortCues(cues []Cue) {
	order := func(c Cue) int {
		if c.Layer == "flash" {
			return 0
		}
		return 1
	}
	slices.SortStableFunc(cues, func(a, b Cue) int {
		if c := cmp.Compare(a.T, b.T); c != 0 {
			return c
		}
		return cmp.Compare(order(a), order(b))
	})
}

// ColorPrefix marca um preset de cor criada para a música: "cor:#c8a2c8"
// acende todos os canhões nessa cor.
const ColorPrefix = "cor:"

// Presets resolve ids de preset ("qlc:47", "cor:#c8a2c8") em Sources do engine.
type Presets struct {
	ws        *qlc.Workspace
	positions []*fixture.Fixture // fixtures no quadro lógico, para as cores criadas
}

// NewPresets cria o resolvedor a partir do workspace do QLC+. positions são
// os canhões com endereço lógico (o quadro dos presets).
func NewPresets(ws *qlc.Workspace, positions []*fixture.Fixture) *Presets {
	return &Presets{ws: ws, positions: positions}
}

// ParseColor lê "#rrggbb" (com ou sem #).
func ParseColor(s string) (fixture.Color, bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) != 6 {
		return fixture.Color{}, false
	}
	n, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return fixture.Color{}, false
	}
	return fixture.Color{R: uint8(n >> 16), G: uint8(n >> 8), B: uint8(n)}, true
}

// colorSource acende os canhões numa cor fixa.
type colorSource struct {
	c         fixture.Color
	positions []*fixture.Fixture
}

func (s colorSource) Render(_ time.Duration, f *dmx.Frame) {
	for _, p := range s.positions {
		p.Render(f, fixture.State{Color: s.c, Intensity: 1})
	}
}

// Resolve devolve o nome e o Source do preset.
func (p *Presets) Resolve(id string) (string, engine.Source, error) {
	if hex, ok := strings.CutPrefix(id, ColorPrefix); ok {
		c, ok := ParseColor(hex)
		if !ok {
			return "", nil, fmt.Errorf("cor %q inválida (use #rrggbb)", hex)
		}
		if len(p.positions) == 0 {
			return "", nil, fmt.Errorf("cor %q: nenhum canhão configurado", hex)
		}
		return fmt.Sprintf("COR | #%02x%02x%02x", c.R, c.G, c.B), colorSource{c, p.positions}, nil
	}
	num, ok := strings.CutPrefix(id, "qlc:")
	if !ok {
		return "", nil, fmt.Errorf("preset %q: origem desconhecida", id)
	}
	n, err := strconv.Atoi(num)
	if err != nil {
		return "", nil, fmt.Errorf("preset %q inválido", id)
	}
	name, src, ok := p.ws.Function(n)
	if !ok {
		return "", nil, fmt.Errorf("preset %q não existe no workspace", id)
	}
	return name, src, nil
}

// step é um cue resolvido.
type step struct {
	at   time.Duration
	end  time.Duration // só flash
	rate float64
	src  engine.Source // nil = apagado
	cue  *Cue
}

// Program é a timeline compilada: dado o instante da música, gera o quadro.
type Program struct {
	base  []step
	flash []step
}

// Compile resolve os presets da timeline. Flashes são limitados a maxFlash.
func Compile(t *Timeline, presets *Presets, maxFlash time.Duration) (*Program, error) {
	p := &Program{}
	for i := range t.Cues {
		c := &t.Cues[i]
		s := step{at: secs(c.T), rate: c.Rate, cue: c}
		if s.rate <= 0 {
			s.rate = 1
		}
		switch c.Kind {
		case "preset":
			name, src, err := presets.Resolve(c.Preset)
			if err != nil {
				return nil, fmt.Errorf("cue %d (%.2fs): %w", i, c.T, err)
			}
			s.src = src
			if c.Name == "" {
				c.Name = name
			}
		case "blackout":
		default:
			return nil, fmt.Errorf("cue %d (%.2fs): tipo %q desconhecido", i, c.T, c.Kind)
		}
		switch c.Layer {
		case "base":
			p.base = append(p.base, s)
		case "flash":
			if s.src == nil {
				return nil, fmt.Errorf("cue %d (%.2fs): flash precisa de preset", i, c.T)
			}
			s.end = s.at + min(secs(c.Duration), maxFlash)
			p.flash = append(p.flash, s)
		default:
			return nil, fmt.Errorf("cue %d (%.2fs): camada %q inválida", i, c.T, c.Layer)
		}
	}
	sort.SliceStable(p.base, func(i, j int) bool { return p.base[i].at < p.base[j].at })
	sort.SliceStable(p.flash, func(i, j int) bool { return p.flash[i].at < p.flash[j].at })
	return p, nil
}

// Render escreve no quadro a luz do instante pos da música.
func (p *Program) Render(pos time.Duration, f *dmx.Frame) {
	if b := p.baseAt(pos); b != nil && b.src != nil {
		b.src.Render(time.Duration(float64(pos-b.at)*b.rate), f)
	}
	for i := range p.flash {
		if s := &p.flash[i]; pos >= s.at && pos < s.end {
			s.src.Render(pos-s.at, f)
		}
	}
}

// Current devolve o cue de base ativo em pos e o próximo cue (qualquer camada).
func (p *Program) Current(pos time.Duration) (cur, next *Cue) {
	if b := p.baseAt(pos); b != nil {
		cur = b.cue
	}
	var n *step
	for _, list := range [][]step{p.base, p.flash} {
		for i := range list {
			if list[i].at > pos && (n == nil || list[i].at < n.at) {
				n = &list[i]
				break
			}
		}
	}
	if n != nil {
		next = n.cue
	}
	return cur, next
}

func (p *Program) baseAt(pos time.Duration) *step {
	i := sort.Search(len(p.base), func(i int) bool { return p.base[i].at > pos })
	if i == 0 {
		return nil
	}
	return &p.base[i-1]
}

func secs(s float64) time.Duration { return time.Duration(s * float64(time.Second)) }
