package web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/ncastellani/DMXController/internal/show"
	"github.com/ncastellani/DMXController/internal/sound"
)

type setlistMsg struct {
	Type    string    `json:"type"`
	Dir     string    `json:"dir"`
	Songs   []songMsg `json:"songs"`
	Orphans []string  `json:"orphans"` // .wav da pasta ainda sem timeline
}

type songMsg struct {
	Index    int     `json:"index"`
	File     string  `json:"file"`
	Title    string  `json:"title"`
	Duration float64 `json:"duration"`
	Genre    string  `json:"genre,omitempty"`
	Mood     string  `json:"mood,omitempty"`
	Error    string  `json:"error,omitempty"`
	Link     bool    `json:"link"`          // emenda com a próxima ao terminar
	Gap      float64 `json:"gap,omitempty"` // intervalo da emenda (s)
	WAV      string  `json:"wav,omitempty"` // arquivo de áudio (para reanalisar)
	Temp     string  `json:"temp,omitempty"`
	AI       bool    `json:"ai"`     // gerada com IA
	Free     bool    `json:"free"`   // gerada no modo livre
	Mix      bool    `json:"mix"`    // gerada no modo misto
	Edited   bool    `json:"edited"` // a timeline foi editada à mão
}

type timelineMsg struct {
	Type     string         `json:"type"`
	Timeline *show.Timeline `json:"timeline"` // null se nada carregado
	Edited   bool           `json:"edited"`   // há edições (existe X.json.orig)
}

// songTimelineMsg avisa que a timeline de uma música mudou (edição,
// restauração ou análise nova), carregada ou não.
type songTimelineMsg struct {
	Type     string         `json:"type"`
	File     string         `json:"file"` // X.json, como na setlist
	Timeline *show.Timeline `json:"timeline"`
	Edited   bool           `json:"edited"`
}

type playerMsg struct {
	Type        string  `json:"type"`
	Current     int     `json:"current"` // índice na setlist, -1 = nada
	Playing     bool    `json:"playing"`
	Position    float64 `json:"position"`
	Duration    float64 `json:"duration"`
	Section     string  `json:"section"`
	Cue         string  `json:"cue"`
	Next        string  `json:"next"`
	NextAt      float64 `json:"nextAt"`
	Audio       bool    `json:"audio"` // há saída de som
	Calibrating bool    `json:"calibrating"`
	Volume      float64 `json:"volume"`    // 0..1
	Latency     int     `json:"latencyMs"` // atraso compensado da saída
}

func (s *Server) setlist() setlistMsg {
	m := setlistMsg{Type: "setlist", Dir: s.show.Dir(), Songs: []songMsg{}, Orphans: []string{}}
	used := map[string]bool{}
	for _, so := range s.show.Songs() {
		sm := songMsg{Index: so.Index, File: so.File, Title: so.Title, Duration: so.Duration, Genre: so.Genre, Mood: so.Mood, Error: so.Err, Link: so.Link, Gap: so.Gap.Seconds(),
			Temp: so.Temp, AI: so.AI, Free: so.Free, Mix: so.Mix, Edited: exists(so.JSON + ".orig")}
		if so.WAV != "" {
			sm.WAV = filepath.Base(so.WAV)
			used[sm.WAV] = true
		}
		m.Songs = append(m.Songs, sm)
	}
	wavs, _ := filepath.Glob(filepath.Join(s.show.Dir(), "*.wav"))
	for _, w := range wavs {
		if b := filepath.Base(w); !used[b] && !strings.HasPrefix(b, ".") {
			m.Orphans = append(m.Orphans, b)
		}
	}
	return m
}

func (s *Server) player2msg() playerMsg {
	st := s.show.Status()
	m := playerMsg{
		Type: "player", Current: st.Current, Playing: st.Playing,
		Position: round3(st.Position), Duration: round3(st.Duration),
		Section: st.Section, Cue: st.Cue, Next: st.Next, NextAt: st.NextAt, Audio: st.Audio,
		Calibrating: st.Calibrating,
	}
	if s.player != nil {
		m.Volume = s.player.Volume()
		m.Latency = int(s.player.Latency() / time.Millisecond)
	}
	return m
}

