package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"luzes/internal/analysis"
)

// Importer recebe faixas .wav pela interface e gera a timeline de cada uma.
type Importer struct {
	Dir     string            // pasta da setlist
	Catalog []analysis.Preset // presets disponíveis para o coreógrafo
	APIKey  string            // OpenRouter; vazio = só regras
	Model   string
}

const (
	maxUpload = 2 << 30 // 2 GiB
	keepJobs  = 10
)

type job struct {
	ID      int    `json:"id"`
	File    string `json:"file"`
	AI      bool   `json:"ai"`
	Temp    string `json:"temp"`   // auto | quente | fria
	Lyrics  bool   `json:"lyrics"` // há letra colada para a IA
	Voice   bool   `json:"voice"`  // há versão cantada (X.voz.wav) para a IA
	Stage   string `json:"stage"`
	Done    bool   `json:"done"`
	Error   string `json:"error,omitempty"`
	Warning string `json:"warning,omitempty"`
}

type jobsMsg struct {
	Type  string `json:"type"`
	AI    bool   `json:"ai"` // há chave do OpenRouter
	Items []job  `json:"items"`
}

// SetImporter liga o envio de músicas pela interface.
func (s *Server) SetImporter(imp *Importer) {
	s.importer = imp
	s.jobCh = make(chan *job, 32)
}

func (s *Server) jobsMsg() jobsMsg {
	m := jobsMsg{Type: "jobs", Items: []job{}}
	if s.importer != nil {
		m.AI = s.importer.APIKey != ""
	}
	for _, j := range s.jobs {
		m.Items = append(m.Items, *j)
	}
	return m
}

// sendJobs envia a fila. Requer s.mu.
func (s *Server) sendJobs() {
	msg, _ := json.Marshal(s.jobsMsg())
	s.broadcast(msg)
}

func (s *Server) updateJob(j *job, f func()) {
	s.mu.Lock()
	f()
	s.sendJobs()
	s.mu.Unlock()
}

// importWorker processa a fila, uma música por vez, até ctx acabar.
func (s *Server) importWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case j := <-s.jobCh:
			s.runJob(ctx, j)
		}
	}
}

func (s *Server) runJob(ctx context.Context, j *job) {
	imp := s.importer
	wav := filepath.Join(imp.Dir, j.File)
	key, lyrics, voice := "", "", ""
	if j.AI {
		key = imp.APIKey
		if data, err := os.ReadFile(lyricsPath(wav)); err == nil {
			lyrics = string(data)
		}
		if j.Voice {
			voice = analysis.VoicePath(wav)
		}
	}
	t, warn, err := analysis.Generate(ctx, wav, imp.Catalog, analysis.Options{
		APIKey: key, Model: imp.Model, Temperature: j.Temp, Lyrics: lyrics, Voice: voice,
		Progress: func(stage string) { s.updateJob(j, func() { j.Stage = stage }) },
	})
	jsonPath := strings.TrimSuffix(wav, filepath.Ext(wav)) + ".json"
	if err == nil {
		if err = analysis.Save(t, jsonPath); err == nil {
			os.Remove(jsonPath + ".orig") // edições da versão anterior não valem mais
		}
	}
	s.updateJob(j, func() {
		j.Done, j.Warning = true, warn
		if err != nil {
			j.Error, j.Stage = err.Error(), "falhou"
		} else {
			j.Stage = "pronta"
		}
	})
	if err != nil {
		s.ctl.Notice("♪ análise de %s falhou: %v", j.File, err)
		return
	}
	s.show.Scan()
	// Reanálise da música carregada (ou já emendada): vale na hora.
	if err := s.show.Update(jsonPath, t); err != nil {
		s.ctl.Notice("♪ %s: %v", t.Song.Title, err)
	}
	s.sendSongTimeline(jsonPath, t)
	s.sendSetlist()
	s.ctl.Notice("♪ timeline pronta: %s", t.Song.Title)
}

// lyricsPath é onde fica a letra de X.wav (X.letra.txt).
func lyricsPath(wav string) string {
	return strings.TrimSuffix(wav, filepath.Ext(wav)) + ".letra.txt"
}

// maxLyricsFile é o tamanho máximo da letra guardada (bytes).
const maxLyricsFile = 64 << 10

