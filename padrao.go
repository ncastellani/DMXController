package main

import (
	"embed"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// A configuração do repositório vai embutida no binário: assim o programa
// roda só com o executável (ex.: o .exe copiado sozinho para um Windows). Os
// arquivos de config/ continuam sendo a configuração de verdade; os
// embutidos só preenchem os que faltam.
//
//go:embed config/console.yaml config/fixtures.yaml config/patch.yaml config/biblioteca.yaml
var defaultConfig embed.FS

// ensureConfig grava a configuração padrão nos arquivos que faltam, entre os
// de nome conhecido (console.yaml, fixtures.yaml, patch.yaml,
// biblioteca.yaml). Nunca mexe num arquivo que existe. Devolve os criados.
func ensureConfig(paths ...string) ([]string, error) {
	var created []string
	for _, p := range paths {
		data, err := defaultConfig.ReadFile("config/" + filepath.Base(p))
		if err != nil {
			continue // não é um dos arquivos padrão
		}
		if _, err := os.Stat(p); !errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return created, err
		}
		if err := os.WriteFile(p, data, 0o644); err != nil {
			return created, err
		}
		created = append(created, p)
	}
	return created, nil
}

// configFiles são os arquivos de configuração usados com essas opções (a
// biblioteca fica ao lado do console.yaml, como no padrão).
func configFiles(console, profiles, patch string) []string {
	return []string{console, profiles, patch, filepath.Join(filepath.Dir(console), "biblioteca.yaml")}
}
