package analysis

import (
	"fmt"
	"math"
	"strings"

	"github.com/ncastellani/DMXController/internal/control"
	"github.com/ncastellani/DMXController/internal/fixture"
	"github.com/ncastellani/DMXController/internal/library"
	"github.com/ncastellani/DMXController/internal/show"
)

// Preset descreve um botão do console para as escolhas do coreógrafo.
type Preset struct {
	ID       string   // id na biblioteca ("pista:club")
	Name     string   // nome de exibição ("PISTA | CLUB")
	Caption  string   // legenda do botão
	Group    string   // grupo no console
	Layer    string   // base | strobe | flash
	Kind     string   // cena | sequencia
	Colors   []string // cores nomeadas que aparecem nos canhões
	Agitacao string   // calma | media | agitada (sequências), fixa | strobo (cenas)
	StepMs   float64  // passo típico (sequências)
	Fade     bool     // a sequência tem fades
	Temp     string   // temperatura das cores: quente | fria | mista | neutra
	Hex      string   // cor das cenas de cor fixa ("#rrggbb")
	Library  bool     // cor fixa da biblioteca que não está nos botões
}

// Temperaturas pedidas na análise (Options.Temperature).
const (
	TempAuto  = "auto"
	TempWarm  = "quente"
	TempCold  = "fria"
	TempMixed = "mista"
	TempNone  = "neutra"
)

// colorTemp classifica uma cor nomeada: quente, fria, neutra (branco) ou ""
// (rosa e magenta ficam no meio do caminho).
func colorTemp(name string) string {
	first, _, _ := strings.Cut(name, " ")
	switch first {
	case "VERMELHO", "ÂMBAR", "DOURADO", "LARANJA", "FOGO", "BORDÔ", "AMARELO":
		return TempWarm
	case "AZUL", "CIANO", "VERDE", "ROXO", "VIOLETA":
		return TempCold
	case "BRANCO":
		return TempNone
	}
	return ""
}

// presetTemp é a temperatura da maioria das cores (pesadas por quantas vezes
// aparecem nos canhões). Com a minoria acima de 25%, o preset é misto.
func presetTemp(weights map[string]int) string {
	warm, cold := 0, 0
	for name, w := range weights {
		switch colorTemp(name) {
		case TempWarm:
			warm += w
		case TempCold:
			cold += w
		}
	}
	switch {
	case warm+cold == 0:
		return TempNone
	case float64(min(warm, cold)) > 0.25*float64(warm+cold):
		return TempMixed
	case warm > cold:
		return TempWarm
	}
	return TempCold
}

// Fits diz se o preset combina com a temperatura pedida (mistos e neutros
// combinam com qualquer uma).
func (p Preset) Fits(temp string) bool {
	switch temp {
	case TempWarm, TempCold:
		return p.Temp == temp || p.Temp == TempMixed || p.Temp == TempNone
	}
	return true
}

// Catalog descreve os presets para as escolhas do coreógrafo: os botões do
// console e, como "biblioteca de cores", as cores fixas que não são botões.
func Catalog(c *control.Console) []Preset {
	lib := c.Library
	// A cor nomeada mais parecida (para dizer de que cor é um strobo).
	colorName := func(t library.Tone) string {
		rgb := func(t library.Tone) (r, g, b, m float64) {
			c, _ := show.ParseColor(t.Hex())
			return float64(c.R), float64(c.G), float64(c.B), float64(max(c.R, c.G, c.B))
		}
		r, g, b, m := rgb(t)
		best, bd := "", math.Inf(1)
		for _, pc := range lib.Colors {
			pt, _ := library.ParseTone(pc.Hex)
			pr, pg, pb, pm := rgb(pt)
			if d := sq(r/m-pr/pm) + sq(g/m-pg/pm) + sq(b/m-pb/pm); d < bd {
				best, bd = pc.Name, d
			}
		}
		return best
	}
	color := func(p *Preset, name string, t library.Tone) {
		p.Kind, p.Agitacao, p.Colors, p.Hex = "cena", "fixa", []string{name}, t.Hex()
		p.Temp = presetTemp(map[string]int{name: 1})
	}

	var presets []Preset
	inConsole := map[string]bool{}
	for _, b := range c.Buttons {
		inConsole[b.Preset] = true
		p := Preset{ID: b.Preset, Name: b.Name, Caption: b.Caption, Group: b.Group, Layer: b.Layer.String()}
		if e := lib.Effect(b.Preset); e != nil {
			var durs []float64
			for _, st := range e.Parsed() {
				durs = append(durs, float64(st.Duration.Milliseconds()))
			}
			p.Kind, p.Colors, p.Agitacao, p.Temp, p.StepMs, p.Fade = "sequencia", e.Colors, e.Agitacao, e.Temp, medianF(durs), e.Fades()
		} else if col := lib.Color(b.Preset); col != nil {
			t, _ := library.ParseTone(col.Hex)
			color(&p, col.Name, t)
		} else if st := lib.Strobe(b.Preset); st != nil {
			t, _ := library.ParseTone(st.Hex)
			color(&p, colorName(t), t)
			p.Agitacao, p.Hex = "strobo", ""
		}
		presets = append(presets, p)
	}
	// Biblioteca de cores: as cores fixas que não viraram botão (ex.: VERDE
	// LIMÃO), para a análise e o editor.
	for i := range lib.Colors {
		col := &lib.Colors[i]
		if inConsole[col.ID] {
			continue
		}
		t, _ := library.ParseTone(col.Hex)
		p := Preset{ID: col.ID, Name: col.FullName(), Caption: col.Name, Group: "BIBLIOTECA DE CORES", Layer: "base", Library: true}
		color(&p, col.Name, t)
		presets = append(presets, p)
	}
	return presets
}

func sq(x float64) float64 { return x * x }

// hexOf devolve a cor que o canhão emite (já com o dimmer) em "#rrggbb".
func hexOf(c fixture.Color) string { return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B) }
