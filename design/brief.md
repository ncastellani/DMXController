# Briefing — redesenho da interface do DMXController

## O que é

Um console de iluminação para o pequeno show de um cantor (playback + voz ao
vivo), com 3 canhões de LED RGB (esquerda, centro, direita). Um programa roda
num Raspberry Pi e serve esta página; ela é aberta **no iPad (toque) e no
computador ao mesmo tempo**, ao lado do palco, **no escuro**, durante o show
ao vivo.

A página atual funciona bem; o objetivo é deixá-la **mais bonita, mais clara e
mais rápida de operar sob pressão**. Arquivo: `dmxcontroller-demo.html` (abre sozinho,
com dados de exemplo — selo roxo "MODO DEMONSTRAÇÃO" no canto). Prints de
todas as abas em computador, iPad e celular estão em `prints/`.

## O que tem na tela

**Sempre visível (topo):**
- Palco: os 3 canhões com a cor que cada um está emitindo agora (⚡ = strobo).
- BLACKOUT (apaga tudo) e PARAR TUDO (para também a música). Não há botão
  "parar strobe": o strobo contínuo desliga tocando de novo no botão dele.
- Os raios ⚡ na barra da música marcam as fritações — manter.
- Status: o que está tocando; mini player (play/pausa + música + tempo).
- Faixa de alertas (laranja) quando algo dá errado (interface DMX
  desconectada, caixa Bluetooth caiu...).

**Abas:**
1. **Show** — player de ponta a ponta no topo; embaixo, setlist à esquerda
   e "Luz da música" à direita: a lista das trocas de luz (horário,
   Luz/Fritação/Apagar, preset, duração da fritação), **só leitura** (não
   edita) e com "Aqui" em cada troca (leva o show até ela), destacando a
   troca atual e rolando junto com a música (inclusive ao pular para
   frente/trás) — manter isso. "Editar" leva ao editor da aba Músicas.
   Player: música atual, play/pausa/stop, barra da música com as
   partes (intro, verso, refrão… coloridas pela energia), curva de energia,
   marcas das trocas de luz e dos acentos ⚡, posição atual, e uma **régua
   de tempo** em cima (o passo se ajusta à largura: 15 s no computador, 1
   min no celular); tocar na barra ou na régua pula. Volume.
   Setlist (lista de músicas; tocar carrega; arrastar pela alça ⠿ muda a
   ordem; entre duas músicas, "⤓ emendar com a próxima" — ligado, mostra
   "emenda · 0,50 s" com − / + / ✕ e a música seguinte entra sem corte;
   "Adicionar" leva à aba Músicas; a setlist mostra só as análises em
   andamento).
2. **Console** — 55 botões de presets em 4 grupos, cada um com a cor que o
   iluminador escolheu: Modos de pista (32, liga/desliga, um por vez), Cores
   fixas (12), Fritação (7, **acende só enquanto o dedo está no botão**),
   Strobe contínuo (4, liga/desliga). Atalhos de teclado no computador.
3. **Músicas** (preparação, usada mais no computador) — à esquerda, a lista
   das músicas da pasta (com etiquetas: IA ou regras, cores quentes/frias,
   versão cantada ou letra, editada; arquivos ainda "sem timeline") e a fila de análise etapa
   por etapa. Tocar numa música abre à direita a **prévia e o editor da luz**
   dela (sem carregá-la no show):
   - **Palco 2D**, em duas vistas da plateia: **"Pendurados"** (padrão;
     treliça, fachos para baixo) e **"No chão"** — como no palco: canhões no
     chão, o da esquerda mirando para a direita, o da direita para a
     esquerda (fachos cruzando até a parede, o cantor em silhueta no meio) e
     o do centro virado para a plateia. Cor calculada para aquele
     instante. Strobo (⚡) e troca rápida de cor (≋) têm ícone; o strobo
     pisca de verdade enquanto a prévia toca, no máximo 3 vezes por segundo
     ("Piscar strobo e fritação", dá para desligar).
   - Play/pausa/voltar ao início da **prévia** (o som sai no próprio
     aparelho, não na caixa do show; os canhões não acendem), tempo, parte,
     "luz aqui", a próxima troca e, se a música tem versão cantada,
     **"Cantando"** (a frase da letra naquele instante, ou a próxima) e a
     opção **"Ouvir com voz"** (a prévia toca a versão cantada para conferir
     as cores na palavra).
   - Barra da música (como na aba Show) e, colada embaixo, a **faixa de
     cores**: uma trilha fina por canhão com a cor ao longo da música;
     trechos de strobo/troca rápida em listrado parado. Tocar ou arrastar
     na barra/faixa leva a prévia até ali; um cursor único atravessa as duas.
     Com versão cantada, uma **trilha da letra** fina embaixo: um bloco por
     frase cantada, a atual destacada.
   - **Editor**: lista das trocas de luz (horário, tipo Luz/Fritação/Apagar,
     preset, duração da fritação em batidas, ◀ ▶ uma batida, "Aqui" = leva a
     prévia até a troca, "Selecionar" = move a troca para o ponto atual da
     prévia, ✕), "+ Luz/Fritação/Apagar aqui", Desfazer, Restaurar gerada e
     nomes das partes; salva sozinho (se a música está no show, vale na
     hora). A lista rola junto com a prévia: as trocas que já passaram ficam
     mais escuras, a atual em amarelo, as que vêm em cor normal (vale também
     para a lista da aba Show). O preset de uma troca de "Luz"
     pode ser também uma cor da biblioteca, uma cor criada para a música ou
     "Nova cor…" (seletor de cor do sistema). A troca que veio de uma cor
     citada na letra mostra o trecho embaixo ("♪ “teu vestido verde-limão”").
   - "Analisar de novo…" troca o editor pelo formulário de análise.
   O formulário serve para **enviar .wav novos** (tocar ou arrastar) ou
   **analisar de novo** uma música: temperatura das cores (Automático /
   Quentes / Frias), "Usar IA", a **versão cantada** (opcional: um segundo
   .wav, a mesma faixa com a voz; a IA transcreve a letra e acende as cores
   citadas na palavra; mostra "guardada ✓" com Trocar/Tirar ao analisar de
   novo) e a **letra** colada (opcional; ajuda a transcrição ou, sem versão
   cantada, a IA casa a letra com as partes). Aviso quando a reanálise
   apaga edições. Nunca mostrar o nome do modelo de IA.
