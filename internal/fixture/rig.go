package fixture

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/ncastellani/DMXController/internal/dmx"
)

// Rig liga as posições do palco (ordem do patch: esquerda → direita) aos
// canhões físicos.
//
// Os presets são escritos num quadro "lógico" em que cada posição ocupa um
// bloco fixo de canais (o layout do workspace do QLC+). Na saída, cada bloco
// é copiado para o endereço DMX do canhão que está naquela posição. Trocar
// canhões de posição é só trocar os endereços.
type Rig struct {
	Fixtures []*Fixture // físicos, na ordem das posições
	logical  []int      // endereço lógico de cada posição
	path     string     // patch.yaml
}

// NewRig cria o rig. logical[i] é o endereço do bloco da posição i no quadro
// lógico. patchPath é onde Save grava as posições.
func NewRig(fixtures []*Fixture, logical []int, patchPath string) (*Rig, error) {
	if len(fixtures) != len(logical) {
		return nil, fmt.Errorf("patch tem %d canhões, mas os presets usam %d", len(fixtures), len(logical))
	}
	return &Rig{Fixtures: fixtures, logical: logical, path: patchPath}, nil
}

// Logical devolve o fixture da posição i com o endereço lógico, para ler e
// escrever no quadro lógico (prévia, controle manual, identificar).
func (r *Rig) Logical(i int) *Fixture {
	f := *r.Fixtures[i]
	f.Address = r.logical[i]
	return &f
}

// Moves devolve o mapeamento do quadro lógico para o físico.
func (r *Rig) Moves() []dmx.Move {
	moves := make([]dmx.Move, len(r.Fixtures))
	for i, f := range r.Fixtures {
		moves[i] = dmx.Move{From: r.logical[i], To: f.Address, Len: len(f.Profile.Channels)}
	}
	return moves
}

// Swap troca de lugar os canhões das posições i e j (endereço e perfil vão
// juntos; o nome da posição fica).
func (r *Rig) Swap(i, j int) error {
	if i < 0 || j < 0 || i >= len(r.Fixtures) || j >= len(r.Fixtures) || i == j {
		return fmt.Errorf("posições inválidas %d e %d", i, j)
	}
	a, b := r.Fixtures[i], r.Fixtures[j]
	if len(a.Profile.Channels) != len(b.Profile.Channels) {
		return fmt.Errorf("%q e %q têm perfis com quantidades de canais diferentes", a.Name, b.Name)
	}
	a.Address, b.Address = b.Address, a.Address
	a.ProfileID, b.ProfileID = b.ProfileID, a.ProfileID
	a.Profile, b.Profile = b.Profile, a.Profile
	return nil
}

// Save grava o patch atual (escrita atômica).
func (r *Rig) Save() error {
	type entry struct {
		Name    string `yaml:"name"`
		Profile string `yaml:"profile"`
		Address int    `yaml:"address"`
	}
	var pf struct {
		Fixtures []entry `yaml:"fixtures"`
	}
	for _, f := range r.Fixtures {
		pf.Fixtures = append(pf.Fixtures, entry{f.Name, f.ProfileID, f.Address})
	}
	data, err := yaml.Marshal(&pf)
	if err != nil {
		return err
	}
	header := "# Endereço inicial de cada canhão, na ordem das posições (esquerda → direita).\n" +
		"# Tem que bater com o display do aparelho (menu \"dXXX\").\n" +
		"# Também é alterado pela interface web (trocar posições).\n"
	tmp, err := os.CreateTemp(filepath.Dir(r.path), ".patch-*.yaml")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(append([]byte(header), data...)); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), r.path)
}

// Path devolve o caminho do patch.yaml.
func (r *Rig) Path() string { return r.path }
