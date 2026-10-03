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
	"github.com/ncastellani/DMXController/internal/library"
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
	Cues     []Cue    `json:"cues"`
}

// Analysis é a grade da música (librosa).
type Analysis struct {
	Duration  float64   `json:"duration"`
	BPM       float64   `json:"bpm"`
	Beats     []float64 `json:"beats"`
	Downbeats []float64 `json:"downbeats"`
	// Hits são os toques fortes da música (ataques de bateria, guitarra...),
	// com pelo menos MinEffectStep entre um e outro; os efeitos do modo livre
	// com sync "hits" trocam de passo neles.
	Hits []float64 `json:"hits,omitempty"`
	// Drums diz que a análise ouviu a bateria separada da música (o "ouvido
	// de bateria"); HitKind é então a peça de cada toque de Hits, um caractere
	// por toque: k = bumbo, s = caixa, h = chimbal, - = não é bateria.
	Drums   bool   `json:"drums,omitempty"`
	HitKind string `json:"hit_kind,omitempty"`
	// HitStrength é a força percussiva de cada toque de Hits (0 a 1).
	HitStrength []float64 `json:"hit_strength,omitempty"`
	// Accents são os toques que se destacam dos vizinhos (uma virada, uma
	// batida solta no meio do verso): a luz dá um realce neles.
	Accents []Accent `json:"accents,omitempty"`
	// SoundEnd é onde o som acaba (s) e FadeOut, a duração da cauda final:
	// a luz vai até SoundEnd, apagando aos poucos durante a cauda.
	SoundEnd float64 `json:"sound_end,omitempty"`
	FadeOut  float64 `json:"fade_out,omitempty"`
	Energy   struct {
		Hop    float64   `json:"hop"`
		Values []float64 `json:"values"`
	} `json:"energy"`
	Sections []Section `json:"sections"`
}

// Accent é um toque em destaque: T em segundos, V a força do realce (0 a 1).
type Accent struct {
	T float64 `json:"t"`
	V float64 `json:"v"`
}

// Section é uma parte da música.
type Section struct {
	Start  float64 `json:"start"`
	End    float64 `json:"end"`
	Bars   int     `json:"bars"`
	Energy float64 `json:"energy"`
	Pulse  float64 `json:"pulse,omitempty"` // batida: força da percussão na parte, 0 a 1
	Group  string  `json:"group"`
	Name   string  `json:"name"`
}

