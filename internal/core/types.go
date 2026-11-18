// Package core defines rosebud's domain types and ports. It has no I/O.
package core

import (
	"fmt"
	"time"
)

type (
	FilmID  string
	SceneID int64
	JobID   string
)

// Film is one film bundle. Bundle is the directory path on disk.
type Film struct {
	ID       FilmID
	Title    string
	Year     int
	Bundle   string
	Duration time.Duration
	FPS      float64   // frames per second of the video; 0 if unknown
	Chapters []Chapter // in time order; may be empty
	Source   string    // the video file the scenes were cut from; "" if unknown
	Spoken   string    // the language spoken in it, such as "jpn"; "" if unknown
}

// Media is what ffprobe reports about a film's file. Zero values mean unknown.
type Media struct {
	Duration time.Duration
	FPS      float64
	Chapters []Chapter
}

// Chapter is a named point in a film.
type Chapter struct {
	Start time.Duration
	Title string
}

// Scene is a cut. Frame is a bundle-relative path; pixels never cross Client.
type Scene struct {
	ID    SceneID
	Film  FilmID
	Start time.Duration
	End   time.Duration
	Frame string
}

// Hit is a search result with its matching line.
type Hit struct {
	Scene       Scene
	Text        string
	Subtitle    string // the on-screen subtitle, empty if none was read
	Translation string // the English translation, empty if none was made
	Score       float64
}

// SearchQuery searches one film, or all when Film is empty.
type SearchQuery struct {
	Film  FilmID
	Text  string
	Limit int
}

// JobState starts at 1 so the zero value means unset.
type JobState int

const (
	JobUnknown JobState = iota
	JobQueued
	JobRunning
	JobDone
	JobFailed
	JobCancelled
)

// JobSpec names a stage to run on a film.
type JobSpec struct {
	Stage  string
	Film   FilmID
	Params map[string]string
}

// JobInfo is a job's current state. Err is empty unless State is JobFailed.
type JobInfo struct {
	ID    JobID
	Spec  JobSpec
	State JobState
	Done  int64
	Total int64
	Err   string
}

// SkipPrefix starts the message of a stage that had nothing to do for this
// film. It survives as text in JobInfo.Err, which is how callers spot it.
const SkipPrefix = "skipped: "

// Skip is the error of a stage with nothing to do, such as ocr on a film
// without bitmap subtitles.
func Skip(format string, args ...any) error {
	return fmt.Errorf(SkipPrefix+format, args...)
}
