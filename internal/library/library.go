// Package library é a biblioteca de luz do programa (config/biblioteca.yaml):
// as cores fixas, os modos de pista (sequências de passos) e os strobos que os
// botões do console, as timelines e a análise usam. Nasceu da conversão do
// workspace do QLC+ do Amós; os ids antigos ("qlc:43") continuam valendo como
// alias, para as timelines já gravadas.
package library

import (
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/ncastellani/DMXController/internal/dmx"
	"github.com/ncastellani/DMXController/internal/fixture"
)

// Prefixos dos ids.
const (
	ColorPrefix  = "todos:" // cor fixa nos 3 canhões ("todos:azul")
	EffectPrefix = "pista:" // modo de pista ("pista:club")
)

// Mood é a pontuação de clima de um efeito (0 a 1 em cada eixo).
type Mood struct {
	Energy  float64 `yaml:"energy" json:"energia"`  // parado → explosivo
	Joy     float64 `yaml:"joy" json:"alegria"`     // sombrio → alegre
	Tension float64 `yaml:"tension" json:"tensao"`  // tranquilo → dramático
	Romance float64 `yaml:"romance" json:"romance"` // frio → íntimo, sensual
	Label   string  `yaml:"label,omitempty" json:"clima"`
}

// Clamp põe as notas entre 0 e 1 e encurta o rótulo.
func (m *Mood) Clamp() {
	fix := func(x *float64) { *x = float64(int(max(0, min(1, *x))*100+0.5)) / 100 }
	fix(&m.Energy)
	fix(&m.Joy)
	fix(&m.Tension)
	fix(&m.Romance)
	if r := []rune(strings.TrimSpace(m.Label)); len(r) > 40 {
		m.Label = string(r[:40])
	} else {
		m.Label = string(r)
	}
}

func (m *Mood) String() string {
	s := fmt.Sprintf("energia %.1f, alegria %.1f, tensão %.1f, romance %.1f", m.Energy, m.Joy, m.Tension, m.Romance)
	if m.Label != "" {
		s += " (" + m.Label + ")"
	}
	return s
}

// Color é uma cor fixa nomeada (acende os 3 canhões).
type Color struct {
	ID    string `yaml:"id"`   // "todos:azul"
	Name  string `yaml:"name"` // "AZUL"
	Hex   string `yaml:"hex"`  // tom: "#rrggbb" ou "#rrggbb@dimmer"
	Temp  string `yaml:"temp,omitempty"`
	Alias string `yaml:"alias,omitempty"` // id antigo ("qlc:1")
}

// Strobe é um strobo nativo do canhão numa cor (fritação ou strobo contínuo).
type Strobe struct {
	ID    string `yaml:"id"`   // "fritacao:branco-maximo"
	Name  string `yaml:"name"` // "FRITAÇÃO | BRANCO MÁXIMO"
	Hex   string `yaml:"hex"`
	Speed int    `yaml:"speed"` // canal de velocidade, 1..255
	Alias string `yaml:"alias,omitempty"`
}

// Effect é um modo de pista: passos tocados em loop.
type Effect struct {
	ID       string   `yaml:"id"`   // "pista:club"
	Name     string   `yaml:"name"` // "CLUB"
	Alias    string   `yaml:"alias,omitempty"`
	StepMs   float64  `yaml:"step_ms,omitempty"` // duração dos passos que não trazem a sua
	Agitacao string   `yaml:"agitation"`
	Temp     string   `yaml:"temp"`
	Colors   []string `yaml:"colors,flow"` // nomes das cores que aparecem
	Mood     *Mood    `yaml:"mood,omitempty,flow"`
	// Steps: um passo por linha, os tons de esquerda, centro e direita e, se o
	// passo tem tempo próprio, "350ms" e "fade900ms" (o fade conta dentro da
	// duração e parte do passo anterior).
	Steps []string `yaml:"steps"`

	steps []Step
}

// Library é o conteúdo de config/biblioteca.yaml.
type Library struct {
	Colors  []Color  `yaml:"colors"`
	Strobes []Strobe `yaml:"strobes"`
	Effects []Effect `yaml:"effects"`
}

// Tone é o estado de um canhão num passo: a cor crua e o dimmer.
type Tone struct {
	Color  fixture.Color
	Dimmer uint8
}

