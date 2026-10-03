// Modo demonstração da interface do DMXController (abrir com ?demo=1, ou numa cópia
// com <html data-demo>). Simula o servidor (README.md, "Protocolo da interface
// web") com dados de exemplo, para desenhar e testar a interface sem o
// programa rodando.
// Não faz parte do show: a página normal nunca carrega este arquivo.
(function () {
  "use strict";
  const D = {"config":{"type":"config","groups":[{"name":"MODOS DE PISTA","layer":"base","buttons":[{"id":0,"caption":"AZUL ROSA","color":"#3f55c8","preset":"qlc:30"},{"id":1,"caption":"BATIDÃO","color":"#b01540","preset":"qlc:51"},{"id":2,"caption":"BRANCO VERMELHO","color":"#d62828","preset":"qlc:57"},{"id":3,"caption":"CALOR","color":"#c75a18","preset":"qlc:63"},{"id":4,"caption":"CAOS QUENTE","color":"#c42b0a","preset":"qlc:73"},{"id":5,"caption":"CHILL","color":"#35539e","preset":"qlc:47"},{"id":6,"caption":"CINEMA","color":"#7a2626","preset":"qlc:69"},{"id":7,"caption":"CLUB","color":"#5a39c8","preset":"qlc:43"},{"id":8,"caption":"COLOR FLOW","color":"#3a93a8","preset":"qlc:53"},{"id":9,"caption":"CONTRAMÃO","color":"#b24f14","preset":"qlc:65"},{"id":10,"caption":"DOURADO VERMELHO","color":"#cf8012","preset":"qlc:71"},{"id":11,"caption":"FESTA","color":"#bd3ba2","preset":"qlc:49"},{"id":12,"caption":"FLASH","color":"#e8e8e8","preset":"qlc:55"},{"id":13,"caption":"FOGO","color":"#e8470f","preset":"qlc:61"},{"id":14,"caption":"FOGO CRUZADO","color":"#e05410","preset":"qlc:89"},{"id":15,"caption":"GOLDEN","color":"#d4a017","preset":"qlc:59"},{"id":16,"caption":"IMPACTO","color":"#d02828","preset":"qlc:83"},{"id":17,"caption":"MENINOS E MENINAS 1","color":"#8e2a80","preset":"qlc:79"},{"id":18,"caption":"MENINOS E MENINAS 2","color":"#8e2a80","preset":"qlc:33"},{"id":19,"caption":"MENINOS E MENINAS 3","color":"#8e2a80","preset":"qlc:81"},{"id":20,"caption":"PERSEGUIÇÃO","color":"#d4860f","preset":"qlc:85"},{"id":21,"caption":"POP","color":"#d13c8c","preset":"qlc:41"},{"id":22,"caption":"PULSO QUENTE","color":"#a8481a","preset":"qlc:67"},{"id":23,"caption":"PULSO QUENTE 2","color":"#a8481a","preset":"qlc:93"},{"id":24,"caption":"QUENTE","color":"#d14a14","preset":"qlc:45"},{"id":25,"caption":"RED 200","color":"#c02020","preset":"qlc:77"},{"id":26,"caption":"SURTO QUENTE","color":"#bd350a","preset":"qlc:91"},{"id":27,"caption":"TRICOLOR","color":"#6a4ac0","preset":"qlc:35"},{"id":28,"caption":"VERDE AZUL ROSA","color":"#1f8f8f","preset":"qlc:39"},{"id":29,"caption":"VERDE ROSA","color":"#2e9460","preset":"qlc:37"},{"id":30,"caption":"VERMELHO BRANCO 2","color":"#e03535","preset":"qlc:87"},{"id":31,"caption":"ÂMBAR 200","color":"#d79415","preset":"qlc:75"}]},{"name":"CORES FIXAS","layer":"base","buttons":[{"id":32,"caption":"BRANCO","color":"#ffffff","key":"q","preset":"qlc:6"},{"id":33,"caption":"VERMELHO","color":"#ff0000","key":"w","preset":"qlc:0"},{"id":34,"caption":"VERMELHO PROFUNDO","color":"#c80000","key":"e","preset":"qlc:22"},{"id":35,"caption":"ROSA VERMELHO","color":"#ff0a2d","key":"r","preset":"qlc:24"},{"id":36,"caption":"ÂMBAR","color":"#ff2d00","key":"t","preset":"qlc:16"},{"id":37,"caption":"ÂMBAR DOURADO","color":"#ff3700","key":"y","preset":"qlc:18"},{"id":38,"caption":"DOURADO","color":"#ff5000","key":"u","preset":"qlc:17"},{"id":39,"caption":"LARANJA QUEIMADO","color":"#ff1900","key":"i","preset":"qlc:20"},{"id":40,"caption":"ROXO PROFUNDO","color":"#5a00c8","key":"o","preset":"qlc:31"},{"id":41,"caption":"AZUL","color":"#0000ff","key":"p","preset":"qlc:1"},{"id":42,"caption":"AZUL PROFUNDO","color":"#0000a0","key":"k","preset":"qlc:12"},{"id":43,"caption":"CIANO","color":"#00ffff","key":"l","preset":"qlc:8"}]},{"name":"FRITAÇÃO","layer":"flash","buttons":[{"id":44,"caption":"BRANCO MÁX. 255","color":"#ffffff","key":"a","preset":"qlc:94"},{"id":45,"caption":"BRANCO 254","color":"#ffffff","key":"s","preset":"qlc:100"},{"id":46,"caption":"VERMELHO 253","color":"#ff0000","key":"d","preset":"qlc:95"},{"id":47,"caption":"LARANJA 252","color":"#ff1900","key":"f","preset":"qlc:96"},{"id":48,"caption":"AZUL PROF. 253","color":"#0000a0","key":"g","preset":"qlc:97"},{"id":49,"caption":"AZUL 252","color":"#0000ff","key":"h","preset":"qlc:98"},{"id":50,"caption":"CIANO 254","color":"#00ffff","key":"j","preset":"qlc:99"}]},{"name":"STROBE CONTÍNUO","layer":"strobe","buttons":[{"id":51,"caption":"BRANCO 254","color":"#ffffff","key":"z","preset":"qlc:101"},{"id":52,"caption":"VERMELHO 253","color":"#ff0000","key":"x","preset":"qlc:102"},{"id":53,"caption":"LARANJA 252","color":"#ff1900","key":"c","preset":"qlc:103"},{"id":54,"caption":"AZUL PROF. 253","color":"#0000a0","key":"v","preset":"qlc:104"}]}],"flashLimit":10,"strobeLimit":60,"library":[{"caption":"ROXO","color":"#ff00ff","preset":"qlc:3"},{"caption":"ROSA","color":"#ff2878","preset":"qlc:4"},{"caption":"LARANJA","color":"#ff3c00","preset":"qlc:5"},{"caption":"AMARELO","color":"#ff6400","preset":"qlc:7"},{"caption":"VERDE","color":"#00ff00","preset":"qlc:9"},{"caption":"VERDE LIMÃO","color":"#8cff00","preset":"qlc:10"},{"caption":"AZUL CLARO","color":"#0064ff","preset":"qlc:11"},{"caption":"MAGENTA","color":"#ff0064","preset":"qlc:13"},{"caption":"VIOLETA","color":"#7800ff","preset":"qlc:14"},{"caption":"VERMELHO QUENTE","color":"#ff1400","preset":"qlc:15"},{"caption":"ÂMBAR ESCURO","color":"#b41900","preset":"qlc:19"},{"caption":"VERMELHO QUENTE ESCURO","color":"#b40800","preset":"qlc:21"},{"caption":"BORDÔ","color":"#b4000e","preset":"qlc:23"},{"caption":"FOGO","color":"#ff1e00","preset":"qlc:25"},{"caption":"ÂMBAR SUAVE","color":"#641400","preset":"qlc:26"},{"caption":"VERMELHO SUAVE","color":"#640000","preset":"qlc:27"},{"caption":"DOURADO SUAVE","color":"#641f00","preset":"qlc:28"}]},"timeline":{"version":1,"song":{"title":"praficarcomigo","file":"praficarcomigo.wav","duration":256.794},"generated":{"at":"2026-10-02T16:36:08+00:00","by":"scripts/analisar.py","model":"google/gemini-3.8-flash"},"personality":{"mode":"presets","intensity":0.75,"palette":["AZUL","CIANO","MAGENTA","ROSA","ROXO PROFUNDO","BRANCO"]},"ai":{"genero":"Electropop / Dance Pop","clima":"Sensual, noturno e dançante","descricao":"Show de pop eletrônico com batida constante de pista e sintetizadores envolventes. Iluminação focada em tons de azul, magenta, roxo e ciano, alternando momentos de contemplação com sequências rítmicas e flashes pontuais nas viradas de refrão.","bpm_percebido":123},"analysis":{"duration":256.794,"bpm":123.05,"energy":{"hop":0.5,"values":[0,0,0,0,0.08,0.066,0.077,0.054,0.098,0.097,0.063,0.045,0.096,0.072,0.042,0.098,0.099,0.152,0.062,0.091,0.107,0.087,0.052,0.111,0.103,0.057,0.052,0.119,0.085,0.051,0.086,0.095,0.141,0.051,0.092,0.096,0.079,0.039,0.099,0.097,0.059,0.057,0.092,0.08,0.055,0.092,0.123,0.149,0.057,0.108,0.097,0.088,0.053,0.113,0.097,0.066,0.081,0.077,0.091,0.071,0.1,0.124,0.107,0.461,0.701,0.74,0.635,0.626,0.664,0.711,0.65,0.655,0.716,0.647,0.669,0.661,0.725,0.659,0.756,0.636,0.757,0.664,0.76,0.788,0.656,0.615,0.654,0.644,0.67,0.678,0.637,0.605,0.356,0.566,0.668,0.758,0.739,0.769,0.838,0.832,0.787,0.693,0.747,0.747,0.756,0.869,0.892,0.815,0.899,0.867,0.905,0.857,0.891,0.941,0.813,0.847,0.901,0.826,0.913,0.837,0.817,0.728,0.553,0.747,0.813,0.79,0.843,0.766,0.835,0.744,0.691,0.724,0.767,0.676,0.653,0.69,0.734,0.656,0.804,0.678,0.761,0.731,0.708,0.832,0.613,0.729,0.754,0.807,0.803,0.781,0.798,0.596,0.426,0.638,0.706,0.647,0.681,0.641,0.721,0.659,0.677,0.741,0.799,0.688,0.735,0.762,0.771,0.743,0.789,0.676,0.763,0.774,0.683,0.827,0.606,0.849,0.84,0.884,0.823,0.872,0.916,0.634,0.733,0.844,0.96,0.897,0.811,0.828,0.79,0.672,0.695,0.725,0.75,0.73,0.685,0.752,0.716,0.754,0.733,0.727,0.803,0.795,0.747,0.813,0.618,0.743,0.7,0.746,0.695,0.715,0.73,0.432,0.512,0.71,0.77,0.687,0.689,0.724,0.751,0.654,0.718,0.758,0.7,0.743,0.666,0.769,0.707,0.779,0.712,0.756,0.765,0.795,0.798,0.759,0.635,0.738,0.711,0.717,0.716,0.675,0.703,0.447,0.573,0.716,0.843,0.785,0.775,0.748,0.837,0.741,0.687,0.78,0.844,0.908,0.895,0.887,0.774,0.807,0.632,0.757,0.659,0.836,0.857,0.751,0.707,0.809,0.84,0.855,0.788,0.77,0.718,0.528,0.732,0.766,0.805,0.752,0.804,0.819,0.811,0.723,0.693,0.788,0.779,0.879,0.97,0.868,0.79,0.944,0.894,0.935,0.871,0.886,0.958,0.974,0.923,0.916,0.987,0.991,0.966,0.95,0.732,0.664,0.919,0.944,0.845,0.922,0.836,0.883,0.788,0.707,0.7,0.756,0.664,0.647,0.682,0.746,0.656,0.793,0.672,0.746,0.745,0.691,0.837,0.618,0.753,0.774,0.862,0.817,0.838,0.836,0.651,0.605,0.755,0.815,0.738,0.716,0.665,0.729,0.65,0.617,0.655,0.735,0.633,0.661,0.715,0.712,0.688,0.747,0.674,0.766,0.772,0.677,0.822,0.6,0.683,0.678,0.84,0.757,0.821,0.866,0.569,0.539,0.745,0.784,0.698,0.697,0.716,0.735,0.692,0.782,0.905,0.918,0.969,0.978,0.947,0.851,0.863,0.842,0.843,0.887,0.874,0.879,0.83,0.65,0.828,0.842,0.833,0.827,0.828,0.883,0.706,0.712,0.871,0.868,0.747,0.725,0.762,0.759,0.678,0.719,0.771,0.686,0.742,0.681,0.768,0.732,0.818,0.818,0.89,0.896,0.893,0.884,0.867,0.783,0.838,0.825,0.794,0.787,0.742,0.779,0.595,0.641,0.451,0.564,0.511,0.593,0.654,0.611,0.513,0.481,0.511,0.542,0.506,0.355,0.541,0.464,0.674,0.65,0.732,0.641,0.666,0.646,0.632,0.654,0.629,0.653,0.849,0.775,0.577,0.665,0.544,0.811,0.726,0.865,0.71,0.702,0.694,0.649,0.731,0.681,0.743,0.737,0.676,0.537,0.626,0.648,0.834,0.768,0.83,0.713,0.707,0.709,0.7,0.752,0.612,0.676,0.653,0.598,0.641,0.778,0.537,0.313,0.164,0.023,0.062,0.065,0,0.099,0.014,0.049,0.022,0.047,0,0.069,0.183,0.127,0.053,0.004,0,0,0,0,0]},"sections":[{"start":0,"end":31.718,"bars":16,"energy":0.079,"group":"A","name":"intro"},{"start":31.718,"end":56.471,"bars":13,"energy":0.694,"group":"B","name":"verso"},{"start":56.471,"end":88.84,"bars":17,"energy":0.747,"group":"B","name":"verso"},{"start":88.84,"end":125.039,"bars":19,"energy":0.734,"group":"C","name":"refrão"},{"start":125.039,"end":163.143,"bars":20,"energy":0.806,"group":"B","name":"verso"},{"start":163.143,"end":185.992,"bars":12,"energy":0.725,"group":"C","name":"refrão"},{"start":185.992,"end":214.576,"bars":15,"energy":0.806,"group":"D","name":"ponte"},{"start":214.576,"end":245.11,"bars":16,"energy":0.65,"group":"E","name":"refrão"},{"start":245.11,"end":256.794,"bars":6,"energy":0.085,"group":"F","name":"final"}]},"cues":[{"t":0,"layer":"base","kind":"preset","preset":"qlc:47","name":"PISTA | CHILL","rate":1,"note":"Introdução atmosférica com sintetizadores e construção gradual de energia em tons frios."},{"t":31.718,"layer":"base","kind":"preset","preset":"qlc:41","name":"PISTA | POP","rate":1.2818,"note":"Entrada do beat e da voz, ritmo pop fluido e colorido acompanhando a cadência da estrofe."},{"t":31.718,"layer":"flash","kind":"preset","preset":"qlc:99","name":"FRITAÇÃO | CIANO","duration":0.975,"note":"Flash em ciano marcando a entrada da bateria e início do verso."},{"t":56.471,"layer":"base","kind":"preset","preset":"qlc:41","name":"PISTA | POP","rate":1.2818,"note":"Continuação da estrofe mantendo coerência estilística com o grupo B."},{"t":88.84,"layer":"base","kind":"preset","preset":"qlc:43","name":"PISTA | CLUB","rate":1.2305,"note":"Explosão melódica do refrão com movimento mais ágil e luzes vibrantes de club."},{"t":88.84,"layer":"flash","kind":"preset","preset":"qlc:98","name":"FRITAÇÃO | AZUL","duration":0.975,"note":"Impacto na cabeça do primeiro refrão destacando a abertura vocal."},{"t":125.039,"layer":"base","kind":"preset","preset":"qlc:41","name":"PISTA | POP","rate":1.2818,"note":"Retorno ao verso com andamento intermediário para valorizar os vocais solo."},{"t":163.143,"layer":"base","kind":"preset","preset":"qlc:43","name":"PISTA | CLUB","rate":1.2305,"note":"Segundo refrão retomando a vibração de pista com transições rápidas."},{"t":185.992,"layer":"base","kind":"preset","preset":"qlc:51","name":"PISTA | BATIDÃO","rate":1.2305,"note":"Clímax rítmico e instrumental da música com batidão pesado e dinâmica agressiva."},{"t":185.992,"layer":"flash","kind":"preset","preset":"qlc:94","name":"FRITAÇÃO | BRANCO MÁXIMO","duration":0.975,"note":"Flash branco máximo no drop da ponte, o momento de maior intensidade da faixa."},{"t":214.576,"layer":"base","kind":"preset","preset":"qlc:79","name":"PISTA | MENINOS E MENINAS 1","rate":0.8203,"note":"Refrão final com paleta profunda de roxo e rosa, sustentando o impacto até a resolução."},{"t":214.576,"layer":"flash","kind":"preset","preset":"qlc:99","name":"FRITAÇÃO | CIANO","duration":0.975,"note":"Acento de virada para a entrada triunfal do refrão final."},{"t":245.11,"layer":"base","kind":"preset","preset":"qlc:31","name":"TODOS | ROXO PROFUNDO","rate":1,"note":"Desaceleração e fade out final em cena fixa intimista de roxo profundo."},{"t":248.5,"layer":"base","kind":"blackout","note":"fim"}]},"state":{"type":"state","blackout":false,"active":{"base":-1,"flash":-1,"strobe":-1},"identify":-1,"show":"praficarcomigo","health":{"dmx":true,"audio":true},"positions":[{"name":"esquerda","address":1,"color":"#00ffff","strobe":false,"channels":[{"role":"dimmer","value":255,"manual":false},{"role":"red","value":0,"manual":false},{"role":"green","value":255,"manual":false},{"role":"blue","value":255,"manual":false},{"role":"empty","value":0,"manual":false},{"role":"mode","value":0,"manual":false},{"role":"speed","value":0,"manual":false}]},{"name":"centro","address":8,"color":"#ff0064","strobe":false,"channels":[{"role":"dimmer","value":255,"manual":false},{"role":"red","value":255,"manual":false},{"role":"green","value":0,"manual":false},{"role":"blue","value":100,"manual":false},{"role":"empty","value":0,"manual":false},{"role":"mode","value":0,"manual":false},{"role":"speed","value":0,"manual":false}]},{"name":"direita","address":15,"color":"#00ffff","strobe":false,"channels":[{"role":"dimmer","value":255,"manual":false},{"role":"red","value":0,"manual":false},{"role":"green","value":255,"manual":false},{"role":"blue","value":255,"manual":false},{"role":"empty","value":0,"manual":false},{"role":"mode","value":0,"manual":false},{"role":"speed","value":0,"manual":false}]}]},"songs":[{"title":"praficarcomigo","duration":256.794,"genre":"Electropop / Dance Pop","mood":"Sensual, noturno e dançante","wav":"praficarcomigo.wav","temp":"auto","ai":true,"edited":true},{"title":"Abertura","duration":92.4,"genre":"Cinemático","mood":"Épico e crescente","wav":"Abertura.wav","temp":"quente","ai":true,"edited":false},{"title":"Balada no Escuro","duration":228.0,"genre":"Pop balada","mood":"Romântico e intimista","wav":"Balada no Escuro.wav","temp":"fria","ai":true,"edited":false},{"title":"Pista Quente","duration":201.5,"genre":"Funk pop","mood":"Explosivo e dançante","wav":"Pista Quente.wav","temp":"quente","ai":false,"edited":false},{"title":"Bis","duration":175.0,"error":"áudio não encontrado: bis.wav","wav":"","temp":"","ai":false,"edited":false}],"sound":{"type":"sound","available":true,"platform":"darwin","missing":[],"outputs":[{"id":"mondial","name":"MONDIAL CM","kind":"bluetooth","default":true},{"id":"speakers","name":"Alto-falantes do MacBook Pro","kind":"builtin","default":false},{"id":"headphones","name":"Fones de ouvido externos","kind":"wired","default":false},{"id":"hdmi","name":"Monitor HDMI","kind":"hdmi","default":false}],"devices":[{"address":"f3-e3-2f-9b-a2-b4","name":"MONDIAL CM","paired":true,"connected":true},{"address":"aa-bb-cc-dd-ee-01","name":"Fone de ouvido","paired":true,"connected":false}],"speaker":"f3-e3-2f-9b-a2-b4","busy":"","warning":"","error":""},"orphans":["Ensaio novo.wav"]};
  const clone = (x) => JSON.parse(JSON.stringify(x));
  const hex = (r, g, b) => "#" + [r, g, b].map((v) => Math.round(Math.max(0, Math.min(255, v))).toString(16).padStart(2, "0")).join("");
  const rgb = (h) => { const n = parseInt(h.slice(1), 16); return [n >> 16, (n >> 8) & 255, n & 255]; };
  const time = () => new Date().toTimeString().slice(0, 8);

  // Timeline de cada música: a real para a 1ª; as outras são a mesma, esticada.
  function timelineFor(i) {
    const base = D.timeline, song = D.songs[i], k = song.duration / base.song.duration;
    const t = clone(base);
    t.song.title = song.title; t.song.duration = song.duration; t.analysis.duration = song.duration;
    for (const s of t.analysis.sections) { s.start *= k; s.end *= k; }
    for (const c of t.cues) { c.t *= k; if (c.duration) c.duration *= k; }
    t.cues.sort((a, b) => a.t - b.t || (a.layer === "flash" ? 0 : 1) - (b.layer === "flash" ? 0 : 1)); // fritação antes da base, como o servidor
    t.analysis.energy.hop *= k;
    if (t.ai) { t.ai.genero = song.genre || ""; t.ai.clima = song.mood || ""; }
    return t;
  }

  const buttons = [];
  for (const g of D.config.groups) for (const b of g.buttons) buttons.push({ ...b, layer: g.layer, group: g.name });
  const byId = (id) => buttons.find((b) => b.id === id);
  const colorOf = (name, preset) => {
    if (preset && preset.startsWith("cor:")) return preset.slice(4);
    const lib = D.config.library.find((x) => x.preset === preset);
    if (lib) return lib.color;
    if (!name) return "#ffffff";
    const b = buttons.find((x) => x.color && name.toUpperCase().includes(x.caption.toUpperCase()));
    return (b && b.color) || "#7f5af0";
  };

  // Cor do canhão i no instante pos de uma música (aproximação visual do
  // programa do servidor: sequência alterna a cor com um tom vizinho a cada batida).
  function songLook(tl, pos, i) {
    const cues = tl.cues, base = cues.filter((c) => c.layer === "base" && c.t <= pos).pop();
    const fl = cues.find((c) => c.layer === "flash" && c.t <= pos && pos < c.t + c.duration);
    if (fl) return { color: colorOf(fl.name, fl.preset), strobe: true };
    if (!base || base.kind === "blackout") return { color: "#000000", strobe: false };
    if (base.kind === "effect") return { color: fxLook(base.effect, pos - base.t, i, tl.analysis.bpm || 120), strobe: false };
    const c = colorOf(base.name, base.preset);
    const fixed = base.preset && (base.preset.startsWith("cor:") || D.config.library.some((x) => x.preset === base.preset));
    if (fixed || (Math.floor(pos / (60 / 123)) + i) % 2 === 0) return { color: c, strobe: false };
    const [r, g, b] = rgb(c); return { color: hex(b, r, g), strobe: false };
  }

  // Efeito do modo livre (kind "effect"), como o servidor desenha.
  const FXN = { fixed: "CORES FIXAS", alternate: "ALTERNADO", chase: "CORRIDA", pulse: "PULSO", fade: "TRANSIÇÃO", sequence: "SEQUÊNCIA" };
  function fxLook(e, t, i, bpm) {
    let step = ((e.sync === "hits" ? 1 : e.beats || 1) * 60) / bpm; // "no toque": aqui, uma troca por batida while (step < 0.34) step *= 2;
    const cs = e.colors || [], n = cs.length, k = Math.floor(t / step), ph = (t % step) / step;
    const lv = e.level || 1, out = (c, l = lv) => hex(...rgb(c).map((v) => v * l));
    switch (e.type) {
      case "sequence": {
        const m = e.steps.length, cur = rgb(e.steps[k % m][i % 3]), prev = rgb(e.steps[(k + m - 1) % m][i % 3]), p = e.fade && ph < 0.5 ? ph / 0.5 : 1;
        return out(hex(...cur.map((v, j) => prev[j] + (v - prev[j]) * p)));
      }
      case "alternate": return out(cs[(i + k) % n]);
      case "chase": { const x = k % 4; return out(i === (x === 3 ? 1 : x) ? cs[0] : cs[1] || "#000000"); }
      case "pulse": return out(cs[i % n], lv * (0.3 + 0.7 * (1 - ph) * (1 - ph)));
      case "fade": { const a = rgb(cs[(i + k) % n]), b = rgb(cs[(i + k + 1) % n]); return out(hex(...a.map((v, j) => v + (b[j] - v) * ph))); }
    }
    return out(cs[i % n]);
  }
  // Troca as sequências de uma timeline por efeitos (análise no modo livre).
  function freeCues(tl) {
    const pals = [["#ff0a2d", "#0064ff"], ["#7800ff", "#00ffff"], ["#ff00aa", "#7800ff", "#0064ff"]], kinds = [["fade", 4], ["alternate", 2], ["chase", 1], ["pulse", 1]];
    let k = 0;
    for (const c of tl.cues) {
      if (c.layer !== "base" || c.kind !== "preset" || k % 4 === 3 && k++) continue; // uma em cada quatro fica no preset
      const [type, beats] = kinds[k % kinds.length], [a, b, d = "#00ffff"] = pals[k % pals.length];
      const effect = k % 2 ? { type, colors: pals[k % pals.length].slice(0, type === "chase" ? 2 : 4), beats } // a maioria são sequências escritas passo a passo
        : { type: "sequence", sync: "hits", beats, ...(beats > 2 ? { fade: true } : {}), steps: [[a, b, d], [d, a, b], [b, d, a], [a, b, a], [b, a, b], [d, "#000000", "#000000"], ["#000000", d, "#000000"], ["#000000", "#000000", d]] };
      Object.assign(c, { kind: "effect", effect, name: "LIVRE | " + FXN[effect.type] });
      delete c.preset; delete c.rate; k++;
    }
  }

  class DemoSocket {
    constructor() {
      this.readyState = 0;
      const st = clone(D.state);
      this.s = {
        active: { base: -1, strobe: -1, flash: -1 }, blackout: false, identify: -1, identUntil: 0,
        positions: st.positions.map((p) => ({ name: p.name, address: p.address, manual: p.channels.map(() => null) })),
        held: [], strobeSince: 0,
        song: 0, tl: null, playing: false, pos: 95, volume: 0.8, latency: 180, calibrating: false,
        sound: clone(D.sound),
      };
      this.events = [
        { type: "event", time: time(), text: "DMX conectado" },
        { type: "event", time: time(), text: "♪ carregada: " + D.songs[0].title },
      ];
      this.tls = {}; // X.json → timeline (editada ou gerada)
      this.s.tl = this.tlOf(this.fileOf(0));
      setTimeout(() => this.open(), 200);
    }

    fileOf(i) { const so = D.songs[i]; return so ? so.file || so.title + ".json" : ""; }
    songOf(file) { return D.songs.find((so, i) => this.fileOf(i) === file); }
    tlOf(file) {
      if (!this.tls[file]) { const i = D.songs.findIndex((so, k) => this.fileOf(k) === file); if (i < 0) return null; this.tls[file] = timelineFor(i); }
      return this.tls[file];
    }
    // GET /api/timeline e /api/preview, simulados.
    timelineOf(file) {
      const t = this.tlOf(file); if (!t) throw new Error("música não está na setlist");
      return { type: "songTimeline", file, timeline: clone(t), edited: !!this.songOf(file).edited };
    }
    previewOf(file, fps) {
      const t = this.tlOf(file); if (!t) throw new Error("música não está na setlist");
      const n = Math.ceil(t.song.duration * fps), pos = this.s.positions.map((p) => ({ name: p.name, colors: "", fx: "" }));
      for (let j = 0; j < n; j++) {
        pos.forEach((p, i) => { const l = songLook(t, j / fps, i); p.colors += l.color.slice(1); p.fx += l.strobe ? "1" : "0"; });
      }
      return { file, fps, duration: t.song.duration, positions: pos };
    }
    emitSongTimeline(file) {
      const t = this.tlOf(file), so = this.songOf(file);
      this.emit({ type: "songTimeline", file, timeline: clone(t), edited: !!(so && so.edited) });
      if (this.fileOf(this.s.song) === file && !this.s.calibrating) { this.s.tl = t; this.emit({ type: "timeline", timeline: t, edited: !!(so && so.edited) }); }
    }

    open() {
      this.readyState = 1;
      if (this.onopen) this.onopen();
      this.emit(D.config);
      this.emit({ type: "events", items: this.events });
      this.emitSetlist();
      this.emit({ type: "timeline", timeline: this.s.tl });
      this.emit(this.soundMsg());
      this.jobs = [{ id: 1, file: "praficarcomigo.wav", ai: true, temp: "auto", stage: "pronta", done: true }];
      this.emitJobs();
      this.last = performance.now();
      this.timer = setInterval(() => this.tick(), 100);
      this.tick();
    }

    send(raw) { this.handle(JSON.parse(raw)); this.tick(); }
    close() { clearInterval(this.timer); }
    emit(m) { if (this.onmessage) this.onmessage({ data: JSON.stringify(m) }); }
    event(text) { const e = { type: "event", time: time(), text }; this.events.push(e); this.emit(e); }

    emitSetlist() {
      D.songs.forEach((s) => { s.file = s.file || s.title + ".json"; });
      this.emit({ type: "setlist", dir: "musicas", orphans: D.orphans, songs: D.songs.map((s, i) => ({ index: i, file: s.file, title: s.title, duration: s.duration, genre: s.genre, mood: s.mood, error: s.error, link: !!s.link, gap: s.gap || 0,
        wav: s.wav, temp: s.temp, ai: !!s.ai, free: !!s.free, edited: !!s.edited })) });
    }
    soundMsg() { return { type: "sound", ...this.s.sound }; }
    emitJobs() { this.emit({ type: "jobs", ai: true, items: this.jobs }); }

    // Simula o envio de um .wav (fica na pasta, sem timeline, até o analyze).
    fakeUpload(name) {
      if (!D.orphans.includes(name) && !D.songs.some((s) => s.wav === name)) D.orphans.push(name);
      this.emitSetlist();
      return name;
    }

    // Simula a análise no servidor (nova ou de novo), etapa por etapa.
    fakeAnalyze(m) {
      const ai = !!m.ai, temp = m.temp || "auto", free = !!m.free;
      const j = { id: this.jobs.length + 1, file: m.str, ai, temp, free, stage: "na fila", done: false };
      this.jobs.push(j); this.emitJobs();
      const stages = ["lendo o áudio", "analisando batidas e partes"]
        .concat(ai ? ["a IA está ouvindo a música", "IA respondeu (9840+2371 tokens, US$ 0.0163)"] : []);
      stages.forEach((st, i) => setTimeout(() => { j.stage = st; this.emitJobs(); }, 900 * (i + 1)));
      setTimeout(() => {
        j.stage = "pronta"; j.done = true; this.emitJobs();
        let so = D.songs.find((s) => s.wav === m.str);
        if (!so) {
          D.orphans = D.orphans.filter((w) => w !== m.str);
          so = { title: m.str.replace(/\.wav$/i, ""), wav: m.str, duration: 210 };
          D.songs.push(so);
        }
        Object.assign(so, { temp, ai, free, edited: false, genre: ai ? so.genre || "Pop" : "", mood: ai ? so.mood || "Animado" : "" });
        this.emitSetlist();
        const file = this.fileOf(D.songs.indexOf(so));
        delete this.tls[file]; this.tlOf(file).generated = { at: new Date().toISOString() };
        if (free) freeCues(this.tlOf(file));
        this.emitSongTimeline(file); this.event("♪ timeline pronta: " + so.title);
      }, 900 * (stages.length + 1));
    }

    handle(m) {
      const s = this.s, b = byId(m.id);
      switch (m.type) {
        case "press":
          if (!b || b.layer === "flash") return;
          if (s.active[b.layer] === b.id) { s.active[b.layer] = -1; this.event("■ " + b.group + " | " + b.caption); }
          else { s.active[b.layer] = b.id; if (b.layer === "strobe") s.strobeSince = Date.now(); this.event("▶ " + b.group + " | " + b.caption); }
          break;
        case "flash":
          s.held = s.held.filter((x) => x !== m.id);
          if (m.down) s.held.push(m.id);
          s.active.flash = s.held.length ? s.held[s.held.length - 1] : -1;
          break;
        case "blackout": s.blackout = !s.blackout; this.event("blackout " + (s.blackout ? "ligado" : "desligado")); break;
        case "stopStrobe": if (s.active.strobe >= 0) { this.event("■ " + byId(s.active.strobe).caption); s.active.strobe = -1; } break;
        case "stopAll":
          s.active = { base: -1, strobe: -1, flash: -1 }; s.held = []; s.blackout = false; s.identify = -1;
          for (const p of s.positions) p.manual = p.manual.map(() => null);
          this.event("parar tudo"); break;
        case "manual": s.positions[m.pos].manual[m.ch] = m.value; break;
        case "release": s.positions.forEach((p, i) => { if (m.pos === -1 || m.pos === i) p.manual = p.manual.map(() => null); }); break;
        case "swap": {
          const a = s.positions[m.a], c = s.positions[m.b];
          if (!a || !c) return;
          [a.address, c.address] = [c.address, a.address];
          this.event(`posições: ${a.name} = d${String(a.address).padStart(3, "0")}, ${c.name} = d${String(c.address).padStart(3, "0")}`);
          break;
        }
        case "identify": s.identify = m.pos; s.identUntil = Date.now() + 3000; break;
        case "load":
          if (D.songs[m.id].error) { this.event("web: " + D.songs[m.id].error); return; }
          s.song = m.id; s.tl = this.tlOf(this.fileOf(m.id)); s.pos = 0; s.playing = false; s.calibrating = false;
          this.emit({ type: "timeline", timeline: s.tl, edited: !!D.songs[m.id].edited }); this.event("♪ carregada: " + D.songs[m.id].title); break;
        case "play": if (s.song < 0 && !s.calibrating) { this.event("web: nenhuma música carregada"); break; } s.playing = true; this.event("▶ " + (s.calibrating ? "calibração" : D.songs[s.song].title)); break;
        case "pause": s.playing = false; this.event("❚❚ pausa"); break;
        case "stop": s.playing = false; s.pos = 0; if (s.calibrating) { s.calibrating = false; s.song = -1; this.event("calibração encerrada"); } else this.event("■ parada"); break;
        case "seek": s.pos = Math.max(0, Math.min(m.t, this.duration())); break;
        case "volume": s.volume = m.value / 100; break;
        case "latency": s.latency = m.value; break;
        case "calibrate": s.calibrating = true; s.playing = true; s.pos = 0; s.song = -1; this.emit({ type: "timeline", timeline: null }); this.event("calibração: ajuste o atraso até a piscada cair junto com o clique"); break;
        case "sound": this.soundAction(m.arg, m.str); break;
        case "link": {
          const so = D.songs.find((x) => x.file === m.str);
          if (so) { so.link = !!m.down; so.gap = m.down ? Math.max(0, m.t || 0) : 0; }
          this.emitSetlist(); break;
        }
        case "reorder": {
          const cur = D.songs[s.song];
          D.songs.sort((a, b) => m.files.indexOf(a.file) - m.files.indexOf(b.file));
          if (cur) s.song = D.songs.indexOf(cur);
          this.emitSetlist(); break;
        }
        case "analyze": this.fakeAnalyze(m); break;
        case "removeSong": { // vai para a "lixeira": some da lista
          const i = D.songs.findIndex((x) => x.file === m.str);
          if (i >= 0) {
            if (s.song === i && s.playing) { this.event(`web: ${D.songs[i].title} está tocando: pare a música antes de remover`); break; }
            const cur = D.songs[s.song], removed = D.songs.splice(i, 1)[0];
            s.song = cur && cur !== removed ? D.songs.indexOf(cur) : -1;
            if (s.song < 0) { s.tl = null; s.pos = 0; this.emit({ type: "timeline", timeline: null }); }
          } else D.orphans = D.orphans.filter((w) => w !== m.str);
          this.emitSetlist(); this.event("♪ removida: " + m.str.replace(/\.(json|wav)$/i, "") + " (os arquivos estão em musicas/lixeira)");
          break;
        }
        case "saveTimeline": { // str: X.json (vazio = a carregada)
          const file = m.str || this.fileOf(s.song), so = this.songOf(file);
          if (!so) { this.event("web: música não está na setlist"); return; }
          for (const c of m.timeline.cues) if (c.kind === "effect" && !c.name) c.name = "LIVRE | " + FXN[c.effect.type];
          for (const c of m.timeline.cues) if (c.kind === "preset" && !c.name) c.name = c.preset.startsWith("cor:") ? "COR | " + c.preset.slice(4) : (buttons.find((b) => b.preset === c.preset) || D.config.library.find((b) => b.preset === c.preset) || {}).caption || c.preset;
          m.timeline.cues.sort((a, b) => a.t - b.t || (a.layer === "flash" ? 0 : 1) - (b.layer === "flash" ? 0 : 1));
          this.tls[file] = m.timeline;
          const first = !so.edited; so.edited = true;
          this.emitSongTimeline(file);
          if (first) this.emitSetlist();
          break;
        }
        case "revertTimeline": {
          const file = m.str || this.fileOf(s.song), so = this.songOf(file);
          if (!so || !so.edited) { this.event("web: esta música não tem edições para desfazer"); return; }
          delete this.tls[file]; so.edited = false;
          this.emitSongTimeline(file); this.emitSetlist();
          this.event(`♪ ${so.title}: timeline restaurada para a versão gerada`); break;
        }
      }
    }

    soundAction(arg, str) {
      const sd = this.s.sound;
      const labels = { output: "trocando a saída", scan: "procurando caixas", pair: "pareando", connect: "conectando", disconnect: "desconectando", forget: "esquecendo" };
      sd.busy = labels[arg] + "…"; sd.error = ""; this.emit(this.soundMsg());
      setTimeout(() => {
        const dev = sd.devices.find((d) => d.address === str);
        if (arg === "output") for (const o of sd.outputs) o.default = o.id === str;
        if (arg === "scan" && !sd.devices.some((d) => d.name === "JBL Flip 5")) sd.devices.push({ address: "11-22-33-44-55-66", name: "JBL Flip 5", paired: false, connected: false });
        if ((arg === "pair" || arg === "connect") && dev) { dev.paired = dev.connected = true; sd.speaker = dev.address; }
        if (arg === "disconnect" && dev) { dev.connected = false; if (sd.speaker === str) sd.speaker = ""; }
        if (arg === "forget") sd.devices = sd.devices.filter((d) => d.address !== str);
        sd.busy = ""; this.event("som: " + labels[arg] + " ok"); this.emit(this.soundMsg());
      }, arg === "scan" ? 2500 : 1200);
    }

    duration() { return this.s.calibrating ? 300 : this.s.tl ? this.s.tl.song.duration : 0; }

    // Cor de cada canhão agora (aproximação visual das camadas do engine).
    colors(now) {
      const s = this.s, beat = 60 / 123, step = Math.floor(s.pos / beat);
      const animate = (c, i) => { // sequência: alterna a cor com um tom vizinho a cada batida
        if ((step + i) % 2 === 0) return c;
        const [r, g, b] = rgb(c); return hex(b, r, g);
      };
      const out = s.positions.map(() => ({ color: "#000000", strobe: false }));
      if (s.blackout) return out;
      if (s.identify >= 0 && now < s.identUntil) { out[s.identify].color = "#ffffff"; return out; }
      s.identify = -1;
      const set = (fn) => out.forEach((o, i) => Object.assign(o, fn(i)));
      if (s.active.flash >= 0) set(() => ({ color: byId(s.active.flash).color || "#ffffff", strobe: true }));
      else if (s.active.strobe >= 0) set(() => ({ color: byId(s.active.strobe).color || "#ffffff", strobe: true }));
      else if (s.active.base >= 0) { const b = byId(s.active.base); set((i) => ({ color: b.group === "CORES FIXAS" ? b.color : animate(b.color, i) })); }
      else if (s.calibrating) { const on = (s.pos % 0.6) < 0.09; set(() => ({ color: on ? "#ffffff" : "#000000" })); }
      else if (s.song >= 0 && s.playing) set((i) => songLook(s.tl, s.pos, i)); // parada ou pausada: luz apagada
      return out;
    }

    tick() {
      const s = this.s, now = performance.now();
      if (s.playing) s.pos = Math.min(this.duration(), s.pos + (now - this.last) / 1000);
      if (s.pos >= this.duration()) {
        const cur = D.songs[s.song], next = D.songs[s.song + 1];
        if (s.playing && !s.calibrating && cur && cur.link && next && !next.error) { // emenda: segue para a próxima
          s.song += 1; s.tl = this.tlOf(this.fileOf(s.song)); s.pos = -(cur.gap || 0);
          this.emit({ type: "timeline", timeline: s.tl, edited: false }); this.event("▶ " + next.title + " (emendada)");
        } else s.playing = false;
      }
      this.last = now;
      const cols = this.colors(Date.now());
      this.emit({
        type: "state", blackout: s.blackout, identify: s.identify, active: s.active,
        show: s.song >= 0 && !s.calibrating ? D.songs[s.song].title : s.calibrating ? "calibração" : "",
        health: { dmx: true, audio: true },
        positions: s.positions.map((p, i) => {
          let [r, g, b] = rgb(cols[i].color), dim = 255;
          const m = p.manual;
          if (m[1] != null) r = m[1]; if (m[2] != null) g = m[2]; if (m[3] != null) b = m[3];
          if (m[0] != null) dim = m[0];
          const vals = [dim, r, g, b, 0, cols[i].strobe ? 50 : 0, cols[i].strobe ? 254 : 0];
          return {
            name: p.name, address: p.address, strobe: cols[i].strobe,
            color: hex(r * dim / 255, g * dim / 255, b * dim / 255),
            channels: ["dimmer", "red", "green", "blue", "empty", "mode", "speed"].map((role, ch) => ({ role, value: m[ch] != null ? m[ch] : vals[ch], manual: m[ch] != null })),
          };
        }),
      });
      const tl = s.tl, cues = tl ? tl.cues : [];
      const sec = !s.calibrating && tl ? tl.analysis.sections.find((x) => s.pos >= x.start && s.pos < x.end) : null;
      const cur = cues.filter((c) => c.layer === "base" && c.t <= s.pos).pop();
      const next = cues.find((c) => c.t > s.pos);
      this.emit({
        type: "player", current: s.calibrating ? -1 : s.song, playing: s.playing, position: s.pos, duration: this.duration(),
        section: s.calibrating ? "calibração" : sec ? sec.name : "", cue: !s.calibrating && cur ? (cur.kind === "blackout" ? "blackout" : cur.name) : "",
        next: !s.calibrating && next ? next.name || "blackout" : "", nextAt: next ? next.t : 0,
        audio: true, calibrating: s.calibrating, volume: s.volume, latencyMs: s.latency,
      });
    }
  }

  window.DemoSocket = DemoSocket;
})();
