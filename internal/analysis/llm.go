package analysis

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"regexp"
	"strings"

	"luzes/internal/show"
)

// DefaultModel é o modelo do OpenRouter usado por padrão (aceita áudio).
const DefaultModel = "google/gemini-3.8-flash"

// APIKey devolve a chave do OpenRouter: variável de ambiente
// OPENROUTER_API_KEY ou a linha correspondente do arquivo .env em envPath.
func APIKey(envPath string) string {
	if k := os.Getenv("OPENROUTER_API_KEY"); k != "" {
		return k
	}
	f, err := os.Open(envPath)
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		k, v, ok := strings.Cut(strings.TrimSpace(sc.Text()), "=")
		if ok && strings.TrimSpace(k) == "OPENROUTER_API_KEY" {
			return strings.Trim(strings.TrimSpace(v), `"'`)
		}
	}
	return ""
}

// wav16k gera um WAV mono 16 kHz 16 bits (o suficiente para o modelo ouvir
// a música, e bem menor que o original).
func wav16k(y []float64) []byte {
	const rate = 16000
	n := int(float64(len(y)) * rate / SR)
	var buf bytes.Buffer
	w := func(v any) { binary.Write(&buf, binary.LittleEndian, v) }
	buf.WriteString("RIFF")
	w(uint32(36 + 2*n))
	buf.WriteString("WAVEfmt ")
	w(uint32(16))
	w(uint16(1))
	w(uint16(1))
	w(uint32(rate))
	w(uint32(rate * 2))
	w(uint16(2))
	w(uint16(16))
	buf.WriteString("data")
	w(uint32(2 * n))
	for i := 0; i < n; i++ {
		pos := float64(i) * SR / rate
		j := int(pos)
		v := y[min(j, len(y)-1)]
		if j+1 < len(y) {
			v += (y[j+1] - v) * (pos - float64(j))
		}
		w(int16(math.Max(-1, math.Min(1, v)) * 32767))
	}
	return buf.Bytes()
}

var fence = regexp.MustCompile("(?m)^```(json)?|```$")

// prompt reúne o que vai para o modelo além da análise.
type prompt struct {
	model, key string
	temp       string // TempAuto, TempWarm, TempCold
	lyrics     string // letra colada (sem a versão cantada, a IA casa com as partes)
	sung       *sung  // letra transcrita da versão cantada, com tempos
}

// colorsInLyrics diz se a letra pode trazer cores (aí as cenas de cor fixa
// de qualquer temperatura e as cores novas valem).
func (p prompt) colorsInLyrics() bool { return p.sung != nil || p.lyrics != "" }

// tempRule explica ao modelo a temperatura pedida.
func tempRule(temp string) string {
	switch temp {
	case TempWarm:
		return "- Temperatura das cores pedida: QUENTES (vermelho, âmbar, dourado, laranja, fogo). Use só presets de temperatura quente, mista ou neutra, e prefira os quentes; mistos/neutros só onde faltar opção (ex.: parte muito agitada).\n"
	case TempCold:
		return "- Temperatura das cores pedida: FRIAS (azul, ciano, verde, roxo, violeta). Use só presets de temperatura fria, mista ou neutra, e prefira os frios; mistos/neutros só onde faltar opção (há poucos frios agitados: CLUB, e os mistos BATIDÃO/FLASH).\n"
	}
	return "- Temperatura das cores: livre — escolha a paleta pelo clima da música.\n"
}

// colorRule explica ao modelo como escolher a luz de uma cor citada.
const colorRule = `use a cena de cor fixa da biblioteca mais parecida ("TODOS | ..." ou "CORES FIXAS"/"BIBLIOTECA DE CORES", camada base, cena, agitação fixa — pode ser de qualquer temperatura) em "preset". Se nenhuma cor da biblioteca corresponder (ex.: lilás, turquesa, salmão), crie a cor em "cor", em vez de "preset": {"nome": "LILÁS", "rgb": "#c8a2c8"} (RGB de LED: saturado e claro; nada perto de preto). A cor acende por alguns compassos e depois volta o preset da parte (o programa cuida dos tempos). Deixe de fora a cor que não faz sentido virar luz (ex.: "Branca" como nome de pessoa).`

