package web

import (
	"fmt"

	"github.com/ncastellani/DMXController/internal/analysis"
	"github.com/ncastellani/DMXController/internal/control"
	"github.com/ncastellani/DMXController/internal/engine"
)

type configMsg struct {
	Type   string `json:"type"`
	Groups []struct {
		Name    string      `json:"name"`
		Layer   string      `json:"layer"`
		Buttons []buttonMsg `json:"buttons"`
	} `json:"groups"`
	FlashLimit  float64 `json:"flashLimit"`  // segundos
	StrobeLimit float64 `json:"strobeLimit"` // segundos
	// Library são cores fixas da biblioteca que não estão nos botões (para o
	// editor da timeline; não aparecem no console).
	Library []libraryMsg `json:"library"`
	// Effects são os modos de pista que servem de efeito (os passos copiados
	// para a música), para o editor trocar uma parte por um deles.
	Effects []effectMsg `json:"effects"`
}

type effectMsg struct {
	ID     string     `json:"id"` // "pista:club"
	Name   string     `json:"name"`
	StepMs float64    `json:"stepMs"` // a partir de 1000, o efeito segue esse passo (em batidas) em vez dos toques
	Fade   bool       `json:"fade,omitempty"`
	Mood   string     `json:"mood,omitempty"` // clima em poucas palavras
	Steps  [][]string `json:"steps"`
}

type libraryMsg struct {
	Preset  string `json:"preset"`
	Caption string `json:"caption"`
	Color   string `json:"color"`
}

type buttonMsg struct {
	ID      int    `json:"id"`
	Caption string `json:"caption"`
	Color   string `json:"color,omitempty"`
	Key     string `json:"key,omitempty"`
	Preset  string `json:"preset"` // id usado na timeline ("pista:chill")
}

type stateMsg struct {
	Type      string         `json:"type"`
	Blackout  bool           `json:"blackout"`
	Active    map[string]int `json:"active"` // camada → id do botão (-1 = nada)
	Identify  int            `json:"identify"`
	Show      string         `json:"show"` // música conduzindo a luz ("" = nenhuma)
	Live      liveMsg        `json:"live"` // a prévia da aba Músicas nos canhões
	Health    healthMsg      `json:"health"`
	Positions []positionMsg  `json:"positions"`
}

// liveMsg diz qual música a prévia leva aos canhões (file vazio = nenhuma)
// e qual navegador a conduz (by = o client do hello dele).
type liveMsg struct {
	File string `json:"file"`
	By   string `json:"by"`
}

// helloMsg é a primeira mensagem da conexão: o id deste navegador.
type helloMsg struct {
	Type   string `json:"type"`
	Client string `json:"client"`
}

// liveErrorMsg vai só ao navegador que pediu a prévia nos canhões.
type liveErrorMsg struct {
	Type  string `json:"type"`
	File  string `json:"file"`
	Error string `json:"error"`
}

type positionMsg struct {
	Name     string       `json:"name"`
	Address  int          `json:"address"`
	Color    string       `json:"color"` // cor estimada que o canhão está emitindo
	Strobe   bool         `json:"strobe"`
	Channels []channelMsg `json:"channels"`
}

type channelMsg struct {
	Role   string `json:"role"`
	Value  int    `json:"value"`
	Manual bool   `json:"manual"`
}

type eventMsg struct {
	Type string `json:"type"`
	Time string `json:"time"`
	Text string `json:"text"`
}

type eventsMsg struct {
	Type  string     `json:"type"`
	Items []eventMsg `json:"items"`
}

func (s *Server) config() configMsg {
	c := s.ctl.Console()
	m := configMsg{
		Type:        "config",
		FlashLimit:  c.Limits.Flash.Seconds(),
		StrobeLimit: c.Limits.Strobe.Seconds(),
	}
	for _, g := range c.Groups {
		var grp struct {
			Name    string      `json:"name"`
			Layer   string      `json:"layer"`
			Buttons []buttonMsg `json:"buttons"`
		}
		grp.Name = g
		for _, b := range c.Buttons {
			if b.Group != g {
				continue
			}
			grp.Layer = b.Layer.String()
			bm := buttonMsg{ID: b.ID, Caption: b.Caption, Color: b.Color, Preset: b.Preset}
			if b.Key != 0 {
				bm.Key = string(b.Key)
			}
			grp.Buttons = append(grp.Buttons, bm)
		}
		m.Groups = append(m.Groups, grp)
	}
	m.Library = []libraryMsg{}
	s.effectsOnce.Do(func() { // a biblioteca não muda com o programa aberto
		s.effects = []effectMsg{}
		for i := range c.Library.Effects {
			e := &c.Library.Effects[i]
			if fx, ok := analysis.LibraryEffect(e, 0, nil, analysis.TempAuto); ok { // os passos como efeito: sem branco total nem apagado
				em := effectMsg{ID: e.ID, Name: e.Name, StepMs: e.TypicalStep(), Fade: e.Fades(), Steps: fx.Steps}
				if e.Mood != nil {
					em.Mood = e.Mood.Label
				}
				s.effects = append(s.effects, em)
			}
		}
	})
	m.Effects = s.effects
	if s.importer != nil {
		for _, p := range s.importer.Catalog {
			if p.Library {
				m.Library = append(m.Library, libraryMsg{Preset: p.ID, Caption: p.Caption, Color: p.Hex})
			}
		}
	}
	return m
}

func (s *Server) snapshot() stateMsg {
	st := s.ctl.State()
	frame := s.eng.Last()
	m := stateMsg{Type: "state", Blackout: st.Blackout, Identify: st.Identify, Active: map[string]int{}}
	m.Show = s.eng.Status().Names[engine.Show]
	lv := s.show.LiveStatus()
	m.Live = liveMsg{File: lv.File, By: lv.Owner}
	m.Health = s.health.snapshot()
	for _, l := range []engine.Layer{engine.Base, engine.Strobe, engine.Flash} {
		m.Active[l.String()] = id(st.Active[l])
	}
	for _, p := range st.Positions {
		c, strobe := p.Logical.Look(&frame)
		pm := positionMsg{
			Name:    p.Name,
			Address: p.Address,
			Color:   fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B),
			Strobe:  strobe,
		}
		for ch, role := range p.Logical.Profile.Channels {
			pm.Channels = append(pm.Channels, channelMsg{
				Role:   string(role),
				Value:  int(frame[p.Logical.Address+ch-1]),
				Manual: p.Manual[ch],
			})
		}
		m.Positions = append(m.Positions, pm)
	}
	return m
}

func id(b *control.Button) int {
	if b == nil {
		return -1
	}
	return b.ID
}
