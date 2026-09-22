package dispatch

import (
	"regexp"
	"strings"
)

var (
	githubOwnerPattern = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,37}[A-Za-z0-9])?$`)
	githubRepoPattern  = regexp.MustCompile(`^[A-Za-z0-9._-]{1,100}$`)
)

// githubRepoURL links a GitHub slug, prefixed or bare, to its github.com
// page. Any other forge, or an unsafe name, gets no link.
func githubRepoURL(fullName string) string {
	name, ok := GitHubRepoName(fullName)
	if !ok {
		return ""
	}
	owner, repoName, _ := strings.Cut(name, "/")
	if repoName == "." || repoName == ".." {
		return ""
	}
	if !githubOwnerPattern.MatchString(owner) || !githubRepoPattern.MatchString(repoName) {
		return ""
	}
	return "https://github.com/" + owner + "/" + repoName
}
