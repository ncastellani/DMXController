package analysis

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/ncastellani/DMXController/internal/library"
	"github.com/ncastellani/DMXController/internal/show"
)

// Como a análise usa a biblioteca (internal/library): além de escolher um
// modo de pista como preset (que toca com os tempos dele), o modo livre pode
// levar o desenho de um deles para a música como efeito (os passos copiados
// para a timeline, trocando a cada toque) e trocar a paleta dele.

// Mood é a pontuação de clima de um efeito ou de uma música.
type Mood = library.Mood

// maxRecolor é o máximo de cores de um efeito que aceita troca de paleta
// (os que têm mais são degradês de tons escuros, que só fazem sentido com as
// cores originais).
const maxRecolor = 8

// slowStepMs separa os efeitos lentos (CHILL, CINEMA, COLOR FLOW), que
// seguem o passo original em batidas, dos que trocam a cada toque da música.
const slowStepMs = 1000

const libWhite, libOff = "#ffffff", "#000000"

// effectFits diz se o efeito, com as cores originais, combina com a
// temperatura.
func effectFits(e *library.Effect, temp string) bool { return Preset{Temp: e.Temp}.Fits(temp) }

// effectPalette são as cores do efeito na ordem em que aparecem (sem branco
// e apagado, que não mudam): as que uma troca de paleta substitui. Vazia se
// o efeito tem cores demais para trocar.
func effectPalette(e *library.Effect) []string {
	var pal []string
	for _, st := range e.StepColors() {
		for _, c := range st {
			if c != libWhite && c != libOff && !slices.Contains(pal, c) {
				pal = append(pal, c)
			}
		}
	}
	if len(pal) > maxRecolor {
		return nil
	}
	return pal
}

// effectPattern escreve os passos com uma letra por cor da paleta ("ABC CAB
// BCA"), W = branco e - = apagado: o desenho do efeito, sem as cores.
func effectPattern(e *library.Effect) string {
	pal := effectPalette(e)
	var b strings.Builder
	for i, st := range e.StepColors() {
		if i > 0 {
			b.WriteByte(' ')
		}
		for _, c := range st {
			switch k := slices.Index(pal, c); {
			case c == libWhite:
				b.WriteByte('W')
			case c == libOff:
				b.WriteByte('-')
			case k >= 0:
				b.WriteByte(byte('A' + k))
			default:
				b.WriteByte('?')
			}
		}
	}
	return b.String()
}

// LibraryEffect devolve o modo de pista e como efeito de uma música no
// andamento bpm: os mesmos passos, trocando a cada toque (os lentos seguem o
// passo original, em batidas). Com colors, a paleta é trocada (a 1ª cor do
// efeito vira colors[0] etc.; branco e apagado ficam) e as cores novas têm de
// caber na temperatura temp; sem colors, vale a temperatura do próprio
// efeito. Um modo que é pisca-pisca ou quase todo apagado (FLASH) não serve
// de efeito.
func LibraryEffect(e *library.Effect, bpm float64, colors []string, temp string) (*show.Effect, bool) {
	steps, name, check := e.StepColors(), e.Name, TempAuto
	pal := effectPalette(e)
	if len(colors) > 0 && len(pal) > 0 {
		if len(colors) < 2 {
			return nil, false
		}
		for _, st := range steps {
			for i, c := range st {
				if k := slices.Index(pal, c); k >= 0 {
					st[i] = colors[k%len(colors)]
				}
			}
		}
		name, check = e.Name+" (cores novas)", temp
	} else if !effectFits(e, temp) {
		return nil, false
	}
	fx, ok := cleanEffect(&llmEffect{Type: show.FXSequence, Steps: steps, Fade: e.Fades()}, check)
	if !ok {
		return nil, false
	}
	fx.Name = name
	if ms := e.TypicalStep(); ms >= slowStepMs { // lento: segue o passo original, em batidas
		if bpm <= 0 {
			bpm = 120
		}
		fx.Sync, fx.Beats = "", math.Max(1, ms*bpm/60000)
		if fx.Clean() != nil {
			return nil, false
		}
	}
	return fx, true
}

