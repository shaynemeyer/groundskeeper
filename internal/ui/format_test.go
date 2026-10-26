package ui

import (
	"os"
	"strings"
	"testing"
)

func TestBytes(t *testing.T) {
	cases := map[int64]string{
		0:                 "0 B",
		999:               "999 B",
		1_000:             "1.0 kB",
		12_400_000_000:    "12.4 GB",
		1_500_000_000_000: "1.5 TB",
	}
	for in, want := range cases {
		if got := Bytes(in); got != want {
			t.Errorf("Bytes(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestDelta(t *testing.T) {
	cases := map[int64]string{2_100_000_000: "+2.1 GB", -800_000_000: "-800.0 MB", 0: "+0 B"}
	for in, want := range cases {
		if got := Delta(in); got != want {
			t.Errorf("Delta(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestPlainTableAlignsColumns(t *testing.T) {
	got := Table([]string{"Name", "Size"}, [][]string{{"npm", "3.1 GB"}, {"Go build cache", "12.0 MB"}}, []int{1}, true)
	want := "Name               Size\n" +
		"npm              3.1 GB\n" +
		"Go build cache  12.0 MB"
	if got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
}

func TestStyledTableHasBorder(t *testing.T) {
	got := Table([]string{"Name"}, [][]string{{"npm"}}, nil, false)
	if !strings.Contains(got, "╭") || !strings.Contains(got, "npm") {
		t.Fatalf("got %q", got)
	}
}

func TestDevNullIsNotATerminal(t *testing.T) {
	f, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if IsTerminal(f) {
		t.Fatal("/dev/null, which launchd gives a job as stdin, counted as a terminal")
	}
}
