# Setup

This guide takes a fresh machine to a working `rose` binary.

## What you need

- **A Mac with Apple Silicon.** Speech (`rose subs`), OCR (`rose ocr`) and translation
  (`rose translate`) run models on MLX, which needs Apple Silicon. Scene cutting,
  search and the TUI work on any machine, but nobody has tried them on other platforms.
- **About 15 GB of free disk** for the models (the Japanese speech model alone is
  4.6 GB). They download on first use into the Hugging Face cache:
  `~/.cache/huggingface`, or `$HF_HOME` if you have set it.
- **A terminal that speaks the kitty graphics protocol** with Unicode placeholders, so
  the TUI can draw frames. Kitty and Ghostty support it. iTerm2 is untested.

## 1. Homebrew packages

```sh
brew install ffmpeg uv mise
```

- `ffmpeg` gives `ffmpeg`, `ffprobe` and `ffplay`. rose cuts scenes and audio with
  them, and plays films from the TUI with `ffplay`. The build must include the
  `silencedetect` and `ametadata` filters, and Homebrew's does.
- `uv` runs the Python model scripts. Each script gets its own pinned environment,
  so you do not install any Python packages yourself. uv fetches Python 3.12 when a
  stage first needs it.
- `mise` installs the pinned Go toolchain and the dev tools.

Check:

```sh
ffmpeg -hide_banner -filters | rg 'silencedetect|ametadata'
ffplay -version | head -n 1
uv --version
```

## 2. Get the code and the toolchain

```sh
git clone https://github.com/tjagithub/rosebud.git
cd rosebud
mise trust && mise install
```

`mise.toml` pins Go, `task`, `golangci-lint`, `govulncheck` and `air`, and sets
`CGO_ENABLED=0`. rose is pure Go: the SQLite driver is a WebAssembly build, so it
needs no C compiler.

## 3. Build and check

```sh
task build      # writes ./rose
task check      # tidy, format, vet, lint, test
./rose --help
```

To have `rose` on your `PATH`, run `go install ./cmd/rose` (it goes to `~/go/bin`).

## 4. Shell completions (optional)

```sh
rose completion zsh > "${fpath[1]}/_rose"
```

Bash and fish work the same way: `rose completion bash` and `rose completion fish`.

## 5. Settings (optional)

The defaults work. To change one, put `KEY=VALUE` lines in
`~/Library/Application Support/rosebud/rose.conf`, for example:

```
FFMPEG_HWACCEL=auto
TRANSLATE_BACKEND=rikai
```

The README lists every key and the order in which sources override each other.

## Models used

You never install these yourself. Each stage downloads its model at a pinned version
the first time it runs.

| Stage | Model | Runs through |
|---|---|---|
| `rose lang` | `openai/whisper-tiny` | `uv run` (torch, transformers) |
| `rose subs` | `mlx-community/parakeet-tdt-0.6b-v3`, `…-tdt_ctc-0.6b-ja` for Japanese | `uvx parakeet-mlx==0.7.0` |
| `rose ocr` | `mlx-community/Nemotron-Parse-2.0-8bit` | `uv run` (mlx) |
| `rose translate` | rikai Bench 265 0.8B, or PLaMo-3 with `--backend plamo` | `uv run` (mlx-lm) |

The first run of each stage is slow because it downloads the model. Later runs use
the cache.

## Troubleshooting

- **`uv: command not found` during a stage:** the stage shells out to `uv`, so `uv`
  must be on the `PATH` of the shell that runs `rose`.
- **The TUI shows no pictures:** your terminal does not support kitty graphics with
  Unicode placeholders. Search and text still work.
- **Playback has no sound:** check that `ffplay` exists. Some minimal ffmpeg builds
  leave it out.
- **A stage says the bundle is locked:** another `rose` process is writing to that
  film, and the message names it. Wait for it to finish, or stop it.
