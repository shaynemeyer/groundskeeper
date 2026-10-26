package notify

import (
	"context"
	"slices"
	"strings"
	"testing"
)

func TestMessage(t *testing.T) {
	cases := map[Summary]string{
		{Free: 118_000_000_000, Reclaimable: 35_000_000_000}: "Rook's report is ready: 118.0 GB free, about 35.0 GB to tidy.",
		{Free: 1, Reclaimable: 1, Failed: 2}:                 "Rook's report is ready: 1 B free, about 1 B to tidy. 2 scanners had problems; see the report.",
		{Free: 1, Reclaimable: 1, Failed: 1, NewSkipped: 3}:  "Rook's report is ready: 1 B free, about 1 B to tidy. 1 scanner had problems, 3 new paths couldn't be read; see the report.",
	}
	for in, want := range cases {
		if got := Message(in); got != want {
			t.Errorf("Message(%+v)\n got %q\nwant %q", in, got, want)
		}
	}
}

func TestSendPassesTextAsArguments(t *testing.T) {
	var name string
	var args []string
	run := func(_ context.Context, n string, a ...string) ([]byte, error) {
		name, args = n, a
		return nil, nil
	}
	evil := `done" & do shell script "touch /tmp/pwned" & "`
	if err := Send(context.Background(), run, evil); err != nil {
		t.Fatal(err)
	}

	if name != "osascript" {
		t.Fatalf("ran %s", name)
	}
	if !slices.Equal(args[len(args)-2:], []string{evil, Title}) {
		t.Fatalf("message not passed as one argument: %q", args)
	}
	for _, a := range args[:len(args)-2] {
		if strings.Contains(a, "touch") || strings.Contains(a, "pwned") {
			t.Fatalf("message text reached the script source: %q", a)
		}
	}
}
