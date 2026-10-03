// Package web serve a interface do console (HTML + WebSocket): prévia dos
// canhões em tempo real, botões, posições e controle manual de canais.
package web

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/ncastellani/DMXController/internal/audio"
	"github.com/ncastellani/DMXController/internal/control"
	"github.com/ncastellani/DMXController/internal/engine"
	"github.com/ncastellani/DMXController/internal/show"
	"github.com/ncastellani/DMXController/internal/sound"
)

//go:embed static
var static embed.FS

// HoldTTL é quanto tempo um flash segurado pela web sobrevive sem ping do
// navegador (protege contra iPad que dorme ou Wi-Fi que cai).
const HoldTTL = time.Second

const maxEvents = 50

// Server é o servidor web do console.
type Server struct {
	ctl    *control.Controller
	eng    *engine.Engine
	show   *show.Show
	player *audio.Player // nil sem saída de som
	sound  *sound.Manager
	health *Health
	token  string

	effectsOnce sync.Once
	effects     []effectMsg // os modos de pista como efeito (config), calculados uma vez

	mu          sync.Mutex
	clients     map[*client]bool
	events      []eventMsg
	state       []byte
	playerState []byte
	nextID      int

	importer *Importer // nil: envio de músicas desligado
	jobCh    chan *job
	jobs     []*job
	nextJob  int
}

type client struct {
	id   string
	send chan []byte
}

// New cria o servidor. player pode ser nil (sem saída de som). Se token não
// for vazio, o acesso exige ?t=<token> na primeira visita (depois fica num
// cookie).
func New(ctl *control.Controller, eng *engine.Engine, sh *show.Show, player *audio.Player, snd *sound.Manager, health *Health, token string) *Server {
	s := &Server{ctl: ctl, eng: eng, show: sh, player: player, sound: snd, health: health, token: token, clients: map[*client]bool{}}
	ctl.OnEvent(s.event)
	sh.OnChange(s.showChanged)
	if snd != nil {
		snd.OnChange(s.sendSound)
	}
	return s
}

// Handler devolve o http.Handler do console.
func (s *Server) Handler() http.Handler {
	sub, _ := fs.Sub(static, "static")
	mux := http.NewServeMux()
	mux.Handle("/", http.FileServerFS(sub))
	mux.HandleFunc("/ws", s.serveWS)
	mux.HandleFunc("/api/upload", s.serveUpload)
	mux.HandleFunc("/api/timeline", s.serveTimeline)
	mux.HandleFunc("/api/preview", s.servePreview)
	mux.HandleFunc("/api/audio", s.serveAudio)
	if s.token == "" {
		return mux
	}
	return s.auth(mux)
}

func (s *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("t") == s.token {
			http.SetCookie(w, &http.Cookie{Name: "dmxcontroller", Value: s.token, Path: "/", MaxAge: 30 * 24 * 3600, HttpOnly: true, SameSite: http.SameSiteStrictMode})
			next.ServeHTTP(w, r)
			return
		}
		if c, err := r.Cookie("dmxcontroller"); err == nil && c.Value == s.token {
			next.ServeHTTP(w, r)
			return
		}
		http.Error(w, "acesso negado: abra o link com ?t=<token> mostrado no terminal", http.StatusUnauthorized)
	})
}

// Run envia o estado aos navegadores sempre que ele muda, até ctx acabar:
// luz a ~30 Hz, player a ~10 Hz.
func (s *Server) Run(ctx context.Context) {
	if s.importer != nil {
		go s.importWorker(ctx)
	}
	ticker := time.NewTicker(33 * time.Millisecond)
	defer ticker.Stop()
	for i := 0; ; i++ {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		if i%3 == 0 {
			s.sendPlayer(false)
			s.show.ExpireLive("", time.Now()) // navegador sumiu sem fechar a conexão
		}
		msg, err := json.Marshal(s.snapshot())
		if err != nil {
			log.Printf("web: %v", err)
			continue
		}
		s.mu.Lock()
		if !bytes.Equal(msg, s.state) {
			s.state = msg
			s.broadcast(msg)
		}
		s.mu.Unlock()
	}
}

// broadcast envia msg a todos; clientes lentos perdem a mensagem. Requer s.mu.
func (s *Server) broadcast(msg []byte) {
	for c := range s.clients {
		select {
		case c.send <- msg:
		default:
		}
	}
}

func (s *Server) event(text string) {
	ev := eventMsg{Type: "event", Time: time.Now().Format("15:04:05"), Text: text}
	msg, _ := json.Marshal(ev)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, ev)
	if len(s.events) > maxEvents {
		s.events = s.events[len(s.events)-maxEvents:]
	}
	s.broadcast(msg)
}