// lyricsBlock explica ao modelo como usar a letra (vazio sem letra). Com a
// versão cantada, a letra vem transcrita com os tempos e as cores citadas já
// têm o instante da palavra; só com a letra colada, não há tempos.
func lyricsBlock(a *show.Analysis, p prompt) string {
	if s := p.sung; s != nil {
		part := func(t float64) int {
			for i, sec := range a.Sections {
				if t < sec.End {
					return i
				}
			}
			return len(a.Sections) - 1
		}
		var lines, colors strings.Builder
		for _, l := range s.Lines {
			fmt.Fprintf(&lines, "  %.1f–%.1fs (parte %d): %s\n", l.T, l.End, part(l.T), l.Text)
		}
		for _, m := range s.Colors {
			fmt.Fprintf(&colors, "  %s: \"%s\" em %.1fs (parte %d), na frase \"%s\"\n", m.ID, m.Word, m.Time, part(m.Time), s.Lines[m.Line].Text)
		}
		out := `
Letra cantada (transcrita da versão com voz; o cantor canta ao vivo por cima do playback, nestes mesmos tempos):
` + lines.String() + `Uso da letra:
- Use a letra para entender o clima e o tema e para nomear as partes (o refrão costuma ser o grupo que se repete).
`
		if colors.Len() == 0 {
			return out + "- A letra não cita cores: continue com sequências de pista em todas as partes.\n"
		}
		return out + `
Cores citadas na letra (o instante é o da palavra):
` + colors.String() + `- Para CADA cor citada, responda em "cores_citadas" com o "id" dela: ` + colorRule + `
- Nas partes, continue escolhendo sequências de pista (a cor citada entra por cima, só na palavra).
`
	}
	if p.lyrics == "" {
		return ""
	}
	return `
Letra da música (colada pela equipe). ATENÇÃO: a voz principal é cantada AO VIVO e NÃO está no áudio — o playback só tem instrumental e vozes de fundo. Então use a letra para entender o clima e o tema e para casar versos/refrão com as partes detectadas (o refrão costuma ser o grupo que se repete), não para achar tempos exatos.
"""
` + p.lyrics + `
"""
Uso da letra:
- Diga em "letra" (em cada seção) o trecho da letra que você acha que cai naquela parte (curto, opcional).
- Se a letra CITAR uma cor (ex.: "vestido vermelho", "céu azul", "verde-limão"), marque em "cores_citadas" a parte em que ela é cantada ("parte": índice) e a "palavra": ` + colorRule + ` Sem a versão cantada não se sabe o segundo da palavra, então a cor acende no início da parte.
- Nas partes, continue escolhendo sequências de pista (a cor citada entra por cima).
`
}

// colorsFormat é o trecho do formato da resposta para as cores citadas.
func colorsFormat(p prompt) string {
	switch {
	case p.sung != nil && len(p.sung.Colors) > 0:
		return `,
  "cores_citadas": [{"id": "c0", "preset": "qlc:NN", "motivo": "curto"}, {"id": "c1", "cor": {"nome": "LILÁS", "rgb": "#c8a2c8"}, "motivo": "curto"}]`
	case p.sung == nil && p.lyrics != "":
		return `,
  "cores_citadas": [{"parte": 0, "palavra": "verde-limão", "preset": "qlc:NN", "motivo": "curto"}]`
	}
	return ""
}

