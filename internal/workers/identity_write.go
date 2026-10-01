package workers

import "time"

const (
	// maxIdentityWriteRetries bounds the wait for a Windows reader to
	// release worker.json. The supervisor's identity poll holds the file
	// for microseconds every 20ms, so 50 retries at 10ms (500ms total)
	// outlasts any single overlapping read without stalling a launch.
	maxIdentityWriteRetries = 50
	identityWriteRetryDelay = 10 * time.Millisecond
)

// renameWithRetry runs rename until it succeeds, fails permanently, or
// exhausts maxRetries retries. A failed rename leaves the destination
// untouched, so retrying the same temporary file is safe.
func renameWithRetry(rename func() error, retry func(error) bool, maxRetries int, delay time.Duration) error {
	var err error
	for attempt := 0; ; attempt++ {
		if err = rename(); err == nil {
			return nil
		}
		if !retry(err) || attempt >= maxRetries {
			return err
		}
		time.Sleep(delay)
	}
}
