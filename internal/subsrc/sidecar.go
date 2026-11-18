package subsrc

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// sidecarCodecs maps a subtitle file's extension to the codec ffprobe names.
var sidecarCodecs = map[string]string{".srt": "subrip", ".vtt": "webvtt", ".ass": "ass", ".ssa": "ssa"}

// titleWords are the name parts that describe a track rather than give its
// language, even where they are also language codes ("sdh" is Southern Kurdish).
var titleWords = map[string]bool{"forced": true, "sdh": true, "cc": true, "hi": true, "signs": true, "full": true, "default": true}

// Sidecars lists the subtitle files beside video named after it, such as
// NAME.srt, NAME.en.vtt or NAME.ja.forced.ass, as YouTube, Plex and Jellyfin name them.
func Sidecars(video string) ([]Track, error) {
	if video == "" {
		return nil, errors.New("subsrc: sidecars: empty video path")
	}
	dir := filepath.Dir(video)
	stem := strings.TrimSuffix(filepath.Base(video), filepath.Ext(video))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("subsrc: sidecars of %s: %w", video, err)
	}
	var out []Track
	for _, e := range entries {
		name := e.Name()
		codec, ok := sidecarCodecs[strings.ToLower(filepath.Ext(name))]
		base := strings.TrimSuffix(name, filepath.Ext(name))
		if e.IsDir() || !ok || base != stem && !strings.HasPrefix(base, stem+".") {
			continue
		}
		t := Track{Codec: codec, Path: filepath.Join(dir, name)}
		t.Lang, t.Title = nameTags(strings.TrimPrefix(strings.TrimPrefix(base, stem), "."))
		out = append(out, t)
	}
	return out, nil
}

// File is the track of any subtitle file, named after the video or not.
func File(path string) (Track, error) {
	codec, ok := sidecarCodecs[strings.ToLower(filepath.Ext(path))]
	if !ok {
		return Track{}, fmt.Errorf("subsrc: %q is not a subtitle file (.srt, .vtt, .ass or .ssa)", path)
	}
	info, err := os.Stat(path)
	if err != nil {
		return Track{}, fmt.Errorf("subsrc: %w", err)
	}
	if info.IsDir() {
		return Track{}, fmt.Errorf("subsrc: %s is a folder", path)
	}
	return Track{Codec: codec, Path: path}, nil
}

// nameTags splits the dotted middle of a sidecar's name, "ja.forced", into a
// language and a title. A part that is neither is taken as the title too.
func nameTags(mid string) (lang, title string) {
	if mid == "" {
		return "", ""
	}
	var words []string
	for _, p := range strings.Split(mid, ".") {
		if lang == "" && !titleWords[strings.ToLower(p)] && isLang(p) {
			lang = canon(p)
			continue
		}
		words = append(words, p)
	}
	return lang, strings.Join(words, " ")
}
