# Development

Do [Setup](setup.md) first.

## Layout

```
cmd/rose/          CLI: one file per command (cobra under fang)
internal/
  core/            shared types and the Client interface
  engine/          in-process Client over the store, jobs and events
  jobs/ events/    bounded job runner, event bus
  store/           SQLite (ncruces, CGO-free) with FTS5 and sqlite-vec, migrations
  plan/            the build graph: steps, what each needs, verdicts
  scenes/          cut detection and frames (ffmpeg, hwaccel)
  subs/            speech to text (Parakeet), audio chunking
  langid/          spoken-language detection (Whisper)
  subsrc/          probing and picking subtitle tracks
  ocr/             bitmap and burned-in subtitle OCR (Nemotron-Parse)
  translate/       rikai / PLaMo backends
  ui/              bubbletea TUI; ui/uitest drives it in tests
  player/          playback: ffmpeg raw frames to kitty, ffplay audio
  config/ cache/ lock/ limit/ shelf/   settings, result cache, bundle lock, timeouts, library
```

The Python scripts next to their Go callers (`*.py`) run through `uv run --with …`
with pinned versions. Change a pin in the Go file that lists the dependencies.

## Everyday commands

```sh
task check        # tidy, fmt, vet, lint, test: run before every commit
task test         # go test ./...
task lint         # golangci-lint
task vuln         # govulncheck
task complexity   # the most complex functions (cccc)
```

Run tests with `-race` when you touch goroutines: `go test -race ./...`.

## How work is done

- **Tests first.** Write a failing test, then the code. Property tests use
  `pgregory.net/rapid`. Goroutine-heavy packages check for leaks with `goleak`. Never
  call `t.Fatal` from a goroutine other than the test's own.
- **Every external call has a timeout.** Every ffmpeg, ffprobe and uv call goes through
  a deadline (`internal/limit`). Check inputs before starting slow work.
- **No real models in unit tests.** Stages take an `Exec` func, so tests inject
  fakes. Run against a real film by hand.
- **Commits:** small changes go straight to `main`. Bigger ones go on a `wt` worktree
  branch and are squash-merged back. Messages look like `area: what changed`, for
  example `ui: play at 24 fps`.
- **README** stays short and user-facing. Longer guides go in `docs/`.

## Debugging

- `--debug` on `ocr` and `translate` prints each command and how long it took.
- `ROSE_PLAY_TRACE=/tmp/trace.log` logs the timing of each playback frame.
  `ROSE_PLAY_FPS=12` overrides the playback rate.
- `ROSE_<KEY>` sets any setting for one run, for example
  `ROSE_FFMPEG_HWACCEL=none rose scenes FILE`.
