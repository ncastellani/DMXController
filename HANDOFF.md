# Handoff — onde paramos e o que vem a seguir

Leia junto com o `CLAUDE.md` (arquitetura e regras) e o `README.md` (o que o
programa faz). Este arquivo guarda só o estado atual e os roteiros das
próximas sessões. Última atualização: 02/10/2026 (fim da 5ª sessão).

## Estado atual

Tudo do README está pronto e testado no Linux. A 5ª sessão fez a **Sessão A
— versão cantada para acertar o tempo das cores**:

- **Envio**: na aba Músicas, o formulário ganhou "Versão cantada
  (opcional)": um segundo `.wav`, a mesma faixa com a voz, gravado como
  `X.voz.wav` (`POST /api/upload?name=X.wav&voz=1`; "Trocar"/"Tirar" ao
  analisar de novo, `analyze` com `noVoice`). O show nunca toca esse arquivo;
  ele não aparece como música nova. `luzes analisar` usa `X.voz.wav` sozinho.
- **Isolar a voz** (`analysis/vocals.go`): GCC-PHAT alinha as versões (até
  ±5 s; outra música dá placar < 10, a mesma faixa > 100 → limiar 30, senão
  a cantada é ignorada com aviso); subtração espectral por banda (ganho do
  quantil baixo, máximo dos quadros vizinhos) → energia da voz → **frases
  cantadas** (lacuna < 0,35 s junta, < 0,3 s sai, > 12 s divide no ponto mais
  fraco). Subtração no tempo a 44,1 kHz (atraso refinado na taxa original)
  dá a voz limpa quando o resíduo nos trechos sem voz fica 20 dB abaixo
  (mediana por quadro); senão a IA ouve a cantada inteira.
- **Transcrição** (`analysis/lyrics.go`): uma chamada com **um áudio por
  frase**; a IA devolve o texto e as cores citadas com o segundo da palavra.
  A letra colada continua opcional e vira referência para a transcrição
  (decisão da usuária: "manter como ajuda"); sem versão cantada, ela segue
  o caminho antigo (casar com as partes).
