"""Translates lines with a prefix-LM Japanese->English model (CAT-Translate family).

Usage: rikai.py IN.jsonl MODEL REVISION SOURCE_LANG. Prints one JSON line per
input line to stdout; library noise goes to stderr.
"""

import json
import sys

real_stdout = sys.stdout
sys.stdout = sys.stderr

import mlx.core as mx  # noqa: E402
import mlx_lm.models.llama as llama  # noqa: E402
from huggingface_hub import snapshot_download  # noqa: E402
from mlx_lm import load  # noqa: E402
from mlx_lm.models.cache import make_prompt_cache  # noqa: E402

NAMES = {"ja": "Japanese", "en": "English"}


def emit(msg):
    print(json.dumps(msg, ensure_ascii=False), file=real_stdout, flush=True)


def main(path, model_id, revision, source):
    if source not in NAMES or source == "en":
        raise SystemExit(f"this backend translates ja to en, not {source!r}")
    root = snapshot_download(model_id, revision=revision, allow_patterns=["mlx-8bit/*"])
    model, tok = load(root + "/mlx-8bit")
    causal = llama.create_attention_mask

    def translate(text, max_tokens=1024):
        msg = [{"role": "user", "content": f"Translate the following {NAMES[source]} text into English.\n\n{text}"}]
        ids = tok.apply_chat_template(msg, add_generation_prompt=True)
        cache = make_prompt_cache(model)
        llama.create_attention_mask = lambda *a, **k: None  # prompt reads both ways
        model(mx.array([ids[:-1]]), cache=cache)
        llama.create_attention_mask = causal  # generation stays causal
        logits, out = model(mx.array([ids[-1:]]), cache=cache), []
        for _ in range(max_tokens):
            t = mx.argmax(logits[0, -1]).item()
            if t in tok.eos_token_ids:
                break
            out.append(t)
            logits = model(mx.array([[t]]), cache=cache)
        return tok.decode(out).strip()

    with open(path, encoding="utf-8") as f:
        for ln in f:
            item = json.loads(ln)
            try:
                emit({"i": item["i"], "text": translate(item["text"])})
            except Exception as e:  # report it and carry on with the next line
                emit({"i": item["i"], "error": f"{type(e).__name__}: {e}"})


if __name__ == "__main__":
    main(*sys.argv[1:5])
