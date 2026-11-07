"""Reads page images with Nemotron-Parse and prints one JSON line per page.

Usage: nemotron.py PAGES_DIR MODEL. Only JSON lines go to stdout; anything the
model libraries print is sent to stderr so the caller can parse stdout.
"""

import json
import os
import sys

real_stdout = sys.stdout
sys.stdout = sys.stderr

from mlx_vlm import generate, load  # noqa: E402
from mlx_vlm.tokenizer_utils import NaiveStreamingDetokenizer  # noqa: E402

# The port seeds the decoder with this, so it must be this exact string.
PROMPT = "</s><s><predict_bbox><predict_classes><output_markdown><predict_no_text_in_pic>"


def emit(msg):
    print(json.dumps(msg, ensure_ascii=False), file=real_stdout, flush=True)


def main(pages_dir, model_id):
    model, processor = load(model_id)
    # the default BPE detokenizer crashes on Japanese
    processor.detokenizer = NaiveStreamingDetokenizer(processor.tokenizer)
    for name in sorted(f for f in os.listdir(pages_dir) if f.endswith(".png")):
        try:
            r = generate(
                model,
                processor,
                PROMPT,
                image=[os.path.join(pages_dir, name)],
                max_tokens=900,
                repetition_penalty=1.1,
                verbose=False,
            )
            emit({"page": name, "text": getattr(r, "text", r)})
        except Exception as e:  # report it and carry on with the next page
            emit({"page": name, "error": f"{type(e).__name__}: {e}"})


if __name__ == "__main__":
    main(sys.argv[1], sys.argv[2])
