package ocr

import "context"

type progressKey struct{}

// WithProgress asks readers that batch their work to report done and total pages.
func WithProgress(ctx context.Context, fn func(done, total int64)) context.Context {
	return context.WithValue(ctx, progressKey{}, fn)
}

func reportProgress(ctx context.Context, done, total int) {
	if fn, _ := ctx.Value(progressKey{}).(func(done, total int64)); fn != nil {
		fn(int64(done), int64(total))
	}
}

// withoutProgress hides the callback, for extra reads that would make the count go backwards.
func withoutProgress(ctx context.Context) context.Context {
	return context.WithValue(ctx, progressKey{}, nil)
}
