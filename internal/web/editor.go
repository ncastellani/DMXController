package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"

	"github.com/ncastellani/DMXController/internal/analysis"
	"github.com/ncastellani/DMXController/internal/show"
)

// songPath devolve o JSON da música file ("X.json", como na setlist); vazio
// = a música carregada no player.
func (s *Server) songPath(file string) (string, error) {
	if file == "" {
		if path := s.show.Current(); path != "" {
			return path, nil
		}
		return "", errors.New("carregue a música antes de editar")
	}
	so, ok := s.show.Song(filepath.Base(file))
	if !ok {
		return "", fmt.Errorf("música %q não está na setlist", file)
	}
	return so.JSON, nil
}

// saveTimeline grava a timeline editada da música file (vazio = a
// carregada) e, se ela está tocando ou emendada, a aplica na hora. Na
// primeira edição guarda a versão gerada em X.json.orig.
func (s *Server) saveTimeline(file string, raw json.RawMessage) error {
	path, err := s.songPath(file)
	if err != nil {
		return err
	}
	var t show.Timeline
	if err := json.Unmarshal(raw, &t); err != nil {
		return fmt.Errorf("timeline inválida: %v", err)
	}
	cur, err := show.LoadTimeline(path)
	if err != nil {
		return err
	}
	if t.Song.File != "" && t.Song.File != cur.Song.File {
		return fmt.Errorf("a música mudou (agora é %s); a edição não foi salva", cur.Song.Title)
	}
	// Campos que o editor não muda vêm da versão carregada.
	t.Version, t.Song, t.Analysis.Beats, t.Analysis.Downbeats, t.Analysis.Energy = 1, cur.Song, cur.Analysis.Beats, cur.Analysis.Downbeats, cur.Analysis.Energy
	t.Analysis.Hits, t.Analysis.HitStrength, t.Analysis.Accents = cur.Analysis.Hits, cur.Analysis.HitStrength, cur.Analysis.Accents
	t.Analysis.SoundEnd, t.Analysis.FadeOut = cur.Analysis.SoundEnd, cur.Analysis.FadeOut
	t.Analysis.Drums, t.Analysis.HitKind = cur.Analysis.Drums, cur.Analysis.HitKind
	t.Analysis.BPM, t.Analysis.Duration = cur.Analysis.BPM, cur.Analysis.Duration
	if len(t.Analysis.Sections) != len(cur.Analysis.Sections) {
		t.Analysis.Sections = cur.Analysis.Sections
	}
	for i := range t.Analysis.Sections { // só o nome das partes é editável
		name := t.Analysis.Sections[i].Name
		t.Analysis.Sections[i] = cur.Analysis.Sections[i]
		t.Analysis.Sections[i].Name = trunc(name, 40)
	}

	var cat []analysis.Preset
	if s.importer != nil {
		cat = s.importer.Catalog
	}
	dur := t.Song.Duration
	for i := range t.Cues {
		c := &t.Cues[i]
		c.T = math.Max(0, math.Min(dur, math.Round(c.T*1000)/1000))
		c.Note = trunc(c.Note, 200)
		if c.Layer == "base" && c.Kind == "preset" && c.Rate <= 0 {
			c.Rate = analysis.RateFor(cat, c.Preset, t.Analysis.BPM) // preset trocado: reencaixa
		}
		if c.Layer == "flash" {
			c.Duration = math.Max(0.1, math.Min(10, c.Duration))
		}
	}
	show.SortCues(t.Cues)
	if err := s.show.Update(path, &t); err != nil { // compila: preset inválido não chega ao disco
		return err
	}
	first := false
	if orig := path + ".orig"; !exists(orig) {
		if data, err := os.ReadFile(path); err == nil {
			first = os.WriteFile(orig, data, 0o644) == nil
		}
	}
	if err := analysis.Save(&t, path); err != nil {
		return err
	}
	s.sendSongTimeline(path, &t)
	if first { // a setlist mostra quais músicas têm edições
		s.sendSetlist()
	}
	return nil
}

// revertTimeline volta a música file (vazio = a carregada) para a versão
// gerada (X.json.orig).
func (s *Server) revertTimeline(file string) error {
	path, err := s.songPath(file)
	if err != nil {
		return err
	}
	orig := path + ".orig"
	t, err := show.LoadTimeline(orig)
	if err != nil {
		return errors.New("esta música não tem edições para desfazer")
	}
	if err := s.show.Update(path, t); err != nil {
		return err
	}
	if err := os.Rename(orig, path); err != nil {
		return err
	}
	s.sendSongTimeline(path, t)
	s.sendSetlist()
	s.ctl.Notice("♪ %s: timeline restaurada para a versão gerada", t.Song.Title)
	return nil
}

// sendSongTimeline avisa todos os navegadores que a timeline da música path
// mudou (editores abertos nela se atualizam) e, se ela é a carregada,
// manda também a timeline do player.
func (s *Server) sendSongTimeline(path string, t *show.Timeline) {
	msg, _ := json.Marshal(songTimelineMsg{Type: "songTimeline", File: filepath.Base(path), Timeline: t, Edited: exists(path + ".orig")})
	s.mu.Lock()
	s.broadcast(msg)
	s.mu.Unlock()
	if s.show.Current() == path {
		s.sendTimeline()
	}
}

func trunc(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}
