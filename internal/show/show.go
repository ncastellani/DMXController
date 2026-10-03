package show

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ncastellani/DMXController/internal/audio"
	"github.com/ncastellani/DMXController/internal/dmx"
	"github.com/ncastellani/DMXController/internal/engine"
)

// Song é uma música da setlist (um par X.json + X.wav na pasta).
type Song struct {
	Index    int
	Title    string
	File     string // nome do X.json (identifica a música na ordem)
	JSON     string
	WAV      string
	Duration float64
	Genre    string
	Mood     string
	Temp     string        // temperatura das cores pedida na análise
	AI       bool          // a timeline foi gerada com IA
	Sung     int           // frases da letra transcrita (versão cantada)
	Err      string        // problema encontrado ao listar
	Link     bool          // ao terminar, emenda com a próxima
	Gap      time.Duration // intervalo até a próxima, se emendada
}

// Status é o estado do player.
type Status struct {
	Current     int // índice na setlist, -1 se nada carregado
	Playing     bool
	Position    float64
	Duration    float64
	Section     string
	Cue         string
	Next        string
	NextAt      float64
	Audio       bool // false: sem saída de som (luz segue um relógio interno)
	Calibrating bool
}

// CalibrationBPM é o andamento dos cliques de calibração: 1,7 piscadas/s,
// abaixo do limite de 3/s recomendado para fotossensibilidade.
const CalibrationBPM = 100

// OrderFile guarda a ordem da setlist na pasta: um X.json por linha,
// opcionalmente seguido de TAB e "emendar=<segundos>" (emenda com a próxima).
const OrderFile = "setlist.txt"

// MaxGap é o maior intervalo de emenda aceito.
const MaxGap = 30 * time.Second

// Show controla a setlist e a reprodução.
//
// O áudio é um fluxo contínuo (audio.Player): músicas emendadas entram no
// mesmo fluxo, sem corte. A luz segue o trecho do fluxo que está saindo na
// caixa (playhead), então troca de timeline exatamente na emenda; Tick
// atualiza a "música atual" para a interface.
//
// Cuidado com locks: o engine chama a camada do show (source.Render) com o
// lock dele; por isso o Show nunca chama o engine segurando s.mu, e o
// playhead tem lock próprio.
type Show struct {
	dir      string
	presets  *Presets
	eng      *engine.Engine
	head     *playhead
	maxFlash time.Duration
	notice   func(string, ...any)

	mu       sync.Mutex
	songs    []Song
	links    map[string]time.Duration // X.json → intervalo, se emenda com a próxima
	cur      int
	tl       *Timeline
	prog     *Program
	cal      bool // tocando os cliques de calibração
	gen      int  // muda a cada carga/parada: invalida emendas sendo preparadas
	queued   string
	onChange func()
}

// New cria o show. player pode ser nil (sem saída de som).
func New(dir string, presets *Presets, eng *engine.Engine, player *audio.Player, maxFlash time.Duration, notice func(string, ...any)) *Show {
	return &Show{dir: dir, presets: presets, eng: eng, head: &playhead{player: player}, maxFlash: maxFlash, notice: notice, cur: -1, links: map[string]time.Duration{}}
}

// OnChange registra quem avisar quando a música atual muda sozinha (emenda).
func (s *Show) OnChange(f func()) {
	s.mu.Lock()
	s.onChange = f
	s.mu.Unlock()
}

// Dir é a pasta da setlist.
func (s *Show) Dir() string { return s.dir }

