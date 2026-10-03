// Teste de interface num navegador de verdade (Chrome headless + CDP).
//
// 1. Suba o luzes:   ./bin/luzes -dev mock -web :8099 -musicas <pasta de teste> -som /tmp/som.yaml
// 2. Suba o Chrome:  ~/.cache/ms-playwright/chromium_headless_shell-*/chrome-headless-shell-linux64/chrome-headless-shell \
//                      --no-sandbox --remote-debugging-port=9333 --window-size=1180,820 about:blank &
// 3. Rode:           node scripts/cdp.mjs http://localhost:8099/ /tmp/print.png
//    (edite o bloco "roteiro" abaixo com os passos do teste)
// 4. Encerre:        pkill -x chrome-headless ; pkill -x luzes
//    NUNCA use `pkill -f` com um padrão que apareça no próprio comando: mata o shell.
//
// ev(js) avalia JavaScript na página (tem acesso a send(), timeline, player...).
import fs from "fs";

const [url = "http://localhost:8099/", out = "/tmp/print.png"] = process.argv.slice(2);
const page = (await (await fetch("http://localhost:9333/json")).json()).find((t) => t.type === "page");
const ws = new WebSocket(page.webSocketDebuggerUrl);
let id = 0;
const pending = new Map();
ws.onmessage = (e) => { const m = JSON.parse(e.data); if (m.id && pending.has(m.id)) { pending.get(m.id)(m); pending.delete(m.id); } };
await new Promise((r) => (ws.onopen = r));
const cmd = (method, params = {}) => new Promise((r) => { const i = ++id; pending.set(i, r); ws.send(JSON.stringify({ id: i, method, params })); });
const ev = async (expr) => {
  const r = await cmd("Runtime.evaluate", { expression: expr, awaitPromise: true, returnByValue: true });
  if (r.result.exceptionDetails) throw new Error(JSON.stringify(r.result.exceptionDetails));
  return r.result.result.value;
};
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const shot = async (file) => { const r = await cmd("Page.captureScreenshot", { format: "png" }); fs.writeFileSync(file, Buffer.from(r.result.data, "base64")); };

await cmd("Page.enable");
await cmd("Runtime.enable");
// Passa por about:blank: navegar para a mesma URL só mudando o #aba não
// recarrega a página (o teste rodaria na versão antiga).
await cmd("Page.navigate", { url: "about:blank" });
await sleep(200);
await cmd("Page.navigate", { url });
await sleep(1500);

// ---- roteiro ----
await ev(`send({type:"volume", value:0})`);
await ev(`document.querySelectorAll("#setlist .song")[0]?.click()`);
await sleep(600);
console.log("setlist:", await ev(`[...document.querySelectorAll("#setlist .song .title")].map(x => x.firstChild.textContent).join(" | ")`));
// ---- fim do roteiro ----

await shot(out);
console.log("print em", out);
process.exit(0);
