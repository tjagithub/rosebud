"""Translates lines with PLaMo-3 and prints one JSON line per input line.

Usage: plamo.py IN.jsonl MODEL REVISION SOURCE_LANG. Only JSON lines go to
stdout; anything the model libraries print is sent to stderr.
"""

import json
import sys

real_stdout = sys.stdout
sys.stdout = sys.stderr

from mlx_lm import generate, load  # noqa: E402
from mlx_lm.sample_utils import make_sampler  # noqa: E402


def emit(msg):
    print(json.dumps(msg, ensure_ascii=False), file=real_stdout, flush=True)


def main(path, model_id, revision, source):
    # remote code runs here, so the revision is pinned by the caller
    model, tok = load(
        model_id,
        revision=revision,
        trust_remote_code=True,
        tokenizer_config={"trust_remote_code": True},
    )
    sampler = make_sampler(temp=0)
    with open(path, encoding="utf-8") as f:
        for ln in f:
            item = json.loads(ln)
            try:
                prompt = tok.apply_chat_template(
                    [{"role": "user", "content": item["text"]}],
                    source_lang=source,
                    target_lang="en",
                    add_generation_prompt=True,
                    tokenize=False,
                )
                out = generate(model, tok, prompt=prompt, max_tokens=512, sampler=sampler)
                emit({"i": item["i"], "text": out.strip()})
            except Exception as e:  # report it and carry on with the next line
                emit({"i": item["i"], "error": f"{type(e).__name__}: {e}"})


if __name__ == "__main__":
    main(*sys.argv[1:5])
