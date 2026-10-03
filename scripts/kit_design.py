#!/usr/bin/env python3
"""Gera o kit para redesenhar a interface no Claude Design.

Uso:  python3 scripts/kit_design.py

Gera em design/:
  luzes-demo.html   a interface atual num arquivo único, já em modo
                    demonstração (dados de exemplo, sem precisar do luzes)
  prints/*.png      todas as abas em computador, iPad e celular
O briefing (design/brief.md) é escrito à mão; este script não mexe nele.
"""

import glob
import os
import shutil
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
STATIC = ROOT / "internal" / "web" / "static"
OUT = ROOT / "design"

TABS = ["show", "console", "musicas", "manual", "palco", "som", "log"]
# Prints extras (o editor parado no "verde-limão" da versão cantada; sem as
# transições de cor do palco, que não andam no tempo virtual do print): aba,
# script rodado numa cópia temporária da página (o
# luzes-demo.html entregue não muda) e tamanhos (no celular o editor fica
# abaixo da lista, fora do print).
EXTRAS = {
    "musicas-editor": ("musicas", 'document.head.insertAdjacentHTML("beforeend", "<style>*{transition:none!important}</style>");'
                       ' setTimeout(() => { document.querySelector("#mSongs .opt").click();'
                       ' setTimeout(() => ed.player.seek(42.6), 300); }, 800);', ("computador", "ipad")),
}
SIZES = {"computador": (1440, 900), "ipad": (1180, 820), "celular": (390, 844)}


def build_demo():
    page = (STATIC / "index.html").read_text()
    demo = (STATIC / "demo.js").read_text()
    if "<html" not in page or "<script>" not in page:
        sys.exit("index.html inesperado")
    page = page.replace('<html lang="pt-BR">', '<html lang="pt-BR" data-demo>', 1)
    # demo.js entra antes do script principal, já carregado.
    i = page.index("<script>")
    page = page[:i] + "<script>\n" + demo + "\n</script>\n" + page[i:]
    out = OUT / "luzes-demo.html"
    out.write_text(page)
    return out


def find_chrome():
    cands = sorted(glob.glob(os.path.expanduser(
        "~/.cache/ms-playwright/chromium_headless_shell-*/chrome-headless-shell-*/chrome-headless-shell")))
    cands += [shutil.which(n) for n in ("chromium", "chromium-browser", "google-chrome")]
    return next((c for c in reversed(cands) if c), None)


def screenshots(demo):
    chrome = find_chrome()
    if not chrome:
        print("aviso: nenhum Chrome/Chromium encontrado; pulei os prints")
        return
    prints = OUT / "prints"
    prints.mkdir(parents=True, exist_ok=True)

    def shot(page, tab, png, w, h):
        subprocess.run([
            chrome, "--headless", "--no-sandbox", "--hide-scrollbars", "--allow-file-access-from-files",
            f"--window-size={w},{h}", "--virtual-time-budget=2500",
            f"--screenshot={png}", f"{page.as_uri()}#{tab}",
        ], check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=60)

    n = 0
    for name, (w, h) in SIZES.items():
        for tab in TABS:
            shot(demo, tab, prints / f"{name}-{tab}.png", w, h)
            n += 1
        for extra, (tab, js, sizes) in EXTRAS.items():
            if name not in sizes:
                continue
            tmp = OUT / f".print-{extra}.html"
            tmp.write_text(demo.read_text().replace("</body>", f"<script>{js}</script>\n</body>", 1))
            try:
                shot(tmp, tab, prints / f"{name}-{extra}.png", w, h)
                n += 1
            finally:
                tmp.unlink()
    print(f"{n} prints em {prints.relative_to(ROOT)}")


def main():
    OUT.mkdir(parents=True, exist_ok=True)
    demo = build_demo()
    print(f"página de demonstração: {demo.relative_to(ROOT)}")
    screenshots(demo)


if __name__ == "__main__":
    main()
