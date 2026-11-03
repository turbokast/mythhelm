package supervisor

import (
	"maps"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestExactBoundConfigSources(t *testing.T) { // I20 (v2 §8: exact config evidence)
	t.Parallel()
	root := t.TempDir()
	user := filepath.Join(root, "home", ".claude", "settings.json")
	userMCP := filepath.Join(root, "home", ".claude.json")
	project := filepath.Join(root, "work", ".claude", "settings.json")
	// The raw static inventory holds candidates the manifest never admitted.
	available := map[string]string{"user": user, "user_mcp": userMCP, "project": project, "managed": filepath.Join(root, "managed", "managed-settings.json")}
	digest := strings.Repeat("c", 64)
	tests := []struct {
		name      string
		available map[string]string
		admitted  map[string]string
		want      map[string]string
		wantErr   string
	}{
		{name: "unadmitted candidates are dropped and only manifest keys are emitted", available: available, admitted: map[string]string{"user": digest, "user_mcp": digest}, want: map[string]string{"user": user, "user_mcp": userMCP}},
		{name: "an admitted source the inventory does not know refuses", available: available, admitted: map[string]string{"user": digest, "plugin_cache": digest}, wantErr: `"plugin_cache"`},
		{name: "an admitted source missing from an empty inventory refuses", available: map[string]string{}, admitted: map[string]string{"user": digest}, wantErr: `"user"`},
		{name: "an empty admitted path refuses", available: map[string]string{"user": ""}, admitted: map[string]string{"user": digest}, wantErr: `"user"`},
		{name: "a relative admitted path refuses", available: map[string]string{"user": filepath.Join("home", "settings.json")}, admitted: map[string]string{"user": digest}, wantErr: `"user"`},
		{name: "an empty manifest maps nothing", available: available, admitted: map[string]string{}, want: nil},
		{name: "no manifest and no inventory maps nothing", available: nil, admitted: nil, want: nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := exactBoundConfigSources(tc.available, tc.admitted)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("exactBoundConfigSources = %v, %v; want an error naming %s", got, err, tc.wantErr)
				}
				if got != nil {
					t.Fatalf("refused mapping still returned %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("exactBoundConfigSources = %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("mapping = %v, want exactly %v", got, tc.want)
			}
		})
	}

	// The result is a fresh map: mutating the inventory afterwards cannot
	// move the handoff.
	in := maps.Clone(available)
	got, err := exactBoundConfigSources(in, map[string]string{"user": digest})
	if err != nil {
		t.Fatal(err)
	}
	in["user"] = "/mutated"
	if got["user"] != user {
		t.Fatalf("mapping aliases the inventory: user = %q", got["user"])
	}
}
