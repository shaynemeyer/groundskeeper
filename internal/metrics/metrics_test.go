package metrics

import "testing"

func TestScannerStatus(t *testing.T) {
	minute := SlowScanner.Milliseconds()
	for _, tc := range []struct {
		s    Scanner
		slow bool
		want string
	}{
		{Scanner{DurationMS: minute - 1}, false, "ok"},
		{Scanner{DurationMS: minute}, true, "slow"},
		{Scanner{DurationMS: 10, Timeouts: 2}, true, "2 timed out"},
		{Scanner{DurationMS: minute, Timeouts: 1, Error: "timeout"}, true, "failed (timeout)"},
	} {
		if got := tc.s.Slow(); got != tc.slow {
			t.Errorf("%+v Slow() = %v, want %v", tc.s, got, tc.slow)
		}
		if got := tc.s.Status(); got != tc.want {
			t.Errorf("%+v Status() = %q, want %q", tc.s, got, tc.want)
		}
	}
}

func TestRunErrorsCountsFailedScanners(t *testing.T) {
	r := Run{Scanners: []Scanner{{Error: "timeout"}, {Timeouts: 3}, {}, {Error: "permission"}}}
	if got := r.Errors(); got != 2 {
		t.Fatalf("Errors() = %d, want 2", got)
	}
}
