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

## Settings

Plain `KEY=VALUE` lines, such as `OCR_MODEL=mlx-community/Nemotron-Parse-2.0-8bit`.
Later sources win: the defaults, `rosebud/rose.conf` in your user config
directory, `rose.conf` in a film's `.film` bundle, `ROSE_`-prefixed
environment variables (`ROSE_OCR_MODEL`), then command-line flags.
Keys: `FFMPEG_HWACCEL` (`auto`, `none`, or a method from `ffmpeg -hwaccels` such as `cuda`;
only VideoToolbox is tested), `FFMPEG_HWACCEL_DEVICE`, `OCR_LANG`, `OCR_MODEL`, `OCR_TIMEOUT`, `OCR_BATCH`, `CACHE_DIR`,
`CACHE_MAX_MB`, `CACHE_MAX_AGE`.

OCR results are cached per page in your user cache directory, trimmed by size
and age on each run. `rose cache clear` deletes them.
