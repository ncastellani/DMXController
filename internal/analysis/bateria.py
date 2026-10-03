"""Separa a bateria de uma música (Demucs) para o DMXController ouvir a batida.

    python bateria.py <musica.wav> <bateria.wav>

Lê a música, separa os instrumentos com o modelo htdemucs e grava só a
bateria (mono, 16 bits, 22,05 kHz: a taxa em que o DMXController analisa). Imprime o andamento em linhas
"progresso <0-100>". O DMXController chama este script (com o Python do
ambiente instalado por scripts/instalar_ouvido.sh) na análise de cada música e
guarda o resultado em cache; sem o ambiente, a análise segue sem ele.
"""
import os
import sys
from pathlib import Path

# O modelo (~80 MB, baixado na primeira vez) fica junto do ambiente, para
# apagar tudo de uma vez; depois de baixado, não precisa de internet.
home = Path(os.environ.setdefault("HF_HOME", str(Path(sys.prefix).parent / "hf")))
if any(home.glob("hub/models--*HTDemucs*/snapshots/*/*.safetensors")):
    os.environ.setdefault("HF_HUB_OFFLINE", "1")

import soundfile as sf
import torch
from demucs.apply import apply_model
from demucs.pretrained import get_model


def main(src: str, dst: str) -> None:
    audio, rate = sf.read(src, dtype="float32", always_2d=True)  # (amostras, canais)
    wav = torch.from_numpy(audio.T.copy())
    if wav.shape[0] == 1:
        wav = wav.repeat(2, 1)
    wav = wav[:2]

    model = get_model("htdemucs")
    model.eval()
    if rate != model.samplerate:
        import julius

        wav = julius.resample_frac(wav, rate, model.samplerate)

    ref = wav.mean(0)
    mean, std = ref.mean(), ref.std() + 1e-8
    total = wav.shape[-1]
    last = [-1]

    def progress(info: dict) -> None:
        if info.get("state") != "start":
            return
        pct = min(99, int(100 * info["segment_offset"] / max(1, info.get("audio_length", total))))
        if pct > last[0]:
            last[0] = pct
            print(f"progresso {pct}", flush=True)

    with torch.no_grad():
        out = apply_model(model, ((wav - mean) / std)[None], device="cpu", shifts=1, split=True,
                          overlap=0.25, progress=False, callback=progress)[0]
    drums = (out[model.sources.index("drums")] * std + mean).mean(0, keepdim=True)  # mono
    import julius

    rate_out = 22050
    mono = julius.resample_frac(drums, model.samplerate, rate_out)[0].clamp(-1, 1).numpy()
    tmp = dst + ".tmp"
    sf.write(tmp, mono, rate_out, subtype="PCM_16", format="WAV")
    os.replace(tmp, dst)
    print("progresso 100", flush=True)


if __name__ == "__main__":
    if len(sys.argv) != 3:
        sys.exit(__doc__)
    main(sys.argv[1], sys.argv[2])