// analyze põe na fila a análise de uma faixa que já está na pasta (nova ou
// reanálise), guardando antes a letra, se veio. lyrics nil = manter a que
// já existe; "" = apagar. noVoice apaga a versão cantada (X.voz.wav).
func (s *Server) analyze(file string, ai bool, temp string, lyrics *string, noVoice bool) error {
	if s.importer == nil {
		return errors.New("análise de músicas desligada")
	}
	name, err := cleanName(file)
	if err != nil {
		return err
	}
	wav := filepath.Join(s.importer.Dir, name)
	if !exists(wav) {
		return fmt.Errorf("%s não está na pasta de músicas", name)
	}
	if noVoice {
		if err := os.Remove(analysis.VoicePath(wav)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if lyrics != nil {
		text := strings.TrimSpace(strings.ReplaceAll(*lyrics, "\r\n", "\n"))
		if len(text) > maxLyricsFile {
			return errors.New("letra grande demais")
		}
		if text == "" {
			os.Remove(lyricsPath(wav))
		} else if err := os.WriteFile(lyricsPath(wav), []byte(text+"\n"), 0o644); err != nil {
			return err
		}
	}
	s.enqueue(name, ai, temp)
	return nil
}

// serveLyrics devolve a letra guardada de uma faixa:
// GET /api/lyrics?file=<arquivo.wav> (vazio se não houver).
func (s *Server) serveLyrics(w http.ResponseWriter, r *http.Request) {
	if s.importer == nil {
		http.Error(w, "análise de músicas desligada", http.StatusServiceUnavailable)
		return
	}
	name, err := cleanName(r.URL.Query().Get("file"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	data, err := os.ReadFile(lyricsPath(filepath.Join(s.importer.Dir, name)))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(data)
}

// enqueue põe uma faixa na fila de análise.
func (s *Server) enqueue(file string, ai bool, temp string) {
	s.mu.Lock()
	s.nextJob++
	ai = ai && s.importer.APIKey != ""
	j := &job{ID: s.nextJob, File: file, AI: ai, Temp: analysis.NormTemp(temp), Stage: "na fila"}
	wav := filepath.Join(s.importer.Dir, file)
	j.Lyrics = ai && exists(lyricsPath(wav))
	j.Voice = ai && exists(analysis.VoicePath(wav))
	s.jobs = append(s.jobs, j)
	if len(s.jobs) > keepJobs {
		s.jobs = s.jobs[len(s.jobs)-keepJobs:]
	}
	s.sendJobs()
	s.mu.Unlock()
	s.jobCh <- j
}

// serveUpload recebe um .wav no corpo da requisição:
// POST /api/upload?name=<arquivo.wav>&ia=1&temp=quente&replace=1
// Com analisar=0 só guarda o arquivo (a análise vem depois, pela mensagem
// analyze, com a letra). Com voz=1, o arquivo é a versão cantada da faixa
// name (gravada como X.voz.wav, substituindo a anterior, sem analisar).
func (s *Server) serveUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "use POST", http.StatusMethodNotAllowed)
		return
	}
	if s.importer == nil {
		http.Error(w, "envio de músicas desligado", http.StatusServiceUnavailable)
		return
	}
	// Só aceita envios vindos da própria página (outro site na rede não pode).
	if o := r.Header.Get("Origin"); o != "" {
		if u, err := url.Parse(o); err != nil || u.Host != r.Host {
			http.Error(w, "origem não permitida", http.StatusForbidden)
			return
		}
	}
	q := r.URL.Query()
	name, err := cleanName(q.Get("name"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	dir := s.importer.Dir
	wav := filepath.Join(dir, name)
	if q.Get("voz") == "1" {
		if !exists(wav) {
			http.Error(w, fmt.Sprintf("envie primeiro o playback %q", name), http.StatusBadRequest)
			return
		}
		if err := receive(http.MaxBytesReader(w, r.Body, maxUpload), dir, analysis.VoicePath(wav)); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		s.sendSetlist()
		writeAccepted(w, name)
		return
	}
	if strings.HasSuffix(strings.ToLower(name), analysis.VoiceSuffix) {
		http.Error(w, "nomes terminados em .voz.wav são das versões cantadas: renomeie o arquivo", http.StatusBadRequest)
		return
	}
	jsonPath := strings.TrimSuffix(wav, filepath.Ext(wav)) + ".json"
	// Um .wav sem timeline (envio anterior que não chegou a ser analisado)
	// pode ser substituído sem perguntar.
	if q.Get("replace") != "1" && exists(jsonPath) {
		http.Error(w, fmt.Sprintf("%q já existe na setlist", name), http.StatusConflict)
		return
	}
	if err := receive(http.MaxBytesReader(w, r.Body, maxUpload), dir, wav); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if q.Get("analisar") != "0" {
		s.enqueue(name, q.Get("ia") == "1", q.Get("temp"))
	}
	writeAccepted(w, name)
}

func writeAccepted(w http.ResponseWriter, name string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"file": name})
}

// receive grava o corpo num arquivo temporário, confere que é WAV e o
// move para dst.
func receive(body io.Reader, dir, dst string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".upload-*.wav")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := io.Copy(tmp, body); err != nil {
		tmp.Close()
		return fmt.Errorf("envio interrompido: %w", err)
	}
	head := make([]byte, 12)
	if _, err := tmp.ReadAt(head, 0); err != nil || string(head[0:4]) != "RIFF" || string(head[8:12]) != "WAVE" {
		tmp.Close()
		return errors.New("o arquivo não é um WAV (exporte a faixa em .wav)")
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), dst)
}

// cleanName reduz o nome a um arquivo .wav seguro dentro da pasta.
func cleanName(name string) (string, error) {
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	ext := filepath.Ext(name)
	if !strings.EqualFold(ext, ".wav") {
		return "", errors.New("envie um arquivo .wav")
	}
	base := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune(" -_().", r) {
			return r
		}
		return '_'
	}, strings.TrimSuffix(name, ext))
	base = strings.TrimLeft(strings.TrimSpace(base), ".")
	if base == "" {
		return "", errors.New("nome de arquivo inválido")
	}
	if r := []rune(base); len(r) > 120 {
		base = string(r[:120])
	}
	return base + ".wav", nil
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