// askLLM pede ao modelo (ouvindo a faixa) os nomes, o clima e as escolhas.
func askLLM(ctx context.Context, y []float64, a *show.Analysis, cat []Preset, p prompt) (*choices, string, error) {
	var offer []Preset // só o que a validação vai aceitar
	for _, pr := range cat {
		if pr.Fits(p.temp) || p.colorsInLyrics() && pr.Kind == "cena" && pr.Agitacao == "fixa" {
			offer = append(offer, pr)
		}
	}
	secs, presets := describe(a, offer)
	text := fmt.Sprintf(`Você é iluminador de um show pequeno: um cantor ao vivo sobre playback, com 3 canhões LED RGB (esquerda, centro, direita).
Ouça a faixa e use a análise abaixo (feita por software, os tempos são exatos — não invente tempos fora da grade).

BPM detectado: %.1f (pode estar no dobro/metade do andamento percebido)
Duração: %.1fs
Partes detectadas (grupo = partes parecidas entre si, ex.: refrões):
%s
Presets disponíveis (use SÓ estes ids):
%s%s
Regras:
- Para CADA parte, escolha um preset da camada "base" (de preferência sequências de pista; cenas de cor fixa só se a parte pedir luz parada). Partes do mesmo grupo devem, em geral, usar o mesmo preset, mas partes VIZINHAS com papéis diferentes (ex.: verso → refrão) devem mudar de preset, para a luz marcar a virada.
- A agitação do preset deve acompanhar a energia e o clima da parte.
%s- Acentos (opcionais, no máximo 1 a cada 20 s): presets da camada "flash" em momentos de impacto (drop, virada, entrada do refrão), duração de 1 a 4 batidas. "FRITAÇÃO | BRANCO MÁXIMO" é o efeito mais agressivo: use no máximo uma vez, só no ápice da música.
- Responda APENAS com JSON neste formato:
{
  "genero": "...", "clima": "...", "descricao": "1-2 frases",
  "bpm_percebido": numero,
  "intensidade": numero de 0 a 1,
  "paleta": ["nomes de cores do catálogo"],
  "secoes": [{"indice": 0, "nome": "intro|verso|pré-refrão|refrão|ponte|solo|final|...", "preset": "qlc:NN", "letra": "trecho (opcional)", "motivo": "curto"}],
  "acentos": [{"tempo": segundos, "preset": "qlc:NN", "duracao_batidas": 1, "motivo": "curto"}]%s
}`, a.BPM, a.Duration, secs, presets, lyricsBlock(a, p), tempRule(p.temp), colorsFormat(p))

	answer, usage, err := chat(ctx, p, 0.4, []any{
		map[string]any{"type": "text", "text": text},
		audioPart(y),
	})
	if err != nil {
		return nil, "", err
	}
	var c choices
	if err := json.Unmarshal([]byte(answer), &c); err != nil {
		return nil, "", fmt.Errorf("o modelo não devolveu JSON válido: %v", err)
	}
	return &c, usage, nil
}

// audioPart é um trecho de áudio (mono a SR) para mandar ao modelo.
func audioPart(y []float64) map[string]any {
	return map[string]any{"type": "input_audio", "input_audio": map[string]any{"data": base64.StdEncoding.EncodeToString(wav16k(y)), "format": "wav"}}
}

// chat manda uma mensagem (texto e áudios) ao modelo e devolve a resposta em
// JSON (sem cercas de código) e o uso de tokens.
func chat(ctx context.Context, p prompt, temperature float64, content []any) (answer, usage string, err error) {
	body, _ := json.Marshal(map[string]any{
		"model":           p.model,
		"messages":        []any{map[string]any{"role": "user", "content": content}},
		"response_format": map[string]any{"type": "json_object"},
		"temperature":     temperature,
	})
	req, err := http.NewRequestWithContext(ctx, "POST", "https://openrouter.ai/api/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Authorization", "Bearer "+p.key)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Title", "luzes")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("OpenRouter %d: %s", resp.StatusCode, trunc(string(data), 300))
	}
	var r struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			Prompt     int     `json:"prompt_tokens"`
			Completion int     `json:"completion_tokens"`
			Cost       float64 `json:"cost"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(data, &r); err != nil || len(r.Choices) == 0 {
		return "", "", fmt.Errorf("resposta inesperada do OpenRouter: %s", trunc(string(data), 300))
	}
	answer = strings.TrimSpace(fence.ReplaceAllString(strings.TrimSpace(r.Choices[0].Message.Content), ""))
	usage = fmt.Sprintf("%d+%d tokens", r.Usage.Prompt, r.Usage.Completion)
	if r.Usage.Cost > 0 {
		usage += fmt.Sprintf(", US$ %.4f", r.Usage.Cost)
	}
	return answer, usage, nil
}
