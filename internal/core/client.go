package core

import (
	"context"
	"iter"
)

// Client is the one surface every frontend uses, in-process or over the socket.
// Methods take a ctx and must stop promptly when it is cancelled.
type Client interface {
	Films(ctx context.Context) iter.Seq2[Film, error]
	Scenes(ctx context.Context, film FilmID) iter.Seq2[Scene, error]

	// Search returns at most q.Limit hits, best first.
	Search(ctx context.Context, q SearchQuery) ([]Hit, error)

	StartJob(ctx context.Context, spec JobSpec) (JobID, error)
	CancelJob(ctx context.Context, id JobID) error
	Jobs(ctx context.Context) ([]JobInfo, error)

	// Subscribe streams events until ctx is done, then closes the channel.
	// A slow reader loses coalescable events, never the others.
	Subscribe(ctx context.Context) (<-chan Event, error)

	Close() error
}