// Scan relê a pasta: cada X.json com versão válida vira uma música. A ordem
// e as emendas vêm de setlist.txt; músicas que não estão lá vão para o fim.
func (s *Show) Scan() error {
	files, err := filepath.Glob(filepath.Join(s.dir, "*.json"))
	if err != nil {
		return err
	}
	sort.Strings(files)
	order, links := s.readOrder()
	pos := map[string]int{}
	for i, name := range order {
		pos[name] = i + 1
	}
	sort.SliceStable(files, func(i, j int) bool {
		a, b := pos[filepath.Base(files[i])], pos[filepath.Base(files[j])]
		switch {
		case a == 0:
			return false
		case b == 0:
			return true
		}
		return a < b
	})
	var songs []Song
	for _, f := range files {
		song := Song{Index: len(songs), JSON: f, File: filepath.Base(f), Title: strings.TrimSuffix(filepath.Base(f), ".json")}
		song.Gap, song.Link = links[song.File]
		t, err := LoadTimeline(f)
		if err != nil {
			song.Err = err.Error()
		} else {
			song.Title = t.Song.Title
			song.Duration = t.Song.Duration
			song.WAV = filepath.Join(s.dir, t.Song.File)
			if t.AI != nil {
				song.Genre, song.Mood = t.AI.Genero, t.AI.Clima
			}
			var gen struct {
				Temperature string `json:"temperature"`
				Model       string `json:"model"`
			}
			json.Unmarshal(t.Generated, &gen)
			song.Temp, song.AI, song.Sung = gen.Temperature, gen.Model != "", len(t.Lyrics)
			if _, err := os.Stat(song.WAV); err != nil {
				song.Err = "áudio não encontrado: " + t.Song.File
			}
		}
		songs = append(songs, song)
	}
	s.mu.Lock()
	// A música carregada continua a mesma, mesmo que mude de posição.
	cur := -1
	if s.cur >= 0 && s.cur < len(s.songs) {
		for i, so := range songs {
			if so.JSON == s.songs[s.cur].JSON {
				cur = i
			}
		}
	}
	s.songs, s.cur, s.links = songs, cur, links
	s.mu.Unlock()
	return nil
}

// readOrder lê setlist.txt (linhas vazias e # são ignoradas).
func (s *Show) readOrder() (order []string, links map[string]time.Duration) {
	links = map[string]time.Duration{}
	data, err := os.ReadFile(filepath.Join(s.dir, OrderFile))
	if err != nil {
		return nil, links
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, attrs, _ := strings.Cut(line, "\t")
		name = strings.TrimSpace(name)
		order = append(order, name)
		for _, a := range strings.Fields(attrs) {
			if v, ok := strings.CutPrefix(a, "emendar="); ok {
				if secs, err := strconv.ParseFloat(v, 64); err == nil && secs >= 0 {
					links[name] = min(time.Duration(secs*float64(time.Second)), MaxGap)
				}
			}
		}
	}
	return order, links
}

// writeOrder grava a ordem e as emendas.
func (s *Show) writeOrder(files []string, links map[string]time.Duration) error {
	var b strings.Builder
	b.WriteString("# Ordem da setlist (gravada pela interface). Um arquivo .json por linha;\n")
	b.WriteString("# \"<TAB>emendar=<segundos>\" emenda com a próxima depois desse intervalo.\n")
	for _, f := range files {
		b.WriteString(f)
		if gap, ok := links[f]; ok {
			fmt.Fprintf(&b, "\temendar=%s", strconv.FormatFloat(gap.Seconds(), 'f', -1, 64))
		}
		b.WriteString("\n")
	}
	path := filepath.Join(s.dir, OrderFile)
	if err := os.WriteFile(path+".tmp", []byte(b.String()), 0o644); err != nil {
		return err
	}
	return os.Rename(path+".tmp", path)
}

// Reorder grava a nova ordem (lista de X.json) e relê a setlist.
func (s *Show) Reorder(files []string) error {
	known := map[string]bool{}
	for _, so := range s.Songs() {
		known[so.File] = true
	}
	for _, f := range files {
		if !known[f] {
			return fmt.Errorf("música %q não está na setlist", f)
		}
	}
	s.mu.Lock()
	links := s.links
	s.mu.Unlock()
	if err := s.writeOrder(files, links); err != nil {
		return err
	}
	if err := s.Scan(); err != nil {
		return err
	}
	s.requeue() // a próxima pode ter mudado
	return nil
}

