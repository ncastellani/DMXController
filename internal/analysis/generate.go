package analysis

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ncastellani/DMXController/internal/audio"
	"github.com/ncastellani/DMXController/internal/show"
)

// Options configura a geração de uma timeline.
type Options struct {
	APIKey      string             // vazio: só regras
	Model       string             // modelo do OpenRouter (padrão DefaultModel)
	Temperature string             // TempAuto (ou ""), TempWarm, TempCold
	Lyrics      string             // letra colada (só a IA usa; com a versão cantada, ajuda a transcrever)
	Voice       string             // versão cantada (X.voz.wav); vazio = sem
	Progress    func(stage string) // etapas, para mostrar na interface
}

// MaxLyrics é o tamanho máximo da letra (em caracteres) mandada à IA.
const MaxLyrics = 8000

// NormTemp devolve a temperatura válida mais próxima de t (TempAuto se não
// reconhecer).
func NormTemp(t string) string {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case TempWarm, "quentes":
		return TempWarm
	case TempCold, "frias":
		return TempCold
	}
	return TempAuto
}

func (o Options) temp() string { return NormTemp(o.Temperature) }

// Generate analisa wavPath e devolve a timeline. Se o LLM falhar, a
// timeline sai só com as regras e warn explica o motivo.
func Generate(ctx context.Context, wavPath string, cat []Preset, opt Options) (t *show.Timeline, warn string, err error) {
	progress := opt.Progress
	if progress == nil {
		progress = func(string) {}
	}
	progress("lendo o áudio")
	track, err := audio.LoadWAV(wavPath)
	if err != nil {
		return nil, "", err
	}
	y := Mono(track)
	progress("analisando batidas e partes")
	a := Analyze(y)
	temp := opt.temp()
	lyrics := trunc(strings.TrimSpace(opt.Lyrics), MaxLyrics)
	fallback := ruleChoices(&a, cat, temp)
	model := opt.Model
	if model == "" {
		model = DefaultModel
	}
	p := prompt{model: model, key: opt.APIKey, temp: temp, lyrics: lyrics}
	var warns []string
	extra := map[string]any{}

	// Versão cantada: a voz isolada diz quando cada frase é cantada, e a IA
	// transcreve a letra.
	if opt.Voice != "" {
		progress("alinhando a versão cantada")
		v, err := loadVoice(track, y, opt.Voice)
		switch {
		case err != nil:
			warns = append(warns, "versão cantada ignorada: "+err.Error())
		case len(v.Phrases) == 0:
			warns = append(warns, "não achei voz na versão cantada (ela é igual ao playback?)")
		case opt.APIKey == "":
			warns = append(warns, "a letra da versão cantada precisa da IA")
		default:
			extra["voice"] = map[string]any{"shift": round(v.Shift, 3), "clean": v.Clean, "phrases": len(v.Phrases)}
			if v.Note != "" {
				warns = append(warns, v.Note)
			}
			progress(fmt.Sprintf("a IA está transcrevendo a letra (%d frases)", len(v.Phrases)))
			s, usage, err := transcribe(ctx, v, lyrics, p)
			if err != nil {
				warns = append(warns, "não deu para transcrever a letra: "+err.Error())
				break
			}
			p.sung = s
			progress(fmt.Sprintf("letra transcrita: %d frases, %d cores citadas (%s)", len(s.Lines), len(s.Colors), usage))
		}
	}

	var llm *choices
	if opt.APIKey != "" {
		progress("a IA está ouvindo a música")
		c, usage, err := askLLM(ctx, y, &a, cat, p)
		if err != nil {
			warns = append(warns, "IA indisponível, usei só as regras: "+err.Error())
		} else {
			llm = c
			progress("IA respondeu (" + usage + ")")
		}
	}
	if llm == nil {
		model = ""
	}
	secs, accents := validate(llm, &a, cat, fallback, temp)
	colors := validateColors(llm, &a, cat, p.sung)
	title := strings.TrimSuffix(filepath.Base(wavPath), filepath.Ext(wavPath))
	opt.Temperature, opt.Lyrics = temp, lyrics
	return build(title, filepath.Base(wavPath), a, cat, secs, accents, colors, p.sung, llm, model, opt, extra), strings.Join(warns, "; "), nil
}

// loadVoice lê a versão cantada e isola a voz no tempo do playback (y é
// ele em Mono).
func loadVoice(play *audio.Track, y []float64, path string) (*voice, error) {
	sung, err := audio.LoadWAV(path)
	if err != nil {
		return nil, err
	}
	return isolateVoice(play, sung, y)
}

// VoicePath é onde fica a versão cantada de X.wav (X.voz.wav).
func VoicePath(wav string) string {
	return strings.TrimSuffix(wav, filepath.Ext(wav)) + VoiceSuffix
}

// VoiceSuffix termina o nome da versão cantada.
const VoiceSuffix = ".voz.wav"

// Save grava a timeline em path (UTF-8 legível, gravação atômica).
func Save(t *show.Timeline, path string) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", " ")
	if err := enc.Encode(t); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("gravando %s: %w", path, err)
	}
	return os.Rename(tmp, path)
}