// ParseTone lê "#rrggbb" (dimmer no máximo; preto = apagado) ou
// "#rrggbb@ddd" (dimmer de 0 a 255).
func ParseTone(s string) (Tone, error) {
	hex, dim, has := strings.Cut(strings.TrimSpace(s), "@")
	hex = strings.TrimPrefix(hex, "#")
	n, err := strconv.ParseUint(hex, 16, 32)
	if err != nil || len(hex) != 6 {
		return Tone{}, fmt.Errorf("tom %q inválido (use #rrggbb ou #rrggbb@dimmer)", s)
	}
	t := Tone{Color: fixture.Color{R: uint8(n >> 16), G: uint8(n >> 8), B: uint8(n)}, Dimmer: 255}
	if n == 0 {
		t.Dimmer = 0
	}
	if has {
		d, err := strconv.ParseUint(dim, 10, 8)
		if err != nil {
			return Tone{}, fmt.Errorf("tom %q: dimmer inválido (0 a 255)", s)
		}
		t.Dimmer = uint8(d)
	}
	return t, nil
}

// String escreve o tom como ParseTone lê.
func (t Tone) String() string {
	hex := fmt.Sprintf("#%02x%02x%02x", t.Color.R, t.Color.G, t.Color.B)
	if black := t.Color == (fixture.Color{}); t.Dimmer == 255 && !black || t.Dimmer == 0 && black {
		return hex
	}
	return fmt.Sprintf("%s@%d", hex, t.Dimmer)
}

// Hex é a cor que o canhão emite (já com o dimmer), em "#rrggbb".
func (t Tone) Hex() string {
	m := func(v uint8) uint8 { return uint8(float64(v)*float64(t.Dimmer)/255 + 0.5) }
	return fmt.Sprintf("#%02x%02x%02x", m(t.Color.R), m(t.Color.G), m(t.Color.B))
}

func (t Tone) state(strobe uint8) fixture.State {
	return fixture.State{Color: t.Color, Intensity: float64(t.Dimmer) / 255, Strobe: strobe}
}

// Step é um passo de um modo de pista. Duration inclui o FadeIn.
type Step struct {
	Tones    []Tone // um por canhão (esquerda, centro, direita)
	Duration time.Duration
	FadeIn   time.Duration
}

func (e *Effect) parse() error {
	e.steps = nil
	for i, line := range e.Steps {
		st := Step{Duration: time.Duration(e.StepMs * float64(time.Millisecond))}
		for _, tok := range strings.Fields(line) {
			if strings.HasPrefix(tok, "#") {
				t, err := ParseTone(tok)
				if err != nil {
					return fmt.Errorf("passo %d: %w", i+1, err)
				}
				st.Tones = append(st.Tones, t)
				continue
			}
			num, fade := strings.CutPrefix(strings.TrimSuffix(tok, "ms"), "fade")
			ms, err := strconv.ParseFloat(num, 64)
			if err != nil || !strings.HasSuffix(tok, "ms") || ms < 0 {
				return fmt.Errorf("passo %d: %q não é um tom, uma duração (350ms) nem um fade (fade900ms)", i+1, tok)
			}
			if d := time.Duration(ms * float64(time.Millisecond)); fade {
				st.FadeIn = d
			} else {
				st.Duration = d
			}
		}
		if len(st.Tones) == 0 {
			return fmt.Errorf("passo %d sem cores", i+1)
		}
		if st.Duration <= 0 {
			return fmt.Errorf("passo %d sem duração (ponha 350ms no passo ou step_ms no efeito)", i+1)
		}
		st.FadeIn = min(st.FadeIn, st.Duration)
		e.steps = append(e.steps, st)
	}
	if len(e.steps) == 0 {
		return fmt.Errorf("sem passos")
	}
	return nil
}

// Parsed são os passos do efeito.
func (e *Effect) Parsed() []Step { return e.steps }

// StepColors são as cores emitidas em cada passo, 3 "#rrggbb" (esquerda,
// centro, direita): o desenho do efeito, sem os tempos.
func (e *Effect) StepColors() [][]string {
	out := make([][]string, len(e.steps))
	for i, st := range e.steps {
		for k := 0; k < 3; k++ {
			out[i] = append(out[i], st.Tones[k%len(st.Tones)].Hex())
		}
	}
	return out
}

// TypicalStep é a duração mediana dos passos, em ms.
func (e *Effect) TypicalStep() float64 {
	var d []float64
	for _, st := range e.steps {
		d = append(d, float64(st.Duration.Milliseconds()))
	}
	slices.Sort(d)
	if n := len(d); n%2 == 0 {
		return (d[n/2-1] + d[n/2]) / 2
	}
	return d[len(d)/2]
}

// Fades diz se algum passo entra em fade.
func (e *Effect) Fades() bool {
	return slices.ContainsFunc(e.steps, func(s Step) bool { return s.FadeIn > 0 })
}

