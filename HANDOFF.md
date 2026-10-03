# Handoff — onde paramos e o que vem a seguir

Leia junto com o `CLAUDE.md` (arquitetura e regras) e o `README.md` (o que o
programa faz). Este arquivo guarda só o estado atual e os roteiros das
próximas sessões. Última atualização: 03/10/2026 (fim da 7ª sessão).

## Estado atual

Tudo do README está pronto e testado no Linux.

**Letra e versão cantada foram removidas** (decisão da usuária, 03/10/2026,
6ª sessão: "manter só o playback e sem as letras"). Saiu tudo o que a 5ª
sessão (Sessão A) tinha feito e a letra colada da 2ª: `X.voz.wav`,
`X.letra.txt`, `analysis/vocals.go` e `lyrics.go`, `cores_citadas` e
`placeColors`, `timeline.lyrics`, `generated.voice`, `/api/lyrics`,
`upload?voz=1`, `/api/audio?voz=1`, os campos `lyrics`/`noVoice`/`voice`/
`sung`/`voices` do protocolo, e na tela os campos "Versão cantada" e "Letra",
a trilha da letra, "Cantando" e "Ouvir com voz". Não trazer de volta. Ficou o
que não depende de letra: a biblioteca de cores e `cor:#rrggbb` ("Nova cor…")
no editor. Timelines antigas com `lyrics` ou trocas "letra: …" continuam
abrindo (o campo é ignorado e some na próxima gravação; as trocas viram trocas
comuns). Se sobrar algum `X.voz.wav` numa pasta de músicas, ele passa a
aparecer como faixa "sem timeline": é só remover pela tela.

O Claude Design **não está mais em uso**: só rodar `scripts/kit_design.py`
se ela pedir (o kit em `design/` ainda descreve a letra e a versão cantada;
atualizar o `brief.md` se voltar a usar).

Na 5ª sessão: **remover música** (aba Músicas: "Remover…" no editor
e "Remover" no formulário da faixa sem timeline; mensagem `removeSong`). Os
arquivos (timeline, `.orig` e áudio) vão para `musicas/lixeira/<nome> (<data>)/` (recuperar = mover de
volta); recusa a música tocando no show ou na fila de análise, descarrega a
carregada parada. Testado (`web.TestRemoveSong`, Chrome e modo
demonstração, que também deixou de quebrar sem música carregada).

A 6ª sessão fez o **modo livre** (a IA cria a luz sem depender só dos
presets):

- **Efeitos** (`show/effect.go`): troca de base `kind: "effect"` com
  `effect {type, colors | steps + fade, beats, level}`; tipo `sequence`
  (passos escritos um a um, 3 cores por passo) e os simples `fixed`,
  `alternate`, `chase`, `pulse`, `fade`, gerados no BPM da música. Segurança: passo mínimo de
  0,34 s aplicado ao compilar (menos de 3 trocas/s em qualquer andamento,
  mesmo em arquivo editado à mão); nunca liga o strobo do canhão.
- **Análise**: opção "Efeitos: Presets | Livre" na aba Músicas (`analyze`
  com `free`; `DMXController analisar -livre`). Decisão da usuária:
  **misturar** — a IA cria um efeito por parte ou usa um preset onde ele
  servir melhor. A 1ª versão saiu "parada" (2 cores num efeito simples por
  20–40 s; ela testou em `DoceSLIA`) e a usuária pediu **variedade parecida
  com a dos presets**: agora a IA escreve sequências de 8–24 passos, 4–6
  cores, em frases que mudam, e os tipos simples ficam para momentos
  pontuais. Efeito inválido, fora da temperatura, quase todo apagado ou
  pisca-pisca é recusado e a parte fica com a sequência das regras
  (`ruleSequence`: paleta de 4 cores por grupo, ritmo pela energia; é também
  o modo livre sem IA). Acentos seguem iguais.
