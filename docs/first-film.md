# Your first film

rose reads a film file and writes a **bundle** next to it: `Film.mkv` gets
`Film.mkv.film/`. The bundle holds the scene frames and a SQLite database
(`rose.db`) with each scene's times, speech, subtitles and translation. rose never
changes the film itself.

## 1. Build everything at once

```sh
rose build ~/Films/Sonatine.mkv
```

`build` runs the stages in order: `scenes`, then `subs` (speech), `ocr` (the
subtitle track) and `translate`. If one stage fails, only the stages that need it
stop; the others still run. It also skips stages that have nothing to do, such as
`ocr` on a film with no subtitle track.

Useful flags:

```sh
rose build FILE --plan                 # list the steps, run nothing
rose build FILE --skip ocr,translate   # leave stages out
rose build FILE --scenes.max-len 30s   # pass a flag to one stage
```

A feature-length film takes a while, mostly in `subs` and `ocr`. A progress bar
shows each stage.

## 2. Or run the stages one by one

```sh
rose lang FILE        # which language is spoken? (optional; subs does this itself)
rose scenes FILE      # cut into scenes, one frame each — run this first
rose subs FILE        # transcribe the speech
rose ocr FILE         # read subtitles: text (SRT, ASS, also NAME.en.vtt beside FILE) as is, DVD/Blu-ray by OCR
rose ocr FILE --track other.srt   # or exactly this subtitle file (or track number)
rose ocr FILE --burned   # or read subtitles burned into the picture
rose translate FILE   # add English, so English searches find Japanese lines
```

Each stage runs again safely. Running `scenes` again keeps the text of scenes whose
span did not change. If the cuts move, run `subs` and `ocr` again.

## 3. Search

```sh
rose search --bundle ~/Films/Sonatine.mkv.film "sink it"
```

The output has one line per scene, best match first: the time, then the translation,
the subtitle and the spoken text. Search matches all three, and Japanese matches on
parts of words too.

## 4. Browse in the TUI

```sh
rose tui --dir ~/Films                       # every bundle in a folder
rose tui --bundle ~/Films/Sonatine.mkv.film  # one film
```

**Library:** `tab` moves between the menu and the films, `enter` opens a film, `s`
searches all films, `/` filters the list, `esc` quits.

**Scene:**

| Key | Does |
|---|---|
| `←` / `→` | previous / next scene |
| `p` | play from this scene; `esc` stops on the scene reached |
| `s` | scene picker |
| `t` | jump to a timecode (`HH:MM:SS:FF`) |
| `/` | filter the dialogue list |
| `tab` | move focus between text boxes; `↑`/`↓` scroll them |
| `esc` | back |

## 5. Clean up

```sh
rm -r ~/Films/Sonatine.mkv.film   # forget a film (the film file stays)
rose cache clear                  # drop cached OCR and translation results
```
