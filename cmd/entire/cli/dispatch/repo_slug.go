package dispatch

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/entireio/cli/cmd/entire/cli/gitremote"
)

// GitHubForge is the forge token entire.io gives GitHub repos. A slug with no
// forge prefix means this forge: the gateway kept accepting bare owner/repo
// as GitHub when it learned the prefixed form.
const GitHubForge = "gh"

var repoNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*/[A-Za-z0-9._-]+$`)

// SplitRepoSlug parses "forge/owner/repo" or the legacy "owner/repo" into a
// forge token and an owner/repo name. A bare name is GitHub. ok is false for
// any other shape or an unknown forge token.
func SplitRepoSlug(value string) (forge, name string, ok bool) {
	value = strings.TrimSpace(value)
	forge = GitHubForge
	if head, rest, found := strings.Cut(value, "/"); found && strings.Contains(rest, "/") {
		if !gitremote.IsForgePathToken(head) {
			return "", "", false
		}
		forge, value = head, rest
	}
	if !repoNamePattern.MatchString(value) {
		return "", "", false
	}
	return forge, value, true
}

// GitHubRepoName returns the owner/repo of a GitHub slug, prefixed or bare.
// ok is false for every other forge: there is no github.com page for it.
func GitHubRepoName(slug string) (string, bool) {
	forge, name, ok := SplitRepoSlug(slug)
	if !ok || forge != GitHubForge {
		return "", false
	}
	return name, true
}

func normalizeRepoSlug(value string) (string, error) {
	forge, name, ok := SplitRepoSlug(value)
	if !ok {
		return "", fmt.Errorf("invalid repo %q: expected owner/repo, gh/owner/repo, or et/owner/repo", value)
	}
	return forge + "/" + name, nil
}

// normalizeRepoSlugs qualifies every slug with its forge and drops
// duplicates, so "owner/repo" and "gh/owner/repo" count once.
func normalizeRepoSlugs(values []string) ([]string, error) {
	normalized := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		slug, err := normalizeRepoSlug(value)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[slug]; ok {
			continue
		}
		seen[slug] = struct{}{}
		normalized = append(normalized, slug)
	}
	return normalized, nil
}

// repoSlugsEqual matches a gateway-echoed name against a requested slug
// whichever spelling each side used: the forge must agree and the name
// compares case-insensitively.
func repoSlugsEqual(a, b string) bool {
	forgeA, nameA, ok := SplitRepoSlug(a)
	if !ok {
		return false
	}
	forgeB, nameB, ok := SplitRepoSlug(b)
	if !ok {
		return false
	}
	return forgeA == forgeB && strings.EqualFold(nameA, nameB)
}