// Cue é uma ação da luz num instante da música.
type Cue struct {
	T        float64 `json:"t"`
	Layer    string  `json:"layer"` // base | flash
	Kind     string  `json:"kind"`  // preset | effect | blackout
	Preset   string  `json:"preset,omitempty"`
	Effect   *Effect `json:"effect,omitempty"` // kind effect (modo livre)
	Name     string  `json:"name,omitempty"`
	Rate     float64 `json:"rate,omitempty"`
	Duration float64 `json:"duration,omitempty"`
	Fade     float64 `json:"fade,omitempty"` // blackout: a luz apaga aos poucos nos Fade segundos antes dele
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

// Presets resolve ids de preset ("pista:chill", "cor:#c8a2c8") em Sources do engine.
type Presets struct {
	lib       *library.Library
	positions []*fixture.Fixture // fixtures no quadro lógico, onde os presets são desenhados
}

// NewPresets cria o resolvedor a partir da biblioteca. positions são os
// canhões com endereço lógico (o quadro dos presets).
func NewPresets(lib *library.Library, positions []*fixture.Fixture) *Presets {
	return &Presets{lib: lib, positions: positions}
}

// Canonical troca um id antigo de preset ("qlc:47", do tempo do QLC+) pelo
// da biblioteca.
func (p *Presets) Canonical(id string) string {
	if strings.HasPrefix(id, ColorPrefix) {
		return id
	}
	return p.lib.Canonical(id)
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
	name, src, ok := p.lib.Preset(id, p.positions)
	if !ok {
		return "", nil, fmt.Errorf("preset %q não existe na biblioteca", id)
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
	// Realce dos toques em destaque (Analysis.Accents) sobre a luz de base.
	accents   []Accent
	positions []*fixture.Fixture
	fades     []fadeOut
}

// fadeOut é o trecho antes de um blackout em que a luz apaga aos poucos.
type fadeOut struct{ from, to time.Duration }

// O realce de um pico da música: o canhão do centro dá um flash branco (até
// branco total num pico cheio) e os dos lados clareiam de leve (accentSide),
// voltando à luz de base em accentDecay. Segurança: nunca apaga nada, não
// liga strobo e dois realces ficam a pelo menos accentMinGap um do outro
// (menos de 1 por segundo), mesmo num arquivo editado à mão.
const (
	accentSide   = 0.3
	accentDecay  = 280 * time.Millisecond
	accentMinGap = time.Second
)

// maxCueFade é o fade mais longo de um blackout (s).
const maxCueFade = 30.0

// Compile resolve os presets da timeline. Flashes são limitados a maxFlash.
func Compile(t *Timeline, presets *Presets, maxFlash time.Duration) (*Program, error) {
	p := &Program{positions: presets.positions}
	for _, a := range t.Analysis.Accents {
		if n := len(p.accents); a.V > 0 && (n == 0 || secs(a.T-p.accents[n-1].T) >= accentMinGap) {
			a.V = min(1, a.V)
			p.accents = append(p.accents, a)
		}
	}
	for i := range t.Cues {
		c := &t.Cues[i]
		s := step{at: secs(c.T), rate: c.Rate, cue: c}
		if s.rate <= 0 {
			s.rate = 1
		}
		switch c.Kind {
		case "preset":
			c.Preset = presets.Canonical(c.Preset)
			name, src, err := presets.Resolve(c.Preset)
			if err != nil {
				return nil, fmt.Errorf("cue %d (%.2fs): %w", i, c.T, err)
			}
			s.src = src
			if c.Name == "" {
				c.Name = name
			}
		case "effect": // modo livre: só na base, sempre no andamento da música
			if c.Layer != "base" {
				return nil, fmt.Errorf("cue %d (%.2fs): efeito só vale na camada base", i, c.T)
			}
			name, src, err := presets.Effect(c.Effect, &t.Analysis, c.T)
			if err != nil {
				return nil, fmt.Errorf("cue %d (%.2fs): %w", i, c.T, err)
			}
			s.src, s.rate = src, 1
			c.Preset, c.Rate = "", 0
			if c.Name == "" {
				c.Name = name
			}
		case "blackout":
			if c.Fade = max(0, min(maxCueFade, c.Fade)); c.Fade > 0 && c.Layer == "base" {
				p.fades = append(p.fades, fadeOut{s.at - secs(c.Fade), s.at})
			}
		default:
			return nil, fmt.Errorf("cue %d (%.2fs): tipo %q desconhecido", i, c.T, c.Kind)
		}
		if c.Kind != "effect" {
			c.Effect = nil
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
		p.bump(pos, f)
		for _, fo := range p.fades {
			if pos >= fo.from && pos < fo.to {
				for _, fx := range p.positions {
					fx.Dim(f, float64(fo.to-pos)/float64(fo.to-fo.from))
				}
			}
		}
	}
	for i := range p.flash {
		if s := &p.flash[i]; pos >= s.at && pos < s.end {
			s.src.Render(pos-s.at, f)
		}
	}
}

// bump aplica o realce do toque em destaque mais recente, se ainda dura.
func (p *Program) bump(pos time.Duration, f *dmx.Frame) {
	i := sort.Search(len(p.accents), func(i int) bool { return secs(p.accents[i].T) > pos })
	if i == 0 {
		return
	}
	a := p.accents[i-1]
	since := pos - secs(a.T)
	if since >= accentDecay {
		return
	}
	left := 1 - float64(since)/float64(accentDecay)
	for i, fx := range p.positions {
		amount := a.V * left * left
		if i != len(p.positions)/2 { // só o do centro vai ao branco
			amount *= accentSide
		}
		fx.Bump(f, amount)
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
