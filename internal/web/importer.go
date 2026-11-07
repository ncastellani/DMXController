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

	"github.com/ncastellani/DMXController/internal/analysis"
	"github.com/ncastellani/DMXController/internal/library"
)

// Importer recebe faixas .wav pela interface e gera a timeline de cada uma.
type Importer struct {
	Dir     string            // pasta da setlist
	Catalog []analysis.Preset // presets disponíveis para o coreógrafo
	Library *library.Library  // modos de pista e cores para o modo livre
	// Separator é o ouvido de bateria (nil = não instalado: a batida é
	// estimada na música inteira).
	Separator *analysis.Separator
	APIKey    string // OpenRouter; vazio = só regras
	Model     string
}

const (
	maxUpload = 2 << 30 // 2 GiB
	keepJobs  = 10
)

type job struct {
	ID      int    `json:"id"`
	File    string `json:"file"`
	AI      bool   `json:"ai"`
	Temp    string `json:"temp"` // auto | quente | fria
	Free    bool   `json:"free"` // modo livre: efeitos criados para a música
	Mix     bool   `json:"mix"`  // modo misto: presets e efeitos combinados
	Stage   string `json:"stage"`
	Done    bool   `json:"done"`
	Error   string `json:"error,omitempty"`
	Warning string `json:"warning,omitempty"`
}

type jobsMsg struct {
	Type  string `json:"type"`
	AI    bool   `json:"ai"`  // há chave do OpenRouter
	Ear   bool   `json:"ear"` // o ouvido de bateria está instalado
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
		m.Ear = s.importer.Separator != nil
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
	key := ""
	if j.AI {
		key = imp.APIKey
	}
	t, warn, err := analysis.Generate(ctx, wav, imp.Catalog, analysis.Options{
		APIKey: key, Model: imp.Model, Temperature: j.Temp, Free: j.Free, Mixed: j.Mix, Library: imp.Library, Separator: imp.Separator,
		Progress: func(stage string) { s.updateJob(j, func() { j.Stage = stage }) },
	})
	jsonPath := strings.TrimSuffix(wav, filepath.Ext(wav)) + ".json"
	if err == nil {
		if err = analysis.Save(t, jsonPath); err == nil {
			os.Remove(jsonPath + ".orig") // edições da versão anterior não valem mais
		}
	}
	if err == nil {
		// Antes de dar a análise por pronta: quem vê "pronta" já encontra a
		// música na setlist. Reanálise da música carregada (ou já emendada)
		// vale na hora.
		s.show.Scan()
		if uerr := s.show.Update(jsonPath, t); uerr != nil {
			s.ctl.Notice("♪ %s: %v", t.Song.Title, uerr)
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
	s.sendSongTimeline(jsonPath, t)
	s.sendSetlist()
	s.ctl.Notice("♪ timeline pronta: %s", t.Song.Title)
}

// analyze põe na fila a análise de uma faixa que já está na pasta (nova ou
// reanálise). free = modo livre; mix = modo misto (presets e efeitos).
func (s *Server) analyze(file string, ai bool, temp string, free, mix bool) error {
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
	s.enqueue(name, ai, temp, free, mix)
	return nil
}

// enqueue põe uma faixa na fila de análise.
func (s *Server) enqueue(file string, ai bool, temp string, free, mix bool) {
	s.mu.Lock()
	s.nextJob++
	ai = ai && s.importer.APIKey != ""
	j := &job{ID: s.nextJob, File: file, AI: ai, Temp: analysis.NormTemp(temp), Free: free || mix, Mix: mix, Stage: "na fila"}
	s.jobs = append(s.jobs, j)
	if len(s.jobs) > keepJobs {
		s.jobs = s.jobs[len(s.jobs)-keepJobs:]
	}
	s.sendJobs()
	s.mu.Unlock()
	s.jobCh <- j
}

// serveUpload recebe um .wav no corpo da requisição:
// POST /api/upload?name=<arquivo.wav>&ia=1&temp=quente&livre=1&replace=1
// Com analisar=0 só guarda o arquivo (a análise vem depois, pela mensagem
// analyze).
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
		s.enqueue(name, q.Get("ia") == "1", q.Get("temp"), q.Get("livre") == "1", q.Get("misto") == "1")
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
	if windowsReserved(base) {
		base = "_" + base
	}
	return base + ".wav", nil
}

// windowsReserved diz se o nome (até o primeiro ponto) é um dos nomes de
// aparelho do Windows, que não servem de nome de arquivo nem com extensão:
// CON, PRN, AUX, NUL, COM1 a COM9 e LPT1 a LPT9. Vale em todo sistema, para
// a pasta de músicas poder ir de um computador para outro.
func windowsReserved(base string) bool {
	stem, _, _ := strings.Cut(base, ".")
	stem = strings.ToUpper(strings.TrimSpace(stem))
	switch stem {
	case "CON", "PRN", "AUX", "NUL":
		return true
	}
	return len(stem) == 4 && (strings.HasPrefix(stem, "COM") || strings.HasPrefix(stem, "LPT")) && stem[3] >= '1' && stem[3] <= '9'
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
