# Protocolo da interface web

Contrato entre o servidor (`internal/web`) e o frontend
(`internal/web/static/index.html`, embutido no binário). Um redesenho do
frontend pode mudar tudo de visual, desde que fale este protocolo.

Conexão: WebSocket em `/ws` (mesmo host da página). Mensagens JSON com campo
`type`. Se o servidor usar `-token`, a página precisa ter sido aberta com
`?t=<token>` uma vez (fica num cookie).

## Servidor → navegador

**`config`** — logo ao conectar. Botões do console agrupados.

```json
{ "type": "config", "flashLimit": 10, "strobeLimit": 60,
  "groups": [ { "name": "CORES FIXAS", "layer": "base",
    "buttons": [ { "id": 32, "caption": "BRANCO", "color": "#ffffff", "key": "q" } ] } ] }
```

`layer`: `base` (toggle, um por vez), `strobe` (toggle), `flash` (segurar).
`library`: cores fixas do workspace que não são botões (`[{"preset": "qlc:10",
"caption": "VERDE LIMÃO", "color": "#8cff00"}]`) — só para o editor da
timeline, não aparecem no console.
`key` é opcional (atalho de teclado no navegador). `preset` é o id usado nas
timelines (`"qlc:47"`) — é com ele que o editor escolhe presets.

**`state`** — sempre que algo muda (até ~30×/s).

```json
{ "type": "state", "blackout": false, "identify": -1,
  "active": { "base": 8, "strobe": -1, "flash": -1 },
  "positions": [ { "name": "esquerda", "address": 1, "color": "#ff2a6d", "strobe": false,
    "channels": [ { "role": "dimmer", "value": 255, "manual": false } ] } ] }
```

- `active.*`: id do botão ativo na camada, `-1` = nenhum.
- `positions`: na ordem do palco (esquerda → direita). `color` é a cor
  estimada que o canhão emite agora; `strobe` = strobo nativo ligado.
- `channels[].role`: `dimmer red green blue empty mode speed`. `manual` =
  canal fixado pelo controle manual.
- `identify`: posição sendo identificada (acesa em branco), `-1` = nenhuma.

- `show`: título da música cuja timeline está conduzindo a luz (`""` = nenhuma).

**`setlist`** — ao conectar e ao recarregar a pasta.

```json
{ "type": "setlist", "dir": "musicas",
  "songs": [ { "index": 0, "file": "praficarcomigo.json", "title": "praficarcomigo", "duration": 256.8,
    "genre": "Electropop", "mood": "Sensual, noturno", "error": "" } ] }
```

`error` preenchido = música não pode ser carregada (JSON inválido, WAV ausente).
Para a aba Músicas, cada música traz também `wav` (arquivo de áudio, usado em
`analyze`), `temp` (temperatura pedida na última análise: `auto`, `quente`,
`fria`), `ai` (gerada com IA), `lyrics` (há letra guardada em `X.letra.txt`),
`voice` (há versão cantada em `X.voz.wav`), `sung` (frases da letra transcrita
na timeline) e `edited` (há edições à mão). `orphans`: arquivos `.wav` da pasta
que ainda não têm timeline (enviados sem análise); `voices`: os órfãos que já
têm versão cantada. As versões cantadas nunca aparecem como músicas.
`file` identifica a música para reordenar. `link`/`gap`: ao terminar, emenda
com a próxima depois de `gap` segundos (0 = sem nenhum corte). Ordem e emendas
ficam em `musicas/setlist.txt`.

**`timeline`** — ao conectar e ao carregar uma música: o JSON inteiro da
música (formato em `TIMELINE.md`), ou `null`.

```json
{ "type": "timeline", "edited": false, "timeline": { "song": {...}, "analysis": {...}, "cues": [...] } }
```

`edited`: a timeline foi editada (a versão gerada está guardada em `X.json.orig`).

**`songTimeline`** — a timeline de uma música mudou (edição salva, restauração
ou análise nova), carregada no player ou não. O editor da aba Músicas que
estiver com essa música aberta se atualiza (e pede a prévia de novo).

```json
{ "type": "songTimeline", "file": "praficarcomigo.json", "edited": true, "timeline": { "song": {...}, "cues": [...] } }
```

`file` é o mesmo da `setlist`. Se a música é a carregada, chega também uma `timeline`.

**`player`** — ~10×/s enquanto muda. Numa emenda, `current` muda sozinho e
chega uma `timeline` nova.

```json
{ "type": "player", "current": 0, "playing": true, "position": 32.36, "duration": 256.79,
  "section": "verso", "cue": "PISTA | POP", "next": "PISTA | CLUB", "nextAt": 88.8,
  "audio": true, "volume": 0.8, "latencyMs": 180 }
```

`position` é a do instante do envio; para animar a barra, some o tempo
decorrido desde a chegada enquanto `playing`. `audio: false` = sem saída de
som (a luz segue um relógio interno).

**`sound`** — ao conectar e sempre que o som muda (a cada ~3 s, no mínimo).

