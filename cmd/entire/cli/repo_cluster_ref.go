package cli

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/entireio/cli/internal/coreapi"
)

// defaultClusterSlug is the cluster a mirror command targets when --cluster is
// omitted: `mirror remove` and `access list` default the flag to it outright,
// and `mirror add` falls back to it when there is no terminal to offer a
// picker on. The no-arg add wizard and the interactive one-shot `add <repo>`
// instead enumerate real clusters from the catalog (GET /api/v1/clusters, see
// availableRegions and resolveOneShotClusterHost in repo_mirror_add_wizard.go);
// this stays as the fixed fallback for non-interactive invocations, so scripts
// keep a stable default.
const defaultClusterSlug = "aws-us-east-2"

// validateClusterSlug rejects a --cluster value that is not a bare catalog
// slug. A slug reaches the control plane as a URL path segment
// (DELETE /repos/{repoId}/native-mirrors/{clusterSlug}) and as a map key into
// the catalog, so it is held to one DNS label — the same shape every real slug
// has (`aws-us-east-2`). Checking it here is what makes "unknown cluster" the
// answer to a typo and keeps a value carrying path or scheme metacharacters
// from ever being interpolated.
//
// This is the slug counterpart of validateClusterHost, which still guards every
// HOST the catalog hands back before it reaches a clone URL or the STS
// audience.
func validateClusterSlug(slug string) error {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return fmt.Errorf("cluster is empty; pass a cluster slug such as %s (run `entire cluster list` to see them)", defaultClusterSlug)
	}
	if !clusterHostLabelRe.MatchString(slug) {
		return fmt.Errorf("%q is not a cluster slug: expected a name like %s, not a host or URL (run `entire cluster list` to see them)", slug, defaultClusterSlug)
	}
	return nil
}

// fetchClusterCatalog reads the control plane's cluster catalog through the
// ACTIVE context. It is the one place a slug can be turned into a host and
// back, so a command that needs either direction fetches it once and passes the
// slice along rather than dialing per lookup.
func fetchClusterCatalog(cmd *cobra.Command) ([]coreapi.Cluster, error) {
	var clusters []coreapi.Cluster
	if err := runCore(cmd, func(ctx context.Context, c *coreapi.Client) error {
		out, err := c.ListClusters(ctx)
		if err != nil {
			return err
		}
		clusters = out.Clusters
		return nil
	}); err != nil {
		return nil, err
	}
	return clusters, nil
}

// clusterSlugByHost inverts clusterHostBySlug: it names the cluster a host
// belongs to, for the output side, where every cluster is shown by the slug the
// reader would type back. A cluster whose publicUrl fails validation is absent
// from both directions (see clusterHostBySlug for the spoofing trick that
// guards).
func clusterSlugByHost(clusters []coreapi.Cluster) map[string]string {
	m := make(map[string]string, len(clusters))
	for slug, host := range clusterHostBySlug(clusters) {
		m[strings.ToLower(host)] = slug
	}
	return m
}

// clusterHostForSlug resolves a catalog slug to the validated public host the
// cluster-addressed commands dial (runCoreForCluster discovers the core
// fronting a HOST, and the entire:// clone URL is built from one). The slug is
// what the user types and what `entire cluster list` prints; the host is an
// implementation coordinate they should never have to know.
//
// It costs one GET /clusters. `mirror add` already paid for that call whenever
// --cluster was omitted; `mirror remove` and `access list` now pay it too,
// which is the price of naming clusters one way.
func clusterHostForSlug(cmd *cobra.Command, slug string) (string, error) {
	if err := validateClusterSlug(slug); err != nil {
		return "", err
	}
	clusters, err := fetchClusterCatalog(cmd)
	if err != nil {
		return "", err
	}
	return hostForClusterSlug(clusters, slug)
}

// hostForClusterSlug is clusterHostForSlug's pure half: the catalog lookup and
// its error text, unit-testable without a control plane.
//
// A slug that is in the catalog but whose publicUrl failed validation is
// reported as that, not as unknown: clusterHostBySlug drops such an entry (see
// its doc for the host@evil.com trick it is guarding), and telling the user
// their correctly-spelled cluster does not exist would send them hunting for a
// typo that isn't there.
func hostForClusterSlug(clusters []coreapi.Cluster, slug string) (string, error) {
	if host := clusterHostBySlug(clusters)[slug]; host != "" {
		return host, nil
	}
	known := make([]string, 0, len(clusters))
	for _, cl := range clusters {
		if cl.Slug == slug {
			return "", fmt.Errorf("cluster %q advertises an unusable public URL; it cannot be addressed by this command", slug)
		}
		known = append(known, cl.Slug)
	}
	slices.Sort(known)
	if len(known) == 0 {
		return "", fmt.Errorf("unknown cluster %q: the control plane returned no clusters", slug)
	}
	return "", fmt.Errorf("unknown cluster %q; available: %s", slug, strings.Join(known, ", "))
}
