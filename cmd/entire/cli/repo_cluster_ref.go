package cli

import (
	"context"
	"fmt"
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

// hostForClusterSlug is clusterHostForSlug's pure half: the catalog lookup and
// its error text, unit-testable without a control plane.
//
// The match folds case, through the same clusterBySlug every other cluster
// lookup in this feature uses. Matching exactly here while the native path
// folded meant one spelling of --cluster was "unknown cluster" to `repo clone`
// and a valid target to `repo mirror add`. The catalog's own spelling is then
// what indexes the host map, so a mixed-case value cannot pass the lookup and
// still miss the host.
//
// A slug that is in the catalog but whose publicUrl failed validation is
// reported as that, not as unknown: clusterHostBySlug drops such an entry (see
// its doc for the host@evil.com trick it is guarding), and telling the user
// their correctly-spelled cluster does not exist would send them hunting for a
// typo that isn't there.
func hostForClusterSlug(clusters []coreapi.Cluster, slug string) (string, error) {
	cl, ok := clusterBySlug(clusters, slug)
	if !ok {
		if len(clusters) == 0 {
			return "", fmt.Errorf("unknown cluster %q: the control plane returned no clusters", slug)
		}
		return "", fmt.Errorf("unknown cluster %q; available: %s", slug, strings.Join(clusterSlugs(clusters), ", "))
	}
	if host := clusterHostBySlug(clusters)[cl.Slug]; host != "" {
		return host, nil
	}
	return "", fmt.Errorf("cluster %q advertises an unusable public URL; it cannot be addressed by this command", cl.Slug)
}