// SetLink liga/desliga a emenda da música file com a próxima, com o
// intervalo gap.
func (s *Show) SetLink(file string, on bool, gap time.Duration) error {
	var files []string
	found := false
	for _, so := range s.Songs() {
		files = append(files, so.File)
		found = found || so.File == file
	}
	if !found {
		return fmt.Errorf("música %q não está na setlist", file)
	}
	s.mu.Lock()
	links := map[string]time.Duration{}
	for k, v := range s.links {
		links[k] = v
	}
	if on {
		links[file] = min(max(0, gap), MaxGap)
	} else {
		delete(links, file)
	}
	s.mu.Unlock()
	if err := s.writeOrder(files, links); err != nil {
		return err
	}
	if err := s.Scan(); err != nil {
		return err
	}
	s.requeue()
	return nil
}

// Songs devolve a setlist.
func (s *Show) Songs() []Song {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Song(nil), s.songs...)
}

// prepare lê e compila uma música (timeline, programa e, com saída de som, o áudio).
func (s *Show) prepare(song Song) (*Timeline, *Program, *audio.Track, error) {
	if song.Err != "" {
		return nil, nil, nil, errors.New(song.Err)
	}
	t, err := LoadTimeline(song.JSON)
	if err != nil {
		return nil, nil, nil, err
	}
	prog, err := Compile(t, s.presets, s.maxFlash)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("%s: %w", song.Title, err)
	}
	var track *audio.Track
	if s.head.player != nil {
		if track, err = audio.LoadWAV(song.WAV); err != nil {
			return nil, nil, nil, err
		}
	}
	return t, prog, track, nil
}

// Load carrega a música i (parada no início, luz apagada) e põe a timeline
// na camada Show.
func (s *Show) Load(i int) error {
	s.mu.Lock()
	if i < 0 || i >= len(s.songs) {
		s.mu.Unlock()
		return fmt.Errorf("música %d não existe", i)
	}
	song := s.songs[i]
	s.mu.Unlock()

	t, prog, track, err := s.prepare(song)
	if err != nil {
		return err
	}
	s.head.load(track, pseg{dur: secs(t.Song.Duration), prog: prog, tl: t, song: song.JSON})

	s.mu.Lock()
	s.cur, s.tl, s.prog, s.cal, s.queued = i, t, prog, false, ""
	s.gen++
	gen := s.gen
	s.mu.Unlock()
	s.eng.Play(engine.Show, song.Title, source{s.head}, time.Now())
	s.notice("♪ carregada: %s", song.Title)
	s.queueNext(gen)
	return nil
}

// queueNext prepara em segundo plano a próxima música, se a atual emenda
// com ela, e a encaixa no fluxo depois do intervalo.
func (s *Show) queueNext(gen int) {
	s.mu.Lock()
	if gen != s.gen || s.cal || s.cur < 0 || s.cur+1 >= len(s.songs) || s.queued != "" {
		s.mu.Unlock()
		return
	}
	cur, next := s.songs[s.cur], s.songs[s.cur+1]
	gap, linked := s.links[cur.File]
	if linked {
		s.queued = next.JSON // reserva: evita preparar duas vezes
	}
	s.mu.Unlock()
	if !linked {
		return
	}
	go func() {
		fail := func(err error) {
			s.notice("emenda com %s falhou: %v", next.Title, err)
			s.mu.Lock()
			if s.gen == gen {
				s.queued = ""
			}
			s.mu.Unlock()
		}
		t, prog, track, err := s.prepare(next)
		if err != nil {
			fail(err)
			return
		}
		s.mu.Lock()
		ok := gen == s.gen && s.cur >= 0 && s.songs[s.cur].JSON == cur.JSON
		s.mu.Unlock()
		if !ok {
			return // a música mudou enquanto preparava
		}
		if err := s.head.append(track, pseg{dur: secs(t.Song.Duration), prog: prog, tl: t, song: next.JSON}, gap); err != nil {
			fail(err)
		}
	}()
}

// requeue refaz a emenda da música atual (ordem ou emenda mudaram), se a
// próxima ainda não começou a tocar.
func (s *Show) requeue() {
	seg, _, ok := s.head.now()
	if !ok || seg.song == "" {
		return
	}
	if !s.head.truncate(seg.start + seg.dur) {
		return // já está tocando a emenda: fica como está
	}
	s.mu.Lock()
	if s.cal || s.cur < 0 {
		s.mu.Unlock()
		return
	}
	s.queued = ""
	s.gen++
	gen := s.gen
	s.mu.Unlock()
	s.queueNext(gen)
}

