# DMXController — luz automática para show com DMX

Programa em Go que comanda 3 canhões LED por DMX no show de um cantor (voz ao
vivo sobre playback). Ele toca as faixas, gera uma **timeline de luz** para
cada música (análise do áudio, com IA opcional) e oferece os **presets
manuais** do iluminador (modos de pista, cores, fritação, strobo, blackout)
numa interface web usada no iPad e no computador ao mesmo tempo.

Os presets ficam na **biblioteca** do próprio programa
(`config/biblioteca.yaml`): cores, strobos e modos de pista, escritos passo
a passo num arquivo de texto. Ela nasceu do workspace do QLC+ montado pelo
Amós (convertido com o DMX conferido quadro a quadro); o programa não usa
mais o QLC+ nem o `.qxw`. Para criar ou mudar um preset, edite o arquivo
(o formato está explicado no topo dele) e reinicie o programa.

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
  Em **Efeitos**, "Presets" usa as sequências prontas do console, "Misto"
  combina os dois na mesma música (modos de pista prontos em algumas partes,
  em geral as mais agitadas, e sequências criadas para a música nas outras;
  cada tipo em pelo menos um terço das partes) e "Livre"
  cria sequências próprias para a música (escritas passo a passo, com várias
  cores e frases que mudam, como as de pista), trocando de passo a cada toque da bateria ou da guitarra (detectado no
  áudio) e sem piscar rápido, misturadas aos presets e aos **modos de pista levados como efeito** (o desenho de um modo de pista, que a IA usa como está ou com as cores da música, escolhendo pelo clima de cada um); no editor dá para mudar a cor de cada canhão em
  cada passo, o tempo do passo e o fade.
  A análise também **segue a batida**: divide uma parte onde a bateria entra
  ou some (cada pedaço com a sua luz), considera a força da percussão para
  decidir a agitação e dá um flash branco no canhão do centro nos picos.
  Com o **ouvido de bateria** instalado (opcional, ver Instalação), ela separa
  a bateria do resto da música e ouve a batida nela: os efeitos trocam no
  bumbo e na caixa (não na voz nem no sintetizador), as partes começam
  exatamente onde a bateria entra ou sai, e os picos são a entrada da
  bateria e as batidas soltas.
  **Remover** uma música: ela sai da setlist e os arquivos vão para
  `musicas/lixeira/` (dá para recuperar de lá). Não remove a que está
  tocando no show.
- **Prévia e editor da luz** (aba Músicas): escolha qualquer música, sem
  carregá-la no show, e ouça no próprio aparelho com o **palco 2D** (os 3
  canhões acendendo) e a **faixa de cores** de cada canhão sob a barra da
  música. Edite as trocas de luz, fritações e apagões encaixados na batida,
  com cores da biblioteca ou criadas na hora, desfazer e restaurar a versão
  gerada. Duas vistas do palco: pendurados (padrão) e **no chão, como no
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

Só um programa por vez pode usar a interface DMX: se o QLC+ estiver aberto, feche antes.

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

### Ouvido de bateria (opcional)

Uma biblioteca de separação de instrumentos (Demucs, em Python) que isola a
bateria de cada música na hora da análise. Fica num ambiente à parte, na
pasta `.ouvido` do projeto, e não mexe no Python do sistema:

```bash
scripts/instalar_ouvido.sh      # ~1,2 GB em disco, precisa de internet só aqui
rm -rf .ouvido                  # para desinstalar
```

Com ele, a análise de uma música leva de 40 s a 1 min a mais na primeira vez
(num computador comum; num Raspberry Pi, alguns minutos) e usa uns 2 GB de
memória; a bateria separada fica guardada, então analisar a mesma música de
novo é imediato. Sem ele o programa funciona igual e estima a batida na
música inteira. A aba Músicas diz se o ouvido está ligado.

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

**Preparar o show**: na aba **Músicas**, envie os `.wav`, toque em cada música e confira a luz na prévia (o som sai no
computador ou iPad, não na caixa), ajustando no editor. Em **Posições**, confirme a ordem dos
canhões ("Identificar"). Em **Som**, escolha a saída e calibre o atraso.
Para testar a interface DMX e descobrir canais, use "Identificar" e a aba
**Manual** (um controle por canal).

Para analisar uma pasta inteira de uma vez (por exemplo, depois que o Amós
criar presets novos):

```bash
./bin/DMXController analisar musicas/      # -sem-ia, -forcar (refaz as existentes), -temperatura quente|fria, -livre, -misto
```

Depois de criar ou mudar um modo de pista na biblioteca, peça à IA o clima
dele (é o que o modo livre usa para escolher):

```bash
./bin/DMXController biblioteca             # avalia os que ainda não têm clima; -reavaliar refaz todos
```

## Arquivos

| Caminho | O que é | No git? |
|---|---|---|
| `main.go`, `analisar.go`, `biblioteca.go` | o programa `DMXController` (servidor) e os subcomandos `DMXController analisar` e `DMXController biblioteca` | sim |
| `internal/` | código: `dmx`, `fixture`, `library`, `engine`, `control`, `show`, `audio`, `analysis`, `sound`, `web` | sim |
| `config/console.yaml` | botões do console (cada um aponta para um preset da biblioteca) e limites de segurança | sim |
| `config/fixtures.yaml`, `config/patch.yaml` | perfil dos canhões e endereços (o patch é regravado ao trocar posições) | sim |
| `config/biblioteca.yaml` | a biblioteca de presets: cores, strobos (fritação e strobo contínuo) e modos de pista, com o clima de cada modo | sim |
| `config/som-<sistema>.yaml` | preferências de som desta máquina (atraso, volume, caixa) | não |
| `musicas/` | `X.wav` + timeline `X.json` (+ `X.json.orig` se editada), a ordem em `setlist.txt` e as músicas removidas em `lixeira/` | não (áudio e conteúdo do show) |
| `tmp/` | sobras e testes locais | não |
| `.env` | `OPENROUTER_API_KEY` | não |
| `*.md`, `design/` | documentação (abaixo) | sim, menos o kit gerado em `design/` |
| `scripts/` | `instalar_ouvido.sh` (ouvido de bateria), `kit_design.py` (kit do Claude Design), `cdp.mjs` (teste da interface no Chrome) | sim |
| `.ouvido/` | o ouvido de bateria instalado: Python próprio, Demucs, modelo e as baterias já separadas | não |

**Faça backup da pasta `musicas/`**: as timelines editadas à mão só existem
lá.

## Documentação

- [TIMELINE.md](TIMELINE.md): formato do JSON de cada música,
  presets, efeitos do modo livre, biblioteca e temperatura.
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
