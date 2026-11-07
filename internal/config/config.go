// Package config reads flat KEY=VALUE settings from layered sources.
package config

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

var keyRE = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

// Parse reads KEY=VALUE lines. Blank lines and # comments are skipped, an
// "export " prefix is allowed, and a later duplicate key wins.
func Parse(r io.Reader) (map[string]string, error) {
	out := map[string]string{}
	sc := bufio.NewScanner(r)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		k, v, ok := strings.Cut(line, "=")
		k = strings.TrimSpace(k)
		if !ok || !keyRE.MatchString(k) {
			return nil, fmt.Errorf("line %d: want KEY=VALUE with an upper-case KEY", n)
		}
		out[k] = unquote(strings.TrimSpace(v))
	}
	return out, sc.Err()
}

func unquote(v string) string {
	if len(v) >= 2 && v[0] == v[len(v)-1] && (v[0] == '"' || v[0] == '\'') {
		if v[0] == '"' {
			if s, err := strconv.Unquote(v); err == nil {
				return s
			}
		}
		return v[1 : len(v)-1]
	}
	if i := strings.Index(v, " #"); i >= 0 {
		return strings.TrimSpace(v[:i])
	}
	return v
}

// Key is one setting a stage registers.
type Key struct {
	Name    string
	Default string
	Doc     string
	Check   func(string) error // nil accepts any value
}

// Schema is the set of known keys.
type Schema []Key

// Layer is one source of values. A Loose layer, such as the environment,
// skips keys the schema does not know instead of failing on them.
type Layer struct {
	Name   string
	Values map[string]string
	Loose  bool
}

// Config holds the checked, merged values.
type Config struct{ vals map[string]string }

// Load merges the defaults and then layers in order, later ones winning, and
// checks every value. Errors name the layer and key at fault.
func (s Schema) Load(layers ...Layer) (Config, error) {
	known := make(map[string]Key, len(s))
	vals := make(map[string]string, len(s))
	for _, k := range s {
		known[k.Name] = k
		vals[k.Name] = k.Default
	}
	for _, l := range layers {
		for name, v := range l.Values {
			k, ok := known[name]
			if !ok {
				if l.Loose {
					continue
				}
				return Config{}, fmt.Errorf("%s: unknown setting %s", l.Name, name)
			}
			if k.Check != nil {
				if err := k.Check(v); err != nil {
					return Config{}, fmt.Errorf("%s: %s: %w", l.Name, name, err)
				}
			}
			vals[name] = v
		}
	}
	return Config{vals}, nil
}

// Get is the value of key, or "" for a key the schema lacks.
func (c Config) Get(key string) string { return c.vals[key] }

// Int is the value of key as a number; Load has already checked it.
func (c Config) Int(key string) int { n, _ := strconv.Atoi(c.vals[key]); return n }

// Duration is the value of key as a time; Load has already checked it.
func (c Config) Duration(key string) time.Duration {
	d, _ := time.ParseDuration(c.vals[key])
	return d
}

// IsDuration accepts a positive Go duration such as 45m.
func IsDuration(v string) error {
	d, err := time.ParseDuration(v)
	if err != nil {
		return fmt.Errorf("%q is not a duration such as 45m or 2h", v)
	}
	if d <= 0 {
		return fmt.Errorf("%q must be positive", v)
	}
	return nil
}

// IsInt accepts a whole number.
func IsInt(v string) error {
	if _, err := strconv.Atoi(v); err != nil {
		return fmt.Errorf("%q is not a whole number", v)
	}
	return nil
}

// IsPositiveInt accepts a whole number above zero.
func IsPositiveInt(v string) error {
	if n, err := strconv.Atoi(v); err != nil || n <= 0 {
		return fmt.Errorf("%q is not a whole number above zero", v)
	}
	return nil
}

// OneOf accepts only the listed values.
func OneOf(allowed ...string) func(string) error {
	return func(v string) error {
		if !slices.Contains(allowed, v) {
			return fmt.Errorf("%q is not one of %s", v, strings.Join(allowed, ", "))
		}
		return nil
	}
}
