package mtpserver

import (
	"bytes"
	"log"
	"strings"
	"testing"
	"time"
)

func TestAggLoggerFlushStopsPendingTimerAndPrintsImmediately(t *testing.T) {
	var buf bytes.Buffer
	agg := newAggLogger(log.New(&buf, "", 0), time.Second)

	agg.Printf("mtproto: route cooldown active dc=%d target-dc=%d via %s", 2, 2, "149.154.167.220")
	agg.Flush()

	got := strings.TrimSpace(buf.String())
	want := "mtproto: route cooldown active dc=2 target-dc=2 via 149.154.167.220"
	if got != want {
		t.Fatalf("unexpected flush output: got %q want %q", got, want)
	}

	time.Sleep(50 * time.Millisecond)
	if strings.Count(strings.TrimSpace(buf.String()), want) != 1 {
		t.Fatalf("expected flush to stop pending timer callback, got %q", buf.String())
	}
}

func TestAggLoggerAggregatesMatchingMessages(t *testing.T) {
	var buf bytes.Buffer
	agg := newAggLogger(log.New(&buf, "", 0), 40*time.Millisecond)

	agg.Printf("mtproto: new connection from %s", "127.0.0.1:40123")
	agg.Printf("mtproto: new connection from %s", "127.0.0.1:51234")

	time.Sleep(120 * time.Millisecond)

	got := strings.TrimSpace(buf.String())
	want := "2x: mtproto: new connection from 127.0.0.1:*"
	if got != want {
		t.Fatalf("unexpected aggregated output: got %q want %q", got, want)
	}
}
