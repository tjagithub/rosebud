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
	{Name: "OCR_LANG", Default: "ja", Doc: "language code of the subtitle track to read"},
	{Name: "OCR_MODEL", Default: nemotronModel, Doc: "OCR model"},
	{Name: "OCR_TIMEOUT", Default: "30m", Doc: "longest the render and OCR steps may take", Check: config.IsDuration},
}

// loadConfig merges the user file, the film's bundle file and the environment.
func loadConfig(ctx context.Context, bundle string, environ []string) (config.Config, error) {
	var layers []config.Layer
	paths := []string{filepath.Join(bundle, "rose.conf")}
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