// Tick percebe quando o fluxo passou para a música emendada e atualiza a
// música atual. Chamar com frequência (ex.: a cada 20 ms).
func (s *Show) Tick() {
	seg, _, ok := s.head.now()
	if !ok || seg.song == "" {
		return
	}
	s.mu.Lock()
	if s.cal || s.cur < 0 || s.cur >= len(s.songs) || s.songs[s.cur].JSON == seg.song {
		s.mu.Unlock()
		return
	}
	idx := -1
	for i, so := range s.songs {
		if so.JSON == seg.song {
			idx = i
		}
	}
	s.cur, s.tl, s.queued = idx, seg.tl, ""
	if p, ok := seg.prog.(*Program); ok {
		s.prog = p
	}
	gen, title, onChange := s.gen, seg.tl.Song.Title, s.onChange
	s.mu.Unlock()

	s.head.release(seg.start - 2*time.Second) // libera a memória das que já passaram
	s.eng.Play(engine.Show, title, source{s.head}, time.Now())
	s.notice("▶ %s (emendada)", title)
	s.queueNext(gen)
	if onChange != nil {
		onChange()
	}
}

// Play começa/continua a música carregada.
func (s *Show) Play() error {
	if !s.loaded() {
		return errors.New("nenhuma música carregada")
	}
	seg, _, _ := s.head.now()
	if !s.head.playing() && s.head.stream() >= s.head.end() {
		s.head.seek(seg.start) // terminou: toca a atual de novo
	}
	s.head.play()
	s.mu.Lock()
	cal, gen := s.cal, s.gen
	title := ""
	if s.tl != nil {
		title = s.tl.Song.Title
	}
	s.mu.Unlock()
	if cal {
		return nil
	}
	s.eng.Play(engine.Show, title, source{s.head}, time.Now())
	s.notice("▶ %s", title)
	s.queueNext(gen) // depois de um Stop, refaz a emenda
	return nil
}

// Calibrate toca cliques e pisca flash (ex.: todos os canhões em branco) em
// cada clique, para ajustar o atraso da saída de som. Para com Stop ou ao
// carregar uma música.
func (s *Show) Calibrate(flash engine.Source) error {
	if s.head.player == nil {
		return errors.New("sem saída de som para calibrar")
	}
	const dur = 5 * time.Minute
	s.head.load(audio.Clicks(CalibrationBPM, dur), pseg{dur: dur, prog: clicks{flash}})
	s.mu.Lock()
	s.cur, s.tl, s.prog, s.cal, s.queued = -1, nil, nil, true, ""
	s.gen++
	s.mu.Unlock()
	s.eng.Play(engine.Show, "calibração", source{s.head}, time.Now())
	s.head.play()
	s.notice("calibração: ajuste o atraso até a piscada cair junto com o clique")
	return nil
}

// clicks pisca flash por 90 ms no início de cada clique.
type clicks struct{ flash engine.Source }

func (c clicks) Render(pos time.Duration, f *dmx.Frame) {
	period := time.Minute / CalibrationBPM
	if pos%period < 90*time.Millisecond {
		c.flash.Render(0, f)
	}
}

// Pause pausa (a luz do show apaga até voltar a tocar).
func (s *Show) Pause() {
	if s.loaded() {
		s.head.pause()
		s.notice("❚❚ pausa")
	}
}

// Stop para, volta ao início da música atual, desfaz a emenda preparada e
// tira a timeline da luz.
func (s *Show) Stop() {
	if !s.loaded() {
		return
	}
	s.head.pause()
	seg, _, _ := s.head.now()
	s.head.truncate(seg.start + seg.dur)
	s.head.seek(seg.start)
	s.eng.Stop(engine.Show)
	s.mu.Lock()
	cal := s.cal
	s.cal, s.queued = false, ""
	s.gen++
	s.mu.Unlock()
	if cal {
		s.notice("calibração encerrada")
		return
	}
	s.notice("■ parada")
}

