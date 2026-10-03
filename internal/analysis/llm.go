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

	"github.com/ncastellani/DMXController/internal/library"
	"github.com/ncastellani/DMXController/internal/show"
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
	free       bool   // modo livre: a IA pode criar efeitos no lugar dos presets
	mixed      bool   // modo misto: presets e efeitos combinados na mesma música
	lib        *library.Library
}

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

// beatRule pede atenção à batida dentro das partes.
const beatRule = `- SIGA A BATIDA: a agitação da luz acompanha a "batida" tanto quanto a "energia". Uma parte de energia baixa com batida alta (ex.: verso de trap ou funk só com o beat e a voz) pede luz ritmada, de cortes secos e bem marcada, não fade lento. Uma parte com batida baixa pede luz mais contínua.
- Grupos com "+" ou "-" (ex.: C- depois de C, D+ depois de D) são trechos da MESMA parte em que a percussão muda no meio: "+" = a bateria entra ou cresce, "-" = a bateria some ou recua (ex.: os compassos de respiro antes do refrão). Dê a esses trechos o mesmo nome da parte com o que mudou (ex.: "verso (bateria entra)", "refrão (respiro)") e MUDE a luz ali: mais agitada e contrastada no "+", mais parada e fechada no "-", de preferência com as mesmas cores da parte, para parecer uma variação e não outra música.
`

// partRule diz ao modelo como escolher a luz de cada parte; no modo livre,
// ele pode criar um efeito em vez de usar um preset.
func partRule(p prompt) string {
	rule := beatRule + `- Para CADA parte, escolha um preset da camada "base" (de preferência sequências de pista; cenas de cor fixa só se a parte pedir luz parada). Partes do mesmo grupo devem, em geral, usar o mesmo preset, mas partes VIZINHAS com papéis diferentes (ex.: verso → refrão) devem mudar de preset, para a luz marcar a virada.
- A agitação do preset deve acompanhar a energia e o clima da parte.
`
	if !p.free {
		return rule
	}
	return beatRule + `- MODO LIVRE: você cria a luz desta música, com a mesma riqueza das sequências de pista prontas (elas têm de 12 a 24 passos, 4 a 6 cores e "frases" que mudam o tempo todo). Para CADA parte, crie um efeito em "efeito" (sem "preset") ou, onde uma sequência pronta servir melhor (ex.: a parte mais agitada), use um preset da camada "base" em "preset" (sem "efeito"). Prefira os efeitos: a ideia é a música ter uma luz própria, pensada para o clima dela.
- Em quase todas as partes o efeito deve ser uma SEQUÊNCIA escrita por você: {"tipo": "sequence", "fade": true|false, "intensidade": 0.4 a 1, "passos": [[esquerda, centro, direita], ...]}.
  - Os passos avançam sozinhos A CADA TOQUE FORTE DA MÚSICA (batida da bateria, palhetada da guitarra, entrada de um acorde), detectado pelo programa no áudio: você não escolhe a duração do passo. Nas partes cheias isso dá uma troca por batida, mais ou menos; nas calmas, as trocas acompanham os toques mais espaçados.
  - Cada passo tem as 3 cores "#rrggbb" dos canhões (esquerda, centro, direita). Cores de LED: saturadas e claras. SEM EXTREMOS: nenhum canhão branco, nenhum canhão apagado ("#000000") e nenhum tom escuro (o maior canal de cada cor fica de 128 para cima); o contraste vem de mudar de cor, não de apagar.
  - De 8 a 24 passos, tocados em loop. Monte a sequência em FRASES de 2 a 4 passos e mude de frase várias vezes, como as sequências prontas fazem: rodízio de 3 cores entre os canhões; dois tons alternando (A B A → B A B); um canhão numa cor viva por vez com os outros dois num tom mais fechado da mesma cor; os 3 iguais trocando de cor; tom fechado → tom vivo da mesma cor; laterais numa cor e centro em outra. NÃO use branco ("#ffffff") nas sequências: o branco é reservado aos picos da música, em que o programa já dá sozinho um flash branco no canhão do centro.
  - Use de 4 a 6 cores por sequência (não só 2), combinando com o clima; para segurar um quadro por dois toques, repita o passo.
  - Partes calmas: "fade": true (cada passo se transforma no seguinte), mudando um canhão de cada vez e com cores vizinhas, para a troca a cada toque ficar suave. Partes agitadas: cortes secos e contraste alto entre um passo e o seguinte.
  - Exemplo (parte agitada, 16 passos): [["#ff2878","#ff00ff","#0000ff"],["#0000ff","#ff2878","#ff00ff"],["#ff00ff","#0000ff","#ff2878"],["#ff0064","#00ffff","#ff0064"],["#00ffff","#ff0064","#00ffff"],["#7800ff","#3c0080","#3c0080"],["#3c0080","#7800ff","#3c0080"],["#3c0080","#3c0080","#7800ff"],["#b41c55","#b41c55","#b41c55"],["#ff2878","#ff2878","#ff2878"],["#ff2878","#00ffff","#ff2878"],["#00ffff","#0000ff","#00ffff"],["#0000ff","#00ffff","#0000ff"],["#ff0064","#7800ff","#00ffff"],["#00ffff","#ff0064","#7800ff"],["#7800ff","#00ffff","#ff0064"]]. Não copie: crie as suas.
- Efeitos simples (só para momentos pontuais, como um final parado; também trocam a cada toque): {"tipo", "cores", "intensidade"} com "tipo": "fixed" (cores paradas: 1 cor = os 3 canhões iguais; 3 cores = esquerda, centro, direita), "pulse" (as cores acendem forte a cada toque e caem suave), "fade" (2 a 4 cores se transformando uma na outra), "alternate" (os canhões trocam de cor entre si), "chase" (a 1ª cor corre de um canhão ao outro sobre a 2ª).
- Partes do mesmo grupo devem, em geral, repetir a mesma sequência, mas partes VIZINHAS com papéis diferentes (ex.: verso → refrão) devem mudar bastante (cores e frases), para a luz marcar a virada. A música inteira deve ter uma paleta coerente (5 a 7 cores no total), com o refrão mais vivo e mais variado.
- O efeito nunca pisca rápido: fritação e strobo continuam só nos acentos.
` + libRule(p) + mixRule(p)
}

