package analysis

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ncastellani/DMXController/internal/audio"
	"github.com/ncastellani/DMXController/internal/library"
	"github.com/ncastellani/DMXController/internal/show"
)

// Options configura a geração de uma timeline.
type Options struct {
	APIKey      string             // vazio: só regras
	Model       string             // modelo do OpenRouter (padrão DefaultModel)
	Temperature string             // TempAuto (ou ""), TempWarm, TempCold
	Free        bool               // modo livre: efeitos criados para a música (a IA pode pôr um preset onde servir melhor)
	Mixed       bool               // modo misto: presets e efeitos combinados, cada um em pelo menos um terço das partes
	Library     *library.Library   // biblioteca: modos de pista e cores para o modo livre (pode ser nil)
	Separator   *Separator         // ouvido de bateria: separa a bateria da música para ouvir a batida nela (pode ser nil)
	Progress    func(stage string) // etapas, para mostrar na interface
}

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
	var warns []string
	var drums []float64
	if opt.Separator != nil { // sem o ouvido, ou se ele falhar, a batida é estimada na música inteira
		progress("separando a bateria")
		path, err := opt.Separator.Drums(ctx, wavPath, progress)
		if err == nil {
			var dt *audio.Track
			if dt, err = audio.LoadWAV(path); err == nil {
				drums = Mono(dt)
			}
		}
		if err != nil {
			warns = append(warns, "sem o ouvido de bateria nesta análise: "+err.Error())
		}
	}
	progress("analisando batidas e partes")
	a := AnalyzeDrums(y, drums)
	temp := opt.temp()
	model := opt.Model
	if model == "" {
		model = DefaultModel
	}
	free := opt.Free || opt.Mixed
	p := prompt{model: model, key: opt.APIKey, temp: temp, free: free, mixed: opt.Mixed, lib: opt.Library}
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
	fallback := ruleChoices(&a, cat, temp)
	presets := slices.Clone(fallback.Sections) // as partes só com presets
	if free {
		var song *Mood // o clima que a IA ouviu orienta também as partes que ficam com as regras
		if llm != nil && llm.Score != nil {
			llm.Score.Clamp()
			song = llm.Score
		}
		ruleEffects(&a, temp, fallback.Sections, opt.Library, song)
	}
	effects := slices.Clone(fallback.Sections) // as partes só com efeitos
	if opt.Mixed {
		mixByEnergy(&a, fallback.Sections, presets)
	}
	secs, accents := validate(llm, &a, cat, fallback, temp, free, opt.Library)
	if opt.Mixed {
		balance(&a, secs, presets, effects)
	}
	title := strings.TrimSuffix(filepath.Base(wavPath), filepath.Ext(wavPath))
	opt.Temperature = temp
	return build(title, filepath.Base(wavPath), a, cat, secs, accents, llm, model, opt), strings.Join(warns, "; "), nil
}

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
