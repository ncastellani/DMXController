# Formato da timeline (`musicas/<faixa>.json`)

Gerado ao lado de cada `.wav` pelo próprio `DMXController` (ao enviar a música pela
interface) ou por `DMXController analisar <pasta>`; depois editado pela interface web
(aba Músicas, com a prévia dos canhões).
Tempos sempre em **segundos desde o início do áudio**.

```jsonc
{
  "version": 1,
  "song": { "title": "praficarcomigo", "file": "praficarcomigo.wav", "duration": 256.794 },
  // temperature: cores pedidas (auto | quente | fria); free: analisada no
  // modo livre (efeitos criados para a música).
  "generated": { "at": "2026-10-02T18:14:19Z", "by": "DMXController", "model": "google/gemini-3.8-flash", "temperature": "fria", "free": true },

  // Como a luz ficou nesta música. mode: presets | misto (presets e efeitos) | freestyle (só efeitos)
  "personality": { "mode": "presets", "intensity": 0.75, "palette": ["AZUL", "CIANO", "ROSA"] },

  // Leitura do LLM (null se gerado só por regras).
  "ai": { "genero": "...", "clima": "...", "descricao": "...", "bpm_percebido": 123 },

  // Análise do áudio (internal/analysis, porte do librosa). Não editar à mão:
  // é a grade para encaixar cues. bpm vem do ajuste das batidas (preciso).
  "analysis": {
    "duration": 256.794,
    "bpm": 125.97,
    "beats": [1.672, 2.159, ...],          // cada batida
    "downbeats": [3.135, 5.039, ...],      // o "1" de cada compasso (4/4 estimado)
    // Toques fortes da música (ataques de bateria, guitarra, entradas), com pelo
    // menos 0,34 s entre eles; os efeitos livres com sync "hits" trocam de passo aqui.
    "hits": [1.672, 2.159, 2.531, ...],
    "energy": { "hop": 0.5, "values": [0.01, 0.03, ...] },  // 0..1 a cada 0,5 s
    "sections": [
      { "start": 0.0, "end": 31.7, "bars": 16, "energy": 0.08, "group": "A", "name": "intro" }
      // group: partes parecidas recebem a mesma letra (ex.: refrões).
      // name: as que se repetem saem numeradas ("verso 1", "refrão 2"); trechos
      // vizinhos com o mesmo nome levam o mesmo número.
    ]
  },

  // O que a luz faz. Ordenado por t; no mesmo instante, a fritação vem antes
  // da base (ela acaba em poucas batidas e a base fica valendo).
  "cues": [
    // Troca a camada base para um preset (sequência ou cor). rate acelera/
    // desacelera a sequência para os passos caírem na batida (1 = original).
    { "t": 0.0, "layer": "base", "kind": "preset", "preset": "pista:chill", "name": "PISTA | CHILL", "rate": 1.0, "note": "..." },

    // Flash momentâneo (fritação) por `duration` segundos, por cima da base.
    { "t": 31.7, "layer": "flash", "kind": "preset", "preset": "fritacao:ciano", "name": "FRITAÇÃO | CIANO", "duration": 0.975 },

    // Cor criada para a música (no editor, em "Nova cor…"): acende os 3
    // canhões nessa cor.
    { "t": 62.18, "layer": "base", "kind": "preset", "preset": "cor:#c8a2c8", "name": "COR | LILÁS" },

    // Modo livre: um efeito criado para a música no lugar do preset (só na base).
    // Quase sempre uma sequência escrita passo a passo (as 3 cores dos canhões em cada passo).
    { "t": 55.38, "layer": "base", "kind": "effect", "name": "LIVRE | SEQUÊNCIA",
      "effect": { "type": "sequence", "sync": "hits", "beats": 1, "level": 0.9,
                  "steps": [["#ff2878", "#00ffff", "#ff2878"], ["#00ffff", "#ff0064", "#00ffff"], ["#7800ff", "#000000", "#000000"]] } },
    { "t": 125.04, "layer": "base", "kind": "effect", "name": "LIVRE | CORRIDA",
      "effect": { "type": "chase", "colors": ["#00ffff", "#7800ff"], "beats": 1, "level": 0.8 } },

    // Apaga a base.
    { "t": 248.5, "layer": "base", "kind": "blackout" }
  ]
}
```

## Presets

