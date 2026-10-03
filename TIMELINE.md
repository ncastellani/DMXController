# Formato da timeline (`musicas/<faixa>.json`)

Gerado ao lado de cada `.wav` pelo próprio `DMXController` (ao enviar a música pela
interface) ou por `DMXController analisar <pasta>`; depois editado pela interface web
(aba Músicas, com a prévia dos canhões).
Tempos sempre em **segundos desde o início do áudio**.

```jsonc
{
  "version": 1,
  "song": { "title": "praficarcomigo", "file": "praficarcomigo.wav", "duration": 256.794 },
  // temperature: cores pedidas (auto | quente | fria); lyrics: a IA usou a letra colada (X.letra.txt).
  // voice: veio da versão cantada (X.voz.wav): shift = atraso dela em relação ao playback (s),
  // clean = a subtração deixou a voz limpa, phrases = frases cantadas encontradas.
  "generated": { "at": "2026-10-02T18:14:19Z", "by": "DMXController", "model": "google/gemini-3.8-flash", "temperature": "fria",
                 "voice": { "shift": 0, "clean": true, "phrases": 38 } },

  // Como a luz deve "se comportar" nesta música. mode: presets | freestyle | misto
  "personality": { "mode": "presets", "intensity": 0.75, "palette": ["AZUL", "CIANO", "ROSA"] },

  // Leitura do LLM (null se gerado só por regras).
  "ai": { "genero": "...", "clima": "...", "descricao": "...", "bpm_percebido": 123 },

  // Letra transcrita da versão cantada (só com X.voz.wav e IA): cada frase
  // cantada, de t a end. O editor mostra numa trilha sob a faixa de cores.
  "lyrics": [{ "t": 98.6, "end": 101.9, "text": "teu vestido verde-limão" }],

  // Análise do áudio (internal/analysis, porte do librosa). Não editar à mão:
  // é a grade para encaixar cues. bpm vem do ajuste das batidas (preciso).
  "analysis": {
    "duration": 256.794,
    "bpm": 125.97,
    "beats": [1.672, 2.159, ...],          // cada batida
    "downbeats": [3.135, 5.039, ...],      // o "1" de cada compasso (4/4 estimado)
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
    { "t": 0.0, "layer": "base", "kind": "preset", "preset": "qlc:47", "name": "PISTA | CHILL", "rate": 1.0, "note": "..." },

    // Flash momentâneo (fritação) por `duration` segundos, por cima da base.
    { "t": 31.7, "layer": "flash", "kind": "preset", "preset": "qlc:99", "name": "FRITAÇÃO | CIANO", "duration": 0.975 },

    // Cor citada na letra: acende na batida da palavra (note começa com
    // "letra: “frase”") e, alguns compassos depois, a sequência da parte volta.
    { "t": 99.12, "layer": "base", "kind": "preset", "preset": "qlc:10", "name": "TODOS | VERDE LIMÃO", "note": "letra: “teu vestido verde-limão” · ..." },
    { "t": 102.93, "layer": "base", "kind": "preset", "preset": "qlc:41", "name": "PISTA | POP", "rate": 1.28, "note": "volta depois da cor" },

    // Cor criada para a música (ex.: citada na letra e sem igual na biblioteca):
    // acende os 3 canhões nessa cor.
    { "t": 62.18, "layer": "base", "kind": "preset", "preset": "cor:#c8a2c8", "name": "COR | LILÁS", "note": "letra: “no salão lilás”" },

    // Apaga a base.
    { "t": 248.5, "layer": "base", "kind": "blackout" }

    // Futuro (freestyle): { "t": ..., "layer": "base", "kind": "effect", "effect": "pulse", "params": {...} }
  ]
}
```

## Presets

`preset` é um id com prefixo de origem:

- `qlc:<ID>` — Function do workspace do QLC+ (`config/aniversario_STROBE.qxw`), o ID numérico
  dele. `name` é só para leitura humana; o id é que vale.
- `cor:#rrggbb` — cor fixa criada para esta música (pela IA, a partir da
  letra, ou no editor em "Nova cor…"). Não depende do QLC+.
- `user:<id>` — (futuro) presets criados na interface.

As cores `TODOS | X` do workspace que não são botões do console (ex.: VERDE
LIMÃO) também valem como `qlc:<ID>`: a análise e o editor as oferecem como
"biblioteca de cores".

## Análise: temperatura e letra

- **Temperatura** (`auto`, `quente`, `fria`): cada preset é classificado pela
  maioria das cores que acende (quentes: vermelho, âmbar, dourado, laranja,
  fogo, bordô, amarelo; frias: azul, ciano, verde, roxo, violeta; rosa/magenta
  não contam; só branco = neutro; minoria acima de 25% = misto). Pedindo
  quente/fria, regras e IA usam só presets daquela temperatura, mistos e
  neutros (estes só onde faltar opção — há poucos frios agitados).
- **Versão cantada** (`X.voz.wav`, a mesma faixa do playback com a voz; só a
  IA usa): o programa alinha as duas versões (correlação cruzada, até ±5 s),
  tira o playback da cantada (sobra a voz) e acha as **frases cantadas** pela
  energia da voz. A IA ouve cada frase e transcreve a letra (vai para
  `lyrics`) e marca as cores citadas com o segundo da palavra. A coreografia
  escolhe a luz de cada cor citada — uma cor fixa da biblioteca (de qualquer
  temperatura) ou uma cor nova `cor:#rrggbb` — e ela acende **na batida da
  palavra**, fica até o fim da frase (**2 a 4 compassos**) e a sequência da
  parte volta. Se a parte seguinte começar antes, ela assume. Duas cores a
  menos de 2 batidas uma da outra: só a primeira. Se a cantada não bate com o
  playback (outra gravação), ela é ignorada com um aviso. No show toca só o
  playback.
- **Letra colada** (`X.letra.txt`, opcional): com a versão cantada, só ajuda
  a IA a acertar as palavras na transcrição. Sem ela, a IA casa a letra com as
  partes e a cor citada acende **no início da parte**, por 4 compassos. O
  ajuste fino é no editor.

## Regras

- Cada `base` vale até o próximo `base` (ou `blackout`).
- `flash` é limitado a `limits.flash` do console (10 s) e o script gera no
  máximo 4 batidas, com pelo menos 20 s entre acentos.
- Presets acionados à mão durante a música ficam **por cima** da timeline e,
  ao serem desligados, a timeline volta.
- `DMXController analisar` **não sobrescreve** um JSON existente (pode ter edições); use
  `-forcar` para refazer (`-temperatura quente|fria`; usa `X.voz.wav` e
  `X.letra.txt` se existirem). Na interface, "Analisar de novo" substitui depois de confirmar.
