// DMXController é o servidor do show: toca os modos de pista, cores e strobos da
// biblioteca (config/biblioteca.yaml) e serve a interface web (iPad/computador), com prévia
// dos canhões em tempo real. Todo o controle é feito pela interface.
//
//	DMXController [opções]                 servidor do show (interface web)
//	DMXController analisar [opções] <...>  gera as timelines de uma pasta (ver analisar.go)
//	DMXController biblioteca [opções]      avalia com a IA o clima dos modos de pista (ver biblioteca.go)
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/ncastellani/DMXController/internal/analysis"
	"github.com/ncastellani/DMXController/internal/audio"
	"github.com/ncastellani/DMXController/internal/control"
	"github.com/ncastellani/DMXController/internal/dmx"
	"github.com/ncastellani/DMXController/internal/engine"
	"github.com/ncastellani/DMXController/internal/fixture"
	"github.com/ncastellani/DMXController/internal/show"
	"github.com/ncastellani/DMXController/internal/sound"
	"github.com/ncastellani/DMXController/internal/web"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "analisar" {
		if err := analisar(os.Args[2:]); err != nil {
			fatal(err)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "biblioteca" {
		if err := biblioteca(os.Args[2:]); err != nil {
			fatal(err)
		}
		return
	}
	if err := run(); err != nil {
		fatal(err)
	}
}

// fatal mostra o erro e encerra. No Windows, aberto com dois cliques, espera
// um Enter antes de a janela fechar.
func fatal(err error) {
	log.Print(err)
	waitBeforeClosing()
	os.Exit(1)
}

func run() error {
	device := flag.String("dev", "auto", `porta serial da interface, "auto" ou "mock" (só prévia)`)
	consolePath := flag.String("console", "config/console.yaml", "botões do show (aponta para a biblioteca de presets)")
	profiles := flag.String("profiles", "config/fixtures.yaml", "perfis de fixtures")
	patch := flag.String("patch", "config/patch.yaml", "posições e endereços dos canhões")
	addr := flag.String("web", ":8080", "endereço da interface web")
	token := flag.String("token", "", "se definido, a interface web exige ?t=<token> no primeiro acesso")
	songsDir := flag.String("musicas", "musicas", "pasta da setlist (X.wav + X.json gerado pela aba Músicas ou por DMXController analisar)")
	// Um arquivo por sistema: ids de saída e endereços Bluetooth mudam de formato.
	showDMX := flag.Bool("ver-dmx", false, "mostra no terminal os canais DMX enviados (quando mudam, até 10 linhas/s)")
	model := flag.String("modelo", analysis.DefaultModel, "modelo do OpenRouter para analisar músicas novas")
	earDir := flag.String("ouvido", ".ouvido", "pasta do ouvido de bateria (separa a bateria na análise; instale com scripts/instalar_ouvido.sh)")
	soundPath := flag.String("som", "config/som-"+runtime.GOOS+".yaml", "preferências de som (atraso, volume, saída, caixa)")
	flag.Parse()

	fixtures, err := fixture.Load(*profiles, *patch)
	if err != nil {
		return err
	}
	rig, err := control.NewRig(fixtures, *patch)
	if err != nil {
		return err
	}
	positions := control.Positions(rig) // endereços lógicos (o quadro dos presets)
	console, err := control.LoadConsole(*consolePath, positions)
	if err != nil {
		return err
	}

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		return fmt.Errorf("interface web: %w (o DMXController já está aberto em outro terminal?)", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	eng := &engine.Engine{}
	ctl := control.NewController(console, eng, rig)
	health := &web.Health{}
	if runtime.GOOS == "linux" && os.Geteuid() == 0 {
		const msg = "o DMXController está rodando como root (sudo): o som não passa pelo PipeWire do seu usuário e ignora a saída escolhida (sai no HDMI/placa padrão). Rode sem sudo — veja a regra udev no README.md."
		log.Print("ATENÇÃO: " + msg)
		health.Warn(msg)
	}

	// A saída DMX nunca derruba o programa: sem a interface (ou se o cabo
	// cair), a música e a interface web continuam e ela é reaberta sozinha.
	var out dmx.Output
	if *device == "mock" {
		out = dmx.NewMock(nil)
		health.SetDMX(true, nil)
	} else {
		out = dmx.NewResilient(func() (dmx.Output, error) { return dmx.Open(*device, nil) }, func(ok bool, err error) {
			health.SetDMX(ok, err)
			if ok {
				log.Print("DMX conectado")
				ctl.Notice("DMX conectado")
			} else {
				log.Printf("DMX desconectado: %v (tentando de novo a cada 1 s)", err)
				ctl.Notice("DMX desconectado: %v", err)
			}
		})
	}
	defer out.Close()
	if *showDMX {
		out = dmx.NewLogger(out, os.Stdout, dmxFormatter(rig))
	}

	ref := dmx.NewRefresher(out, 40)
	refDone := make(chan error, 1)
	go func() { refDone <- ref.Run(ctx) }()
	go eng.Run(ctx, ref, 40)

	player, err := audio.NewPlayer()
	health.SetAudio(err)
	if err != nil {
		log.Printf("aviso: sem saída de som (%v); a luz do show segue um relógio interno", err)
		player = nil
	}
	sh := show.New(*songsDir, show.NewPresets(console.Library, positions), eng, player, console.Limits.Flash, ctl.Notice)
	if err := sh.Scan(); err != nil {
		log.Printf("aviso: setlist: %v", err)
	}

	// Interface nil (não um *audio.Player nil) quando não há saída de som.
	var soundPlayer sound.Player
	if player != nil {
		soundPlayer = player
	}
	snd, err := sound.NewManager(sound.New(), *soundPath, soundPlayer, ctl.Notice)
	if err != nil {
		return err
	}
	go snd.Run(ctx)

	srv := web.New(ctl, eng, sh, player, snd, health, *token)
	importer := &web.Importer{Dir: *songsDir, Catalog: analysis.Catalog(console), APIKey: analysis.APIKey(".env"), Model: *model, Library: console.Library, Separator: analysis.FindSeparator(*earDir)}
	srv.SetImporter(importer)
	if importer.Separator == nil {
		log.Print("aviso: sem o ouvido de bateria (scripts/instalar_ouvido.sh): a batida das músicas novas será estimada na música inteira")
	}
	if importer.APIKey == "" {
		log.Print("aviso: sem OPENROUTER_API_KEY (.env ou ambiente): músicas novas serão analisadas só por regras")
	}
	httpSrv := &http.Server{Handler: srv.Handler()}
	defer httpSrv.Close()
	go srv.Run(ctx)
	go func() {
		if err := httpSrv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("interface web: %v", err)
		}
	}()
	printURLs(ln.Addr().(*net.TCPAddr).Port, *token)
	fmt.Println("Ctrl+C apaga tudo e encerra.")

	// Tetos de segurança e expiração de flashes.
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			if err := <-refDone; err != nil {
				return err
			}
			fmt.Println("\nblackout enviado, saindo")
			return nil
		case err := <-refDone:
			return fmt.Errorf("saída DMX parou: %w", err)
		case now := <-tick.C:
			ctl.Tick(now)
			sh.Tick() // percebe a virada para a música emendada
		}
	}
}

