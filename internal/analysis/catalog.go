package analysis

import (
	"fmt"
	"math"
	"slices"
	"strings"

	"luzes/internal/control"
	"luzes/internal/dmx"
	"luzes/internal/fixture"
)

// Preset descreve um botão do console para as escolhas do coreógrafo.
type Preset struct {
	ID       string   // "qlc:<Function ID>"
	Name     string   // nome da Function no QLC+
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
	Library  bool     // cor "TODOS | X" do workspace que não está nos botões
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

// Catalog descreve os botões do console. positions são os fixtures de cada
// posição com endereço lógico (o quadro dos presets).
func Catalog(c *control.Console, positions []*fixture.Fixture) []Preset {
	ws := c.Workspace
	look := func(f *dmx.Frame) (cols []fixture.Color, strobe bool) {
		for _, p := range positions {
			col, st := p.Look(f)
			cols = append(cols, col)
			strobe = strobe || st
		}
		return cols, strobe
	}

	// Paleta nomeada: as cores fixas "TODOS | X" (cor do primeiro canhão).
	type named struct {
		name string
		c    fixture.Color
	}
	var palette []named
	for _, sc := range ws.Scenes {
		if rest, ok := strings.CutPrefix(sc.Name, "TODOS |"); ok {
			var f dmx.Frame
			sc.Render(0, &f)
			if cols, _ := look(&f); len(cols) > 0 && max(cols[0].R, cols[0].G, cols[0].B) > 0 {
				palette = append(palette, named{strings.TrimSpace(rest), cols[0]})
			}
		}
	}
	slices.SortFunc(palette, func(a, b named) int { return strings.Compare(a.name, b.name) })
	colorName := func(c fixture.Color) string {
		m := float64(max(c.R, c.G, c.B))
		if m < 30 {
			return "apagado"
		}
		best, bd := "", math.Inf(1)
		for _, p := range palette {
			pm := float64(max(p.c.R, p.c.G, p.c.B))
			d := sq(float64(c.R)/m-float64(p.c.R)/pm) + sq(float64(c.G)/m-float64(p.c.G)/pm) + sq(float64(c.B)/m-float64(p.c.B)/pm)
			if d < bd {
				best, bd = p.name, d
			}
		}
		return best
	}

	var presets []Preset
	scores := map[string]float64{}
	inConsole := map[int]bool{}
	for _, b := range c.Buttons {
		inConsole[b.Function] = true
		p := Preset{ID: fmt.Sprintf("qlc:%d", b.Function), Name: b.Name, Caption: b.Caption, Group: b.Group, Layer: b.Layer.String()}
		colors := map[string]int{}
		if seq, ok := ws.Sequences[b.Function]; ok {
			p.Kind = "sequencia"
			var durs []float64
			dark, total := 0, 0
			for _, st := range seq.Steps {
				durs = append(durs, float64(st.Duration.Milliseconds()))
				p.Fade = p.Fade || st.FadeIn > 0
				var f dmx.Frame
				for _, v := range st.Values {
					f.Set(v.Channel, v.Value)
				}
				cols, _ := look(&f)
				for _, col := range cols {
					total++
					if n := colorName(col); n == "apagado" {
						dark++
					} else {
						colors[n]++
					}
				}
			}
			p.StepMs = medianF(durs)
			darkRatio := float64(dark) / float64(max(total, 1))
			score := math.Log(1000/math.Max(p.StepMs, 1)) + 1.5*darkRatio
			if p.Fade {
				score -= 0.8
			}
			scores[p.ID] = score
		} else {
			p.Kind = "cena"
			var f dmx.Frame
			b.Src.Render(0, &f)
			cols, strobe := look(&f)
			own, isColor := strings.CutPrefix(b.Name, "TODOS |")
			for _, col := range cols {
				if isColor { // a cena de cor fixa tem o nome da própria cor
					colors[strings.TrimSpace(own)]++
				} else {
					colors[colorName(col)]++
				}
			}
			if len(cols) > 0 && !strobe {
				p.Hex = hexOf(cols[0])
			}
			p.Agitacao = "fixa"
			if strobe {
				p.Agitacao = "strobo"
			}
		}
		for n := range colors {
			p.Colors = append(p.Colors, n)
		}
		slices.Sort(p.Colors)
		p.Temp = presetTemp(colors)
		presets = append(presets, p)
	}

	// Biblioteca: as cores fixas do workspace que não viraram botão
	// (ex.: VERDE LIMÃO), para a IA usar quando a letra citar a cor.
	var ids []int
	for id, sc := range ws.Scenes {
		if strings.HasPrefix(sc.Name, "TODOS |") && !inConsole[id] {
			ids = append(ids, id)
		}
	}
	slices.SortFunc(ids, func(a, b int) int { return strings.Compare(ws.Scenes[a].Name, ws.Scenes[b].Name) })
	for _, id := range ids {
		sc := ws.Scenes[id]
		var f dmx.Frame
		sc.Render(0, &f)
		cols, _ := look(&f)
		if len(cols) == 0 || max(cols[0].R, cols[0].G, cols[0].B) == 0 {
			continue // TODOS | BLACKOUT
		}
		name := strings.TrimSpace(strings.TrimPrefix(sc.Name, "TODOS |"))
		presets = append(presets, Preset{
			ID: fmt.Sprintf("qlc:%d", id), Name: sc.Name, Caption: name, Group: "BIBLIOTECA DE CORES",
			Layer: "base", Kind: "cena", Agitacao: "fixa", Colors: []string{name},
			Temp: presetTemp(map[string]int{name: 1}), Hex: hexOf(cols[0]), Library: true,
		})
	}

	// Agitação das sequências em terços (calma / média / agitada).
	var sorted []float64
	for _, s := range scores {
		sorted = append(sorted, s)
	}
	slices.Sort(sorted)
	if len(sorted) > 0 {
		lo, hi := sorted[len(sorted)/3], sorted[2*len(sorted)/3]
		for i := range presets {
			if s, ok := scores[presets[i].ID]; ok {
				switch {
				case s < lo:
					presets[i].Agitacao = "calma"
				case s < hi:
					presets[i].Agitacao = "media"
				default:
					presets[i].Agitacao = "agitada"
				}
			}
		}
	}
	return presets
}

func sq(x float64) float64 { return x * x }

// hexOf devolve a cor que o canhão emite (já com o dimmer) em "#rrggbb".
func hexOf(c fixture.Color) string { return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B) }
