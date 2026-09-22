package dispatch

import (
	"strings"
	"testing"
)

func TestNormalizeRepoSlugs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		in      []string
		want    string
		wantErr string
	}{
		{name: "bare is github", in: []string{"entireio/cli"}, want: "gh/entireio/cli"},
		{name: "github prefix kept", in: []string{"gh/entireio/cli"}, want: "gh/entireio/cli"},
		{name: "native prefix kept", in: []string{"et/myproject/service"}, want: "et/myproject/service"},
		{name: "whitespace trimmed", in: []string{"  gh/entireio/cli "}, want: "gh/entireio/cli"},
		{name: "punctuation in repo", in: []string{"entireio/entire.io"}, want: "gh/entireio/entire.io"},
		{name: "dedupes across spellings", in: []string{"entireio/cli", "gh/entireio/cli", "et/entireio/cli"}, want: "gh/entireio/cli,et/entireio/cli"},
		{name: "dedupes case variants, first wins", in: []string{"entireio/cli", "ENTIREIO/CLI", "gh/EntireIO/cli"}, want: "gh/entireio/cli"},
		{name: "keeps order", in: []string{"c/d", "a/b"}, want: "gh/c/d,gh/a/b"},
		{name: "empty", in: nil, want: ""},
		{name: "missing slash", in: []string{"entireio"}, wantErr: `invalid repo "entireio"`},
		{name: "unknown forge", in: []string{"gl/entireio/cli"}, wantErr: `invalid repo "gl/entireio/cli"`},
		{name: "uppercase forge", in: []string{"GH/entireio/cli"}, wantErr: `invalid repo "GH/entireio/cli"`},
		{name: "too many segments", in: []string{"gh/entireio/cli/issues"}, wantErr: `invalid repo "gh/entireio/cli/issues"`},
		{name: "traversal", in: []string{"../../etc/passwd"}, wantErr: `invalid repo "../../etc/passwd"`},
		{name: "empty owner", in: []string{"gh//cli"}, wantErr: `invalid repo "gh//cli"`},
		{name: "empty repo", in: []string{"gh/entireio/"}, wantErr: `invalid repo "gh/entireio/"`},
		{name: "second bad after good", in: []string{"a/b", "nope"}, wantErr: `invalid repo "nope"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := normalizeRepoSlugs(tt.in)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("normalizeRepoSlugs(%q) err = %v, want %q", tt.in, err, tt.wantErr)
				}
				if !strings.HasSuffix(err.Error(), "expected owner/repo, gh/owner/repo, or et/owner/repo") {
					t.Fatalf("error must list the accepted forms, got %q", err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeRepoSlugs(%q) unexpected error: %v", tt.in, err)
			}
			if joined := strings.Join(got, ","); joined != tt.want {
				t.Fatalf("normalizeRepoSlugs(%q) = %q, want %q", tt.in, joined, tt.want)
			}
		})
	}
}

func TestGitHubRepoName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		slug   string
		want   string
		wantOK bool
	}{
		{slug: "entireio/cli", want: "entireio/cli", wantOK: true},
		{slug: "gh/entireio/cli", want: "entireio/cli", wantOK: true},
		{slug: " gh/entireio/cli ", want: "entireio/cli", wantOK: true},
		{slug: "et/myproject/service"},
		{slug: "gl/entireio/cli"},
		{slug: "entireio"},
		{slug: "gh/entireio/cli/issues"},
		{slug: ""},
	}

	for _, tt := range tests {
		t.Run(tt.slug, func(t *testing.T) {
			t.Parallel()

			got, ok := GitHubRepoName(tt.slug)
			if ok != tt.wantOK || got != tt.want {
				t.Fatalf("GitHubRepoName(%q) = %q, %v; want %q, %v", tt.slug, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestRepoSlugsEqual(t *testing.T) {
	t.Parallel()

	tests := []struct {
		a, b string
		want bool
	}{
		{a: "gh/entireio/cli", b: "entireio/cli", want: true},
		{a: "entireio/cli", b: "gh/entireio/cli", want: true},
		{a: "gh/EntireIO/CLI", b: "entireio/cli", want: true},
		{a: "et/entireio/cli", b: "et/entireio/cli", want: true},
		{a: "et/entireio/cli", b: "entireio/cli", want: false},
		{a: "et/entireio/cli", b: "gh/entireio/cli", want: false},
		{a: "gh/entireio/cli", b: "gh/entireio/cli2", want: false},
		{a: "not a slug", b: "gh/entireio/cli", want: false},
		{a: "gh/entireio/cli", b: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.a+" vs "+tt.b, func(t *testing.T) {
			t.Parallel()

			if got := repoSlugsEqual(tt.a, tt.b); got != tt.want {
				t.Fatalf("repoSlugsEqual(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}
