# Handoff — onde paramos e o que vem a seguir

Leia junto com o `CLAUDE.md` (arquitetura e regras) e o `README.md` (o que o
programa faz). Este arquivo guarda só o estado atual e os roteiros das
próximas sessões. Última atualização: 02/10/2026 (fim da 4ª sessão).

## Estado atual

Tudo do README está pronto e testado no Linux. A 4ª sessão fez a **Sessão B
— prévia 2D e editor na aba Músicas**:

- **Editar qualquer música sem carregá-la no show**: tocar numa música da
  aba Músicas abre o editor (lista de trocas, Selecionar, cores, Nova cor…,
  Desfazer, Restaurar gerada, nomes das partes). `saveTimeline` e
  `revertTimeline` aceitam `str` = o `X.json`; a resposta é a mensagem nova
  `songTimeline` para todos os navegadores. Se a música está carregada (ou
  já emendada no fluxo), a edição vale na hora (`Show.Update(path, t)`).
  Edição feita sem conexão fica pendente e sai ao reconectar.
- **Prévia**: `GET /api/preview` compila o `Program` e lê cada canhão com
  `Look` (`show.RenderPreview`, 10 amostras/s; ~55 KB e 3 ms por música).
  Na página, **palco 2D** (treliça, 3 canhões com facho e poça de luz) e
  **faixa de cores** com uma trilha por canhão sob a barra da música; um
  cursor único, tocar/arrastar pula.
- **Segurança**: onde a cor troca mais de 3×/s (CLUB, BATIDÃO, FLASH,
  IMPACTO, SURTO QUENTE, VERMELHO BRANCO 2, CAOS QUENTE, BRANCO VERMELHO), o
  servidor manda a cor média com a marca "troca rápida"; strobo nativo
  (fritações e strobos) vem marcado. Os dois aparecem parados, com ícone
  (⚡ / ≋) no palco e listrado na faixa. Medido no navegador: ≤ 1,3 troca
  forte/s na prévia do CLUB.
- **Ouvir no aparelho**: `GET /api/audio` (WAV com Range) num `<audio>`; a
  prévia pausa ao sair da aba. Sem áudio (modo demonstração ou navegador
  que não toca), segue um relógio sem som.
- Pedidos da usuária no fim da sessão: palco 2D com duas vistas
  ("Pendurados", o padrão, e "No chão": esquerda mira à direita, direita à
  esquerda, centro para a plateia); strobo/fritação **piscam na prévia**
  a 2,9×/s no máximo, só tocando, com opção de desligar (o console ao vivo
  continua sem piscar); trocas que já passaram ficam mais escuras nas
  listas; "Marcar" virou "Selecionar"; a análise **enumera as partes que
  se repetem** ("verso 1", "refrão 2"; trechos vizinhos com o mesmo nome
  levam o mesmo número; nomes únicos como "intro" ficam sem número). As
  músicas já analisadas só ganham os números ao analisar de novo. A lista
  da aba Show ganhou "Aqui" (leva o show até a troca) e as barras da música
  (Show e Músicas), uma régua de tempo. A mira vem da posição (pontas
  cruzam, meio para a frente), não de configuração.
- **Aba Show só exibe** a lista de trocas (acompanhando a música);
  "Editar" leva ao editor da aba Músicas com a música carregada.
- `demo.js`, `PROTOCOLO.md`, `design/brief.md`, README e CLAUDE.md
  atualizados; o kit do Claude Design ganhou prints do editor
  (`computador-musicas-editor.png`, `ipad-musicas-editor.png`).
- Testes: `show.TestPreview` e `web.TestEditSongNotLoaded` (endpoints,
  edição de música não carregada, Range, 404 fora da setlist).

Não testado: a prévia com som no **iPad/Safari** (o WAV deveria tocar; se
não tocar, a prévia anda sem som e avisa). **O primeiro commit ainda não
foi feito** (`git init` feito na 3ª sessão).

## Pendências pequenas

- **Pergunta em aberto para a usuária**: hoje a cor citada na letra vale a
  **parte inteira** (ex.: refrão de 60 s todo em lilás parado). Alternativa:
  a cor por 2–4 compassos e depois a sequência de pista da parte. A Sessão A
  muda isso de qualquer forma (a cor passa a entrar na hora do verso).
- Colar as letras reais e refazer `praficarcomigo.json` pela aba Músicas. O
  atual ainda é do script Python antigo (BPM 123,05; o real é 126).
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

A Sessão B está feita (acima). Ordem sugerida: **C** (usa o editor da B);
**A** é independente; **D** quando o Pi chegar.

### Sessão A — versão cantada para acertar o tempo das cores

Pedido: enviar, além do playback, a mesma faixa **com o Amós cantando** (mesma
duração e mesmo tempo), para que a análise saiba **em que segundo** cada verso
é cantado. Exemplo: o "verde-limão" de *Doce* (Jão) acender na palavra, não
no começo da parte.

- Guardar como `X.voz.wav` ao lado de `X.wav` (o show continua tocando só o
  playback). Na aba Músicas, um segundo campo de arquivo opcional: "Versão
  cantada".
- Conferir o encaixe: mesma taxa e duração (±50 ms); estimar o deslocamento
  por correlação cruzada entre as duas versões e corrigir se for pequeno.
- **Isolar a voz por subtração**: como o instrumental é idêntico,
  `voz = cantada − playback` dá a voz quase limpa (só funciona se for a mesma
  mixagem/master; se o resíduo ficar alto, cair para o plano B).
- Detectar as frases cantadas (energia da voz isolada → inícios e fins) e
  mandar ao LLM a letra + a lista de frases com tempo (e o áudio da voz) para
  ele casar **linha da letra ↔ instante**. Plano B: mandar a versão cantada
  inteira ao LLM e pedir os tempos de cada linha.
- A cor citada entra no instante da palavra/linha, encaixada na batida. A
  duração dela (até o fim da frase? alguns compassos?) é a pergunta em
  aberto acima.
- Guardar os tempos das linhas na timeline (ex.: `analysis.lyrics: [{t,
  line}]`) para o editor da aba Músicas mostrar a letra junto das trocas
  (ex.: uma trilha de letra sob a faixa de cores).

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
  - a letra e o pedido.
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
