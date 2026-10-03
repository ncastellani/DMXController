# DMXController — luz automática para show com DMX

Programa em Go que comanda 3 canhões LED por DMX no show de um cantor (voz ao
vivo sobre playback). Ele toca as faixas, gera uma **timeline de luz** para
cada música (análise do áudio, com IA opcional) e oferece os **presets
manuais** do iluminador (modos de pista, cores, fritação, strobo, blackout)
numa interface web usada no iPad e no computador ao mesmo tempo.

Os presets vêm do workspace do QLC+ montado pelo Amós
(`config/aniversario_STROBE.qxw`): o programa lê o arquivo e toca as
mesmas cenas e sequências, sem precisar do QLC+ aberto. Quando o Amós mudar
algo no QLC+, basta copiar o `.qxw` novo para `config/`.

## Funcionalidades

- **Console**: 55 botões em 4 grupos (modos de pista, cores fixas, fritação,
  strobo contínuo), com atalhos de teclado. A fritação só fica acesa
  **enquanto o botão está segurado**, e fritação e strobo têm tempo máximo
  (10 s e 60 s).
- **Show**: setlist (arrastar para reordenar), player com barra da música
  (partes, energia, ⚡ fritações), volume e **músicas emendadas** sem corte. A
  luz segue a música em sincronia, e a lista de trocas de luz acompanha (só
  leitura, com "Aqui" para levar o show até uma troca; "Editar" leva ao
  editor). Régua de tempo sobre a barra da música. Os presets manuais ficam por cima da
  timeline, e ela volta quando eles saem.
- **Luz apagada por padrão**: ao iniciar, com música carregada, pausada ou
  parada, tudo fica preto. Só acende com a música tocando ou com um preset
  manual.
- **Músicas**: enviar `.wav` e analisar (ou analisar de novo) com
  **temperatura das cores** (automática, quentes, frias) e IA opcional.
- **Versão cantada** (opcional): junto do playback, envie a mesma faixa
  **com a voz**. O programa alinha as duas, isola a voz e acha as frases
  cantadas; a IA transcreve a letra e, quando ela cita uma cor
  ("verde-limão"), a cor acende **na palavra** (encaixada na batida) por
  alguns compassos, criando a cor se ela não existir, e depois volta a
  sequência da parte. No show toca só o playback. A letra colada continua
  opcional: ajuda a IA a acertar as palavras (sem a versão cantada, a cor
  citada acende no começo da parte).
- **Prévia e editor da luz** (aba Músicas): escolha qualquer música, sem
  carregá-la no show, e ouça no próprio aparelho com o **palco 2D** (os 3
  canhões acendendo) e a **faixa de cores** de cada canhão sob a barra da
  música. Edite as trocas de luz, fritações e apagões encaixados na batida,
  com cores da biblioteca ou criadas na hora, desfazer e restaurar a versão
  gerada. Com a versão cantada, a letra aparece numa trilha sob a faixa de
  cores e em "Cantando", e dá para **ouvir com voz** para conferir as cores
  na palavra. Duas vistas do palco: pendurados (padrão) e **no chão, como no
  show** (os das pontas cruzando, o do centro para a plateia). A prévia não acende os
  canhões; strobo e fritação piscam nela no máximo 3 vezes por segundo
  (dá para desligar), e as trocas rápidas de cor aparecem pela média.
- **PARAR TUDO** (luz manual, blackout **e a música**) e **BLACKOUT**
  sempre à mão.
- **Preparação**: controle manual por canal, troca de posição dos canhões no
  palco, saída de som e caixa Bluetooth, calibração do atraso do som e log de
  eventos.
- A interface DMX pode cair e voltar: o programa reconecta sozinho, e os
  alertas aparecem na tela.

## Hardware

| Peça | Detalhes |
|---|---|
| Interface DMX | SourLight SL1000 = Enttec Open DMX (FTDI FT232R `0403:6001`). Sem microcontrolador: o computador gera o sinal DMX (250 kbaud 8N2, ~28 quadros/s) |
| Canhões | 3× Mini Men MM-T114 em modo 7 canais, endereços **d001, d008, d015** (esquerda, centro, direita): `dimmer, R, G, B, vazio, função, velocidade`. Função 50 + velocidade alta = strobo nativo |
| Som | Caixa Mondial CM-400 por Bluetooth ou cabo P2 |

Só um programa por vez pode usar a interface DMX: feche o QLC+ antes.

## Instalação

Precisa do Go 1.26 ou mais novo.

```bash
go build -o bin/ .                         # gera bin/DMXController
cp .env.example .env                       # opcional: chave do OpenRouter para a IA
```

Outras plataformas (compila sem cgo):

```bash
GOOS=linux  GOARCH=arm64 CGO_ENABLED=0 go build -o bin/linux-arm64/ .    # Raspberry Pi (64 bits)
GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -o bin/darwin-arm64/ .   # Mac M1/M2
```

**Linux (Ubuntu, Raspberry Pi OS)**: dê permissão da porta ao usuário (não
rode com `sudo`: o som iria para o HDMI e os arquivos ficariam do root).

```bash
echo 'SUBSYSTEM=="tty", ATTRS{idVendor}=="0403", ATTRS{idProduct}=="6001", TAG+="uaccess"' | sudo tee /etc/udev/rules.d/70-sl1000.rules
sudo udevadm control --reload && sudo udevadm trigger --subsystem-match=tty
```

