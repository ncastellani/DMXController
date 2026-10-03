# Projeto: Luzes automáticas para show (Go + DMX)

> Responda sempre em **português do Brasil**. **Onde paramos e o que vem a seguir: `HANDOFF.md`** (leia primeiro). Visão de uso, instalação e mapa de arquivos: `README.md`.

Programa em Go que controla 3 canhões LED via DMX no pequeno show de um cantor (voz ao vivo sobre playback): toca as faixas, gera e segue uma "timeline de luz" por música, e oferece presets manuais (fritação/strobo, blackout, cores) por uma interface web usada no iPad e no computador ao mesmo tempo. Desenvolvido no Ubuntu; **no show vai rodar num Raspberry Pi 5 4 GB** (antes era o MacBook Pro M2 do Amós, namorado da usuária; nada foi testado em nenhum dos dois).

## Comandos

```bash
go build -o bin/ .                            # bin/luzes (um binário só; package main na raiz)
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o bin/linux-arm64/ .   # Raspberry Pi
for a in arm64 amd64; do GOOS=darwin GOARCH=$a CGO_ENABLED=0 go build -o bin/darwin-$a/ .; done
go test -race ./...
./bin/luzes                     # servidor web :8080 (DMX auto, -dev mock, -ver-dmx, -token, -musicas, -modelo)
./bin/luzes analisar musicas/   # gera X.json para cada X.wav (-sem-ia, -forcar, -temperatura quente|fria; usa X.letra.txt)
python3 scripts/kit_design.py   # kit para o Claude Design (design/)
```

O `luzes` **não tem interface no terminal** (só imprime os endereços); todo controle e log ficam na web.

## Hardware

- **Interface SourLight SL1000** = Enttec Open DMX (FTDI FT232R `0403:6001`, serial `A77AE62U`, sem microcontrolador: o PC gera BREAK/MAB/start code/512 canais a 250 kbaud 8N2, reenviando ~28 quadros/s). Linux: `termios2`/BOTHER + `TIOCSBRK`, ioctls repetem em `EINTR`. macOS: `IOSSIOSPEED` + `TIOCDRAIN`, porta `/dev/cu.usbserial-*` (**não testado**). `-dev auto` detecta nos dois. Só um programa por vez na porta (fechar o QLC+).
- **3× Mini Men MM-T114**, modo 7 canais, endereços **1, 8, 15** (esquerda, centro, direita): `1 dimmer | 2 R | 3 G | 4 B | 5 vazio | 6 função | 7 velocidade`. **CH6=50 + CH7 alto = strobo nativo** (descoberto pelo Amós). Fora de strobo, CH5–7 = 0.
- **Caixa Mondial CM-400** por Bluetooth ou P2 (decide na hora). Dev: dongle clone CSR/Barrot `0a12:0001` (só funcionou na porta USB traseira).
- **Não rodar com sudo** (áudio vai para o HDMI e arquivos ficam do root). Permissão da porta no Linux: regra udev `TAG+="uaccess"` para `0403:6001` (comando no README).

## Arquitetura (`internal/`)

