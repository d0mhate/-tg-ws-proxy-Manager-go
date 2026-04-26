package mtpserver

import (
	"fmt"
	"log"
	"regexp"
	"sync"
	"time"
)

// reDynPort matches ephemeral port numbers (4–5 digits) after a colon, e.g.
// "127.0.0.1:55934" → "127.0.0.1:*".  Three-digit service ports (443, 80…)
// are intentionally left intact so they remain readable in the output.
var reDynPort = regexp.MustCompile(`:\d{4,5}\b`)

// normalizeMsg returns a bucket key with variable ephemeral ports replaced by
// "*", so messages that differ only in source/dest port are grouped together.
func normalizeMsg(msg string) string {
	return reDynPort.ReplaceAllString(msg, ":*")
}

// aggLogger aggregates log lines that share the same normalised form within a
// sliding window and flushes them as "Nx: <pattern>" summaries.  Lines that
// appear only once are printed with their original (non-normalised) text.
type aggLogger struct {
	mu      sync.Mutex
	logger  *log.Logger
	window  time.Duration
	buckets map[string]*aggBucket
}

type aggBucket struct {
	count int
	first string // original (un-normalised) message of the first occurrence
	key   string // normalised key used for the aggregated summary line
	timer *time.Timer
}

func newAggLogger(logger *log.Logger, window time.Duration) *aggLogger {
	return &aggLogger{
		logger:  logger,
		window:  window,
		buckets: make(map[string]*aggBucket),
	}
}

// Printf aggregates messages with the same normalised form within the window,
// then flushes them.
func (a *aggLogger) Printf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	key := normalizeMsg(msg)

	a.mu.Lock()
	defer a.mu.Unlock()

	if b, ok := a.buckets[key]; ok {
		// Already pending - just increment, reset the flush timer.
		b.count++
		b.timer.Reset(a.window)
		return
	}

	// First occurrence - start a flush timer.
	b := &aggBucket{count: 1, first: msg, key: key}
	b.timer = time.AfterFunc(a.window, func() {
		a.mu.Lock()
		bucket, ok := a.buckets[key]
		if ok {
			delete(a.buckets, key)
		}
		a.mu.Unlock()

		if !ok {
			return
		}
		if bucket.count == 1 {
			a.logger.Print(bucket.first)
		} else {
			a.logger.Printf("%dx: %s", bucket.count, bucket.key)
		}
	})
	a.buckets[key] = b
}

// Flush prints all pending aggregated messages immediately (e.g. on shutdown).
func (a *aggLogger) Flush() {
	a.mu.Lock()
	buckets := a.buckets
	a.buckets = make(map[string]*aggBucket)
	a.mu.Unlock()

	for _, b := range buckets {
		b.timer.Stop()
		if b.count == 1 {
			a.logger.Print(b.first)
		} else {
			a.logger.Printf("%dx: %s", b.count, b.key)
		}
	}
}