```json
{ "type": "sound", "available": true, "platform": "linux", "missing": [],
  "outputs": [ { "id": "bluez_output...", "name": "MONDIAL CM", "kind": "bluetooth", "default": true } ],
  "devices": [ { "address": "F3:E3:...", "name": "MONDIAL CM", "paired": true, "connected": true } ],
  "speaker": "F3:E3:...", "busy": "", "warning": "", "error": "" }
```

- `kind`: `bluetooth`, `wired` (P2), `builtin`, `hdmi`, `other`.
- `speaker`: caixa preferida (reconectada sozinha se cair; `warning` avisa).
- `missing`: ferramentas a instalar (no Mac: `blueutil`, `switchaudio-osx`).
- `busy`: ação em andamento (procurar/parear demoram); `error`: erro da última ação.

O `player` também traz `calibrating: true` durante a calibração de atraso.

**`jobs`** — fila de análise de músicas enviadas (ao conectar e a cada etapa).

```json
{ "type": "jobs", "ai": true,
  "items": [ { "id": 1, "file": "Nova.wav", "ai": true, "stage": "a IA está ouvindo a música",
    "temp": "quente", "voice": true, "lyrics": true, "done": false, "error": "", "warning": "" } ] }
```

`ai`: há chave do OpenRouter (mostrar a opção "Usar IA"). Em cada item, `temp`,
`voice` (a IA vai transcrever a versão cantada) e `lyrics` (a IA vai usar a
letra colada) são as opções daquela análise. Etapas: `na fila`, `lendo o
áudio`, `analisando batidas e partes`, `alinhando a versão cantada`, `a IA
está transcrevendo a letra (N frases)`, `letra transcrita: ...`, `a IA está
ouvindo a música`, `IA respondeu (...)`, `pronta` / `falhou`. `warning` =
algo ficou de fora (IA indisponível e a timeline saiu só pelas regras;
versão cantada que não bate com o playback; voz não encontrada...), com o
motivo. Ao terminar, chega um `setlist` novo.

**`event`** / **`events`** — log (`events` traz o histórico ao conectar).

```json
{ "type": "event", "time": "13:06:14", "text": "▶ MODOS DE PISTA | COLOR FLOW" }
{ "type": "events", "items": [ { "type": "event", "time": "...", "text": "..." } ] }
```

## Navegador → servidor

| Mensagem | Efeito |
|---|---|
| `{"type":"press","id":N}` | liga/desliga botão `base` ou `strobe` |
| `{"type":"flash","id":N,"down":true}` / `false` | começa/solta um flash |
| `{"type":"ping"}` | **a cada 250 ms enquanto segura flash**; sem ping por 1 s o servidor solta |
| `{"type":"blackout"}` | liga/desliga blackout |
| `{"type":"stopStrobe"}` | desliga o strobo contínuo |
| `{"type":"stopAll"}` | para tudo: camadas, flashes, manual, blackout **e a música** |
| `{"type":"manual","pos":P,"ch":C,"value":V}` | fixa canal `C` (0-based) da posição `P` |
| `{"type":"release","pos":P}` | solta os canais manuais da posição (`-1` = todas) |
| `{"type":"swap","a":P,"b":Q}` | troca canhões de posição (grava o patch) |
| `{"type":"identify","pos":P}` | acende só aquele canhão por 3 s |
| `{"type":"scan"}` | relê a pasta da setlist |
| `{"type":"reorder","files":["b.json","a.json",...]}` | nova ordem da setlist |
| `{"type":"link","str":"a.json","down":true,"t":0.5}` | emendar `a.json` com a próxima depois de `t` s (`down:false` desliga) |
| `{"type":"analyze","str":"X.wav","ai":true,"temp":"quente","lyrics":"...","noVoice":false}` | analisa (ou analisa de novo) uma faixa da pasta; `lyrics` substitui a letra guardada (`""` apaga, `null`/ausente mantém); `noVoice: true` apaga a versão cantada antes. Com IA e `X.voz.wav`, a letra é transcrita dela. A timeline existente é substituída e a música carregada passa a seguir a nova na hora |
| `{"type":"saveTimeline","str":"X.json","timeline":{...}}` | grava a timeline editada da música `X.json` (sem `str`: a carregada); se ela está tocando ou emendada, vale na hora (ver abaixo) |
| `{"type":"revertTimeline","str":"X.json"}` | volta a música para a versão gerada (`.orig`; sem `str`: a carregada) |
| `{"type":"load","id":N}` | carrega a música `N` da setlist (parada no início; **para a atual**) |
| `{"type":"play"}` / `{"type":"pause"}` | toca / pausa |
| `{"type":"stop"}` | para, volta ao início e tira a timeline da luz |
| `{"type":"seek","t":S}` | vai para `S` segundos |
| `{"type":"volume","value":V}` | volume 0–100 |
| `{"type":"latency","value":MS}` | atraso da saída de som a compensar (0–1000 ms), salvo |
| `{"type":"calibrate"}` | cliques a 100 BPM + todos os canhões piscando; `stop` encerra |
| `{"type":"sound","arg":"output","str":ID}` | troca a saída de som do sistema |
| `{"type":"sound","arg":"scan"}` | procura caixas Bluetooth (~12 s) |
| `{"type":"sound","arg":"pair"\|"connect"\|"disconnect"\|"forget","str":ADDR}` | ações na caixa |

