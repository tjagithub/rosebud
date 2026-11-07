package main

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/tjagithub/rosebud/internal/config"
)

// settings lists every key a stage reads from config files and ROSE_ variables.
var settings = config.Schema{
	{Name: "FFMPEG_HWACCEL", Default: "auto", Doc: "hardware video decode for scene detection: auto or none", Check: config.OneOf("auto", "none")},
	{Name: "OCR_LANG", Default: "ja", Doc: "language code of the subtitle track to read"},
	{Name: "OCR_MODEL", Default: nemotronModel, Doc: "OCR model"},
	{Name: "OCR_TIMEOUT", Default: "30m", Doc: "longest the render and OCR steps may take", Check: config.IsDuration},
	{Name: "OCR_BATCH", Default: "16", Doc: "pages read per model start; finished batches are cached", Check: config.IsPositiveInt},
	{Name: "CACHE_DIR", Default: "", Doc: "where cached results go (default: your user cache directory)"},
	{Name: "CACHE_MAX_MB", Default: "500", Doc: "cache size limit in megabytes", Check: config.IsPositiveInt},
	{Name: "CACHE_MAX_AGE", Default: "2160h", Doc: "drop cache entries unused for this long", Check: config.IsDuration},
}

// loadConfig merges the user file, the film's bundle file and the environment.
func loadConfig(ctx context.Context, bundle string, environ []string) (config.Config, error) {
	var layers []config.Layer
	var paths []string
	if bundle != "" {
		paths = append(paths, filepath.Join(bundle, "rose.conf"))
	}
	if dir, err := os.UserConfigDir(); err == nil {
		paths = append([]string{filepath.Join(dir, "rosebud", "rose.conf")}, paths...)
	}
	for _, p := range paths {
		l, err := config.File(ctx, p, 15*time.Second)
		if err != nil {
			return config.Config{}, err
		}
		layers = append(layers, l)
	}
	return settings.Load(append(layers, config.Env(environ))...)
}

// cacheDir is the configured cache directory, or the user's cache directory.
func cacheDir(cfg config.Config) (string, error) {
	if d := cfg.Get("CACHE_DIR"); d != "" {
		return d, nil
	}
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "rosebud", "ocr"), nil
}

// hwaccelFor turns the FFMPEG_HWACCEL setting into an ffmpeg -hwaccel method,
// or "" for software decoding.
func hwaccelFor(setting, goos string) string {
	if setting == "auto" && goos == "darwin" {
		return "videotoolbox"
	}
	return ""
}
