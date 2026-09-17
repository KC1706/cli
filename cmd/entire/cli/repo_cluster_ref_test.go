package cli

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/entireio/cli/internal/coreapi"
)

// TestValidateClusterSlug pins that --cluster takes a bare catalog slug and
// nothing else. A host is the value this flag used to take, so it is the
// mistake worth naming; the rest are the metacharacter shapes that must never
// reach a URL path segment or a catalog key.
func TestValidateClusterSlug(t *testing.T) {
	t.Parallel()
	t.Run("a bare slug is accepted", func(t *testing.T) {
		t.Parallel()
		for _, slug := range []string{"aws-us-east-2", "eu", defaultClusterSlug} {
			require.NoError(t, validateClusterSlug(slug), slug)
		}
	})

	t.Run("anything that is not one label is refused", func(t *testing.T) {
		t.Parallel()
		for _, slug := range []string{
			"aws-us-east-2.entire.io",          // the host form this flag used to take
			"https://aws-us-east-2.entire.io",  // a URL
			"aws-us-east-2.entire.io@evil.com", // the userinfo trick validateClusterHost guards
			"aws-us-east-2/x",                  // a second path segment
			"-leading",
			"trailing-",
			"",
		} {
			require.Error(t, validateClusterSlug(slug), slug)
		}
	})
}

// TestHostForClusterSlug pins the catalog lookup behind every --cluster value:
// the host is the catalog's, never derived from the slug, and the two failures
// are told apart because they need different reactions from the user.
func TestHostForClusterSlug(t *testing.T) {
	t.Parallel()
	catalog := []coreapi.Cluster{
		{Slug: "aws-eu-central-1", PublicUrl: "https://aws-eu-central-1.entire.io"},
		{Slug: "aws-us-east-2", PublicUrl: "https://aws-us-east-2.entire.io"},
		{Slug: "unsafe", PublicUrl: "https://real.entire.io@evil.com"},
	}

	t.Run("a known slug resolves to its catalog host", func(t *testing.T) {
		t.Parallel()
		host, err := hostForClusterSlug(catalog, "aws-eu-central-1")
		require.NoError(t, err)
		require.Equal(t, "aws-eu-central-1.entire.io", host)
	})

	t.Run("an unknown slug names the available ones", func(t *testing.T) {
		t.Parallel()
		_, err := hostForClusterSlug(catalog, "aws-ap-south-1")
		require.ErrorContains(t, err, "unknown cluster")
		require.ErrorContains(t, err, "aws-eu-central-1, aws-us-east-2, unsafe")
	})

	// A correctly spelled slug reported as unknown would send the user hunting
	// for a typo that isn't there, so a cluster the catalog lists but cannot
	// safely reduce to a host says exactly that instead.
	t.Run("a slug whose public URL is unusable is not reported as unknown", func(t *testing.T) {
		t.Parallel()
		_, err := hostForClusterSlug(catalog, "unsafe")
		require.ErrorContains(t, err, "unusable public URL")
		require.NotContains(t, err.Error(), "unknown cluster")
	})

	t.Run("an empty catalog says so rather than listing nothing", func(t *testing.T) {
		t.Parallel()
		_, err := hostForClusterSlug(nil, "aws-us-east-2")
		require.ErrorContains(t, err, "no clusters")
	})
}
