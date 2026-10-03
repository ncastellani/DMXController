package web

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ncastellani/DMXController/internal/show"
)

// TrashDir é a pasta (dentro da pasta de músicas) para onde vão as músicas
// removidas: cada uma numa subpasta "<nome> (<data>)", com todos os arquivos
// dela. Para recuperar, é só mover os arquivos de volta.
const TrashDir = "lixeira"

// removeSong tira uma música da pasta. name é o X.json de uma música da
// setlist ou o X.wav de uma faixa ainda sem timeline. A timeline (e o
// .orig) e o áudio vão para a lixeira. Devolve
// onde ficaram (relativo à pasta de músicas).
func (s *Server) removeSong(name string) (string, error) {
	dir := s.show.Dir()
	name = filepath.Base(name)
	var trash string
	move := func(title string, files ...string) error {
		trash = filepath.Join(TrashDir, fmt.Sprintf("%s (%s)", title, time.Now().Format("2006-01-02 15h04m05")))
		if err := os.MkdirAll(filepath.Join(dir, trash), 0o755); err != nil {
			return err
		}
		for _, f := range files {
			if f == "" || !exists(f) {
				continue
			}
			if err := os.Rename(f, filepath.Join(dir, trash, filepath.Base(f))); err != nil {
				return fmt.Errorf("movendo %s para a lixeira: %w", filepath.Base(f), err)
			}
		}
		return nil
	}

	if strings.HasSuffix(strings.ToLower(name), ".json") {
		so, ok := s.show.Song(name)
		if !ok {
			return "", fmt.Errorf("%s não está na setlist", name)
		}
		wav := so.WAV
		if wav == "" { // timeline ilegível: o áudio de mesmo nome
			wav = strings.TrimSuffix(so.JSON, filepath.Ext(so.JSON)) + ".wav"
		}
		if err := s.checkNotAnalyzing(filepath.Base(wav)); err != nil {
			return "", err
		}
		base := strings.TrimSuffix(name, filepath.Ext(name))
		err := s.show.Remove(name, func(show.Song) error {
			return move(base, so.JSON, so.JSON+".orig", wav)
		})
		return trash, err
	}

	wavName, err := cleanName(name)
	if err != nil {
		return "", err
	}
	wav := filepath.Join(dir, wavName)
	if !exists(wav) {
		return "", fmt.Errorf("%s não está na pasta de músicas", wavName)
	}
	for _, so := range s.show.Songs() {
		if so.WAV == wav {
			return "", fmt.Errorf("%s é o áudio de %s: remova a música", wavName, so.Title)
		}
	}
	if err := s.checkNotAnalyzing(wavName); err != nil {
		return "", err
	}
	err = move(strings.TrimSuffix(wavName, filepath.Ext(wavName)), wav)
	return trash, err
}

// checkNotAnalyzing recusa mexer numa faixa que está na fila de análise.
func (s *Server) checkNotAnalyzing(wav string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, j := range s.jobs {
		if j.File == wav && !j.Done {
			return errors.New(wav + " está na fila de análise: espere terminar")
		}
	}
	return nil
}
