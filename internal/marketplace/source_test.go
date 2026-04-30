package marketplace

import "testing"

func TestParseSource(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		want    Source
		wantErr bool
	}{
		{
			name:  "github shorthand",
			input: "anthropics/skills",
			want:  Source{Kind: SourceGit, URL: "https://github.com/anthropics/skills.git", Raw: "anthropics/skills"},
		},
		{
			name:  "github shorthand with ref",
			input: "anthropics/skills#main",
			want:  Source{Kind: SourceGit, URL: "https://github.com/anthropics/skills.git", Ref: "main", Raw: "anthropics/skills#main"},
		},
		{
			name:  "https git URL no .git",
			input: "https://github.com/foo/bar",
			want:  Source{Kind: SourceGit, URL: "https://github.com/foo/bar.git", Raw: "https://github.com/foo/bar"},
		},
		{
			name:  "https git URL with .git",
			input: "https://github.com/foo/bar.git",
			want:  Source{Kind: SourceGit, URL: "https://github.com/foo/bar.git", Raw: "https://github.com/foo/bar.git"},
		},
		{
			name:  "gitlab https",
			input: "https://gitlab.com/team/plugins",
			want:  Source{Kind: SourceGit, URL: "https://gitlab.com/team/plugins.git", Raw: "https://gitlab.com/team/plugins"},
		},
		{
			name:  "self-hosted https no rewrite",
			input: "https://git.example.com/team/plugins",
			want:  Source{Kind: SourceGit, URL: "https://git.example.com/team/plugins", Raw: "https://git.example.com/team/plugins"},
		},
		{
			name:  "git URL with ref",
			input: "https://gitlab.com/team/plugins.git#v1.0.0",
			want:  Source{Kind: SourceGit, URL: "https://gitlab.com/team/plugins.git", Ref: "v1.0.0", Raw: "https://gitlab.com/team/plugins.git#v1.0.0"},
		},
		{
			name:  "ssh URL",
			input: "git@github.com:foo/bar.git",
			want:  Source{Kind: SourceGit, URL: "git@github.com:foo/bar.git", Raw: "git@github.com:foo/bar.git"},
		},
		{
			name:  "local relative dir",
			input: "./my-marketplace",
			want:  Source{Kind: SourceLocalDir, Path: "./my-marketplace", Raw: "./my-marketplace"},
		},
		{
			name:  "local absolute dir",
			input: "/tmp/my-marketplace",
			want:  Source{Kind: SourceLocalDir, Path: "/tmp/my-marketplace", Raw: "/tmp/my-marketplace"},
		},
		{
			name:  "local json file",
			input: "./marketplace.json",
			want:  Source{Kind: SourceLocalJSON, Path: "./marketplace.json", Raw: "./marketplace.json"},
		},
		{
			name:  "remote json",
			input: "https://example.com/marketplace.json",
			want:  Source{Kind: SourceRemoteJSON, URL: "https://example.com/marketplace.json", Raw: "https://example.com/marketplace.json"},
		},
		{
			name:    "empty",
			input:   "",
			wantErr: true,
		},
		{
			name:    "ref on local dir rejected",
			input:   "./mp#main",
			wantErr: true,
		},
		{
			name:    "ref on remote json rejected",
			input:   "https://example.com/marketplace.json#main",
			wantErr: true,
		},
		{
			name:    "garbage",
			input:   "what is this",
			wantErr: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ParseSource(c.input)
			if c.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q, got %+v", c.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Kind != c.want.Kind {
				t.Errorf("kind: got %v, want %v", got.Kind, c.want.Kind)
			}
			if got.URL != c.want.URL {
				t.Errorf("URL: got %q, want %q", got.URL, c.want.URL)
			}
			if got.Path != c.want.Path {
				t.Errorf("Path: got %q, want %q", got.Path, c.want.Path)
			}
			if got.Ref != c.want.Ref {
				t.Errorf("Ref: got %q, want %q", got.Ref, c.want.Ref)
			}
		})
	}
}

func TestSourceCacheKey(t *testing.T) {
	s, _ := ParseSource("anthropics/skills#main")
	if got := s.CacheKey(); got != "github.com_anthropics_skills@main" {
		t.Errorf("got %q", got)
	}

	s, _ = ParseSource("git@github.com:foo/bar.git")
	if got := s.CacheKey(); got != "github.com_foo_bar" {
		t.Errorf("got %q", got)
	}
}
