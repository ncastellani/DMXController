package main

// "DMXController analisar" gera a timeline de luz (X.json) de cada faixa X.wav: BPM,
// batidas, partes e escolha de presets, com IA opcional (OpenRouter).
// A aba Músicas faz o mesmo, uma música por vez; este subcomando é para
// processar uma pasta inteira de uma vez.
//
//	DMXController analisar musicas/            todas as .wav da pasta
//	DMXController analisar musicas/x.wav       uma faixa
//
// Um JSON existente não é sobrescrito (pode ter edições), a não ser com -forcar.
// Se existir X.voz.wav (a versão cantada) ao lado de X.wav, a IA transcreve a
// letra dela e a cor citada acende na palavra; X.letra.txt (letra colada)
// ajuda a transcrever ou, sem a versão cantada, é casada com as partes.

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/ncastellani/DMXController/internal/analysis"
	"github.com/ncastellani/DMXController/internal/control"
	"github.com/ncastellani/DMXController/internal/fixture"
)

func analisar(args []string) error {
	flag := flag.NewFlagSet("analisar", flag.ExitOnError)
	consolePath := flag.String("console", "config/console.yaml", "botões do show")
	profiles := flag.String("profiles", "config/fixtures.yaml", "perfis de fixtures")
	patch := flag.String("patch", "config/patch.yaml", "posições dos canhões")
	noAI := flag.Bool("sem-ia", false, "não consulta o LLM (só regras)")
	model := flag.String("modelo", analysis.DefaultModel, "modelo do OpenRouter")
	force := flag.Bool("forcar", false, "sobrescreve JSONs existentes")
	temp := flag.String("temperatura", analysis.TempAuto, "cores: auto | quente | fria")
	flag.Parse(args)
	if flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "uso: DMXController analisar [opções] <pasta ou arquivo.wav>...")
		flag.PrintDefaults()
		os.Exit(2)
	}

	fixtures, err := fixture.Load(*profiles, *patch)
	if err != nil {
		return err
	}
	console, err := control.LoadConsole(*consolePath)
	if err != nil {
		return err
	}
	rig, err := control.NewRig(fixtures, console, *patch)
	if err != nil {
		return err
	}
	var positions []*fixture.Fixture
	for i := range rig.Fixtures {
		positions = append(positions, rig.Logical(i))
	}
	cat := analysis.Catalog(console, positions)

	key := ""
	if !*noAI {
		if key = analysis.APIKey(".env"); key == "" {
			log.Print("aviso: OPENROUTER_API_KEY não encontrada (.env ou ambiente); usando só as regras")
		}
	}

	var files []string
	for _, arg := range flag.Args() {
		if st, err := os.Stat(arg); err == nil && st.IsDir() {
			m, _ := filepath.Glob(filepath.Join(arg, "*.wav"))
			for _, f := range m {
				if !strings.HasSuffix(f, analysis.VoiceSuffix) {
					files = append(files, f)
				}
			}
		} else {
			files = append(files, arg)
		}
	}
	for _, wav := range files {
		out := strings.TrimSuffix(wav, filepath.Ext(wav)) + ".json"
		if _, err := os.Stat(out); err == nil && !*force {
			fmt.Printf("%s: %s já existe (use -forcar para refazer)\n", filepath.Base(wav), filepath.Base(out))
			continue
		}
		fmt.Printf("%s:\n", filepath.Base(wav))
		lyrics, _ := os.ReadFile(strings.TrimSuffix(wav, filepath.Ext(wav)) + ".letra.txt")
		if len(lyrics) > 0 && key != "" {
			fmt.Println("  usando a letra colada")
		}
		voice := analysis.VoicePath(wav)
		if _, err := os.Stat(voice); err != nil {
			voice = ""
		} else {
			fmt.Println("  usando a versão cantada")
		}
		t, warn, err := analysis.Generate(context.Background(), wav, cat, analysis.Options{
			APIKey: key, Model: *model, Temperature: *temp, Lyrics: string(lyrics), Voice: voice,
			Progress: func(s string) { fmt.Println("  " + s) },
		})
		if err != nil {
			log.Printf("  erro: %v", err)
			continue
		}
		if warn != "" {
			fmt.Println("  aviso: " + warn)
		}
		if err := analysis.Save(t, out); err != nil {
			log.Printf("  erro: %v", err)
			continue
		}
		fmt.Printf("  %.1f BPM, %d batidas, %d partes → %s\n", t.Analysis.BPM, len(t.Analysis.Beats), len(t.Analysis.Sections), out)
	}
	return nil
}