// FullName é o nome de exibição ("PISTA | CLUB").
func (e *Effect) FullName() string { return "PISTA | " + e.Name }

// FullName é o nome de exibição ("TODOS | AZUL").
func (c *Color) FullName() string { return "TODOS | " + c.Name }

// Source desenha um preset nos canhões.
type Source interface {
	Render(elapsed time.Duration, f *dmx.Frame)
}

// scene acende todos os canhões no mesmo tom (com ou sem strobo).
type scene struct {
	state     fixture.State
	positions []*fixture.Fixture
}

func (s scene) Render(_ time.Duration, f *dmx.Frame) {
	for _, p := range s.positions {
		p.Render(f, s.state)
	}
}

// sequence toca os passos em loop. O fade de um passo parte dos valores do
// passo anterior (no primeiro, do último, como no loop).
type sequence struct {
	steps     []Step
	cycle     time.Duration
	positions []*fixture.Fixture
}

func (s sequence) draw(st Step, f *dmx.Frame) {
	for i, p := range s.positions {
		p.Render(f, st.Tones[i%len(st.Tones)].state(0))
	}
}

func (s sequence) Render(elapsed time.Duration, f *dmx.Frame) {
	t := elapsed % s.cycle
	i := 0
	for t >= s.steps[i].Duration {
		t -= s.steps[i].Duration
		i++
	}
	cur := s.steps[i]
	if cur.FadeIn <= 0 || t >= cur.FadeIn {
		s.draw(cur, f)
		return
	}
	var from, to dmx.Frame
	s.draw(s.steps[(i+len(s.steps)-1)%len(s.steps)], &from)
	s.draw(cur, &to)
	frac := float64(t) / float64(cur.FadeIn)
	for _, p := range s.positions {
		for ch := p.Address; ch < p.Address+len(p.Profile.Channels); ch++ {
			a := float64(from[ch-1])
			f.Set(ch, byte(a+(float64(to[ch-1])-a)*frac+0.5))
		}
	}
}

// Canonical devolve o id atual de um preset (troca um alias antigo, como
// "qlc:43", pelo id da biblioteca); um id desconhecido volta como veio.
func (l *Library) Canonical(id string) string {
	id = strings.TrimSpace(id)
	for i := range l.Colors {
		if c := &l.Colors[i]; c.ID == id || c.Alias == id {
			return c.ID
		}
	}
	for i := range l.Strobes {
		if s := &l.Strobes[i]; s.ID == id || s.Alias == id {
			return s.ID
		}
	}
	for i := range l.Effects {
		if e := &l.Effects[i]; e.ID == id || e.Alias == id {
			return e.ID
		}
	}
	return id
}

// Color devolve a cor fixa de id dado.
func (l *Library) Color(id string) *Color {
	id = l.Canonical(id)
	for i := range l.Colors {
		if l.Colors[i].ID == id {
			return &l.Colors[i]
		}
	}
	return nil
}

// Strobe devolve o strobo de id dado.
func (l *Library) Strobe(id string) *Strobe {
	id = l.Canonical(id)
	for i := range l.Strobes {
		if l.Strobes[i].ID == id {
			return &l.Strobes[i]
		}
	}
	return nil
}

// Effect devolve o modo de pista de id dado.
func (l *Library) Effect(id string) *Effect {
	if l == nil {
		return nil
	}
	id = l.Canonical(strings.ToLower(id))
	for i := range l.Effects {
		if l.Effects[i].ID == id {
			return &l.Effects[i]
		}
	}
	return nil
}

// Preset devolve o nome de exibição e o Source do preset id (ou de um alias
// dele) desenhado nos canhões positions (endereços do quadro lógico).
func (l *Library) Preset(id string, positions []*fixture.Fixture) (name string, src Source, ok bool) {
	if c := l.Color(id); c != nil {
		t, _ := ParseTone(c.Hex)
		return c.FullName(), scene{t.state(0), positions}, true
	}
	if s := l.Strobe(id); s != nil {
		t, _ := ParseTone(s.Hex)
		return s.Name, scene{t.state(uint8(s.Speed)), positions}, true
	}
	if e := l.Effect(id); e != nil {
		seq := sequence{steps: e.steps, positions: positions}
		for _, st := range e.steps {
			seq.cycle += st.Duration
		}
		return e.FullName(), seq, true
	}
	return "", nil, false
}

var slugger = strings.NewReplacer("á", "a", "à", "a", "â", "a", "ã", "a", "é", "e", "ê", "e", "í", "i", "ó", "o", "ô", "o", "õ", "o", "ú", "u", "ç", "c")

