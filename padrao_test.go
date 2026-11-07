package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestEnsureConfig: sem config/, o programa cria os quatro arquivos com a
// configuração embutida (iguais aos do repositório) e nunca sobrescreve um
// arquivo que já existe.
func TestEnsureConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	files := configFiles(filepath.Join(cfg, "console.yaml"), filepath.Join(cfg, "fixtures.yaml"), filepath.Join(cfg, "patch.yaml"))
	created, err := ensureConfig(files...)
	if err != nil || len(created) != 4 {
		t.Fatalf("criados %v, erro %v", created, err)
	}
	for _, f := range files {
		got, _ := os.ReadFile(f)
		want, _ := os.ReadFile(filepath.Join("config", filepath.Base(f)))
		if len(got) == 0 || string(got) != string(want) {
			t.Errorf("%s diferente do config/ do repositório", filepath.Base(f))
		}
	}

	edited := []byte("fixtures: []\n")
	os.WriteFile(files[2], edited, 0o644)
	os.Remove(files[0])
	created, err = ensureConfig(files...)
	if err != nil || len(created) != 1 || created[0] != files[0] {
		t.Fatalf("só o console.yaml faltava: criados %v, erro %v", created, err)
	}
	if got, _ := os.ReadFile(files[2]); string(got) != string(edited) {
		t.Error("o patch.yaml editado foi sobrescrito")
	}

	other := filepath.Join(dir, "meu-patch.yaml")
	if created, _ := ensureConfig(other); len(created) != 0 {
		t.Errorf("nome fora do padrão não deveria ser criado: %v", created)
	}
}
