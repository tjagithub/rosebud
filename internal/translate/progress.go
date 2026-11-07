package translate

import "context"

type progressKey struct{}

// WithProgress returns a context whose translators report done/total lines to fn.
func WithProgress(ctx context.Context, fn func(done, total int64)) context.Context {
	return context.WithValue(ctx, progressKey{}, fn)
}

func report(ctx context.Context, done, total int) {
	if fn, ok := ctx.Value(progressKey{}).(func(done, total int64)); ok && fn != nil {
		fn(int64(done), int64(total))
	}
}
