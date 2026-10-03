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
	free := flag.Bool("livre", false, "modo livre: cria efeitos para a música (misturados aos presets) em vez de usar só os presets")
	earDir := flag.String("ouvido", ".ouvido", "pasta do ouvido de bateria (instale com scripts/instalar_ouvido.sh)")
	noEar := flag.Bool("sem-ouvido", false, "não separa a bateria (a batida é estimada na música inteira)")
	mixed := flag.Bool("misto", false, "modo misto: combina presets e efeitos na mesma música (cada um em pelo menos um terço das partes)")
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
	rig, err := control.NewRig(fixtures, *patch)
	if err != nil {
		return err
	}
	console, err := control.LoadConsole(*consolePath, control.Positions(rig))
	if err != nil {
		return err
	}
	cat := analysis.Catalog(console)

	var sep *analysis.Separator
	if !*noEar {
		if sep = analysis.FindSeparator(*earDir); sep == nil {
			log.Print("aviso: sem o ouvido de bateria (scripts/instalar_ouvido.sh); a batida será estimada na música inteira")
		}
	}
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
			files = append(files, m...)
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
		t, warn, err := analysis.Generate(context.Background(), wav, cat, analysis.Options{
			APIKey: key, Model: *model, Temperature: *temp, Free: *free, Mixed: *mixed, Library: console.Library, Separator: sep,
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