func (s *Server) serveWS(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer conn.CloseNow()
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	s.mu.Lock()
	s.nextID++
	c := &client{id: fmt.Sprintf("web%d", s.nextID), send: make(chan []byte, 64)}
	cfg, _ := json.Marshal(s.config())
	hist, _ := json.Marshal(eventsMsg{Type: "events", Items: append([]eventMsg{}, s.events...)})
	setlist, _ := json.Marshal(s.setlist())
	tl, _ := json.Marshal(s.timelineMsg())
	hello, _ := json.Marshal(helloMsg{Type: "hello", Client: c.id})
	c.send <- hello
	c.send <- cfg
	c.send <- hist
	c.send <- setlist
	c.send <- tl
	if snd, err := json.Marshal(s.soundMsg()); err == nil {
		c.send <- snd
	}
	if jobs, err := json.Marshal(s.jobsMsg()); err == nil {
		c.send <- jobs
	}
	for _, m := range [][]byte{s.state, s.playerState} {
		if m != nil {
			c.send <- m
		}
	}
	s.clients[c] = true
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.clients, c)
		s.mu.Unlock()
		s.ctl.FlashUp(c.id, nil, time.Now())
		s.show.ExpireLive(c.id, time.Now()) // a prévia que ele levava aos canhões sai
	}()

	go func() {
		defer cancel()
		for {
			select {
			case <-ctx.Done():
				return
			case msg := <-c.send:
				wctx, wcancel := context.WithTimeout(ctx, 5*time.Second)
				err := conn.Write(wctx, websocket.MessageText, msg)
				wcancel()
				if err != nil {
					return
				}
			}
		}
	}()

	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return
		}
		var m inMsg
		if err := json.Unmarshal(data, &m); err != nil {
			continue
		}
		if err := s.handle(c, &m, time.Now()); err != nil {
			s.ctl.Notice("web: %v", err)
		}
	}
}

// inMsg é uma mensagem do navegador.
type inMsg struct {
	Type     string          `json:"type"`
	ID       int             `json:"id"`
	T        float64         `json:"t"`
	Down     bool            `json:"down"`
	Pos      int             `json:"pos"`
	Ch       int             `json:"ch"`
	Value    int             `json:"value"`
	A        int             `json:"a"`
	B        int             `json:"b"`
	Arg      string          `json:"arg"`      // ação de som: output, scan, pair, connect, disconnect, forget
	Str      string          `json:"str"`      // argumento da ação (id da saída, endereço Bluetooth)
	Files    []string        `json:"files"`    // nova ordem da setlist (X.json)
	Timeline json.RawMessage `json:"timeline"` // timeline editada
	AI       bool            `json:"ai"`       // analyze: usar a IA
	Temp     string          `json:"temp"`     // analyze: auto | quente | fria
	Free     bool            `json:"free"`     // analyze: modo livre (efeitos criados para a música)
	Mix      bool            `json:"mix"`      // analyze: modo misto (presets e efeitos combinados)
	Playing  bool            `json:"playing"`  // live: a prévia está tocando
}

func (s *Server) handle(c *client, m *inMsg, now time.Time) error {
	buttons := s.ctl.Console().Buttons
	button := func() (*control.Button, error) {
		if m.ID < 0 || m.ID >= len(buttons) {
			return nil, fmt.Errorf("botão %d não existe", m.ID)
		}
		return buttons[m.ID], nil
	}
	switch m.Type {
	case "press":
		b, err := button()
		if err != nil {
			return err
		}
		s.ctl.Press(b, now)
	case "flash":
		b, err := button()
		if err != nil {
			return err
		}
		if m.Down {
			s.ctl.FlashDown(c.id, b, HoldTTL, now)
		} else {
			s.ctl.FlashUp(c.id, b, now)
		}
	case "ping":
		s.ctl.Refresh(c.id, HoldTTL, now)
	case "blackout":
		s.ctl.ToggleBlackout()
	case "stopStrobe":
		s.ctl.StopStrobe()
	case "stopAll": // PARAR TUDO: luz manual, blackout, a música e a prévia nos canhões
		s.ctl.StopAll()
		s.show.StopLive("PARAR TUDO")
		s.show.Stop()
		s.sendPlayer(true)
	case "manual":
		if m.Value < 0 || m.Value > 255 {
			return fmt.Errorf("valor %d inválido", m.Value)
		}
		return s.ctl.SetManual(m.Pos, m.Ch, byte(m.Value))
	case "release":
		s.ctl.ClearManual(m.Pos)
	case "swap":
		return s.ctl.Swap(m.A, m.B)
	case "identify":
		return s.ctl.Identify(m.Pos, now)
	case "live": // a prévia da aba Músicas nos canhões (Str = X.json)
		if !m.Down {
			if s.show.LiveStatus().Owner == c.id {
				s.show.StopLive("")
			}
			return nil
		}
		if err := s.show.Live(c.id, m.Str, m.T, m.Playing, m.Arg == "sync"); err != nil {
			msg, _ := json.Marshal(liveErrorMsg{Type: "liveError", File: m.Str, Error: err.Error()})
			select {
			case c.send <- msg:
			default:
			}
			return err
		}
	default:
		if handled, err := s.handleShow(m); handled {
			return err
		}
		return fmt.Errorf("mensagem desconhecida %q", m.Type)
	}
	return nil
}