O controle de som e Bluetooth usa `wpctl`, `pw-dump` (PipeWire) e
`bluetoothctl`.

**macOS** (ainda não testado): `brew install blueutil switchaudio-osx`. Se o
sistema bloquear o binário, rode `xattr -d com.apple.quarantine bin/darwin-arm64/*`.

## Uso

```bash
./bin/DMXController                 # abre a interface em http://<ip>:8080 (o terminal mostra os endereços)
./bin/DMXController -dev mock       # sem interface DMX: só a prévia na tela
```

| Opção | Padrão | Para quê |
|---|---|---|
| `-dev` | `auto` | porta da interface DMX, `auto` ou `mock` |
| `-web` | `:8080` | endereço da interface web |
| `-musicas` | `musicas` | pasta da setlist |
| `-token` | — | exige `?t=<token>` no primeiro acesso |
| `-ver-dmx` | — | mostra no terminal os canais enviados |
| `-modelo` | `google/gemini-3.8-flash` | modelo do OpenRouter |
| `-console`, `-profiles`, `-patch`, `-som` | `config/…` | arquivos de configuração |

O `DMXController` não tem interface no terminal: todo o controle e o log ficam na
página. Ctrl+C apaga os canhões e encerra.

**Preparar o show**: na aba **Músicas**, envie os `.wav` (com a versão
cantada, se tiver), toque em cada música e confira a luz na prévia (o som sai no
computador ou iPad, não na caixa), ajustando no editor. Em **Posições**, confirme a ordem dos
canhões ("Identificar"). Em **Som**, escolha a saída e calibre o atraso.
Para testar a interface DMX e descobrir canais, use "Identificar" e a aba
**Manual** (um controle por canal).

Para analisar uma pasta inteira de uma vez (por exemplo, depois que o Amós
criar presets novos):

```bash
./bin/DMXController analisar musicas/      # -sem-ia, -forcar (refaz as existentes), -temperatura quente|fria
```

Para usar a versão cantada pela linha de comando, deixe `X.voz.wav` ao lado
de `X.wav` (e, se quiser, a letra em `X.letra.txt`).

## Arquivos

| Caminho | O que é | No git? |
|---|---|---|
| `main.go`, `analisar.go` | o programa `DMXController` (servidor) e o subcomando `DMXController analisar` | sim |
| `internal/` | código: `dmx`, `fixture`, `qlc`, `engine`, `control`, `show`, `audio`, `analysis`, `sound`, `web` | sim |
| `config/console.yaml` | botões do console e limites de segurança | sim |
| `config/fixtures.yaml`, `config/patch.yaml` | perfil dos canhões e endereços (o patch é regravado ao trocar posições) | sim |
| `config/aniversario_STROBE.qxw` | workspace do QLC+ do Amós (de onde vêm os presets) | sim |
| `config/som-<sistema>.yaml` | preferências de som desta máquina (atraso, volume, caixa) | não |
| `musicas/` | `X.wav` + timeline `X.json` (+ `X.voz.wav` versão cantada, `X.json.orig` se editada, `X.letra.txt`) e a ordem em `setlist.txt` | não (áudio e conteúdo do show) |
| `tmp/` | sobras e testes locais | não |
| `.env` | `OPENROUTER_API_KEY` | não |
| `*.md`, `design/` | documentação (abaixo) | sim, menos o kit gerado em `design/` |
| `scripts/` | `kit_design.py` (kit do Claude Design), `cdp.mjs` (teste da interface no Chrome) | sim |

**Faça backup da pasta `musicas/`**: as timelines editadas à mão só existem
lá.

## Documentação

- [TIMELINE.md](TIMELINE.md): formato do JSON de cada música,
  presets, temperatura, versão cantada e letra.
- [PROTOCOLO.md](PROTOCOLO.md): o contrato entre o servidor
  e a página (WebSocket e HTTP).
- [design/brief.md](design/brief.md): briefing para redesenhar a
  interface no Claude Design. O kit é gerado com `python3 scripts/kit_design.py`.
- [HANDOFF.md](HANDOFF.md): onde paramos e o que vem a seguir.
- [CLAUDE.md](CLAUDE.md): arquitetura e regras para quem desenvolve com o
  Claude Code.

## Desenvolvimento

```bash
go test -race ./...          # os testes com a faixa real são pulados se musicas/ estiver vazia
gofmt -l .
```

O modo `-dev mock` e o modo demonstração da página (`?demo=1`, sem servidor)
precisam continuar funcionando. Para testar a interface num navegador de
verdade, veja o roteiro em `scripts/cdp.mjs`.

## Segurança (fotossensibilidade)

Strobo e fritação têm tempo máximo. A fritação só fica acesa com o botão
segurado (na web: segurar, com ping a cada 250 ms; ela solta se a página
perder o foco ou a conexão). A calibração pisca no máximo 3 vezes por segundo,
e o console ao vivo **nunca pisca** a tela para representar o strobo. Na
prévia da aba Músicas, strobo e fritação piscam no máximo 3 vezes por segundo
(opção "Piscar strobo e fritação"; vem desligada se o aparelho pede menos
movimento), e trechos que trocam de cor mais rápido que isso aparecem com a
cor média e um ícone.
