#!/usr/bin/env bash
# Instala o "ouvido de bateria" do DMXController: um ambiente Python à parte,
# na pasta .ouvido do projeto, com o Demucs (separação de instrumentos). Com
# ele, a análise de cada música separa a bateria e ouve a batida nela (bumbo,
# caixa, entradas e saídas da bateria). Sem ele, o programa funciona igual e
# estima a batida na música inteira.
#
#   scripts/instalar_ouvido.sh          instala (ou atualiza) em .ouvido
#   rm -rf .ouvido                      desinstala (nada fica fora da pasta)
#
# Ocupa cerca de 1,2 GB (PyTorch só de CPU + modelo de 80 MB) e precisa de
# internet só na instalação. Não usa sudo nem mexe no Python do sistema.
set -euo pipefail
cd "$(dirname "$0")/.."
mkdir -p "${1:-.ouvido}"
DIR="$(cd "${1:-.ouvido}" && pwd)"

case "$(uname -s)-$(uname -m)" in
  Linux-x86_64) UV=uv-x86_64-unknown-linux-gnu ;;
  Linux-aarch64) UV=uv-aarch64-unknown-linux-gnu ;; # Raspberry Pi
  Darwin-arm64) UV=uv-aarch64-apple-darwin ;;
  Darwin-x86_64) UV=uv-x86_64-apple-darwin ;;
  *) echo "sistema não suportado: $(uname -s) $(uname -m)" >&2; exit 1 ;;
esac

# uv: baixa um Python próprio (3.12) e instala os pacotes, tudo dentro da pasta.
mkdir -p "$DIR/bin"
if [ ! -x "$DIR/bin/uv" ]; then
  echo "baixando o instalador (uv)..."
  curl -LsSf "https://github.com/astral-sh/uv/releases/latest/download/$UV.tar.gz" | tar xz --strip-components=1 -C "$DIR/bin"
fi
export UV_PYTHON_INSTALL_DIR="$DIR/python" UV_CACHE_DIR="$DIR/cache"

echo "criando o ambiente Python..."
"$DIR/bin/uv" venv --quiet --allow-existing --python 3.12 "$DIR/venv"
PY="$DIR/venv/bin/python"

echo "instalando o PyTorch e o Demucs (cerca de 1 GB)..."
if [ "$(uname -s)" = Linux ]; then # só CPU: a versão com placa de vídeo ocupa vários GB a mais
  "$DIR/bin/uv" pip install --quiet --python "$PY" torch torchaudio --index-url https://download.pytorch.org/whl/cpu
else
  "$DIR/bin/uv" pip install --quiet --python "$PY" torch torchaudio
fi
"$DIR/bin/uv" pip install --quiet --python "$PY" demucs soundfile
rm -rf "$DIR/cache"

echo "baixando o modelo de separação (80 MB)..."
HF_HOME="$DIR/hf" "$PY" -c 'from demucs.pretrained import get_model; get_model("htdemucs")' >/dev/null

echo "pronto: ouvido de bateria instalado em $DIR ($(du -sh "$DIR" | cut -f1))."
echo "Abra o DMXController de novo; as próximas análises já separam a bateria."