// Seek vai para t segundos da música atual.
func (s *Show) Seek(t float64) error {
	if !s.loaded() {
		return errors.New("nenhuma música carregada")
	}
	seg, _, _ := s.head.now()
	s.head.seek(seg.start + min(max(0, secs(t)), seg.dur))
	return nil
}

func (s *Show) loaded() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.prog != nil || s.cal
}

// Current devolve o caminho do JSON da música carregada ("" se nenhuma).
func (s *Show) Current() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cur < 0 || s.cur >= len(s.songs) || s.prog == nil {
		return ""
	}
	return s.songs[s.cur].JSON
}

// Song devolve a música da setlist cujo JSON se chama file (ex.: "X.json").
func (s *Show) Song(file string) (Song, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, so := range s.songs {
		if so.File == file {
			return so, true
		}
	}
	return Song{}, false
}

// Compile compila uma timeline com os presets do show (valida sem tocar).
func (s *Show) Compile(t *Timeline) (*Program, error) {
	return Compile(t, s.presets, s.maxFlash)
}

// Update troca a timeline da música path (caminho do JSON) sem parar o
// áudio: se ela está carregada ou já emendada no fluxo, a luz passa a
// seguir a nova na hora. Uma timeline inválida é recusada antes de mexer
// em qualquer coisa. A gravação em disco é de quem chama.
func (s *Show) Update(path string, t *Timeline) error {
	prog, err := s.Compile(t)
	if err != nil {
		return err
	}
	s.head.setProgram(path, prog, t)
	s.mu.Lock()
	current := s.prog != nil && s.cur >= 0 && s.cur < len(s.songs) && s.songs[s.cur].JSON == path
	if current {
		s.tl, s.prog = t, prog
	}
	s.mu.Unlock()
	if current && s.eng.Status().Names[engine.Show] != "" { // só se a luz do show estava no ar
		s.eng.Play(engine.Show, t.Song.Title, source{s.head}, time.Now())
	}
	return nil
}

// Timeline devolve a timeline carregada (nil se nenhuma).
func (s *Show) Timeline() *Timeline {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.tl
}

// Status devolve o estado do player (posição dentro da música atual).
func (s *Show) Status() Status {
	s.mu.Lock()
	cur, cal, loaded := s.cur, s.cal, s.prog != nil || s.cal
	s.mu.Unlock()
	st := Status{Current: cur, Audio: s.head.player != nil, Calibrating: cal}
	seg, pos, ok := s.head.now()
	if !ok || !loaded {
		return st
	}
	st.Playing = s.head.playing()
	st.Position = pos.Seconds()
	st.Duration = seg.dur.Seconds()
	if cal {
		st.Section = "calibração"
		return st
	}
	if seg.tl == nil {
		return st
	}
	for _, sec := range seg.tl.Analysis.Sections {
		if st.Position >= sec.Start && st.Position < sec.End {
			st.Section = sec.Name
		}
	}
	if prog, ok := seg.prog.(*Program); ok {
		c, n := prog.Current(pos)
		if c != nil {
			st.Cue = cueLabel(c)
		}
		if n != nil {
			st.Next, st.NextAt = cueLabel(n), n.T
		}
	}
	return st
}

func cueLabel(c *Cue) string {
	if c.Kind == "blackout" {
		return "blackout"
	}
	return c.Name
}

// renderer é o que um trecho do fluxo desenha (Program ou cliques).
type renderer interface {
	Render(pos time.Duration, f *dmx.Frame)
}

// source é a camada Show: ignora o elapsed do engine e desenha a luz do
// trecho do fluxo que está saindo na caixa agora. Só acende com a música
// tocando: carregada, parada ou pausada, a luz fica apagada (o padrão é
// tudo preto quando nada está tocando).
type source struct{ head *playhead }

func (src source) Render(_ time.Duration, f *dmx.Frame) {
	if !src.head.playing() {
		return
	}
	if seg, pos, ok := src.head.now(); ok && seg.prog != nil {
		seg.prog.Render(pos, f)
	}
}

// pseg é uma música (ou a calibração) no fluxo.
type pseg struct {
	start, dur time.Duration // no fluxo
	prog       renderer
	tl         *Timeline
	song       string // caminho do JSON ("" = calibração)
}

