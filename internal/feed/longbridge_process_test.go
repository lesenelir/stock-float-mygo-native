//go:build !windows

package feed

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"stock-float-mygo-native/internal/market"
)

// The CLI stand-ins are shell scripts.

// fakeCLI writes an executable stand-in for the longbridge CLI and returns
// its path.
func fakeCLI(t *testing.T, script string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "longbridge")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// hasExited reports whether the process whose id a fake CLI wrote to path
// is gone within five seconds.
func hasExited(t *testing.T, path string) bool {
	t.Helper()
	text, err := os.ReadFile(path)
	if err != nil {
		return true // the CLI was stopped before its first line ran
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(text)))
	if err != nil {
		return true
	}
	for range 250 {
		if syscall.Kill(pid, 0) != nil {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}

// collect runs p until stop says to, and returns the events it emitted.
func collect(t *testing.T, p Provider, stop func(Event) bool) []Event {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make(chan Event, 64)
	done := make(chan struct{})
	symbol, _ := market.ParseID("usAAPL")
	go func() {
		defer close(done)
		p.Run(ctx, []market.Symbol{symbol}, func(e Event) {
			select {
			case events <- e:
			case <-ctx.Done():
			}
		})
	}()
	var got []Event
	timeout := time.After(10 * time.Second)
	for {
		select {
		case e := <-events:
			got = append(got, e)
			if stop(e) {
				cancel()
				<-done
				return got
			}
		case <-timeout:
			t.Fatalf("timed out after %+v", got)
		}
	}
}

const snapshotLine = `{"jsonrpc":"2.0","id":1,"result":{"subscribed":[],"quotes":[{"symbol":"AAPL.US","last_done":"333.000","prev_close":"330.000","timestamp":"2026-10-02T20:00:00Z"}]}}`

func TestTalksToTheCLIAndExplainsItsExit(t *testing.T) {
	cli := fakeCLI(t, `echo "$1" > "$0.log"
read -r line; echo "$line" >> "$0.log"
echo '`+snapshotLine+`'
read -r line; echo "$line" >> "$0.log"
echo '{"jsonrpc":"2.0","method":"quote.updated","params":{"symbol":"AAPL.US","last_done":"334.000","timestamp":"2026-10-02T20:05:00Z","trade_session":"Post"}}'
echo 'Error: Not authenticated. Please run `+"`longbridge auth login`"+` first.' >&2
exit 1
`)

	events := collect(t, LongbridgeProvider{SearchPaths: []string{cli}}, func(e Event) bool { return e.Offline != nil })

	if len(events) != 4 || !events[0].Online || events[1].Quote == nil || events[2].Quote == nil ||
		*events[3].Offline != (Outage{Kind: LongbridgeLoggedOut}) {
		t.Fatalf("got %+v", events)
	}
	quotes := []market.Quote{*events[1].Quote, *events[2].Quote}
	want := []market.Quote{
		{Symbol: "usAAPL", Name: "AAPL", Price: 333, PrevClose: 330, Time: at(1_790_971_200)},
		// The time follows the extended price once that is the newest one.
		{Symbol: "usAAPL", Name: "AAPL", Price: 333, PrevClose: 330, Time: at(1_790_971_500),
			Extended: &market.Extended{Session: market.Post, Price: 334, PrevClose: 333}},
	}
	if !equalQuotes(quotes, want) {
		t.Errorf("quotes = %+v", quotes)
	}

	data, err := os.ReadFile(cli + ".log")
	if err != nil {
		t.Fatal(err)
	}
	log := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(log) != 3 || log[0] != "serve" || !strings.Contains(log[1], `"method":"quote.subscribe"`) ||
		!strings.Contains(log[2], `"method":"quote.quote"`) || !strings.Contains(log[2], `"symbols":["AAPL.US"]`) {
		t.Errorf("log = %q", log)
	}
}

func TestReportsAMissingCLI(t *testing.T) {
	events := collect(t, LongbridgeProvider{SearchPaths: []string{"/nonexistent/longbridge"}}, func(Event) bool { return true })

	if events[0].Offline == nil || *events[0].Offline != (Outage{Kind: LongbridgeMissing}) {
		t.Errorf("got %+v", events)
	}
}

func TestStopsTheCLIWhenTheStreamIsDropped(t *testing.T) {
	cli := fakeCLI(t, `echo $$ > "$0.pid"
echo '`+snapshotLine+`'
exec sleep 600
`)

	events := collect(t, LongbridgeProvider{SearchPaths: []string{cli}}, func(Event) bool { return true })

	if !events[0].Online {
		t.Errorf("got %+v", events)
	}
	if !hasExited(t, cli+".pid") {
		t.Error("the CLI is still running")
	}
}

// The CLI never answers, so the outcome does not depend on how fast the
// machine starts it.
func TestRestartsACLIThatStopsAnswering(t *testing.T) {
	cli := fakeCLI(t, `echo $$ > "$0.pid"
exec sleep 600
`)

	events := collect(t, LongbridgeProvider{SearchPaths: []string{cli}, ReconcileInterval: 200 * time.Millisecond},
		func(Event) bool { return true })

	if events[0].Offline == nil || *events[0].Offline != (Outage{Kind: Unreachable}) {
		t.Errorf("got %+v", events)
	}
	if !hasExited(t, cli+".pid") {
		t.Error("the CLI is still running")
	}
}
