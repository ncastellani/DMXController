package analysis

import (
	"bufio"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// O "ouvido de bateria": uma biblioteca de separação de instrumentos
// (Demucs, em Python) isola a bateria da música, e a análise passa a ouvir
// bumbo, caixa e chimbal limpos, sem voz nem sintetizador no meio. É
// opcional: fica num ambiente Python à parte (pasta .ouvido, instalada por
// scripts/instalar_ouvido.sh); sem ele, a análise segue só com o que há em Go.

//go:embed bateria.py
var bateriaPy []byte

// separatorTimeout é o tempo máximo de uma separação (num Raspberry Pi leva
// alguns minutos por música).
const separatorTimeout = 30 * time.Minute

// Separator separa a bateria de uma música.
type Separator struct {
	Python string // o Python do ambiente com o Demucs
	Cache  string // onde ficam as baterias já separadas (uma por música, pelo conteúdo do arquivo)
}

// FindSeparator devolve o ouvido instalado em dir (".ouvido"), ou nil.
func FindSeparator(dir string) *Separator {
	py := filepath.Join(dir, "venv", "bin", "python")
	if st, err := os.Stat(py); err != nil || st.IsDir() {
		return nil
	}
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	return &Separator{Python: filepath.Join(dir, "venv", "bin", "python"), Cache: filepath.Join(dir, "bateria")}
}

// fileKey identifica o conteúdo de um arquivo.
func fileKey(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil))[:24], nil
}

// Drums devolve o caminho de um WAV só com a bateria da música wav (mono,
// 22,05 kHz). A separação leva de dezenas de segundos a minutos; o resultado
// fica em cache, então analisar a mesma música de novo é imediato. progress
// recebe o andamento.
func (s *Separator) Drums(ctx context.Context, wav string, progress func(string)) (string, error) {
	key, err := fileKey(wav)
	if err != nil {
		return "", err
	}
	out := filepath.Join(s.Cache, key+".wav")
	if st, err := os.Stat(out); err == nil && st.Size() > 44 {
		return out, nil
	}
	if err := os.MkdirAll(s.Cache, 0o755); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, separatorTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, s.Python, "-", wav, out)
	cmd.Stdin = strings.NewReader(string(bateriaPy))
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return "", err
	}
	sc := bufio.NewScanner(stdout)
	for sc.Scan() {
		if pct, ok := strings.CutPrefix(sc.Text(), "progresso "); ok && progress != nil {
			if n, err := strconv.Atoi(strings.TrimSpace(pct)); err == nil {
				progress(fmt.Sprintf("separando a bateria (%d%%)", n))
			}
		}
	}
	if err := cmd.Wait(); err != nil {
		os.Remove(out + ".tmp")
		lines := strings.Split(strings.TrimSpace(stderr.String()), "\n")
		if ctx.Err() != nil {
			return "", errors.New("a separação da bateria demorou demais")
		}
		return "", fmt.Errorf("separação da bateria: %v: %s", err, trunc(lines[len(lines)-1], 200))
	}
	if st, err := os.Stat(out); err != nil || st.Size() <= 44 {
		return "", errors.New("separação da bateria: o arquivo não foi gravado")
	}
	return out, nil
}
