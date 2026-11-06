package core

// Event is something a subscriber is told about.
// CoalesceKey returns "" for events that must always be delivered.
type Event interface {
	CoalesceKey() string
}

// JobProgress is superseded by a newer one for the same job.
type JobProgress struct {
	Job   JobID
	Done  int64
	Total int64
}

func (e JobProgress) CoalesceKey() string { return "progress:" + string(e.Job) }

// JobChanged reports a state transition and is never dropped.
type JobChanged struct{ Info JobInfo }

func (JobChanged) CoalesceKey() string { return "" }

// ConfigChanged tells frontends to reload config, theme and keybindings.
type ConfigChanged struct{}

func (ConfigChanged) CoalesceKey() string { return "config" }
