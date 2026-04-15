package updater

import "testing"

func TestIsNewer(t *testing.T) {
	cases := []struct {
		latest  string
		current string
		want    bool
	}{
		{"v1.1.0", "v1.0.0", true},
		{"v2.0.0", "v1.9.9", true},
		{"v1.0.1", "v1.0.0", true},
		{"v1.0.0", "v1.0.0", false},
		{"v0.9.9", "v1.0.0", false},
		{"v1.0.0", "v1.0.1", false},
		// without "v" prefix
		{"1.1.0", "1.0.0", true},
		// dev / non-numeric treated as 0
		{"v1.0.0", "dev", true},
	}
	for _, tc := range cases {
		got := IsNewer(tc.latest, tc.current)
		if got != tc.want {
			t.Errorf("IsNewer(%q, %q) = %v, want %v", tc.latest, tc.current, got, tc.want)
		}
	}
}

func TestAssetURL(t *testing.T) {
	url, err := AssetURL("v1.2.3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if url == "" {
		t.Error("AssetURL returned empty string")
	}
}