// sendPlayer envia o estado do player se mudou (ou sempre, com force).
func (s *Server) sendPlayer(force bool) {
	msg, _ := json.Marshal(s.player2msg())
	s.mu.Lock()
	defer s.mu.Unlock()
	if force || !bytes.Equal(msg, s.playerState) {
		s.playerState = msg
		s.broadcast(msg)
	}
}

func (s *Server) sendSetlist() {
	msg, _ := json.Marshal(s.setlist())
	s.mu.Lock()
	s.broadcast(msg)
	s.mu.Unlock()
}

func (s *Server) sendTimeline() {
	msg, _ := json.Marshal(s.timelineMsg())
	s.mu.Lock()
	s.broadcast(msg)
	s.mu.Unlock()
}

// handleShow trata as mensagens do player. handled=false se não é do player.
func (s *Server) handleShow(m *inMsg) (handled bool, err error) {
	switch m.Type {
	case "scan":
		err = s.show.Scan()
		s.sendSetlist()
	case "removeSong": // Str: X.json da setlist ou X.wav sem timeline; vai para a lixeira
		var where string
		where, err = s.removeSong(m.Str)
		if where != "" && err == nil {
			s.ctl.Notice("♪ removida: %s (os arquivos estão em %s)", strings.TrimSuffix(m.Str, filepath.Ext(m.Str)), filepath.Join(filepath.Base(s.show.Dir()), where))
		}
		s.sendSetlist()
		s.sendTimeline()
		s.sendPlayer(true)
	case "analyze": // analisar (de novo) uma faixa da pasta
		err = s.analyze(m.Str, m.AI, m.Temp, m.Free, m.Mix)
	case "saveTimeline": // Str: X.json da música (vazio = a carregada)
		err = s.saveTimeline(m.Str, m.Timeline)
	case "revertTimeline":
		err = s.revertTimeline(m.Str)
	case "reorder":
		err = s.show.Reorder(m.Files)
		s.sendSetlist()
	case "link": // emendar a música Str com a próxima, depois de T segundos
		err = s.show.SetLink(m.Str, m.Down, time.Duration(m.T*float64(time.Second)))
		s.sendSetlist()
	case "load":
		if err = s.show.Load(m.ID); err == nil {
			s.sendTimeline()
		}
	case "play":
		err = s.show.Play()
	case "pause":
		s.show.Pause()
	case "stop":
		s.show.Stop()
	case "seek":
		err = s.show.Seek(m.T)
	case "volume":
		if s.sound == nil {
			return true, fmt.Errorf("sem saída de som")
		}
		s.sound.SetVolume(float64(m.Value) / 100)
	case "latency":
		if s.sound == nil {
			return true, fmt.Errorf("sem saída de som")
		}
		s.sound.SetLatency(m.Value)
	case "calibrate":
		if err = s.show.Calibrate(s.ctl.AllWhite()); err == nil {
			s.sendTimeline()
		}
	case "sound":
		if s.sound == nil {
			return true, fmt.Errorf("controle de som indisponível")
		}
		err = s.sound.Do(context.Background(), m.Arg, m.Str)
	default:
		return false, nil
	}
	s.sendPlayer(true)
	return true, err
}

func round3(x float64) float64 { return float64(int64(x*1000+0.5)) / 1000 }

type soundMsg struct {
	Type string `json:"type"`
	sound.State
	Available bool `json:"available"`
}

func (s *Server) soundMsg() soundMsg {
	if s.sound == nil {
		return soundMsg{Type: "sound"}
	}
	return soundMsg{Type: "sound", State: s.sound.State(), Available: true}
}

func (s *Server) sendSound() {
	msg, _ := json.Marshal(s.soundMsg())
	s.mu.Lock()
	s.broadcast(msg)
	s.mu.Unlock()
}

func (s *Server) timelineMsg() timelineMsg {
	m := timelineMsg{Type: "timeline", Timeline: s.show.Timeline()}
	if p := s.show.Current(); p != "" {
		m.Edited = exists(p + ".orig")
	}
	return m
}

// showChanged é chamado quando a música atual muda sozinha (emenda).
func (s *Server) showChanged() {
	s.sendTimeline()
	s.sendPlayer(true)
}
