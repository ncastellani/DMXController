package web

import (
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/ncastellani/DMXController/internal/analysis"
	"github.com/ncastellani/DMXController/internal/fixture"
	"github.com/ncastellani/DMXController/internal/show"
)

// Prévia e edição de qualquer música da setlist pela aba Músicas, sem
// carregá-la no player do show e sem tocar no DMX. A música é sempre
// identificada pelo X.json da setlist (?file=X.json).

// song acha a música de ?file= ou responde 404.
func (s *Server) song(w http.ResponseWriter, r *http.Request) (show.Song, bool) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "use GET", http.StatusMethodNotAllowed)
		return show.Song{}, false
	}
	so, ok := s.show.Song(filepath.Base(r.URL.Query().Get("file")))
	if !ok {
		http.Error(w, "música não está na setlist", http.StatusNotFound)
	}
	return so, ok
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(v)
}

// serveTimeline devolve a timeline de uma música:
// GET /api/timeline?file=X.json → {"file", "timeline", "edited"}.
func (s *Server) serveTimeline(w http.ResponseWriter, r *http.Request) {
	so, ok := s.song(w, r)
	if !ok {
		return
	}
	t, err := show.LoadTimeline(so.JSON)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	writeJSON(w, songTimelineMsg{Type: "songTimeline", File: so.File, Timeline: t, Edited: exists(so.JSON + ".orig")})
}

type previewMsg struct {
	File      string            `json:"file"`
	FPS       int               `json:"fps"`
	Duration  float64           `json:"duration"`
	Positions []previewPosition `json:"positions"`
}

// previewPosition traz as amostras de um canhão em texto compacto: colors
// tem 6 dígitos hex (rrggbb) por amostra; fx, um dígito por amostra (soma
// de 1 = strobo do canhão e 2 = troca rápida mostrada pela média).
type previewPosition struct {
	Name   string `json:"name"`
	Colors string `json:"colors"`
	FX     string `json:"fx"`
}

// servePreview calcula a luz de uma música sem tocar no DMX:
// GET /api/preview?file=X.json&fps=10 → a cor de cada canhão a cada 1/fps s.
func (s *Server) servePreview(w http.ResponseWriter, r *http.Request) {
	so, ok := s.song(w, r)
	if !ok {
		return
	}
	t, err := show.LoadTimeline(so.JSON)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	prog, err := s.show.Compile(t)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	fps, err := strconv.Atoi(r.URL.Query().Get("fps"))
	if err != nil {
		fps = 10
	}
	var fixtures []*fixture.Fixture
	var names []string
	for _, p := range s.ctl.State().Positions {
		fixtures = append(fixtures, p.Logical)
		names = append(names, p.Name)
	}
	dur := t.Song.Duration
	if dur <= 0 {
		dur = t.Analysis.Duration
	}
	pv := show.RenderPreview(prog, fixtures, dur, fps)
	m := previewMsg{File: so.File, FPS: pv.FPS, Duration: dur, Positions: []previewPosition{}}
	for i, samples := range pv.Positions {
		rgb := make([]byte, 0, 3*len(samples))
		fx := make([]byte, len(samples))
		for j, smp := range samples {
			rgb = append(rgb, smp.Color.R, smp.Color.G, smp.Color.B)
			fx[j] = '0' + smp.FX
		}
		m.Positions = append(m.Positions, previewPosition{Name: names[i], Colors: hex.EncodeToString(rgb), FX: string(fx)})
	}
	writeJSON(w, m)
}

// serveAudio entrega o .wav de uma música para tocar no navegador (a
// prévia; o show toca pela saída de som do programa):
// GET /api/audio?file=X.json (&voz=1: a versão cantada). Aceita Range (pular
// na música).
func (s *Server) serveAudio(w http.ResponseWriter, r *http.Request) {
	so, ok := s.song(w, r)
	if !ok {
		return
	}
	path := so.WAV
	if r.URL.Query().Get("voz") == "1" {
		path = analysis.VoicePath(so.WAV)
	}
	f, err := os.Open(path)
	if err != nil {
		http.Error(w, "áudio não encontrado", http.StatusNotFound)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || st.IsDir() {
		http.Error(w, "áudio não encontrado", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "audio/wav")
	w.Header().Set("Cache-Control", "no-cache") // a faixa pode ser substituída
	http.ServeContent(w, r, filepath.Base(path), st.ModTime(), f)
}