## Editor de timeline

O editor fica na aba **Músicas** e abre qualquer música da setlist, sem
carregá-la no player do show (a aba Show só exibe a timeline). Ele lê a
timeline por `GET /api/timeline`, edita uma cópia e manda inteira em
`saveTimeline` com `str` = o `file` da música; a resposta é um
`songTimeline` para todos.
O servidor só aceita mudanças em `cues` e no `name` das partes (o resto, como
a grade e a letra transcrita, vem da versão carregada), ordena as cues, limita fritações a 10 s, valida os
presets e só então grava. Em cue de base com preset trocado, mande
`"rate": 0` (e `"name": ""`): o servidor recalcula o encaixe na batida e o nome.
Além dos `preset` dos botões, uma troca de base aceita as cores de
`config.library` e cores criadas: `"preset": "cor:#rrggbb"` (acende os 3
canhões nessa cor; o nome vira `COR | #rrggbb` se vier vazio).

## Envio de músicas (HTTP)

`POST /api/upload?name=<arquivo.wav>&analisar=0[&replace=1]` com o `.wav` no
corpo (sem multipart; use `XMLHttpRequest` para ter progresso do envio). Com
`analisar=0` o arquivo só é guardado e a resposta traz o nome final
(`{"file": "X.wav"}`); a análise vem depois pela mensagem `analyze` (com as
opções e a letra). Sem `analisar=0`, entra direto na fila
(`&ia=1&temp=quente`).

- `202` aceito.
- `409` já há uma timeline com esse nome → confirmar com a pessoa e reenviar com
  `replace=1` (a timeline é gerada de novo; edições se perdem). Um `.wav` sem
  timeline é substituído sem perguntar.
- `400` nome/arquivo inválido (não é WAV; nomes terminados em `.voz.wav` são
  reservados); `403` origem diferente da página.

**Versão cantada**: `POST /api/upload?name=<X.wav>&voz=1` com o `.wav` da
mesma faixa com a voz. Precisa do playback `X.wav` já na pasta (senão `400`);
fica como `X.voz.wav`, substituindo a anterior, e não entra na fila (a análise
vem pela mensagem `analyze`). O show nunca toca esse arquivo.

`GET /api/lyrics?file=<arquivo.wav>` devolve a letra guardada (texto puro,
vazio se não houver), para preencher o campo ao analisar de novo.

## Prévia e editor (HTTP)

A música é sempre o `file` da `setlist` (`X.json`); fora dela, `404`.

- `GET /api/timeline?file=X.json` → `{"type": "songTimeline", "file", "timeline", "edited"}`
  (o mesmo formato da mensagem).
- `GET /api/preview?file=X.json&fps=10` → a luz calculada da música, sem
  tocar no DMX (o mesmo `Program` do show, lido canhão a canhão como no
  `state`). `fps` de 1 a 30 (padrão 10; ~55 KB para 4 min):

  ```json
  { "file": "X.json", "fps": 10, "duration": 256.8,
    "positions": [ { "name": "esquerda", "colors": "0000a00000a0…", "fx": "0000…1100…22…" } ] }
  ```

  `colors`: 6 dígitos hex (`rrggbb`) por amostra; amostra `i` = instante
  `i / fps`. `fx`: um dígito por amostra, soma de `1` = strobo do canhão e
  `2` = troca rápida. Onde a cor troca mais de 3 vezes por segundo, o
  servidor já manda a cor **média** (marcada com `2`). Na faixa de cores,
  os dois aparecem listrados; no palco 2D, com ícone, e o strobo (`1`) pode
  piscar enquanto a prévia toca, **no máximo 3 vezes por segundo**.
- `GET /api/audio?file=X.json` → o `.wav` da música (`audio/wav`, aceita
  `Range`), para ouvir a prévia **no navegador** (não sai na caixa do show).
  Com `&voz=1`, a versão cantada ("Ouvir com voz"); o atraso dela em relação
  ao playback está em `timeline.generated.voice.shift` (s) e a página o
  desconta para a luz continuar no tempo.

## Regras que o frontend precisa respeitar

- Soltar todos os flashes ao perder foco/visibilidade da página e ao
  desconectar (o servidor também solta, mas assim é imediato).
- Não piscar a tela para representar strobo (segurança): use um indicador.
  A exceção é a prévia da aba Músicas: o strobo pode piscar ali, no máximo 3
  vezes por segundo, só tocando e com opção de desligar.
- Reconectar sozinho; as mensagens enviadas ao conectar reconstroem a tela.
- Pedir confirmação antes de trocar de música com outra tocando.