- `dmx`: `Output`, `OpenDMX` (linux/darwin), `Mock`, `Resilient` (reabre a interface; cabo solto não derruba nada), `Refresher` (goroutine própria), `Logger` (`-ver-dmx`), `Remap`.
- `fixture`: perfis/patch em YAML; `Rig` = posições do palco → endereços físicos (quadro lógico do QLC+ remapeado na saída; trocar posições grava `config/patch.yaml`).
- `qlc`: lê o `.qxw` do Amós (`config/aniversario_STROBE.qxw`; notas do formato no topo de `qlc.go`) — Scenes e Sequences tocadas como quadros DMX. O pacote original do QLC+ (HANDOVER.md, CSVs) fica fora do repo, em `~/Downloads/qlc/QLC_aniversario_pacote/`.
- `engine`: camadas **Show (timeline da música) < Base < Strobe < Flash < Identify**; canais manuais por cima; blackout zera tudo. A camada mais alta define o quadro inteiro (no QLC+ era HTP e misturava cores).
- `control`: `Console` (botões de `config/console.yaml`, gerado da Virtual Console; `limits`: flash 10 s, strobe 60 s) e `Controller` thread-safe (toggle, solo, flash segurado por "holder" com ping/TTL, manual, posições, identificar).
- `show`: presets `qlc:<ID>` (workspace) e `cor:#rrggbb` (cor criada para a música, acende os 3 canhões); setlist (`musicas/*.json` + `.wav`; ordem e emendas em `musicas/setlist.txt`: `X.json<TAB>emendar=<s>`), timelines (`TIMELINE.md`), `Program` (cues → quadros, `rate` encaixa sequências no BPM), player com **fluxo contínuo** (músicas emendadas sem corte), `Tick` detecta a virada, `Update(path, t)` aplica edições sem parar o áudio (na carregada ou já emendada), `RenderPreview` = a luz de uma música amostrada canhão a canhão (via `Look`; trocas > 3/s viram a média com marca, a prévia nunca pisca).
- `audio`: WAV → PCM 16 bits/estéreo/44,1 kHz; `Player` (oto/v3, sem cgo: CoreAudio no Mac, PulseAudio/PipeWire/ALSA no Linux) toca um fluxo (`Load`/`Append(gap)`/`Truncate`/`Release`); `Position` desconta buffer + atraso calibrado.
- `analysis`: porte em Go do librosa (HPSS, onset, tempo, beat tracking DP, compasso, energia, partes por Ward) + **BPM refinado por regressão das batidas** (librosa dizia 123,05; o real era 126); catálogo de presets (cores nomeadas, agitação, **temperatura** quente/fria/mista/neutra; inclui a "biblioteca": cores `TODOS | X` que não são botões); opções de análise: temperatura pedida e **letra colada** (`X.letra.txt`; a IA casa a letra com as partes e põe a cor citada no início da parte — da biblioteca ou `cor:` nova); coreógrafo por regras ou **LLM via OpenRouter** (`OPENROUTER_API_KEY` no `.env`; padrão `google/gemini-3.8-flash`; o modelo ouve WAV mono 16 kHz; tudo validado e encaixado na batida).
- `sound`: saída de som e Bluetooth (Linux: `pw-dump`/`wpctl`/`bluetoothctl`; Mac: `SwitchAudioSource`/`blueutil` via Homebrew, **não testado**); preferências em `config/som-<GOOS>.yaml`; caixa preferida reconectada sozinha.
- `web`: HTML único embutido (`static/index.html`, JS puro) + WebSocket `/ws` (contrato em `PROTOCOLO.md`), aba **Músicas** (enviar .wav com `POST /api/upload?analisar=0` + mensagem `analyze` com temperatura/IA/letra; analisar de novo; `GET /api/lyrics`) + fila de análise + **prévia 2D e editor de qualquer música** sem carregá-la no show (`GET /api/timeline|preview|audio?file=X.json`; áudio tocado no navegador; `saveTimeline`/`revertTimeline` com `str`=arquivo, `.orig`; resposta `songTimeline`); a aba Show só exibe a timeline. `Health` (alertas). `static/demo.js` = modo demonstração (`?demo=1`) para o Claude Design.

Locks: o engine chama `source.Render` segurando o lock dele → `Show` nunca chama o engine segurando `s.mu`; o playhead tem lock próprio.

## Regras e decisões

- **Segurança (fotossensibilidade)**: strobo/flash com teto de tempo; fritação só enquanto o botão está segurado (na web: pointerdown/up + ping 250 ms, solta ao perder foco ou conexão); calibração ≤ 3 piscadas/s; o console ao vivo (topo, aba Show) **nunca pisca** a tela para representar strobo. Só a prévia da aba Músicas pisca strobo/fritação (pedido da usuária), **no máximo 2,9×/s** (`FLASH_HZ`), só tocando, com opção de desligar (desligada com `prefers-reduced-motion`); trocas rápidas de cor continuam pela média do servidor.
- PARAR TUDO para luz manual, blackout **e a música**. Não há botão "parar strobe".
- **Padrão é tudo apagado**: a camada Show só desenha com a música tocando (carregada/parada/pausada = preto).
- Presets manuais ficam por cima da timeline e ela volta quando saem.
- Tema **escuro por padrão** (ao redesenhar no Claude Design ele tende a voltar para o claro — corrigir na integração). Não mostrar o nome do modelo de IA na interface.
- Design: Claude Design desenha (kit em `design/`: `brief.md` à mão + `luzes-demo.html`/prints gerados), Claude Code integra mantendo protocolo, lógica de flash, reconexão, alertas, sincronização da lista e modo demonstração.
- Configuração em YAML/arquivos, nada de endereços hardcoded; sempre manter o modo mock funcionando; `gofmt` + testes.
- Git: `musicas/`, `tmp/`, `.env`, `bin/`, `config/som-*.yaml` e o kit gerado do Claude Design ficam fora (ver `.gitignore`). Sobras e testes locais vão para `tmp/`.
- Testes de interface reais: Chrome headless do cache do Playwright + `scripts/cdp.mjs`. **Nunca `pkill -f` com padrão que apareça no próprio comando** (mata o shell); use `pkill -x luzes` / `pkill -x chrome-headless`.
