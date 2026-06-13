package console

import "testing"

func TestProgressWriteReportsFullLength(t *testing.T) {
	p := NewProgress(func() Snapshot {
		return Snapshot{Total: 10, Processed: 3, Current: "SomeAsset", EtaSeconds: 12}
	})

	b := []byte("header line\n")
	if n, err := p.Write(b); err != nil || n != len(b) {
		t.Fatalf("passthrough write: n=%d err=%v want n=%d", n, err, len(b))
	}

	p.Start()
	b = []byte("a failure line\n")
	if n, err := p.Write(b); err != nil || n != len(b) {
		t.Fatalf("write after start: n=%d err=%v want n=%d", n, err, len(b))
	}

	p.Stop()
	p.Stop()
}

func TestFormatETA(t *testing.T) {
	cases := map[float64]string{
		5:    "5s",
		65:   "1m05s",
		3725: "1h02m",
	}
	for in, want := range cases {
		if got := formatETA(in); got != want {
			t.Errorf("formatETA(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("short", 10); got != "short" {
		t.Errorf("truncate no-op = %q", got)
	}
	if got := truncate("abcdefgh", 5); got != "ab..." {
		t.Errorf("truncate = %q, want %q", got, "ab...")
	}
}
