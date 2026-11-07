// Package core defines rosebud's domain types and ports. It has no I/O.
package core

import "time"

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
	Scene    Scene
	Text     string
	Subtitle string // the on-screen subtitle, empty if none was read
	Score    float64
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
