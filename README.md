# rosebud

Search and browse your own films from the terminal. Type a line of
dialogue, find the frame.

## Requirements

- `ffmpeg` and `ffprobe` with the `silencedetect` and `ametadata` filters.
- `uv`, which runs `parakeet-mlx` for `rose subs` (Apple Silicon).
- `uv` also runs Nemotron-Parse for `rose ocr`, which reads bitmap subtitles.

Speech models are downloaded on first use: NVIDIA Parakeet, licensed
CC-BY-4.0. `parakeet-mlx` is Apache-2.0. Nemotron-Parse
(Apache-2.0) reads bitmap subtitles.
