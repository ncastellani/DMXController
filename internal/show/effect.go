package show

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/ncastellani/DMXController/internal/dmx"
	"github.com/ncastellani/DMXController/internal/engine"
	"github.com/ncastellani/DMXController/internal/fixture"
)

// Modo livre: em vez de um preset da biblioteca, a troca de base descreve um
// efeito (tipo + cores + passo em batidas) e o programa gera os quadros.

// Tipos de efeito.
const (
	FXFixed     = "fixed"     // cores paradas, uma por canhão (1 cor = todos iguais)
	FXAlternate = "alternate" // os canhões trocam de cor entre si a cada passo
	FXChase     = "chase"     // a 1ª cor corre de um canhão ao outro (vai e volta) sobre a 2ª (ou apagado)
	FXPulse     = "pulse"     // as cores acendem forte no passo e caem suave
	FXFade      = "fade"      // as cores se transformam uma na outra
	FXSequence  = "sequence"  // passos escritos um a um (uma cor por canhão em cada passo), como os modos de pista da biblioteca
)

// SyncHits faz o efeito trocar de passo nos toques da música.
const SyncHits = "hits"

// MaxEffectSteps é o máximo de passos de uma sequência.
const MaxEffectSteps = 32

// MaxEffectColors é o máximo de cores de um efeito.
const MaxEffectColors = 4

// MinEffectStep é o passo mais curto de um efeito (segurança: menos de 3
// trocas por segundo). Um passo menor que isso dobra até caber, para
// continuar na batida.
const MinEffectStep = 340 * time.Millisecond

// seqFade é a fração do passo que o fade de uma sequência ocupa.
const seqFade = 0.5

// pulseFloor é até onde a luz cai no efeito pulse (nunca apaga).
const pulseFloor = 0.3

// Effect é um efeito do modo livre.
type Effect struct {
	Type   string   `json:"type"`
	Name   string   `json:"name,omitempty"`   // efeito da biblioteca de onde veio ("CLUB"), para o nome de exibição
	Colors []string `json:"colors,omitempty"` // "#rrggbb", na ordem dos canhões (esquerda, centro, direita)
	// Sequence: as cores de cada passo, uma por canhão ("#000000" = apagado),
	// tocadas em loop; Fade = cada passo entra em fade a partir do anterior.
	Steps [][]string `json:"steps,omitempty"`
	Fade  bool       `json:"fade,omitempty"`
	Beats float64    `json:"beats,omitempty"` // duração de cada passo em batidas: 0.5, 1, 2, 4 ou 8 (padrão 1)
	Sync  string     `json:"sync,omitempty"`  // "hits": o passo troca a cada toque forte da música (analysis.hits), não a cada Beats
	Level float64    `json:"level,omitempty"` // intensidade, 0.2 a 1 (padrão 1)
}

var effectNames = map[string]string{
	FXFixed: "CORES FIXAS", FXAlternate: "ALTERNADO", FXChase: "CORRIDA", FXPulse: "PULSO", FXFade: "TRANSIÇÃO", FXSequence: "SEQUÊNCIA",
}

// EffectName é o nome de exibição do efeito ("LIVRE | CORRIDA", ou
// "EFEITO | CLUB" para um que veio da biblioteca).
func EffectName(e *Effect) string {
	if e.Name != "" {
		return "EFEITO | " + e.Name
	}
	return "LIVRE | " + effectNames[e.Type]
}

// Clean confere o efeito e acerta os valores: cores em "#rrggbb" minúsculo,
// passo numa das durações aceitas, intensidade dentro da faixa.
func (e *Effect) Clean() error {
	if effectNames[e.Type] == "" {
		return fmt.Errorf("efeito %q desconhecido", e.Type)
	}
	if r := []rune(strings.TrimSpace(e.Name)); len(r) > 40 {
		e.Name = string(r[:40])
	} else {
		e.Name = string(r)
	}
	if e.Type == FXSequence {
		e.Colors = nil
		if len(e.Steps) > MaxEffectSteps {
			e.Steps = e.Steps[:MaxEffectSteps]
		}
		if len(e.Steps) < 2 {
			return fmt.Errorf("uma sequência precisa de pelo menos 2 passos")
		}
		for i, st := range e.Steps {
			switch len(st) { // sempre 3 cores: esquerda, centro, direita
			case 0:
				return fmt.Errorf("passo %d sem cores", i+1)
			case 1:
				st = []string{st[0], st[0], st[0]}
			case 2:
				st = []string{st[0], st[1], st[0]}
			default:
				st = st[:3:3]
			}
			if err := cleanColors(st); err != nil {
				return fmt.Errorf("passo %d: %w", i+1, err)
			}
			e.Steps[i] = st
		}
	} else {
		e.Steps, e.Fade = nil, false
	}
	if e.Type == FXChase && len(e.Colors) > 2 {
		e.Colors = e.Colors[:2]
	}
	if len(e.Colors) > MaxEffectColors {
		e.Colors = e.Colors[:MaxEffectColors]
	}
	need := 1
	if e.Type == FXAlternate || e.Type == FXFade {
		need = 2
	}
	if e.Type != FXSequence && len(e.Colors) < need {
		return fmt.Errorf("efeito %s precisa de pelo menos %d cor(es)", e.Type, need)
	}
	if err := cleanColors(e.Colors); err != nil {
		return err
	}
	best := 1.0
	if e.Beats > 0 {
		for _, b := range []float64{0.5, 2, 4, 8} {
			if math.Abs(math.Log(e.Beats/b)) < math.Abs(math.Log(e.Beats/best)) {
				best = b
			}
		}
	}
	e.Beats = best
	if e.Type == FXFixed {
		e.Beats = 0
	}
	if e.Sync != SyncHits || e.Type == FXFixed {
		e.Sync = ""
	}
	switch {
	case e.Level <= 0 || e.Level >= 1:
		e.Level = 0 // omitido = 1
	case e.Level < 0.2:
		e.Level = 0.2
	}
	return nil
}