// mixRule pede a combinação do modo misto.
func mixRule(p prompt) string {
	if !p.mixed {
		return ""
	}
	return `- MODO MISTO (vale no lugar de "prefira os efeitos"): esta música combina os dois. Pelo menos um terço das partes com "preset" (modos de pista prontos, que tocam com os tempos e as rajadas deles: bons para as partes mais agitadas e para marcar o refrão) e pelo menos um terço com "efeito" (sequências suas ou modos de pista com cores novas: dão à música uma cor própria). Sempre que der, partes vizinhas alternam entre preset e efeito. Escolha presets cujas cores conversem com a paleta dos seus efeitos.
`
}

// libRule apresenta ao modelo os modos de pista como efeitos (que ele pode
// usar como estão ou com outra paleta) e as cores nomeadas.
func libRule(p prompt) string {
	if len(usable(p.lib)) == 0 {
		return ""
	}
	rule := `- MODOS DE PISTA COMO EFEITO (lista "Desenho dos modos de pista" abaixo): em vez de escrever os passos, você pode levar o desenho de um modo de pista para a parte, em "efeito": {"modelo": "pista:nome"} (cores originais) ou {"modelo": "pista:nome", "cores": ["#rrggbb", ...]} para manter o desenho e trocar a paleta: a 1ª cor da lista entra no lugar de A, a 2ª no de B etc. (dê uma cor para cada letra). Como efeito, os passos todos brancos (WWW) ou todos apagados (---) do desenho são retirados e um canhão apagado ganha um tom fechado da cor do passo. Como efeito, os passos trocam a cada toque da música, como nas suas sequências; em "preset", o mesmo modo toca com os tempos próprios dele (inclusive rajadas rápidas). Trocar a paleta é a melhor forma de aproveitar um desenho bom com as cores desta música.
  - Em "passos", cada grupo de 3 letras é um passo (esquerda, centro, direita); W = branco, - = apagado. O "clima" de cada um vai de 0 a 1 em energia (parado → explosivo), alegria (sombrio → alegre), tensão (tranquilo → dramático) e romance (frio → íntimo): escolha os de clima parecido com o de cada parte.
  - Varie: numa música, misture sequências escritas por você, modos de pista com cores novas e, no máximo em 1 ou 2 partes, modos de pista com as cores originais. Não use o mesmo desenho em partes de papéis diferentes.
`
	if pal := paletteText(p.lib, p.temp); pal != "" {
		rule += "- PALETA DO SHOW (cores testadas nestes canhões; prefira estes valores quando escrever cores, eles saem mais fiéis que outros tons): " + pal + ".\n"
	}
	return rule
}

