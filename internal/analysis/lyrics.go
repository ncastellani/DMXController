package analysis

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/ncastellani/DMXController/internal/show"
)

// sung é a letra transcrita da versão cantada: as frases com tempo e as
// cores citadas, com o instante da palavra.
type sung struct {
	Lines  []show.Lyric
	Colors []mention
}

// mention é uma cor citada na letra.
type mention struct {
	ID   string  // "c0", "c1"... (como a IA da coreografia a chama)
	Time float64 // s, no começo da palavra
	Word string
	Line int // índice em Lines
}

// clipPad é a folga (s) antes e depois de cada frase mandada para transcrever.
const clipPad = 0.15

// transcribe manda ao modelo um áudio por frase cantada e pede a letra de
// cada uma e as cores citadas. hint é a letra colada (opcional): ajuda a
// acertar as palavras, mas vale o que é cantado.
func transcribe(ctx context.Context, v *voice, hint string, p prompt) (*sung, string, error) {
	src := "a voz isolada (pode sobrar um pouco do instrumental)"
	if !v.Clean {
		src = "a música com a voz (transcreva só a voz principal)"
	}
	ref := ""
	if hint != "" {
		ref = `
Letra de referência (colada pela equipe; pode estar em outra ordem, sem as repetições ou com pequenas diferenças). Use-a para acertar as palavras, mas escreva em cada frase o que é cantado nela:
"""
` + hint + `
"""
`
	}
	text := fmt.Sprintf(`Transcreva a letra cantada de uma música, frase por frase. Cada áudio abaixo é UMA frase, já recortada no tempo certo (com %.2f s de folga antes e depois), e traz %s. As frases estão na ordem da música.
%s
Regras:
- "texto": exatamente o que é cantado naquela frase, no idioma da música, sem pontuação no fim. Vazio ("") se não houver palavra (só instrumental, respiração ou ruído). Vocalizes ("oh", "uh", "lalaiá") podem entrar.
- "cores": só quando a frase CITA uma cor (ex.: "vermelho", "azul-marinho", "verde-limão", "cor-de-rosa", "dourado"); "palavra" é a cor como cantada e "inicio" é o segundo, contado do começo do áudio daquela frase, em que a palavra começa. Sem cor, deixe a lista vazia.
- Responda APENAS com JSON:
{"frases": [{"i": 0, "texto": "...", "cores": [{"palavra": "verde-limão", "inicio": 1.2}]}]}`, clipPad, src, ref)

	content := []any{map[string]any{"type": "text", "text": text}}
	for i, ph := range v.Phrases {
		content = append(content,
			map[string]any{"type": "text", "text": fmt.Sprintf("Frase %d (%.1f s):", i, ph.End-ph.Start+2*clipPad)},
			audioPart(clip(v.Audio, ph.Start, ph.End, clipPad)))
	}
	answer, usage, err := chat(ctx, p, 0.1, content)
	if err != nil {
		return nil, "", err
	}
	var r struct {
		Frases []struct {
			I     int    `json:"i"`
			Texto string `json:"texto"`
			Cores []struct {
				Palavra string  `json:"palavra"`
				Inicio  float64 `json:"inicio"`
			} `json:"cores"`
		} `json:"frases"`
	}
	if err := json.Unmarshal([]byte(answer), &r); err != nil {
		return nil, "", fmt.Errorf("o modelo não devolveu JSON válido: %v", err)
	}
	texts := make([]string, len(v.Phrases))
	type found struct {
		word string
		at   float64
	}
	colors := make([][]found, len(v.Phrases))
	for _, f := range r.Frases {
		if f.I < 0 || f.I >= len(v.Phrases) {
			continue
		}
		texts[f.I] = trunc(strings.Join(strings.Fields(f.Texto), " "), 200)
		for _, c := range f.Cores {
			if w := strings.TrimSpace(c.Palavra); w != "" {
				colors[f.I] = append(colors[f.I], found{trunc(w, 30), c.Inicio})
			}
		}
	}
	s := &sung{Lines: []show.Lyric{}}
	for i, ph := range v.Phrases {
		if texts[i] == "" {
			continue
		}
		s.Lines = append(s.Lines, show.Lyric{T: ph.Start, End: ph.End, Text: texts[i]})
		start := math.Max(0, ph.Start-clipPad) // onde o áudio da frase começa
		for _, c := range colors[i] {
			at := math.Max(ph.Start, math.Min(ph.End, start+c.at))
			s.Colors = append(s.Colors, mention{ID: fmt.Sprintf("c%d", len(s.Colors)), Time: round(at, 3), Word: c.word, Line: len(s.Lines) - 1})
		}
	}
	return s, usage, nil
}