4. **Manual** — para cada canhão, 7 controles deslizantes (dimmer, R, G, B,
   vazio, função, velocidade) que mostram o valor ao vivo e, ao mexer, fixam o
   canal (fica amarelo) até tocar em "Soltar".
5. **Posições** — os 3 canhões na ordem do palco: setas para trocar de lugar e
   "Identificar" (acende só aquele por 3 s).
6. **Som** — saída de som (Bluetooth / cabo P2 / alto-falante / HDMI), caixa
   Bluetooth (procurar, parear, conectar, esquecer), atraso da saída +
   "Calibrar" (cliques com os canhões piscando).
7. **Eventos** — histórico do que foi acionado.

## Prioridades durante o show

1. **BLACKOUT** e **PARAR TUDO**: achar e acertar em menos de 1 s, de qualquer aba.
2. **Play/pausa** da música e qual música está tocando.
3. **Fritação** (segurar) e **cores fixas**: botões grandes, cores fiéis.
4. Ver o palco (as 3 cores) de relance.
5. Músicas, Manual, Posições, Som e Eventos são de preparação: podem ficar mais
   escondidos.

## Regras obrigatórias

- **Um arquivo HTML só, sem internet**: nada de fontes do Google, CDNs ou
  imagens externas (no local do show pode não haver internet). Fontes do
  sistema; ícones em SVG embutido ou caracteres.
- **Tema escuro** (usado no escuro, ao lado do palco), com bom contraste.
- **Toque**: alvos de no mínimo ~48 px; sem zoom por duplo toque, sem seleção
  de texto e sem menu de toque longo nos botões.
- **Fritação = segurar**: acende ao encostar o dedo e apaga ao soltar —
  inclusive se o dedo escorregar para fora do botão. Nunca vira clique.
- **Nunca piscar a tela para representar strobo** (segurança para pessoas com
  epilepsia fotossensível) no console ao vivo: use um ícone/indicador
  estático. Exceção: no palco 2D da prévia (aba Músicas) o strobo pisca, no
  máximo 3 vezes por segundo, com opção de desligar — manter esse limite.
- **Português do Brasil** em todos os textos.
- Funciona em iPad (paisagem e retrato), MacBook e celular.

## Pode mudar livremente

Layout, cores da interface, tipografia, ícones, agrupamentos, como as abas são
organizadas (desde que tudo continue acessível), visual do palco (ex.: desenho
dos canhões), do palco 2D da prévia (o palco em si fica escuro nos dois temas),
da barra da música e da faixa de cores, dos cards, estados de "ativo",
animações suaves (que não pisquem).

## Como entregar

Edite o próprio `dmxcontroller-demo.html` mantendo o modo demonstração funcionando
(o bloco de script `DemoSocket` e a lógica que reage às mensagens). Pode
mudar HTML e CSS à vontade; ao mexer no JavaScript, preserve o que cada botão
envia. No fim, liste o que mudou de estrutura (ids, abas novas/removidas).

---

## Mensagem para colar no Claude Design

> Anexei `dmxcontroller-demo.html` (interface de um console de iluminação de show, já
> em modo demonstração com dados de exemplo), os prints de todas as abas e
> este briefing (`brief.md`). Quero redesenhar a interface seguindo o
> briefing: mais bonita, clara e rápida de operar no escuro, principalmente
> no iPad. Respeite todas as regras obrigatórias (arquivo único sem internet,
> tema escuro, alvos de toque grandes, fritação acionada enquanto o dedo está
> no botão, nunca piscar a tela, textos em português). Edite o próprio HTML
> mantendo o modo demonstração funcionando. Comece propondo a direção visual
> e o layout do topo e da aba Show; depois seguimos para as outras abas.
