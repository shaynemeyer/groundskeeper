package ui

import "testing"

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