// Slug transforma um nome num pedaço de id ("AZUL PROFUNDO" → "azul-profundo").
func Slug(s string) string {
	s = slugger.Replace(strings.ToLower(s))
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			b.WriteRune(r)
		case b.Len() > 0 && !strings.HasSuffix(b.String(), "-"):
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

// Check confere a biblioteca e prepara os passos.
func (l *Library) Check() error {
	seen := map[string]bool{}
	id := func(id, alias string) error {
		if !strings.Contains(id, ":") || strings.ContainsAny(id, " #") || seen[id] {
			return fmt.Errorf("%q: id inválido ou repetido (use grupo:nome, ex.: pista:club)", id)
		}
		seen[id] = true
		if alias != "" {
			if seen[alias] {
				return fmt.Errorf("%q: alias %q repetido", id, alias)
			}
			seen[alias] = true
		}
		return nil
	}
	for i := range l.Colors {
		c := &l.Colors[i]
		if err := id(c.ID, c.Alias); err != nil {
			return err
		}
		t, err := ParseTone(c.Hex)
		if err != nil {
			return fmt.Errorf("%s: %w", c.ID, err)
		}
		c.Hex = t.String()
	}
	for i := range l.Strobes {
		s := &l.Strobes[i]
		if err := id(s.ID, s.Alias); err != nil {
			return err
		}
		t, err := ParseTone(s.Hex)
		if err != nil {
			return fmt.Errorf("%s: %w", s.ID, err)
		}
		s.Hex = t.String()
		if s.Speed < 1 || s.Speed > 255 {
			return fmt.Errorf("%s: speed de 1 a 255", s.ID)
		}
	}
	for i := range l.Effects {
		e := &l.Effects[i]
		if err := id(e.ID, e.Alias); err != nil {
			return err
		}
		if !strings.HasPrefix(e.ID, EffectPrefix) {
			return fmt.Errorf("%s: o id de um modo de pista começa com %s", e.ID, EffectPrefix)
		}
		if err := e.parse(); err != nil {
			return fmt.Errorf("%s: %w", e.ID, err)
		}
		if e.Agitacao == "" {
			e.Agitacao = "media"
		}
		if e.Temp == "" {
			e.Temp = "mista"
		}
		if e.Mood != nil {
			e.Mood.Clamp()
		}
	}
	return nil
}

// Parse lê a biblioteca de um YAML.
func Parse(data []byte) (*Library, error) {
	var l Library
	if err := yaml.Unmarshal(data, &l); err != nil {
		return nil, err
	}
	if err := l.Check(); err != nil {
		return nil, err
	}
	return &l, nil
}

// Load lê a biblioteca de path.
func Load(path string) (*Library, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	l, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return l, nil
}

const header = `# Biblioteca de luz do DMXController: as cores, os strobos e os modos de pista
# que os botões do console (console.yaml), as timelines das músicas e a análise
# usam. Pode ser editada à mão; o programa lê ao iniciar.
#
# Tom: "#rrggbb" (dimmer no máximo; #000000 = apagado) ou "#rrggbb@180" (a cor
# com o dimmer em 180 de 255).
#
# colors:  cores fixas nos 3 canhões.
# strobes: strobo nativo do canhão (speed = canal de velocidade); a fritação
#          e o strobo contínuo do console.
# effects: modos de pista. steps = um passo por linha, os tons de esquerda,
#          centro e direita, tocados em loop; "350ms" = duração só daquele
#          passo (sem isso vale step_ms); "fade900ms" = o passo entra em fade.
#   agitation: calma | media | agitada      temp: quente | fria | mista | neutra
#   mood:      clima de 0 a 1 (energy = parado → explosivo, joy = sombrio →
#              alegre, tension = tranquilo → dramático, romance = frio →
#              íntimo), avaliado pela IA ("DMXController biblioteca").
# alias:   id antigo, do tempo do QLC+; as timelines que o usam continuam
#          abrindo.

`

type flowColor Color
type flowStrobe Strobe

func flow(v any) (any, error) {
	var n yaml.Node
	if err := n.Encode(v); err != nil {
		return nil, err
	}
	n.Style = yaml.FlowStyle
	return &n, nil
}

// MarshalYAML grava a cor numa linha só.
func (c Color) MarshalYAML() (any, error) { return flow(flowColor(c)) }

// MarshalYAML grava o strobo numa linha só.
func (s Strobe) MarshalYAML() (any, error) { return flow(flowStrobe(s)) }

// Save grava a biblioteca em path.
func (l *Library) Save(path string) error {
	data, err := yaml.Marshal(l)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append([]byte(header), data...), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
