package supervisor

import "testing"

func TestStopLadderVersionPerOS(t *testing.T) {
	t.Parallel()
	for _, os := range []string{"windows", "linux", "darwin", "freebsd"} {
		t.Run(os, func(t *testing.T) {
			want := "stop-ladder/v1"
			if os == "windows" {
				want = "stop-ladder/v2-windows"
			}
			if got := stopLadderVersion(os); got != want {
				t.Fatalf("version(%s)=%s, want %s", os, got, want)
			}
		})
	}
}
