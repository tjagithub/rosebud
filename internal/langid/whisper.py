"""Language ID with Whisper: softmax over the language tokens after <|startoftranscript|>.

usage: whisper.py MODEL WAV...   prints one JSON line: a list with, per WAV,
an object of the five likeliest {code: probability}, or null if it was unreadable.
"""
import json
import sys
import wave

import numpy as np
import torch
from transformers import WhisperFeatureExtractor, WhisperForConditionalGeneration, WhisperTokenizer
from transformers.models.whisper.tokenization_whisper import LANGUAGES

SR = 16000


def read(path):
    with wave.open(path) as w:
        if w.getframerate() != SR or w.getnchannels() != 1 or w.getsampwidth() != 2:
            raise ValueError("want 16 kHz mono s16 WAV")
        pcm = np.frombuffer(w.readframes(w.getnframes()), dtype=np.int16)
    return pcm.astype(np.float32) / 32768.0


def main():
    name, wavs = sys.argv[1], sys.argv[2:]
    fe, tok = WhisperFeatureExtractor.from_pretrained(name), WhisperTokenizer.from_pretrained(name)
    model = WhisperForConditionalGeneration.from_pretrained(name).eval()
    codes = list(LANGUAGES)
    ids = [tok.convert_tokens_to_ids(f"<|{c}|>") for c in codes]
    start = tok.convert_tokens_to_ids("<|startoftranscript|>")
    out = []
    for path in wavs:
        try:
            feats = fe(read(path), sampling_rate=SR, return_tensors="pt").input_features
            with torch.no_grad():
                logits = model(input_features=feats, decoder_input_ids=torch.tensor([[start]])).logits[0, -1]
            p = torch.softmax(logits[ids], dim=-1).cpu().numpy()
            top = np.argsort(p)[::-1][:5]
            out.append({codes[i]: round(float(p[i]), 4) for i in top})
        except Exception as e:  # one bad window must not lose the rest
            print(f"{path}: {e}", file=sys.stderr)
            out.append(None)
    print(json.dumps(out))


main()