- **Troca nos toques** (pedido da usuária: "a cada batida da bateria ou da
  guitarra, trocar o que está rolando"): a análise grava `analysis.hits`
  (`pickHits`: picos da curva de ataques do áudio inteiro, mínimo de 0,34 s
  entre eles, batidas da grade onde a música fica parada) e os efeitos com
  `sync: "hits"` trocam de passo neles; é o padrão de tudo o que o modo
  livre gera, e no editor o seletor de passo tem "no toque". Na DoceSLIA:
  367 toques para 463 batidas, 1,6 a 2,2 trocas/s, cerca de metade em cima
  da batida. **Não conferido de ouvido**: se os toques detectados são mesmo
  os que ela percebe como batida de bateria/guitarra; afinar `delta` e as
  janelas em `pickHits` se a luz trocar em momentos estranhos.
- **Editor**: a linha do efeito tem tipo, passo e as cores (toque troca,
  "+"/"−"); na sequência, uma faixa com os passos (3 cores cada; preto =
  apagado), "Fade" e "+ passo"/"− passo"; "Novo efeito livre…" na lista de presets converte uma troca e
  "Trocar por um preset…" desfaz. A prévia e a faixa de cores já mostram os
  efeitos (mesmo `Program`).
- Testado: `show.TestEffects`, `TestEffectsNeverFlash`, `TestEffectErrors`,
  `analysis.TestFreeMode`, `TestColorTempOf`, `web.TestFreeMode`; com a IA de
  verdade em `tmp/livre/` (praficarcomigo, 1ª versão, ainda com letra) e `tmp/livre2/`
  (DoceSLIA com sequências: 7 de 6–16 passos + CLUB no refrão final,
  US$ 0,024); pela página no Chrome (computador e largura de
  iPad) e no modo demonstração, sem erros.
- **Não testado**: os efeitos nos canhões de verdade (o `pulse` usa o canal
  de dimmer do MM-T114; se a curva dele for brusca, o pulso pode parecer
  degrau).

A 7ª sessão **tirou o QLC+ do programa** (pedidos: primeiro "converter os
presets para efeitos para a IA usar também, uma paleta de cores e uma
pontuação de mood pela IA"; depois "incorporar esses presets para eliminar a
relação com o arquivo qxw e o QLC"):

- **Biblioteca** (`internal/library`, `config/biblioteca.yaml`): 29 cores,
  11 strobos (7 de fritação + 4 contínuos) e 32 modos de pista, em texto
  editável, com cor crua + dimmer, duração e fade de cada passo. A conversão
  do `.qxw` foi conferida antes de apagá-lo: os 72 presets geram DMX
  idêntico ao do QLC+ (604.892 quadros comparados, de 1 em 1 ms por dois
  ciclos). Apagados: `internal/qlc`, `config/aniversario_STROBE.qxw` e o
  conversor (o `.qxw` continua no git e em `~/Downloads/qlc/`).
- `config/console.yaml` aponta para os ids da biblioteca (`preset:
  "pista:club"`); `config/fixtures.yaml` ganhou `strobe_mode: 50`. O quadro
  lógico agora é montado pelo patch (1, 8, 15), sem depender do workspace.
- Ids antigos `qlc:NN` valem como `alias`; a timeline passa ao id novo
  quando é aberta no editor e salva.
- **Clima**: a IA avaliou cada modo de pista (energia, alegria, tensão,
  romance de 0 a 1 + rótulo; US$ 0,012). `DMXController biblioteca` avalia
  os que ainda não têm (`-reavaliar` refaz todos).
- **Modo livre**: a IA vê o desenho de cada modo (paleta em letras +
  passos), o clima e a paleta de cores, e em cada parte escreve uma
  sequência, usa um preset, leva um modo de pista como efeito ou mantém o
  desenho e troca as cores (`"modelo"` + `"cores"`). Devolve o clima da
  música (`personality.mood`). Regras (sem IA ou parte recusada): modo de
  pista de energia/clima mais parecido, sem repetir e na mesma família de
  cor.
- **Preset × efeito**: como preset, o modo de pista toca com os tempos dele
  (rajadas de 90–200 ms inclusive); como efeito, um passo por toque e nunca
  3 trocas/s (regra de fotossensibilidade do modo livre), por isso a FLASH
  só existe como preset. Se ela quiser as rajadas também nos efeitos, é uma
  mudança dessa regra: perguntar antes.
- **Sem extremos** (pedido dela ao ver a DoceAI em 2:43.9, o CLUB como
  efeito com "branco total → apagado" de meio segundo cada): `soften` tira
  dos efeitos gerados os passos todos brancos ou apagados, sobe os tons
  escuros e põe a cor do passo, fechada, no canhão apagado; o pedido à IA
  também mudou. `musicas/DoceAI.json` foi corrigida no lugar (cópia de antes
  em `tmp/sem-extremos/`). Os **presets** (botões do console e `kind:
  "preset"` nas timelines, como a `Doce.json`) continuam com as rajadas de
  branco/apagado do original: se ela achar sem graça também ali, o caminho é
  editar os passos em `config/biblioteca.yaml`. A intensidade (`level`) das
  partes calmas (0,5–0,6) também não mudou.
- **CLUB sem as rajadas** (pedido dela): tirei de `pista:club` na biblioteca
  os dois blocos "branco → apagado → branco → apagado" (24 → 16 passos); é
  o primeiro preset que deixou de ser igual ao do QLC+. BATIDÃO, CINEMA,
  IMPACTO e outros ainda têm blocos assim: só mexer se ela pedir.
- **Modo misto** (pedido dela: "uma opção que combine presets e efeitos"):
  terceiro botão em Efeitos (Presets | Misto | Livre), mensagem `analyze`
  com `mix`, `DMXController analisar -misto`. Presets e efeitos na mesma
  música, cada um em pelo menos um terço das partes: a IA é orientada a
  alternar e `balance` corrige se ela mandar um tipo só (grupos mais
  agitados → presets, mais calmos → efeitos); sem IA, a metade mais agitada
  dos grupos fica com presets. Testado: `analysis.TestMixedMode`; com a IA
  em `tmp/misto/ia/` (Doce: 4 efeitos e 4 presets alternados, US$ 0,023) e
  por regras em `tmp/misto/regras/`; botão e etiqueta "misto" no Chrome.
  Por regras e com cores em "Automático", os presets podem misturar quentes
  e frios (já era assim no modo Presets): escolher a temperatura resolve.
- **Atenção à batida** (pedido dela: "no meio de um verso tem uma batida de
  bateria, ou alguma coisa de funk, eu queria que a luz acompanhasse essas
  nuances"; exemplos dela: Doce e "Caso de Amor"). O que os dados mostraram:
  a bateria entrava no meio de uma parte e a luz não mudava; a "energia" era
  só volume (o verso de trap da Caso de Amor, 1:20–1:43, tem a percussão
  mais forte da música e volume baixo); e um toque de destaque era igual a
  qualquer outro. Feito (`analysis/pulse.go`):
  - força percussiva de cada toque (`analysis.hit_strength`) e **batida**
    de cada parte (`sections[].pulse`);
  - **partes divididas** onde a percussão muda de patamar (grupos `X+` /
    `X-`): a Doce foi de 8 para 11 partes (respiros antes do refrão, bateria
    entrando no verso 2 em 1:24), a Caso de Amor de 6 para 7;
  - a agitação (regras) segue volume + batida, e a IA recebe a batida de
    cada parte com a orientação de mudar a luz nos trechos `+`/`-`;
  - **realce** nos toques em destaque (`analysis.accents`, ~20 por música):
    a luz de base clareia e volta em 240 ms, sobre preset ou efeito.
  Testado: `analysis.TestSplitByPulse`, `TestPickAccents`,
  `show.TestAccentBump`; com a IA nas duas músicas em `tmp/batida/` (misto).
  **Não conferido de olho nem de ouvido**: se as divisões caem onde ela
  ouve a mudança e se o realce agrada (é comportamento visual novo; os
  ajustes são `splitRatio`, `accentRatio` em `pulse.go` e `accentBump` em
  `show/timeline.go`). As músicas já analisadas só ganham isso com "Analisar
  de novo". Limite que continua: os efeitos trocam no máximo ~1 vez por
  batida (0,34 s, regra de segurança), e os presets de pista seguem o tempo
  deles, não os toques.
- **Picos com menos ruído e luz até o fim** (retorno dela vendo a Caso de
  Amor: "um branco no canhão do centro, não está ruim, mas está pegando
  muitas variações… muito ruído"; e "a função apagar ocorre antes de a
  música acabar"):
  - o branco vinha do realce (19 picos na música, 10 só na intro) e de um
    passo `vermelho, branco, vermelho` escrito pela IA. Agora pico = toque
    ≥ 2× os vizinhos, com 4 batidas de espaço (Caso de Amor: 19 → 5;
    Alinhamento Milenar: 16 → 6), e o realce virou o que ela descreveu:
    **flash branco no canhão do centro**, lados só clareiam. O branco solto
    saiu das sequências geradas e do pedido à IA;
  - o apagar adiantado era erro de escala de tempo na curva de energia
    (2%) mais um limite de silêncio alto demais. Agora a luz vai até
    `sound_end` e apaga em fade na cauda (`fade` no blackout; a página
    mostra "apaga aos poucos (N s antes)");
  - as 3 músicas de `musicas/` foram atualizadas no lugar, sem refazer a
    luz (cópias de antes em `tmp/picos-e-fim/`): Caso de Amor apagava em
    2:33,5, agora 2:37,0 (fim seco); DoceSL 3:06 → 3:14,6 com fade de
    1,9 s; Alinhamento Milenar nunca apagava, agora 3:23,1 com fade de
    3,9 s. A DoceSL (análise antiga) ganhou os picos, mas não a divisão
    das partes pela batida: só com "Analisar de novo".
  Testado: `TestPickAccents` (com `soundEnd`), `TestAccentBump`,
  `TestFadeOut`, `TestSoften`. **Não conferido de olho**: se 5–6 picos por
  música é a medida certa (`accentRatio` em `pulse.go`) e se o flash no
  centro tem a força que ela quer.
- **Ouvido de bateria** (pedido dela: "use mais alguma outra lib para
  perceber mais nuances, mas vamos trabalhar mais um pouco na percepção das
  batidas"). Escolhi separação de instrumentos (**Demucs**, htdemucs, só
  CPU) porque mostrou onde a análise errava: na DoceSL só 46% dos toques
  eram bateria (o resto, voz e sintetizador); o pré-refrão marcava batida
  0,77 sem ter bateria; a intro da Caso de Amor (sem bateria) dava 10
  flashes.
  - Instalação: `scripts/instalar_ouvido.sh` → `.ouvido/` (uv + Python 3.12
    próprio + torch CPU + demucs 4.1 + modelo; 1,2 GB; nada fora da pasta; o
    Python do sistema aqui é 3.14 e não tem venv). **Já instalado nesta
    máquina.** Separação: 40–53 s por música, 1,7 GB de RAM; cache por
    conteúdo em `.ouvido/bateria/` (as 3 músicas dela já estão lá).
  - Análise com a bateria: toques em bumbo/caixa, `hit_kind`, batida real
    por parte, fronteiras grudadas na entrada/saída da bateria, picos =
    entrada da bateria (drop), toque solto, toque 2× os vizinhos.
  - Página: etapa "separando a bateria (N%)" na fila e aviso no formulário
    (`jobs.ear`).
  - Testado: `TestAnalyzeDrums` (música sintética), `TestSeparator` e
    `TestGenerateWithEar` (Python de mentira; falha → aviso e segue sem), a
    suíte toda; de verdade nas 3 músicas (`tmp/ouvido/novo/`, misto com IA,
    ~58 s cada) e pela página no Chrome.
  - De quebra: a fila só marca "pronta" depois de a música entrar na
    setlist (era uma corrida que aparecia com a máquina carregada).
  - **Não conferido**: de olho/ouvido (se bumbo × caixa estão certos, se as
    divisões e os picos caem onde ela ouve); instalação do zero pelo script
    (rodei sobre a pasta já montada à mão com os mesmos comandos); no
    Raspberry Pi (tempo e memória; torch tem versão para ele).
  - As músicas de `musicas/` **não** foram alteradas: ganham isso com
    "Analisar de novo" (a separação delas já está em cache).
  - Ideias seguintes, ainda não feitas: usar bumbo × caixa na luz (ex.: a
    luz "bombear" no bumbo), detectar viradas e subidas, ouvir também o
    baixo (o modelo já separa baixo, voz e resto).
- **Editor**: grupo "MODOS DE PISTA COMO EFEITO" no seletor (com o rótulo
  do clima); os passos entram na música e são editáveis.
- Testado: `library.TestShowLibrary` (amostras do DMX original),
  `TestTone`, `TestParseAndSave`, `analysis.TestLibraryEffect`,
  `TestLibraryChoices` e o resto da suíte com os ids novos; no Chrome:
  console com os 55 botões, cor e modo de pista acendendo, timeline antiga
  com `qlc:NN` abrindo e sendo salva com os ids novos, troca por efeito, sem
  erros. Com a IA de verdade, já sem o QLC+, em `tmp/semqlc/ia/` (DoceSLIA:
  6 sequências escritas, AZUL ROSA e BATIDÃO com cores novas, 3 fritações
  com os ids novos, US$ 0,028).
- **Não conferido**: nada disso nos canhões de verdade (o DMX é igual ao de
  antes byte a byte, então o risco é baixo); o modo demonstração
  (`demo.js`) ainda usa ids `qlc:` nos dados de exemplo dele; se as notas de
  clima batem com o gosto dela (dá para corrigir à mão no YAML).
- Falta, se ela quiser: criar/editar presets da biblioteca pela tela (hoje
  é só no arquivo).

Sessões anteriores: B (4ª) = prévia 2D com duas vistas, faixa de cores,
editor de qualquer música na aba Músicas, Show só leitura, partes
enumeradas; antes disso, DMX, console, player, aba Músicas, luz apagada por
padrão.

**Não testado**: a prévia com som no **iPad/Safari**. Há mudanças ainda
**não commitadas** (remover música, modo livre, remoção da letra, biblioteca e remoção do QLC+): perguntar
antes de commitar.

## Pendências pequenas

- Faltam **modos de pista frios agitados** (hoje só há o CLUB): escrever
  2–3 direto em `config/biblioteca.yaml` (ou pedir ao Amós).
- Em aberto no equipamento (do HANDOVER do Amós): CH6 = 50 está na fronteira
  das faixas do canal de função (se o strobo parar, testar 60–70); CH7
  252–255 podem dar a mesma velocidade; as cores quentes LARANJA QUEIMADO,
  FOGO, ÂMBAR ESCURO, ÂMBAR, ÂMBAR DOURADO e DOURADO talvez não se distingam
  ao vivo; BORDÔ pode parecer vermelho; `PISTA | ÂMBAR 200` troca 5×/s.
- Ideia aceita, ainda não feita: criar presets na tela (salvar um efeito
  livre como preset reaproveitável entre músicas). Agora o lugar natural é a
  biblioteca: gravar como `fx:<nome>` sem `source` em
  `config/biblioteca.yaml` (o `Merge` já preserva esses ao reconverter).
- `musicas/praficarcomigo.json` continua no modo de presets; a versão livre
  de teste está em `tmp/livre/` (para usar, analisar de novo pela aba
  Músicas com "Livre").

## Próximas sessões (uma por chat)

Ordem sugerida: **C** (usa o editor e a prévia da aba Músicas); **D** quando
o Pi chegar.

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
  - o catálogo de presets com temperatura e o pedido.
- O LLM devolve **operações** em JSON (adicionar, remover ou alterar trocas;
  renomear partes), não um arquivo livre.
- As operações também podem criar/alterar **efeitos livres** (`kind:
  "effect"`), validados por `cleanEffect`.
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
- **Ouvido de bateria no Pi** (opcional): `scripts/instalar_ouvido.sh`
  baixa a versão aarch64; medir tempo por música e memória (aqui: 1,7 GB).
  Se pesar, analisar em casa no computador e copiar `musicas/` para o Pi (a
  timeline já leva tudo; o Pi não precisa do ouvido para tocar).
- Raspberry Pi OS 64 bits (Bookworm) com desktop: já traz PipeWire e
  WirePlumber, que o pacote `sound` usa. Na versão Lite, instalar
  `pipewire wireplumber pipewire-pulse` e `loginctl enable-linger`.
- Iniciar sozinho: serviço systemd **de usuário** (o áudio é da sessão do
  usuário; não usar root). Regra udev da SL1000 como no README.
- Testar no aparelho: DMX (tempo do BREAK no FTDI), latência e calibração do
  som (Bluetooth e USB), reconexão da caixa, tempo da análise de uma música e
  uso de memória.
- Com isso, as pendências do Mac (`brew`, CoreAudio) viram opcionais.
