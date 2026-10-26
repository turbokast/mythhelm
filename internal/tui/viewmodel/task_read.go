package viewmodel

import (
	"context"
	"fmt"
	"os"
	"time"
)

// At most taskReadAttempts reads with taskReadWait between them: 9 waits,
// 450ms in total.
const (
	taskReadAttempts = 10
	taskReadWait     = 50 * time.Millisecond
)

// taskReader reads the admitted task file, retrying only errors transient
// classifies as a momentary denial by another handle.
type taskReader struct {
	read      func(name string) ([]byte, error)
	wait      func(ctx context.Context, d time.Duration) error
	transient func(err error) bool
}

func systemTaskReader() taskReader {
	return taskReader{read: os.ReadFile, wait: waitCtx, transient: transientReadErr}
}

func (r taskReader) readFile(ctx context.Context, name string) ([]byte, error) {
	for attempt := 1; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		b, err := r.read(name)
		if err == nil || !r.transient(err) {
			return b, err
		}
		if attempt == taskReadAttempts {
			return nil, fmt.Errorf("still denied after %d attempts: %w", attempt, err)
		}
		if werr := r.wait(ctx, taskReadWait); werr != nil {
			return nil, fmt.Errorf("retry wait after attempt %d: %w: %w", attempt, werr, err)
		}
	}
}

func waitCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