// askLLM pede ao modelo (ouvindo a faixa) os nomes, o clima e as escolhas.
func askLLM(ctx context.Context, y []float64, a *show.Analysis, cat []Preset, p prompt) (*choices, string, error) {
	var offer []Preset // só o que a validação vai aceitar
	for _, pr := range cat {
		if pr.Fits(p.temp) {
			offer = append(offer, pr)
		}
	}
	secs, presets := describe(a, offer)
	pulseNote := "batida = força da percussão, de 0 a 1"
	if a.Drums {
		pulseNote = `batida = nível da bateria de 0 a 1, medido com a bateria isolada do resto da música; "sem bateria" = ela não toca naquela parte, e a luz deve ser contínua (fades, trocas suaves) mesmo que a energia seja alta`
	}
	freeFormat, score := "", ""
	if p.free && len(usable(p.lib)) > 0 {
		presets += "\nDesenho dos modos de pista (só no modo livre, em \"efeito\": {\"modelo\": ...}):\n" + describeLibrary(p.lib, p.temp)
		score = `
  "clima_pontos": {"energia": 0 a 1, "alegria": 0 a 1, "tensao": 0 a 1, "romance": 0 a 1},`
		freeFormat = `, {"indice": 2, "nome": "verso", "efeito": {"modelo": "pista:nome", "cores": ["#rrggbb", "#rrggbb", "#rrggbb"]}, "motivo": "curto"}`
	}
	if p.free {
		freeFormat = `, {"indice": 1, "nome": "refrão", "efeito": {"tipo": "sequence", "fade": false, "intensidade": 1, "passos": [["#rrggbb", "#rrggbb", "#rrggbb"], ["#rrggbb", "#rrggbb", "#rrggbb"]]}, "motivo": "curto"}` + freeFormat
	}
	text := fmt.Sprintf(`Você é iluminador de um show pequeno: um cantor ao vivo sobre playback, com 3 canhões LED RGB (esquerda, centro, direita).
Ouça a faixa e use a análise abaixo (feita por software, os tempos são exatos — não invente tempos fora da grade).

BPM detectado: %.1f (pode estar no dobro/metade do andamento percebido)
Duração: %.1fs
Partes detectadas (grupo = partes parecidas entre si, ex.: refrões; energia = volume; %s):
%s
Presets disponíveis (use SÓ estes ids):
%s
Regras:
%s%s- Acentos (opcionais, no máximo 1 a cada 20 s): presets da camada "flash" em momentos de impacto (drop, virada, entrada do refrão), duração de 1 a 4 batidas. "FRITAÇÃO | BRANCO MÁXIMO" é o efeito mais agressivo: use no máximo uma vez, só no ápice da música.
- Responda APENAS com JSON neste formato:
{
  "genero": "...", "clima": "...", "descricao": "1-2 frases",
  "bpm_percebido": numero,
  "intensidade": numero de 0 a 1,%s
  "paleta": ["nomes de cores do catálogo"],
  "secoes": [{"indice": 0, "nome": "intro|verso|pré-refrão|refrão|ponte|solo|final|...", "preset": "id da lista", "motivo": "curto"}%s],
  "acentos": [{"tempo": segundos, "preset": "id da lista", "duracao_batidas": 1, "motivo": "curto"}]
}`, a.BPM, a.Duration, pulseNote, secs, presets, partRule(p), tempRule(p.temp), score, freeFormat)

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
	req.Header.Set("X-Title", "DMXController")
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