// Usable diz se o modo de pista serve como efeito do modo livre.
func Usable(e *library.Effect) bool {
	_, ok := LibraryEffect(e, 120, nil, TempAuto)
	return ok
}

// usable são os modos de pista que servem como efeito.
func usable(l *library.Library) []*library.Effect {
	if l == nil {
		return nil
	}
	var out []*library.Effect
	for i := range l.Effects {
		if Usable(&l.Effects[i]) {
			out = append(out, &l.Effects[i])
		}
	}
	return out
}

// describeLibrary lista os efeitos para o prompt, com o desenho de cada um
// (paleta + passos em letras); temp marca os que só valem com cores novas.
func describeLibrary(l *library.Library, temp string) string {
	var b strings.Builder
	for _, e := range usable(l) {
		fmt.Fprintf(&b, "  %s | %s | agitação %s | temperatura %s", e.ID, e.Name, e.Agitacao, e.Temp)
		if e.Fades() {
			b.WriteString(" | em fade")
		}
		if e.Mood != nil {
			b.WriteString(" | clima: " + e.Mood.String())
		}
		if pal := effectPalette(e); len(pal) > 0 {
			b.WriteString(" | paleta")
			for k, c := range pal {
				fmt.Fprintf(&b, " %c=%s", 'A'+k, c)
			}
			b.WriteString(" | passos: " + effectPattern(e))
			if !effectFits(e, temp) {
				b.WriteString(" | fora da temperatura pedida: só com cores novas")
			}
		} else {
			fmt.Fprintf(&b, " | cores: %s | %d passos em tons degradê (não aceita cores novas)", joinOr(e.Colors, "-"), len(e.Steps))
			if !effectFits(e, temp) {
				b.WriteString(" | fora da temperatura pedida: não use")
			}
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// colorHex é a cor que uma cor da biblioteca emite, em "#rrggbb".
func colorHex(c *library.Color) string {
	t, _ := library.ParseTone(c.Hex)
	return t.Hex()
}

// paletteText lista as cores nomeadas para o prompt.
func paletteText(l *library.Library, temp string) string {
	var out []string
	for i := range l.Colors {
		c := &l.Colors[i]
		if (temp == TempWarm || temp == TempCold) && c.Temp != "" && c.Temp != TempNone && c.Temp != temp {
			continue
		}
		out = append(out, fmt.Sprintf("%s %s", c.Name, colorHex(c)))
	}
	return strings.Join(out, ", ")
}

// ScoreMoods pede ao modelo (só texto) a pontuação de clima de cada modo de
// pista que serve de efeito, pelas cores e pelo movimento. all refaz também
// os que já têm.
func ScoreMoods(ctx context.Context, l *library.Library, key, model string, all bool) (usage string, n int, err error) {
	if model == "" {
		model = DefaultModel
	}
	names := map[string]string{}
	for i := range l.Colors {
		names[colorHex(&l.Colors[i])] = l.Colors[i].Name
	}
	var list strings.Builder
	for _, e := range usable(l) {
		if e.Mood != nil && !all {
			continue
		}
		n++
		fmt.Fprintf(&list, "- %s | %s | passo de %.0f ms", e.ID, e.Name, e.TypicalStep())
		if e.Fades() {
			list.WriteString(", em fade")
		}
		list.WriteString(" | cores: " + joinOr(e.Colors, "-"))
		if pal := effectPalette(e); len(pal) > 0 {
			list.WriteString(" | paleta")
			for k, c := range pal {
				fmt.Fprintf(&list, " %c=%s", 'A'+k, c)
				if names[c] != "" {
					list.WriteString(" (" + names[c] + ")")
				}
			}
			list.WriteString(" | passos: " + effectPattern(e))
		} else {
			fmt.Fprintf(&list, " | %d passos em degradê de tons claros e escuros dessas cores", len(e.Steps))
		}
		list.WriteByte('\n')
	}
	if n == 0 {
		return "", 0, nil
	}
	text := fmt.Sprintf(`Você é iluminador de shows. Abaixo estão efeitos de luz para 3 canhões LED RGB (esquerda, centro, direita) num show pequeno de música pop.
Cada efeito é uma sequência de passos em loop; em "passos", cada grupo de 3 letras é um passo (uma letra por canhão: a cor da paleta, W = branco, - = apagado).

Avalie o CLIMA que cada efeito passa para a plateia, pelas cores (matiz, contraste entre elas, tons escuros, branco, apagões) e pelo movimento (velocidade do passo, fade, canhões iguais ou diferentes, um canhão por vez). Dê notas de 0 a 1, usando a escala inteira para os efeitos se distinguirem entre si:
- "energia": 0 = parado, contemplativo; 1 = explosivo, frenético.
- "alegria": 0 = sombrio, pesado; 1 = alegre, festivo, colorido.
- "tensao": 0 = tranquilo, acolhedor; 1 = dramático, agressivo, de suspense.
- "romance": 0 = frio, impessoal; 1 = íntimo, sensual, apaixonado.
- "clima": 2 a 4 palavras em português (ex.: "balada noturna sensual").

Efeitos:
%s
Responda APENAS com JSON: {"efeitos": [{"id": "pista:...", "energia": 0.0, "alegria": 0.0, "tensao": 0.0, "romance": 0.0, "clima": "..."}]} com todos os efeitos da lista.`, list.String())
	answer, usage, err := chat(ctx, prompt{model: model, key: key}, 0.2, []any{map[string]any{"type": "text", "text": text}})
	if err != nil {
		return "", 0, err
	}
	var r struct {
		Effects []struct {
			ID string `json:"id"`
			Mood
		} `json:"efeitos"`
	}
	if err := json.Unmarshal([]byte(answer), &r); err != nil {
		return "", 0, fmt.Errorf("o modelo não devolveu JSON válido: %v", err)
	}
	n = 0
	for _, x := range r.Effects {
		if e := l.Effect(x.ID); e != nil && (e.Mood == nil || all) {
			m := x.Mood
			m.Clamp()
			e.Mood = &m
			n++
		}
	}
	return usage, n, nil
}

// moodEnergy é a energia do efeito: a do clima avaliado ou, sem ele, a da
// agitação.
func moodEnergy(e *library.Effect) float64 {
	if e.Mood != nil {
		return e.Mood.Energy
	}
	return map[string]float64{"calma": 0.2, "media": 0.5, "agitada": 0.85}[e.Agitacao]
}

// pickEffect escolhe o efeito da biblioteca para uma parte de energia alvo
// (0 a 1) numa música de clima song (pode ser nil): o mais parecido que caiba
// na temperatura, sem repetir os de skip enquanto houver outros. Com a
// temperatura livre, prefer (a dos efeitos já escolhidos para a música) puxa
// para a mesma família de cores, para a música não virar um mostruário.
func pickEffect(l *library.Library, energy float64, song *Mood, temp, prefer string, skip map[string]bool) *library.Effect {
	if temp == TempWarm || temp == TempCold {
		prefer = temp
	}
	var best *library.Effect
	bd := math.Inf(1)
	for _, fresh := range []bool{true, false} {
		for _, e := range usable(l) {
			if fresh && skip[e.ID] || !effectFits(e, temp) {
				continue
			}
			d := math.Abs(moodEnergy(e) - energy)
			if prefer != "" && e.Temp != prefer {
				d += 0.3 // mistos, neutros e a outra temperatura só quando faltam opções
			}
			switch {
			case song != nil && e.Mood != nil:
				d += (math.Abs(e.Mood.Joy-song.Joy) + math.Abs(e.Mood.Tension-song.Tension) + math.Abs(e.Mood.Romance-song.Romance)) / 3
			case song != nil: // sem clima avaliado: nem perto nem longe
				d += 0.25
			}
			if d < bd {
				best, bd = e, d
			}
		}
		if best != nil {
			break
		}
	}
	return best
}
