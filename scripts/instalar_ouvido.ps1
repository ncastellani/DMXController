# Instala o "ouvido de bateria" do DMXController no Windows: um ambiente
# Python à parte, na pasta .ouvido do projeto, com o Demucs (separação de
# instrumentos). Com ele, a análise de cada música separa a bateria e ouve a
# batida nela (bumbo, caixa, entradas e saídas da bateria). Sem ele, o
# programa funciona igual e estima a batida na música inteira. É a versão
# Windows de scripts/instalar_ouvido.sh.
#
#   powershell -ExecutionPolicy Bypass -File scripts\instalar_ouvido.ps1   instala (ou atualiza) em .ouvido
#   Remove-Item -Recurse -Force .ouvido                                      desinstala (nada fica fora da pasta)
#
# Ocupa cerca de 1,2 GB (PyTorch só de CPU + modelo de 80 MB) e precisa de
# internet só na instalação. Não pede administrador nem mexe no Python do
# sistema. Só para Windows de 64 bits com processador Intel ou AMD.
param([string]$Pasta = ".ouvido")

$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue" # o Invoke-WebRequest fica muito mais rápido sem a barra
Set-Location (Split-Path -Parent $PSScriptRoot)

if ($env:PROCESSOR_ARCHITECTURE -ne "AMD64") {
  throw "sistema não suportado: Windows $env:PROCESSOR_ARCHITECTURE (o ouvido precisa de Windows de 64 bits com Intel ou AMD)"
}

# Para tudo se o último programa chamado falhou.
function Confere([string]$OQue) {
  if ($LASTEXITCODE -ne 0) { throw "falhou ao $OQue (código $LASTEXITCODE)" }
}

New-Item -ItemType Directory -Force -Path $Pasta | Out-Null
$Dir = (Resolve-Path $Pasta).Path
$Bin = Join-Path $Dir "bin"
New-Item -ItemType Directory -Force -Path $Bin | Out-Null

# uv: baixa um Python próprio (3.12) e instala os pacotes, tudo dentro da pasta.
$Uv = Join-Path $Bin "uv.exe"
if (-not (Test-Path $Uv)) {
  Write-Host "baixando o instalador (uv)..."
  [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
  $Zip = Join-Path $Dir "uv.zip"
  $Tmp = Join-Path $Dir "uv-tmp"
  Invoke-WebRequest "https://github.com/astral-sh/uv/releases/latest/download/uv-x86_64-pc-windows-msvc.zip" -OutFile $Zip -UseBasicParsing
  Expand-Archive $Zip -DestinationPath $Tmp -Force
  Get-ChildItem $Tmp -Recurse -Filter *.exe | Move-Item -Destination $Bin -Force
  Remove-Item -Recurse -Force $Zip, $Tmp
}
$env:UV_PYTHON_INSTALL_DIR = Join-Path $Dir "python"
$env:UV_CACHE_DIR = Join-Path $Dir "cache"
$Venv = Join-Path $Dir "venv"
$Py = Join-Path $Venv "Scripts\python.exe"

Write-Host "criando o ambiente Python..."
& $Uv venv --quiet --allow-existing --python 3.12 $Venv
Confere "criar o ambiente Python"

Write-Host "instalando o PyTorch e o Demucs (cerca de 1 GB)..."
& $Uv pip install --quiet --python $Py torch torchaudio --index-url https://download.pytorch.org/whl/cpu
Confere "instalar o PyTorch"
& $Uv pip install --quiet --python $Py demucs soundfile
Confere "instalar o Demucs"
Remove-Item -Recurse -Force $env:UV_CACHE_DIR -ErrorAction SilentlyContinue

Write-Host "baixando o modelo de separação (80 MB)..."
$env:HF_HOME = Join-Path $Dir "hf"
& $Py -c "from demucs.pretrained import get_model; get_model('htdemucs')" | Out-Null
Confere "baixar o modelo"

$Tamanho = "{0:N1} GB" -f ((Get-ChildItem $Dir -Recurse -File | Measure-Object Length -Sum).Sum / 1GB)
Write-Host "pronto: ouvido de bateria instalado em $Dir ($Tamanho)."
Write-Host "Abra o DMXController de novo; as próximas análises já separam a bateria."