// playhead é a posição no fluxo — a do player de áudio ou, sem saída de
// som, um relógio interno — e quais músicas estão em que trecho.
type playhead struct {
	player *audio.Player

	mu      sync.Mutex
	segs    []pseg
	running bool
	base    time.Duration
	since   time.Time
}

func (h *playhead) load(t *audio.Track, seg pseg) {
	if h.player != nil {
		h.player.Load(t)
	}
	h.mu.Lock()
	seg.start = 0
	h.segs, h.running, h.base = []pseg{seg}, false, 0
	h.mu.Unlock()
}

func (h *playhead) append(t *audio.Track, seg pseg, gap time.Duration) error {
	if h.player != nil {
		start, err := h.player.Append(t, gap)
		if err != nil {
			return err
		}
		seg.start = start
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.segs) == 0 {
		return errors.New("nenhuma música carregada")
	}
	if h.player == nil {
		seg.start = h.endLocked() + max(0, gap)
	}
	h.segs = append(h.segs, seg)
	return nil
}

// truncate tira as músicas que começam em at ou depois (se ainda não
// começaram a ser lidas). Sem nada para tirar, devolve true.
func (h *playhead) truncate(at time.Duration) bool {
	h.mu.Lock()
	queued := len(h.segs) > 0 && h.segs[len(h.segs)-1].start >= at
	h.mu.Unlock()
	if !queued {
		return true
	}
	if h.player != nil && !h.player.Truncate(at) {
		return false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.player == nil && h.clock() >= at {
		return false
	}
	i := len(h.segs)
	for i > 1 && h.segs[i-1].start >= at {
		i--
	}
	h.segs = h.segs[:i]
	return true
}

// release esquece as músicas que terminaram antes de before.
func (h *playhead) release(before time.Duration) {
	if h.player != nil {
		h.player.Release(before)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for len(h.segs) > 1 && h.segs[0].start+h.segs[0].dur < before {
		h.segs = h.segs[1:]
	}
}

// setProgram troca o programa (edição) da música song no fluxo.
func (h *playhead) setProgram(song string, prog *Program, tl *Timeline) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for i := range h.segs {
		if h.segs[i].song == song {
			h.segs[i].prog, h.segs[i].tl = prog, tl
		}
	}
}

// now devolve a música que está saindo agora e a posição dentro dela.
func (h *playhead) now() (pseg, time.Duration, bool) {
	stream := h.stream()
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.segs) == 0 {
		return pseg{}, 0, false
	}
	seg := h.segs[0]
	for _, sg := range h.segs {
		if sg.start <= stream {
			seg = sg
		}
	}
	return seg, min(max(0, stream-seg.start), seg.dur), true
}

func (h *playhead) stream() time.Duration {
	if h.player != nil {
		return h.player.Position()
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.clock()
}

func (h *playhead) end() time.Duration {
	if h.player != nil {
		return h.player.End()
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.endLocked()
}

func (h *playhead) endLocked() time.Duration {
	if len(h.segs) == 0 {
		return 0
	}
	last := h.segs[len(h.segs)-1]
	return last.start + last.dur
}

func (h *playhead) play() {
	if h.player != nil {
		h.player.Play()
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.running && h.base < h.endLocked() {
		h.running, h.since = true, time.Now()
	}
}

func (h *playhead) pause() {
	if h.player != nil {
		h.player.Pause()
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.running {
		h.base = h.clock()
		h.running = false
	}
}

func (h *playhead) seek(d time.Duration) {
	if h.player != nil {
		h.player.Seek(d)
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.base, h.since = min(max(0, d), h.endLocked()), time.Now()
}

// clock calcula a posição do relógio interno. Requer h.mu.
func (h *playhead) clock() time.Duration {
	if !h.running {
		return h.base
	}
	p := h.base + time.Since(h.since)
	if end := h.endLocked(); p >= end {
		h.running, h.base = false, end
		return end
	}
	return p
}

func (h *playhead) playing() bool {
	if h.player != nil {
		return h.player.Playing()
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clock()
	return h.running
}