// cleanColors confere as cores e as deixa em "#rrggbb" minúsculo.
func cleanColors(colors []string) error {
	for i, s := range colors {
		c, ok := ParseColor(s)
		if !ok {
			return fmt.Errorf("cor %q inválida (use #rrggbb)", s)
		}
		colors[i] = fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
	}
	return nil
}

// Step é a duração de um passo do efeito no andamento bpm, nunca menor que
// MinEffectStep.
func (e *Effect) Step(bpm float64) time.Duration {
	if bpm <= 0 {
		bpm = 120
	}
	beats := e.Beats
	if beats <= 0 {
		beats = 1
	}
	d := secs(beats * 60 / bpm)
	for d < MinEffectStep {
		d *= 2
	}
	return d
}

// effectSource desenha um efeito nos canhões (quadro lógico).
type effectSource struct {
	kind      string
	colors    []fixture.Color
	steps     [][]fixture.Color // sequence
	fade      bool
	step      time.Duration
	hits      []time.Duration // sync "hits": instantes das trocas, a partir do início da troca de luz
	level     float64
	positions []*fixture.Fixture
}

func (s effectSource) Render(t time.Duration, f *dmx.Frame) {
	n := len(s.colors)
	k := int(t / s.step)
	phase := float64(t%s.step) / float64(s.step)
	if s.hits != nil { // um passo por toque; a fase vai de um toque ao seguinte
		k = sort.Search(len(s.hits), func(i int) bool { return s.hits[i] > t })
		var start time.Duration
		if k > 0 {
			start = s.hits[k-1]
		}
		end := start + s.step
		if k < len(s.hits) {
			end = s.hits[k]
		}
		phase = min(1, float64(t-start)/float64(end-start))
	}
	for i, p := range s.positions {
		var c fixture.Color
		level := s.level
		if n > 0 {
			c = s.colors[i%n]
		}
		switch s.kind {
		case FXSequence:
			m := len(s.steps)
			cur := s.steps[k%m]
			c = cur[i%len(cur)]
			if s.fade && phase < seqFade { // entra em fade a partir do passo anterior
				prev := s.steps[(k+m-1)%m]
				c = mix(prev[i%len(prev)], c, phase/seqFade)
			}
		case FXAlternate:
			c = s.colors[(i+k)%n]
		case FXChase:
			c = fixture.Color{}
			if n > 1 {
				c = s.colors[1]
			}
			if i == bounce(k, len(s.positions)) {
				c = s.colors[0]
			}
		case FXPulse:
			level *= pulseFloor + (1-pulseFloor)*(1-phase)*(1-phase)
		case FXFade:
			c = mix(s.colors[(i+k)%n], s.colors[(i+k+1)%n], phase)
		}
		p.Render(f, fixture.State{Color: c, Intensity: level})
	}
}

// bounce é a posição de quem vai e volta entre 0 e n-1 no passo k.
func bounce(k, n int) int {
	if n < 2 {
		return 0
	}
	x := k % (2 * (n - 1))
	if x >= n {
		x = 2*(n-1) - x
	}
	return x
}

func mix(a, b fixture.Color, p float64) fixture.Color {
	m := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*p + 0.5) }
	return fixture.Color{R: m(a.R, b.R), G: m(a.G, b.G), B: m(a.B, b.B)}
}

// effectHits devolve os instantes em que um efeito com sync "hits" começando
// em at (s) troca de passo, contados a partir de at: os toques da música (ou
// as batidas, se a análise não tem toques), sempre com pelo menos
// MinEffectStep entre as trocas (segurança, mesmo com o arquivo editado à mão).
func effectHits(a *Analysis, at float64) []time.Duration {
	src := a.Hits
	if len(src) == 0 {
		src = a.Beats
	}
	var out []time.Duration
	start := secs(at)
	last := start
	for _, h := range src {
		if d := secs(h); d-last >= MinEffectStep {
			out = append(out, d-start)
			last = d
		}
	}
	return out
}

// Effect confere o efeito (acertando os valores) e devolve o nome e o Source
// dele para uma troca em at (s) da música analisada em a.
func (p *Presets) Effect(e *Effect, a *Analysis, at float64) (string, engine.Source, error) {
	if e == nil {
		return "", nil, fmt.Errorf("efeito sem descrição")
	}
	if err := e.Clean(); err != nil {
		return "", nil, err
	}
	if len(p.positions) == 0 {
		return "", nil, fmt.Errorf("efeito %s: nenhum canhão configurado", e.Type)
	}
	src := effectSource{kind: e.Type, step: e.Step(a.BPM), level: 1, positions: p.positions}
	if e.Sync == SyncHits {
		src.hits = effectHits(a, at)
	}
	if e.Level > 0 {
		src.level = e.Level
	}
	parse := func(colors []string) (out []fixture.Color) {
		for _, s := range colors {
			c, _ := ParseColor(s)
			out = append(out, c)
		}
		return out
	}
	src.colors, src.fade = parse(e.Colors), e.Fade
	for _, st := range e.Steps {
		src.steps = append(src.steps, parse(st))
	}
	return EffectName(e), src, nil
}
