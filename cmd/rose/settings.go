package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/tjagithub/rosebud/internal/config"
)

// settings lists every key a stage reads from config files and ROSE_ variables.
var settings = config.Schema{
	{Name: "FFMPEG_HWACCEL", Default: "auto", Doc: "hardware video decode for scene detection: auto, none, or a method from ffmpeg -hwaccels", Check: isHWAccelName},
	{Name: "FFMPEG_HWACCEL_DEVICE", Default: "", Doc: "which device to decode on when there are several, such as /dev/dri/renderD129"},
	{Name: "OCR_LANG", Default: "ja", Doc: "language code of the subtitle track to read"},
	{Name: "OCR_MODEL", Default: nemotronModel, Doc: "OCR model"},
	{Name: "OCR_TIMEOUT", Default: "30m", Doc: "longest the render and OCR steps may take", Check: config.IsDuration},
	{Name: "OCR_BATCH", Default: "16", Doc: "pages read per model start; finished batches are cached", Check: config.IsPositiveInt},
	{Name: "TRANSLATE_LANG", Default: "ja", Doc: "language code of the scene text to translate"},
	{Name: "TRANSLATE_MODEL", Default: plamoModel, Doc: "translation model"},
	{Name: "TRANSLATE_TIMEOUT", Default: "30m", Doc: "longest the model may take to load and translate a batch", Check: config.IsDuration},
	{Name: "TRANSLATE_BATCH", Default: "50", Doc: "lines translated per model start; finished batches are cached", Check: config.IsPositiveInt},
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
	switch setting {
	case "", "none":
		return ""
	case "auto":
		if goos == "darwin" {
			return "videotoolbox"
		}
		return "auto"
	}
	return setting
}

var hwName = regexp.MustCompile(`^[a-z0-9_]+$`)

// isHWAccelName accepts auto, none, or a lower-case ffmpeg method name. Whether
// this ffmpeg offers the method is checked when a run starts.
func isHWAccelName(v string) error {
	if !hwName.MatchString(v) {
		return fmt.Errorf("%q is not auto, none or an ffmpeg method such as cuda or vaapi", v)
	}
	return nil
}