`preset` é um id com prefixo de origem:

- um id da **biblioteca** (`config/biblioteca.yaml`): `pista:club` (modo de
  pista), `todos:azul` (cor fixa nos 3 canhões), `fritacao:branco` e
  `strobe:branco-254` (strobo nativo do canhão). `name` é só para leitura
  humana; o id é que vale.
- `cor:#rrggbb` — cor fixa criada para esta música (no editor, em "Nova
  cor…").
- ids antigos `qlc:<número>` (do tempo em que os presets vinham do QLC+)
  continuam abrindo: cada preset da biblioteca guarda o id antigo em `alias`,
  e a timeline passa a usar o id novo na primeira vez que é salva.

As cores da biblioteca que não são botões do console (ex.: VERDE LIMÃO)
aparecem para a análise e o editor como "biblioteca de cores".

## A batida dentro das partes

Além das batidas e dos toques, a análise mede a percussão. Há dois jeitos:
com o **ouvido de bateria** (`analysis.drums: true`), ela ouve a bateria
separada do resto da música; sem ele, estima pela parte percussiva da música
inteira (voz e outros instrumentos entram na conta).

Com o ouvido:

- `hits` são, antes de tudo, os toques de **bumbo e caixa** (do mais forte
  ao mais fraco, a pelo menos 0,34 s um do outro); o chimbal e os ataques
  dos outros instrumentos só preenchem buracos de quase uma batida, e onde
  não há bateria valem os ataques da música, como sem o ouvido.
- `hit_kind`: a peça de cada toque, um caractere por toque (`k` bumbo, `s`
  caixa ou palma, `h` chimbal ou prato, `-` não é bateria).
- `sections[].pulse` é o nível real da bateria na parte (0,01 = não toca).
- A fronteira de uma parte que cai a menos de 2 compassos de uma entrada ou
  saída da bateria é levada para ela; mais longe que isso, a parte é
  dividida ali.
- Os picos (`accents`) são sempre toques de bateria: a **entrada da
  bateria** depois de 2 compassos ou mais sem ela (o drop), um toque
  **solto** num trecho sem bateria e um toque com o dobro da força dos
  toques de bateria vizinhos.

Nos dois casos:

- `analysis.hit_strength`: a força percussiva de cada toque de `hits`, de 0
  a 1.
- `sections[].pulse`: a **batida** da parte (a força típica dos toques
  dela, 0 a 1). A agitação da luz segue o volume (`energy`) e a batida
  juntos: um verso só de beat e voz tem volume baixo e batida alta.
- Uma parte em que a percussão muda de patamar no meio (a bateria entra, ou
  some nos compassos antes do refrão) é **dividida** ali, na batida. O
  pedaço maior fica com o grupo da parte e o menor ganha `+` (percussão mais
  forte) ou `-` (mais fraca): `C` e `C-`. Cada pedaço tem a sua luz.
- `analysis.accents`: os **picos**, toques pelo menos 2× acima dos vizinhos
  (uma virada, uma batida solta no meio do verso), com 4 batidas ou mais
  entre dois: `[{"t": 84.5, "v": 0.8}]`. Neles o canhão do centro dá um
  **flash branco** (força `v`) e os dos lados clareiam de leve, voltando à
  luz de base em 280 ms, seja ela preset ou efeito. Não acontece com tudo
  apagado, não liga strobo e dois flashes ficam a pelo menos 1 s. Para tirar
  os flashes de uma música, apague `accents` do arquivo. O branco fica
  reservado a esses picos: as sequências geradas não têm branco solto.
- `analysis.sound_end` e `fade_out`: onde o som acaba e quanto dura a cauda
  final. O `blackout` do fim fica em `sound_end` com `"fade": N`: a luz
  apaga aos poucos nos N segundos antes dele (qualquer blackout aceita
  `fade`, até 30 s).

## Efeitos (modo livre)

Uma troca de base com `"kind": "effect"` não usa preset: o programa gera os
quadros a partir de `effect` (`internal/show/effect.go`), sempre no andamento
da música (`analysis.bpm`), contando os passos a partir do `t` da troca.

| `type` | O que faz | Cores |
|---|---|---|
| `sequence` | passos escritos um a um em `steps`, tocados em loop, como os modos de pista da biblioteca; com `"fade": true`, cada passo se transforma no seguinte (metade do passo em fade) | 2–32 passos de 3 cores |
| `fixed` | cores paradas: 1 cor = os 3 canhões iguais; 3 = esquerda, centro, direita | 1–4 |
| `alternate` | os canhões trocam de cor entre si a cada passo | 2–4 |
| `chase` | a 1ª cor corre de um canhão ao outro (vai e volta) sobre a 2ª; com 1 cor, os outros ficam apagados | 1–2 |
| `pulse` | as cores acendem forte no passo e caem suave até 30% (não apaga) | 1–4 |
| `fade` | as cores se transformam uma na outra, um passo por cor | 2–4 |

- `name` (opcional): o efeito da biblioteca de onde a sequência veio
  (`"CLUB"`, `"CLUB (cores novas)"`); a troca aparece como `EFEITO | CLUB` em
  vez de `LIVRE | SEQUÊNCIA`. Os passos ficam copiados na música: mudar a
  biblioteca depois não muda esta troca.
- `colors`: `"#rrggbb"`, na ordem dos canhões (não se usa em `sequence`).
- `steps` (só `sequence`): cada passo é `[esquerda, centro, direita]`;
  `"#000000"` = canhão apagado naquele passo. Um passo com 1 cor vale para os
  3 canhões; com 2, laterais e centro. Repetir um passo segura o quadro.
- `beats`: duração de cada passo em batidas (`0.5`, `1`, `2`, `4`, `8`;
  padrão 1; outros valores vão para o mais próximo). **Segurança**: o passo
  nunca fica menor que 0,34 s (menos de 3 trocas por segundo); se ficar, dobra
  até caber (a 126 BPM, meia batida vira 1 batida). Um efeito nunca liga o
  strobo do canhão.
- `sync: "hits"`: o passo troca **a cada toque da música** (`analysis.hits`),
  no lugar de `beats`. Os toques vêm dos ataques do áudio inteiro (bateria,
  palhetada, entrada de acorde): picos locais da curva de ataques; entre dois
  a menos de 0,34 s fica o mais forte, e num trecho sem toque por mais de 4
  batidas entram batidas da grade de 2 em 2, para a luz não congelar. No
  `pulse` a luz acende no toque e cai até o seguinte; com `fade`, a transição
  ocupa a metade do intervalo. Timeline antiga sem `hits`: valem as batidas.
  É o padrão do que a análise gera no modo livre (pedido da usuária); no
  editor, o seletor de passo tem "no toque" e as durações em batidas.
- `level`: intensidade de 0,2 a 1 (omitido = 1).
- `name` vazio vira `LIVRE | <TIPO>`; `rate` e `preset` não se aplicam.

Na análise, o **modo livre** (`-livre` / "Livre" na aba Músicas) deixa a IA
criar um efeito para cada parte ou usar um preset onde ele servir melhor (os
dois se misturam na mesma música). A IA é orientada a escrever **sequências**
(`sequence`) com a riqueza dos presets de pista: 8 a 24 passos, 4 a 6 cores,
montadas em "frases" de 2 a 4 passos que mudam (rodízio de 3 cores, dois tons
alternando, um canhão por vez, todos iguais, um toque de branco ou um passo
apagado); os tipos simples ficam para momentos pontuais. São recusados (e a
parte fica com a sequência das regras): cores fora da temperatura pedida,
efeito simples com cor escura, sequência com menos da metade dos canhões
acesos na média ou com mais de 1/4 dos passos todo apagado. Sem IA, as regras
dão uma paleta de 4 cores a cada grupo de partes e montam a sequência pela
energia (calma: 6 passos em fade, um canhão mudando de cada vez; média: 10
passos; agitada: 16 passos). Tudo o que o modo livre gera troca de passo nos
toques (`sync: "hits"`).
Acentos (fritação) funcionam como no modo de presets. No editor, "Novo efeito livre…" transforma qualquer troca de base
num efeito e "Trocar por um preset…" desfaz.

## Análise: temperatura

- **Temperatura** (`auto`, `quente`, `fria`): cada preset é classificado pela
  maioria das cores que acende (quentes: vermelho, âmbar, dourado, laranja,
  fogo, bordô, amarelo; frias: azul, ciano, verde, roxo, violeta; rosa/magenta
  não contam; só branco = neutro; minoria acima de 25% = misto). Pedindo
  quente/fria, regras e IA usam só presets daquela temperatura, mistos e
  neutros (estes só onde faltar opção — há poucos frios agitados).

## Regras

- Cada `base` vale até o próximo `base` (ou `blackout`).
- `flash` é limitado a `limits.flash` do console (10 s) e o script gera no
  máximo 4 batidas, com pelo menos 20 s entre acentos.
- Presets acionados à mão durante a música ficam **por cima** da timeline e,
  ao serem desligados, a timeline volta.
- `DMXController analisar` **não sobrescreve** um JSON existente (pode ter edições); use
  `-forcar` para refazer (`-temperatura quente|fria`, `-livre`). Na interface, "Analisar de novo" substitui depois de confirmar.

## Biblioteca (`config/biblioteca.yaml`)

Todos os presets do programa, num arquivo de texto editável (o programa lê
ao iniciar; com um erro no arquivo o programa não inicia e diz o que está errado).
Os botões do console (`config/console.yaml`) apontam para ids daqui.

- **Tom**: `#rrggbb` (dimmer no máximo; `#000000` = apagado) ou
  `#rrggbb@180` (a cor com o dimmer em 180 de 255).
- `colors`: cores fixas nos 3 canhões (`todos:<nome>`).
- `strobes`: strobo nativo do canhão numa cor; `speed` é o canal de
  velocidade (o valor do canal de função vem de `strobe_mode` no perfil, em
  `config/fixtures.yaml`). São a fritação e o strobo contínuo do console.
- `effects`: modos de pista (`pista:<nome>`). `steps` tem um passo por linha
  com os tons de esquerda, centro e direita, tocados em loop; `350ms` no
  passo é a duração só dele (sem isso vale `step_ms`) e `fade900ms` faz o
  passo entrar em fade a partir do anterior. `agitation` e `temp` orientam a
  análise; `mood` é o clima de 0 a 1 dado pela IA (`DMXController
  biblioteca`): `energy` (parado → explosivo), `joy` (sombrio → alegre),
  `tension` (tranquilo → dramático), `romance` (frio → íntimo) e um `label`.
- `alias`: o id antigo do QLC+ (`qlc:43`), para as timelines já gravadas.

Um modo de pista pode entrar numa música de dois jeitos:

| | Como **preset** (`kind: "preset"`) | Como **efeito** (`kind: "effect"`) |
|---|---|---|
| Passos | os da biblioteca, lidos na hora | copiados para a timeline, editáveis |
| Tempo | o de cada passo, como foi escrito (inclusive rajadas de 90–200 ms), ajustado ao BPM por `rate` | um passo por toque da música; os lentos (passo ≥ 1 s: CHILL, CINEMA, COLOR FLOW) seguem o passo original em batidas |
| Limite de 3 trocas/s | não se aplica (preset do iluminador) | sempre (`MinEffectStep`) |
| Cores | as da biblioteca, com dimmer | as emitidas (`#rrggbb`); a paleta pode ser trocada |

A FLASH só existe como preset: é um pisca-pisca, e efeito não pisca.

**Sem extremos**: todo efeito que a análise gera (escrito pela IA, pelas
regras ou vindo de um modo de pista) sai sem passos com os 3 canhões brancos
e sem passos apagados; tons escuros sobem até metade do brilho (maior canal
em 128) e um canhão apagado fica com a cor do passo, fechada. O que for
editado à mão no editor não é alterado.

No **modo livre**, a IA recebe o desenho de cada modo de pista (`paleta
A=#0000a0 B=#7800ff ... | passos: ABC CAB BCA ...`, `W` = branco, `-` =
apagado), o clima e a paleta de cores, e pode, em cada parte: escrever uma
sequência, usar um preset, levar um modo de pista como efeito (`"efeito":
{"modelo": "pista:club"}`) ou manter o desenho e **trocar a paleta**
(`"cores": [...]`, uma cor por letra; modos com mais de 8 cores são degradês
e não aceitam troca). Ela também dá o clima da música na mesma escala
(`personality.mood`). As partes que ficam com as regras (ou a análise sem
IA) pegam o modo de pista de energia e clima mais parecidos, na temperatura
pedida, sem repetir e sem misturar famílias de cor. No editor, "MODOS DE
PISTA COMO EFEITO" troca uma parte por um deles.
