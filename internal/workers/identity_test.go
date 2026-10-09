package workers

import (
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestNonceDigestIsLowercaseHexSha256(t *testing.T) {
	t.Parallel()
	if got := NonceDigest("abc"); got != "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad" {
		t.Fatalf("NonceDigest(\"abc\") = %q, want the sha256 hex", got)
	}
	got := NonceDigest("worker-nonce-1")
	if len(got) != 64 {
		t.Fatalf("NonceDigest length = %d, want 64", len(got))
	}
	if _, err := hex.DecodeString(got); err != nil {
		t.Fatalf("NonceDigest(%q) is not hex: %v", got, err)
	}
	if got != strings.ToLower(got) {
		t.Fatalf("NonceDigest(%q) is not lowercase", got)
	}
	if NonceDigest("worker-nonce-1") != got {
		t.Fatal("NonceDigest is not deterministic")
	}
	if NonceDigest("worker-nonce-2") == got {
		t.Fatal("NonceDigest collides on distinct nonces")
	}
}

func TestMatchIdentityRequiresPidStartAndNonce(t *testing.T) { // NFR-5, I06 (v2 §2)
	t.Parallel()
	start := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	raw := "test-raw-nonce"
	digest := NonceDigest(raw)
	base := Identity{PID: 4242, StartTime: start}

	cases := []struct {
		name     string
		expected Identity
		observed Identity
		want     bool
	}{
		{
			name:     "all agree matches",
			expected: Identity{PID: base.PID, StartTime: base.StartTime, Nonce: digest},
			observed: Identity{PID: base.PID, StartTime: base.StartTime, Nonce: raw},
			want:     true,
		},
		{
			name:     "pid mismatch fails",
			expected: Identity{PID: base.PID, StartTime: base.StartTime, Nonce: digest},
			observed: Identity{PID: base.PID + 1, StartTime: base.StartTime, Nonce: raw},
			want:     false,
		},
		{
			name:     "start time mismatch fails",
			expected: Identity{PID: base.PID, StartTime: base.StartTime, Nonce: digest},
			observed: Identity{PID: base.PID, StartTime: start.Add(time.Second), Nonce: raw},
			want:     false,
		},
		{
			name:     "nonce mismatch fails",
			expected: Identity{PID: base.PID, StartTime: base.StartTime, Nonce: digest},
			observed: Identity{PID: base.PID, StartTime: base.StartTime, Nonce: "another-raw-nonce"},
			want:     false,
		},
		{
			name:     "empty observed nonce fails",
			expected: Identity{PID: base.PID, StartTime: base.StartTime, Nonce: digest},
			observed: Identity{PID: base.PID, StartTime: base.StartTime, Nonce: ""},
			want:     false,
		},
		{
			name:     "empty expected nonce fails",
			expected: Identity{PID: base.PID, StartTime: base.StartTime, Nonce: ""},
			observed: Identity{PID: base.PID, StartTime: base.StartTime, Nonce: raw},
			want:     false,
		},
		{
			name:     "pre-change empty nonces on both sides never match",
			expected: Identity{PID: base.PID, StartTime: base.StartTime, Nonce: ""},
			observed: Identity{PID: base.PID, StartTime: base.StartTime, Nonce: ""},
			want:     false,
		},
		{
			name:     "journaled raw value instead of digest never matches",
			expected: Identity{PID: base.PID, StartTime: base.StartTime, Nonce: raw},
			observed: Identity{PID: base.PID, StartTime: base.StartTime, Nonce: raw},
			want:     false,
		},
		{
			name:     "everything differs fails",
			expected: Identity{PID: base.PID, StartTime: base.StartTime, Nonce: digest},
			observed: Identity{PID: base.PID + 1, StartTime: start.Add(time.Second), Nonce: "another-raw-nonce"},
			want:     false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := MatchIdentity(c.expected, c.observed); got != c.want {
				t.Errorf("MatchIdentity = %v, want %v", got, c.want)
			}
		})
	}
}

func TestIdentityNonceJSONShape(t *testing.T) {
	t.Parallel()
	raw, err := json.Marshal(Identity{SchemaVersion: 1, Nonce: "raw-value"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"nonce":"raw-value"`) {
		t.Fatalf("worker.json %s carries no raw nonce", raw)
	}
	var preChange Identity
	if err := json.Unmarshal([]byte(`{"schema_version":1,"pid":7}`), &preChange); err != nil {
		t.Fatal(err)
	}
	if preChange.Nonce != "" {
		t.Fatalf("pre-change worker.json decodes nonce %q, want empty", preChange.Nonce)
	}
}
