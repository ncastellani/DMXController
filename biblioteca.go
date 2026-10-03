package main

// "DMXController biblioteca" pede à IA a pontuação de clima (energia, alegria,
// tensão, romance) dos modos de pista de config/biblioteca.yaml que ainda não
// têm, pelas cores e pelo movimento de cada um, e grava no arquivo. É o que a
// análise no modo livre usa para escolher o modo de pista de cada parte. Rode
// depois de criar ou mudar um modo de pista no arquivo.
//
//	DMXController biblioteca              avalia os que ainda não têm clima
//	DMXController biblioteca -reavaliar   refaz o clima de todos

import (
	"context"
	"errors"
	"flag"
	"fmt"

	"github.com/ncastellani/DMXController/internal/analysis"
	"github.com/ncastellani/DMXController/internal/library"
)

func biblioteca(args []string) error {
	flag := flag.NewFlagSet("biblioteca", flag.ExitOnError)
	path := flag.String("arquivo", "config/biblioteca.yaml", "arquivo da biblioteca")
	redo := flag.Bool("reavaliar", false, "refaz a pontuação de clima de todos os modos de pista")
	model := flag.String("modelo", analysis.DefaultModel, "modelo do OpenRouter")
	flag.Parse(args)

	lib, err := library.Load(*path)
	if err != nil {
		return err
	}
	fmt.Printf("%d cores, %d strobos, %d modos de pista\n", len(lib.Colors), len(lib.Strobes), len(lib.Effects))
	for i := range lib.Effects {
		if e := &lib.Effects[i]; !analysis.Usable(e) {
			fmt.Printf("  %s: só como preset (pisca-pisca ou quase todo apagado: não entra no modo livre)\n", e.ID)
		}
	}
	key := analysis.APIKey(".env")
	if key == "" {
		return errors.New("OPENROUTER_API_KEY não encontrada (.env ou ambiente)")
	}
	fmt.Println("a IA está avaliando o clima dos modos de pista")
	usage, n, err := analysis.ScoreMoods(context.Background(), lib, key, *model, *redo)
	if err != nil {
		return err
	}
	if n == 0 {
		fmt.Println("  todos já tinham clima (use -reavaliar para refazer)")
		return nil
	}
	fmt.Printf("  %d avaliados (%s)\n", n, usage)
	if err := lib.Save(*path); err != nil {
		return err
	}
	fmt.Println("→ " + *path)
	return nil
}