- **Coreografia**: as partes sempre ganham uma sequência; as cores citadas
  vêm em `cores_citadas` (cena fixa da biblioteca de qualquer temperatura ou
  `cor:` nova). `placeColors`: a cor acende na batida da palavra (no início
  da parte, se cair a menos de 1 batida dele; a troca da parte sai), fica até
  o fim da frase, **2 a 4 compassos** (decisão da usuária: "alguns
  compassos"), e a sequência da parte volta ("volta depois da cor"); a parte
  seguinte, se vier antes, assume; cores a < 2 batidas da anterior saem.
  Sem versão cantada (só letra colada): início da parte, 4 compassos.
- **Timeline**: `lyrics: [{t, end, text}]` e `generated.voice {shift,
  clean, phrases}`; o editor nunca muda a letra (vem da versão gravada).
- **Editor**: trilha da letra (um bloco por frase) sob a faixa de cores,
  "Cantando" ao lado do palco, "♪ “trecho”" na troca que veio da letra e
  **"Ouvir com voz"** (`/api/audio?voz=1`, descontando `voice.shift`).
- Testado de ponta a ponta com a IA de verdade, num par montado em
  `tmp/voz/` (playback de praficarcomigo + trechos de "Crimson & Clover" em
  instantes conhecidos, 37 ms de atraso; também com outro "master"): atraso
  exato, frases batendo com os trechos, "Crimson" → BORDÔ/VERMELHO
  PROFUNDO na batida da palavra. Pela página: envio da versão cantada +
  análise + editor (prints em computador e iPad, modo demonstração sem
  erros). ~US$ 0,004 a transcrição + ~US$ 0,02 a coreografia; ~50 s por
  música no PC.
- Testes novos: `TestIsolateVoice` (sintético: atraso de meia amostra,
  frases, faixa diferente recusada), `TestPhrasesSplitLong`,
  `TestSungColors`, `TestLyricsColors` (reescrito), `web.TestUploadVoice`.
- O Claude Design **não está mais em uso**: só rodar `scripts/kit_design.py`
  se ela pedir (o `design/brief.md` foi atualizado antes do aviso).

Sessões anteriores: B (4ª) = prévia 2D com duas vistas, faixa de cores,
editor de qualquer música na aba Músicas, Show só leitura, partes
enumeradas; antes disso, DMX, console, player, aba Músicas, luz apagada por
padrão.

**Não testado**: com uma versão cantada **real** (voz de verdade sobre o
mesmo playback): a detecção de frases foi afinada com o par de teste;
frases muito ligadas podem virar uma só (até 12 s) e backing vocals do
playback não aparecem (estão nas duas versões). "Ouvir com voz" e a prévia
com som no **iPad/Safari**. **O primeiro commit ainda não foi feito**.

## Pendências pequenas

- Enviar a **versão cantada** de `praficarcomigo` (e das outras) e refazer
  pela aba Músicas; conferir com "Ouvir com voz" se as cores caem na
  palavra. O `praficarcomigo.json` atual ainda é do script Python antigo
  (BPM 123,05; o real é 126).
- Sugerir ao Amós 2–3 **sequências frias agitadas** no QLC+ (hoje só há o
  CLUB).
- Em aberto no equipamento (do HANDOVER do Amós): CH6 = 50 está na fronteira
  das faixas do canal de função (se o strobo parar, testar 60–70); CH7
  252–255 podem dar a mesma velocidade; as cores quentes LARANJA QUEIMADO,
  FOGO, ÂMBAR ESCURO, ÂMBAR, ÂMBAR DOURADO e DOURADO talvez não se distingam
  ao vivo; BORDÔ pode parecer vermelho; `PISTA | ÂMBAR 200` troca 5×/s.
- Ideias aceitas, ainda não feitas: modo freestyle (efeitos sem presets) e
  criar presets na tela.

## Próximas sessões (uma por chat)

As Sessões A e B estão feitas (acima). Ordem sugerida: **C** (usa o editor
da B e a letra transcrita da A); **D** quando o Pi chegar.

### Sessão C — ajuste fino com IA (para o Amós)

Pedido: depois de gerada, o Amós ajusta cada música conversando com a IA
("segundo refrão mais agitado", "troca o lilás por rosa", "tira a fritação de
2:37").

- Um painel de conversa na música (aba Músicas, junto do editor e da
  prévia). O antes/depois pode usar a faixa de cores: `RenderPreview` das
  duas timelines (a prévia hoje lê do disco; para o "depois" ainda não
  salvo, aceitar a timeline no corpo de um `POST /api/preview`).
- O servidor manda ao LLM:
  - a timeline atual (cues, partes, resumo da análise; não o vetor inteiro de
    batidas);
  - o catálogo de presets com temperatura;
  - a letra (a transcrita, com os tempos, se houver versão cantada) e o pedido.
- O LLM devolve **operações** em JSON (adicionar, remover ou alterar trocas;
  renomear partes), não um arquivo livre.
- Validar com as mesmas regras da análise (presets existentes, cores
  válidas, encaixe na batida, limites de fritação/strobo e espaçamento) e
  mostrar um **antes/depois** para aprovar. Ao aplicar, usar o mesmo caminho
  do editor (`.orig`, desfazer).
- Histórico da conversa por música (ex.: `X.chat.json`). Não mostrar o nome
  do modelo. Modelo só de texto (mais barato), com o áudio como opção.

### Sessão D — Raspberry Pi 5 (4 GB) no lugar do Mac

Decisão da usuária: o show roda num **Raspberry Pi 5 4 GB**. É uma boa
troca: é Linux, o mesmo sistema em que tudo foi testado (no Mac nada foi
testado).

- **Não tem saída P2** (o Pi 5 tirou o conector de 3,5 mm). Para cabo:
  placa de som USB barata. **Bluetooth 5.0 embutido** (sem dongle). HDMI
  também sai som.
- Bluetooth e Wi-Fi 2,4 GHz dividem o mesmo chip: para o som Bluetooth não
  picotar, usar Wi-Fi em **5 GHz**.
- Ideia: o Pi como **ponto de acesso Wi-Fi** (`nmcli dev wifi hotspot`, em
  5 GHz) para o iPad conectar direto, sem depender da internet do local. A
  análise com IA precisa de internet: fazer antes, em casa.
- Fonte oficial 27 W (5 V/5 A) e cooler ativo (a análise do áudio usa muita
  CPU).
- Raspberry Pi OS 64 bits (Bookworm) com desktop: já traz PipeWire e
  WirePlumber, que o pacote `sound` usa. Na versão Lite, instalar
  `pipewire wireplumber pipewire-pulse` e `loginctl enable-linger`.
- Iniciar sozinho: serviço systemd **de usuário** (o áudio é da sessão do
  usuário; não usar root). Regra udev da SL1000 como no README.
- Testar no aparelho: DMX (tempo do BREAK no FTDI), latência e calibração do
  som (Bluetooth e USB), reconexão da caixa, tempo da análise de uma música e
  uso de memória.
- Com isso, as pendências do Mac (`brew`, CoreAudio) viram opcionais.
