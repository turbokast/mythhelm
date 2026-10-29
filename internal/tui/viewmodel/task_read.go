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
// classifies as a momentary denial by another handle. read takes ctx so
// cancellation is checked around it; os.ReadFile itself is not interruptible.
type taskReader struct {
	read      func(ctx context.Context, name string) ([]byte, error)
	wait      func(ctx context.Context, d time.Duration) error
	transient func(err error) bool
}

func systemTaskReader() taskReader {
	return taskReader{read: readOSFile, wait: waitCtx, transient: transientReadErr}
}

func readOSFile(_ context.Context, name string) ([]byte, error) {
	return os.ReadFile(name) // #nosec G304 -- the state dir's own run path
}

// readFile returns the file's bytes and whether any retry wait elapsed, so
// a caller can rebuild what it assembled before the delay.
func (r taskReader) readFile(ctx context.Context, name string) ([]byte, bool, error) {
	var last error
	for attempt := 1; ; attempt++ {
		if cerr := ctx.Err(); cerr != nil {
			if last != nil {
				return nil, true, fmt.Errorf("cancelled after attempt %d: %w: %w", attempt-1, cerr, last)
			}
			return nil, false, cerr
		}
		b, err := r.read(ctx, name)
		if cerr := ctx.Err(); cerr != nil {
			if err != nil {
				return nil, attempt > 1, fmt.Errorf("read interrupted: %w: %w", cerr, err)
			}
			return nil, attempt > 1, cerr
		}
		if err == nil || !r.transient(err) {
			return b, attempt > 1, err
		}
		last = err
		if attempt == taskReadAttempts {
			return nil, true, fmt.Errorf("still denied after %d attempts: %w", attempt, err)
		}
		if werr := r.wait(ctx, taskReadWait); werr != nil {
			return nil, true, fmt.Errorf("retry wait after attempt %d: %w: %w", attempt, werr, err)
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