// printURLs mostra os endereços para abrir a interface no iPad.
func printURLs(port int, token string) {
	q := ""
	if token != "" {
		q = "/?t=" + token
	}
	fmt.Printf("Interface web:\n  http://localhost:%d%s\n", port, q)
	ifaces, _ := net.Interfaces()
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			if ip, ok := a.(*net.IPNet); ok && ip.IP.To4() != nil {
				fmt.Printf("  http://%s:%d%s   (%s)\n", ip.IP, port, q, ifc.Name)
			}
		}
	}
}

// dmxFormatter descreve o quadro físico: para cada canhão, o endereço, um
// quadradinho com a cor emitida (se o terminal tiver cores) e os canais.
// Os blocos são fixados no início (trocar posições só troca endereços entre
// eles, então a lista de endereços continua a mesma).
func dmxFormatter(rig *fixture.Rig) func(*dmx.Frame) string {
	type block struct{ f fixture.Fixture }
	var blocks []block
	for _, f := range rig.Fixtures {
		blocks = append(blocks, block{*f})
	}
	slices.SortFunc(blocks, func(a, b block) int { return a.f.Address - b.f.Address })
	st, _ := os.Stdout.Stat()
	color := st != nil && st.Mode()&os.ModeCharDevice != 0
	return func(fr *dmx.Frame) string {
		var b strings.Builder
		for i, bl := range blocks {
			if i > 0 {
				b.WriteString("  │ ")
			}
			fmt.Fprintf(&b, "d%03d ", bl.f.Address)
			if color {
				c, strobe := bl.f.Look(fr)
				mark := "  "
				if strobe {
					mark = "⚡"
				}
				fmt.Fprintf(&b, "\x1b[48;2;%d;%d;%dm%s\x1b[0m ", c.R, c.G, c.B, mark)
			}
			for ch := range bl.f.Profile.Channels {
				fmt.Fprintf(&b, "%3d ", fr[bl.f.Address+ch-1])
			}
		}
		return strings.TrimRight(b.String(), " ")
	}
}
