package ocr_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tjagithub/rosebud/internal/ocr"
)

const model = "mlx-community/Nemotron-Parse-2.0-8bit"

func nemotron(ex ocr.Exec) ocr.Nemotron {
	return ocr.Nemotron{Exec: ex, Model: model, Load: time.Minute, PerPage: time.Minute}
}

func pages(n int) []image.Image {
	out := make([]image.Image, n)
	for i := range out {
		out[i] = image.NewGray(image.Rect(0, 0, 40, 30+i))
	}
	return out
}

func line(page, text, errmsg string) string {
	m := map[string]string{"page": page}
	if errmsg != "" {
		m["error"] = errmsg
	} else {
		m["text"] = text
	}
	b, _ := json.Marshal(m)
	return string(b) + "\n"
}

func blockText(top, text string) string {
	return "<x_0.1><y_" + top + ">" + text + "<x_0.2><y_0.9><class_Text>"
}

// scripted is a fake uv: it records the command and the dir it was given,
// checks the pages were written, then prints out.
type scripted struct {
	args    []string
	dir     string
	script  string
	sizes   []image.Point
	scripts string
	out     string
	err     error
}

func (s *scripted) exec(_ context.Context, name string, args ...string) ([]byte, error) {
	s.args = append([]string{name}, args...)
	i := slices.IndexFunc(args, func(a string) bool { return strings.HasSuffix(a, ".py") })
	if i >= 0 && i+1 < len(args) {
		s.script, s.dir = args[i], args[i+1]
		if b, err := os.ReadFile(s.script); err == nil {
			s.scripts = string(b)
		}
		files, _ := filepath.Glob(filepath.Join(s.dir, "*.png"))
		for _, f := range files {
			fh, err := os.Open(f)
			if err != nil {
				continue
			}
			img, err := png.Decode(fh)
			_ = fh.Close()
			if err == nil {
				s.sizes = append(s.sizes, img.Bounds().Size())
			}
		}
	}
	return []byte(s.out), s.err
}

func TestNemotronReadsPagesAndMatchesByName(t *testing.T) {
	s := &scripted{out: line("p0001.png", blockText("0.5", "どうでした?"), "") +
		"\n" + line("p0000.png", blockText("0.018", "はい")+"\n\n"+blockText("0.3", "うん"), "")}
	got, err := nemotron(s.exec).Read(t.Context(), pages(2))
	if err != nil {
		t.Fatal(err)
	}
	want := [][]ocr.Block{{{0.018, "はい"}, {0.3, "うん"}}, {{0.5, "どうでした?"}}}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("got %v\nwant %v", got, want)
	}
	if len(s.sizes) != 2 || s.sizes[0].Y != 30 || s.sizes[1].Y != 31 {
		t.Errorf("pages written = %v", s.sizes)
	}
}

func TestNemotronCommand(t *testing.T) {
	s := &scripted{out: line("p0000.png", "", "")}
	if _, err := nemotron(s.exec).Read(t.Context(), pages(1)); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"uv", "run", "3.12", "timm==1.0.22", "open_clip_torch==3.2.0", model,
		"mlx-vlm @ git+https://github.com/axiom-of-choice/mlx-vlm@e44ee362ceb2298e6b154d8b387a1d27ff2fb121",
	} {
		if !slices.Contains(s.args, want) {
			t.Errorf("command lacks %q: %q", want, s.args)
		}
	}
	for _, want := range []string{"</s><s><predict_bbox><predict_classes><output_markdown><predict_no_text_in_pic>", "NaiveStreamingDetokenizer"} {
		if !strings.Contains(s.scripts, want) {
			t.Errorf("script lacks %q", want)
		}
	}
	if _, err := os.Stat(s.dir); !os.IsNotExist(err) {
		t.Errorf("temp dir %s left behind (%v)", s.dir, err)
	}
}

func TestNemotronOutputProblems(t *testing.T) {
	for name, c := range map[string]struct{ out, want string }{
		"page missing":        {line("p0000.png", "", ""), "page 1"},
		"page error":          {line("p0000.png", "", "") + line("p0001.png", "", "boom"), "boom"},
		"broken json":         {line("p0000.png", "", "") + "{\"page\": \n", "output"},
		"unknown page":        {line("p0000.png", "", "") + line("p0001.png", "", "") + line("p9999.png", "", ""), "p9999.png"},
		"nothing printed":     {"", "page 0"},
		"progress noise only": {"Fetching 12 files\n", "page 0"},
	} {
		s := &scripted{out: c.out}
		_, err := nemotron(s.exec).Read(t.Context(), pages(2))
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want mention of %q", name, err, c.want)
		}
	}
}

func TestNemotronIgnoresNoiseLines(t *testing.T) {
	s := &scripted{out: "Fetching 12 files\n" + line("p0000.png", blockText("0.1", "はい"), "") + "\r\n"}
	got, err := nemotron(s.exec).Read(t.Context(), pages(1))
	if err != nil || len(got) != 1 || len(got[0]) != 1 {
		t.Errorf("got %v, %v", got, err)
	}
}

func TestNemotronNoPagesRunsNothing(t *testing.T) {
	s := &scripted{}
	got, err := nemotron(s.exec).Read(t.Context(), nil)
	if err != nil || len(got) != 0 || s.args != nil {
		t.Errorf("got %v, %v, ran %q", got, err, s.args)
	}
}

func TestNemotronBadConfigRunsNothing(t *testing.T) {
	for name, n := range map[string]ocr.Nemotron{
		"no model":   {Model: "", Load: time.Minute, PerPage: time.Minute},
		"no load":    {Model: model, PerPage: time.Minute},
		"no perpage": {Model: model, Load: time.Minute},
		"no exec":    {Model: model, Load: time.Minute, PerPage: time.Minute},
	} {
		s := &scripted{}
		if name != "no exec" {
			n.Exec = s.exec
		}
		if _, err := n.Read(t.Context(), pages(1)); err == nil {
			t.Errorf("%s: no error", name)
		}
		if s.args != nil {
			t.Errorf("%s: ran %q", name, s.args)
		}
	}
}

func TestNemotronExecError(t *testing.T) {
	boom := errors.New("boom")
	s := &scripted{err: boom}
	if _, err := nemotron(s.exec).Read(t.Context(), pages(1)); !errors.Is(err, boom) {
		t.Errorf("err = %v", err)
	}
}

func TestNemotronTimesOutWithTheModelsName(t *testing.T) {
	hang := func(ctx context.Context, _ string, _ ...string) ([]byte, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	n := ocr.Nemotron{Exec: hang, Model: model, Load: 20 * time.Millisecond, PerPage: 10 * time.Millisecond}
	_, err := n.Read(t.Context(), pages(2))
	want := "Nemotron-Parse-2.0-8bit took over 40ms to complete. Stopping early - to increase the time pass `--timeout 80ms`"
	if !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), want) {
		t.Errorf("err = %v\nwant mention of %q", err, want)
	}
}
