package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/entireio/cli/cmd/entire/cli/interactive"
	"github.com/entireio/cli/internal/coreapi"
)

// Entire-native repositories are mirrored through a different resource than
// GitHub ones. A GitHub mirror is a clone of an upstream, addressed by
// (provider, owner, repo, clusterHost) and created through an asynchronous
// mirror REQUEST. A native mirror is an extra placement of a repo Entire
// already holds, addressed by (repoId, clusterSlug) under
// /repos/{repoId}/native-mirrors.
//
// Three consequences shape everything below:
//
//   - The primary placement is NOT in the native-mirror list. GET returns only
//     the additional ones, so any view of "where does this repo live" joins the
//     repo's own clusterSlug onto that list.
//   - A replica is never promoted: removing one tears down that copy alone,
//     and the primary is not in the list to remove.
//   - v1 places mirrors cross-jurisdiction only, and the primary is fixed to
//     the owning project's region.
//
// The control-plane routes are home-core-scoped: a repo whose cluster is in
// another jurisdiction answers 421, which coreapi's transport follows on its
// own (see internal/coreapi/cross_juris_client.go). So these all run on the
// plain active-context client — no cluster-fronting detour.

// loadNativeRepo reads the repo a native ref names together with the cluster
// catalog, the two things every native mirror verb needs before it can decide
// anything: the repo carries its primary cluster and region, and the catalog
// says which other clusters exist and where they are.
func loadNativeRepo(ctx context.Context, c *coreapi.Client, ref mirrorRepoRef) (*coreapi.Repo, []coreapi.Cluster, error) {
	repo, err := resolveNativeRepo(ctx, c, ref.owner, ref.repo)
	if err != nil {
		return nil, nil, err
	}
	clusters, err := c.ListClusters(ctx)
	if err != nil {
		return nil, nil, err
	}
	return repo, clusters.Clusters, nil
}

// checkNativeMirrorTarget refuses, before any write, everything decidable from
// the repo and the cluster catalog — in the order that gives the most useful
// answer first.
//
// These are not duplicated validation: each has a server-side counterpart that
// answers 400 or 409. Doing them here buys a message that names the repo's own
// primary and region instead of a generic refusal, and costs no round trip
// because both inputs were already fetched.
func checkNativeMirrorTarget(repo *coreapi.Repo, clusters []coreapi.Cluster, clusterSlug, ref string) error {
	if provider := repo.Provider.Or(""); provider != repoProviderEntire {
		// A /gh/ ref never reaches here (the verb dispatches on the forge), so
		// this is a repo the control plane calls something other than native —
		// a mirror addressed by a native-looking path, or a provider this build
		// does not know.
		return fmt.Errorf("repo %s is not an Entire-native repository (provider %q); only native repos have native mirrors", ref, provider)
	}
	// State is an open string on the client, so an UNSET one is "the server
	// did not say" and is left for the server to judge; only a value that is
	// definitely not active is refused here (repoStateActive, repo_readiness.go).
	if state := repo.State.Or(""); state != "" && state != repoStateActive {
		return fmt.Errorf("repo %s is %s, not active; wait for it to finish provisioning before mirroring it", ref, state)
	}
	primary := repo.ClusterSlug.Or("")
	if primary != "" && strings.EqualFold(primary, clusterSlug) {
		return fmt.Errorf("repo %s already lives on %s: that is its primary, not a mirror of it", ref, primary)
	}
	target, ok := clusterBySlug(clusters, clusterSlug)
	if !ok {
		return fmt.Errorf("unknown cluster %q; available: %s", clusterSlug, strings.Join(clusterSlugs(clusters), ", "))
	}
	// v1 places a native mirror in a different jurisdiction from the primary —
	// the point of the feature is reach, not redundancy within one region. The
	// server refuses the same thing; saying it here names both regions.
	if home := repo.Jurisdiction.Or(""); home != "" && strings.EqualFold(home, target.Jurisdiction) {
		return fmt.Errorf("repo %s is already in the %s region, and a native mirror must be in a different one; pick a cluster outside %s (run `entire cluster list`)", ref, target.Jurisdiction, home)
	}
	return nil
}

func clusterBySlug(clusters []coreapi.Cluster, slug string) (coreapi.Cluster, bool) {
	for _, cl := range clusters {
		if strings.EqualFold(cl.Slug, slug) {
			return cl, true
		}
	}
	return coreapi.Cluster{}, false
}

func clusterSlugs(clusters []coreapi.Cluster) []string {
	out := make([]string, 0, len(clusters))
	for _, cl := range clusters {
		out = append(out, cl.Slug)
	}
	slices.Sort(out)
	return out
}

// nativeRefOf renders a parsed native ref back into the /et/<project>/<repo>
// spelling, so every message names the repo the way the user must type it.
func nativeRefOf(ref mirrorRepoRef) string {
	return "/" + nativeCloneForge + "/" + ref.owner + "/" + ref.repo
}

// errNativeMirrorSuspended and errNativeMirrorFailed are the terminal
// placement outcomes. Suspended means an operator parked the placement; it will
// not move on its own and resuming is not a CLI operation.
var (
	errNativeMirrorFailed    = errors.New("native mirror placement failed")
	errNativeMirrorSuspended = errors.New("native mirror placement is suspended")
)

// findNativeMirror returns the placement for clusterSlug among a repo's native
// mirrors. The list holds only the additional placements, so a miss means this
// cluster carries no mirror of the repo (or the primary, which is never here).
func findNativeMirror(placements []coreapi.NativeMirrorPlacement, clusterSlug string) (coreapi.NativeMirrorPlacement, bool) {
	for _, p := range placements {
		if strings.EqualFold(p.ClusterSlug, clusterSlug) {
			return p, true
		}
	}
	return coreapi.NativeMirrorPlacement{}, false
}

func listNativeMirrors(ctx context.Context, c *coreapi.Client, repoID string) ([]coreapi.NativeMirrorPlacement, error) {
	out, err := c.ListNativeMirrors(ctx, coreapi.ListNativeMirrorsParams{RepoId: repoID})
	if err != nil {
		return nil, err
	}
	return out.NativeMirrors, nil
}

// nativeMirrorIsFresh reports whether a create response describes an intent
// that was just made rather than one that already existed. A fresh intent is
// always (pending, processing, 0 attempts) and the endpoint answers identically
// for a repeat call, so this triple is the only way to tell the two apart — and
// telling them apart is what keeps `add` from claiming it created something it
// found.
func nativeMirrorIsFresh(p coreapi.NativeMirrorPlacement) bool {
	return p.Stage == coreapi.NativeMirrorPlacementStagePending &&
		p.Status == coreapi.NativeMirrorPlacementStatusProcessing &&
		p.Attempts == 0
}

// awaitNativeMirrorReady polls until the placement on clusterSlug is readable,
// or a terminal condition says it never will be.
//
// Readiness is status == ready and only that. Stage (pending → provisioned →
// registered → seeded → announced) reports provisioning progress and is shown
// while waiting, but a placement is not readable until its status says so:
// reads are gated on status server-side, so stage "announced" with status
// "processing" still serves nothing.
//
// Creation retries server-side, so attempts, nextRetryAt and creationFailedAt
// are bookkeeping, not verdicts — the loop keeps waiting through them and stops
// only on a terminal status. A vanished row means something else removed the
// intent; a row turned to desiredState "deleted" means a teardown overtook us.
// Neither will ever become ready, so both stop the wait rather than spin.
func awaitNativeMirrorReady(ctx context.Context, c *coreapi.Client, repoID, clusterSlug string) (coreapi.NativeMirrorPlacement, error) {
	ticker := time.NewTicker(mirrorPollInterval)
	defer ticker.Stop()

	var last coreapi.NativeMirrorPlacement
	var consecutiveErrs int
	for {
		placements, err := listNativeMirrors(ctx, c, repoID)
		switch {
		case err != nil:
			if ctx.Err() != nil {
				return last, classifyWaitContextErr(ctx.Err(), "waiting for the native mirror")
			}
			// Tolerate transient glitches: the placement may still be
			// progressing, so retry on the next tick.
			consecutiveErrs++
			if consecutiveErrs >= maxConsecutivePollErrors {
				return last, fmt.Errorf("poll native mirror: %w", err)
			}
		default:
			consecutiveErrs = 0
			p, ok := findNativeMirror(placements, clusterSlug)
			if !ok {
				return last, fmt.Errorf("native mirror on %s is no longer listed; something else removed it", clusterSlug)
			}
			last = p
			if p.DesiredState == coreapi.NativeMirrorPlacementDesiredStateDeleted {
				return p, fmt.Errorf("native mirror on %s is being torn down; wait for it to disappear before creating it again", clusterSlug)
			}
			switch p.Status {
			case coreapi.NativeMirrorPlacementStatusReady:
				return p, nil
			case coreapi.NativeMirrorPlacementStatusFailed:
				return p, errNativeMirrorFailed
			case coreapi.NativeMirrorPlacementStatusSuspended:
				return p, errNativeMirrorSuspended
			case coreapi.NativeMirrorPlacementStatusProcessing:
				// keep waiting; the server retries on its own
			}
		}
		select {
		case <-ctx.Done():
			return last, classifyWaitContextErr(ctx.Err(), "waiting for the native mirror")
		case <-ticker.C:
		}
	}
}

// awaitNativeMirrorRemoved polls until the placement on clusterSlug is gone.
// Teardown is asynchronous: DELETE only records the intent. A row that stops
// being desiredState "deleted" was re-created by someone else, which this wait
// can never satisfy, so it stops rather than looping forever.
func awaitNativeMirrorRemoved(ctx context.Context, c *coreapi.Client, repoID, clusterSlug string) error {
	ticker := time.NewTicker(mirrorPollInterval)
	defer ticker.Stop()

	var consecutiveErrs int
	for {
		placements, err := listNativeMirrors(ctx, c, repoID)
		switch {
		case err != nil:
			if ctx.Err() != nil {
				return classifyWaitContextErr(ctx.Err(), "waiting for the native mirror to be removed")
			}
			consecutiveErrs++
			if consecutiveErrs >= maxConsecutivePollErrors {
				return fmt.Errorf("poll native mirror: %w", err)
			}
		default:
			consecutiveErrs = 0
			p, ok := findNativeMirror(placements, clusterSlug)
			if !ok {
				return nil
			}
			if p.DesiredState != coreapi.NativeMirrorPlacementDesiredStateDeleted {
				return fmt.Errorf("native mirror on %s is no longer being removed; something re-created it", clusterSlug)
			}
		}
		select {
		case <-ctx.Done():
			return classifyWaitContextErr(ctx.Err(), "waiting for the native mirror to be removed")
		case <-ticker.C:
		}
	}
}

// nativeMirrorBeingDeletedHint recognises the one server refusal whose fix is
// not derivable from its own words: creating on a cluster whose previous
// placement is still tearing down. The detail says the mirror is being deleted;
// it does not say that waiting for the row to disappear is the whole remedy, so
// a caller would retry immediately and see it again.
//
// This is deliberately the ONLY message matched on. Every other refusal
// (hosting capability, registry state, trust publication, feature flags) is
// rendered from the server's own problem+json detail by renderCoreError, which
// keeps the CLI from encoding server prose it would then have to chase.
func nativeMirrorBeingDeletedHint(err error, ref, clusterSlug string) error {
	detail := coreapi.APIError(err)
	if detail == "" || !strings.Contains(strings.ToLower(detail), "being deleted") {
		return err
	}
	return fmt.Errorf("%w; a previous mirror of %s on %s is still being torn down \u2014 wait for it to disappear from `entire repo mirror get %s`, then add it again",
		err, ref, clusterSlug, ref)
}

// runNativeMirrorAdd is `repo mirror add /et/<project>/<repo>`: place a replica
// of a native repo on another cluster, then wait for it to become readable
// unless --no-wait says otherwise.
//
// Everything runs on the active-context client. The native-mirror routes are
// home-core-scoped and answer 421 for a repo in another jurisdiction, which
// coreapi's transport follows and re-authenticates on its own — so unlike the
// GitHub path there is no cluster-fronting client to build.
func runNativeMirrorAdd(cmd *cobra.Command, ref mirrorRepoRef, clusterSlug string, opts mirrorAddOptions) error {
	name := nativeRefOf(ref)
	return runCore(cmd, func(ctx context.Context, c *coreapi.Client) error {
		// Zero preserves the caller context without adding a timeout.
		if opts.timeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, opts.timeout)
			defer cancel()
		}
		repo, clusters, err := loadNativeRepo(ctx, c, ref)
		if err != nil {
			return err
		}
		if clusterSlug == "" {
			if clusterSlug, err = pickNativeMirrorCluster(cmd, clusters, repo, name); err != nil {
				return err
			}
		}
		if err := checkNativeMirrorTarget(repo, clusters, clusterSlug, name); err != nil {
			return err
		}
		// The catalog match folds case; the host map that builds the clone URL
		// does not. Take the catalog's own spelling so a mixed-case --cluster
		// cannot pass the check and then yield an empty clone URL.
		if cl, ok := clusterBySlug(clusters, clusterSlug); ok {
			clusterSlug = cl.Slug
		}

		out, errW := cmd.OutOrStdout(), cmd.ErrOrStderr()
		created, err := c.CreateNativeMirror(ctx, &coreapi.CreateNativeMirrorInputBody{ClusterSlug: clusterSlug}, coreapi.CreateNativeMirrorParams{RepoId: repo.ID})
		if err != nil {
			return nativeMirrorBeingDeletedHint(err, name, clusterSlug)
		}
		// The endpoint is idempotent per (repo, cluster) and answers identically
		// either way, so the row's own state is the only thing that can tell the
		// user whether this call made the placement or found it.
		if nativeMirrorIsFresh(*created) {
			fmt.Fprintf(errW, "Placing %s on %s\n", name, clusterSlug)
		} else {
			fmt.Fprintf(errW, "%s is already placed on %s\n", name, clusterSlug)
		}

		cloneURL := nativeCloneURL(clusters, clusterSlug, repo)
		if opts.noWait {
			reportNativeMirrorPlaced(out, *created, cloneURL)
			return nil
		}

		stop := startSpinner(errW, fmt.Sprintf("Seeding %s on %s", name, clusterSlug))
		final, waitErr := awaitNativeMirrorReady(ctx, c, repo.ID, clusterSlug)
		stop(waitErr == nil)
		if waitErr != nil {
			return nativeMirrorWaitError(errW, waitErr, final, name, clusterSlug)
		}
		reportNativeMirrorReady(out, cloneURL)
		return nil
	})
}

// pickNativeMirrorCluster chooses the target when --cluster was omitted. Only
// clusters outside the repo's own region are offered, because v1 places a
// native mirror cross-jurisdiction — listing the primary's region would offer a
// choice that checkNativeMirrorTarget then refuses.
//
// Without a terminal there is nothing to fall back to: the GitHub path's fixed
// default cluster cannot serve here, since it may be the repo's own region and
// the right answer depends on the repo. Saying so beats guessing.
func pickNativeMirrorCluster(cmd *cobra.Command, clusters []coreapi.Cluster, repo *coreapi.Repo, ref string) (string, error) {
	home := repo.Jurisdiction.Or("")
	eligible := make([]regionChoice, 0, len(clusters))
	for _, r := range clustersToRegions(clusters) {
		if home != "" && strings.EqualFold(r.jurisdiction, home) {
			continue
		}
		eligible = append(eligible, r)
	}
	if len(eligible) == 0 {
		return "", fmt.Errorf("no cluster outside %s's region (%s) is available to mirror into", ref, home)
	}
	if !interactive.CanPromptInteractively() {
		return "", fmt.Errorf("pass --cluster to say where to mirror %s: a native mirror goes in a region other than the repo's own (%s), so there is no safe default (available: %s)",
			ref, home, strings.Join(regionSlugs(eligible), ", "))
	}
	if len(eligible) == 1 {
		fmt.Fprintf(cmd.ErrOrStderr(), "Using cluster %s\n", eligible[0].slug)
		return eligible[0].slug, nil
	}
	// The picker's option values are hosts (shared with the GitHub wizard), so
	// map the choice back to the slug this verb acts on.
	host, err := pickOneCluster(cmd.Context(), cmd.ErrOrStderr(), eligible, "")
	if err != nil {
		return "", err
	}
	for _, r := range eligible {
		if r.host == host {
			return r.slug, nil
		}
	}
	return "", NewSilentError(errors.New("mirror add cancelled"))
}

func regionSlugs(regions []regionChoice) []string {
	out := make([]string, 0, len(regions))
	for _, r := range regions {
		out = append(out, r.slug)
	}
	slices.Sort(out)
	return out
}

// nativeCloneURL builds the entire:// URL for a native placement on
// clusterSlug. The path is the repo's own server-provided one, so the URL names
// the repo exactly as the control plane does; only the host is swapped for the
// mirror's cluster. Empty when the cluster's public URL could not be validated
// — a dashed URL is better than a spoofable one.
func nativeCloneURL(clusters []coreapi.Cluster, clusterSlug string, repo *coreapi.Repo) string {
	host := clusterHostBySlug(clusters)[clusterSlug]
	path := strings.TrimSpace(repo.Path.Or(""))
	if host == "" || path == "" {
		return ""
	}
	return entireCloneURLScheme + host + "/" + strings.TrimPrefix(path, "/")
}

func reportNativeMirrorPlaced(w io.Writer, p coreapi.NativeMirrorPlacement, cloneURL string) {
	fmt.Fprintf(w, "\nPlacement %s registered on %s\n", p.PlacementId, p.ClusterSlug)
	if cloneURL != "" {
		fmt.Fprintf(w, "Seeding may still be in progress; `git clone %s` will work once it completes.\n", cloneURL)
	}
}

func reportNativeMirrorReady(w io.Writer, cloneURL string) {
	if cloneURL == "" {
		fmt.Fprintln(w, "\nMirror is ready.")
		return
	}
	fmt.Fprintf(w, "\nClone it:\n  git clone %s\n", cloneURL)
}

// nativeMirrorWaitError turns a failed wait into the message that helps. The
// process exits 1 for a timeout exactly as it does for a failure, so the text
// has to carry the difference: a timed-out seed is still running and worth
// checking on, while a failed one is not.
func nativeMirrorWaitError(errW io.Writer, err error, p coreapi.NativeMirrorPlacement, ref, clusterSlug string) error {
	if detail := strings.TrimSpace(p.LastError.Or("")); detail != "" {
		fmt.Fprintf(errW, "Server reported: %s\n", detail)
	}
	switch {
	case errors.Is(err, errNativeMirrorFailed):
		return fmt.Errorf("seeding %s on %s failed", ref, clusterSlug)
	case errors.Is(err, errNativeMirrorSuspended):
		return fmt.Errorf("the mirror of %s on %s is suspended; an operator has to resume it", ref, clusterSlug)
	case errors.Is(err, context.DeadlineExceeded):
		return fmt.Errorf("%w; the mirror of %s on %s is still being created — check on it with `entire repo mirror get %s`", err, ref, clusterSlug, ref)
	}
	return err
}

// runNativeMirrorRemove is `repo mirror remove /et/<project>/<repo>`: tear down
// one replica. Teardown is asynchronous, so the command records the intent and
// then waits for the placement to disappear.
//
// The primary is refused by name: it is not in the native-mirror list at all,
// so the endpoint could only answer "not found", which reads as though the repo
// were not there.
func runNativeMirrorRemove(cmd *cobra.Command, ref mirrorRepoRef, clusterSlug string) error {
	name := nativeRefOf(ref)
	return runCore(cmd, func(ctx context.Context, c *coreapi.Client) error {
		repo, clusters, err := loadNativeRepo(ctx, c, ref)
		if err != nil {
			return err
		}
		// Take the catalog's own spelling before the slug reaches the API, the
		// same correction `add` makes. Sending a mixed-case slug to a
		// case-sensitive delete would 404, and the catalog check below folds
		// case — so the miss would then be reported as "no mirror on <cluster>"
		// for a mirror that exists.
		if cl, ok := clusterBySlug(clusters, clusterSlug); ok {
			clusterSlug = cl.Slug
		}
		if primary := repo.ClusterSlug.Or(""); primary != "" && strings.EqualFold(primary, clusterSlug) {
			return fmt.Errorf("%s lives on %s: that is its primary, not a mirror, and removing it is `entire repo delete %s`", name, primary, name)
		}
		if _, err := c.DeleteNativeMirror(ctx, coreapi.DeleteNativeMirrorParams{RepoId: repo.ID, ClusterSlug: clusterSlug}); err != nil {
			if isCoreNotFound(err) {
				// Deliberately not %w-wrapped: renderCoreError would replace this
				// with the server's own detail, which cannot name the command
				// that lists where the repo actually is.
				//
				// The catalog is consulted only to WORD the miss, after the
				// delete was attempted: refusing an unknown slug up front would
				// also refuse to tear down a placement whose cluster has since
				// left the catalog, which is the one case you most need to.
				if _, known := clusterBySlug(clusters, clusterSlug); !known {
					return fmt.Errorf("unknown cluster %q; available: %s", clusterSlug, strings.Join(clusterSlugs(clusters), ", "))
				}
				return fmt.Errorf("no mirror of %s on %s (run `entire repo mirror get %s` to see its placements)", name, clusterSlug, name)
			}
			return err
		}
		errW := cmd.ErrOrStderr()
		stop := startSpinner(errW, fmt.Sprintf("Removing the mirror of %s from %s", name, clusterSlug))
		err = awaitNativeMirrorRemoved(ctx, c, repo.ID, clusterSlug)
		stop(err == nil)
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "✓ Removed the mirror of %s from %s\n", name, clusterSlug)
		return nil
	})
}

// runNativeMirrorGet is `repo mirror get /et/<project>/<repo>`: the repo's
// identity, then every cluster holding a copy of it.
//
// The view joins two reads because neither is complete on its own: the repo
// carries its primary placement, and /native-mirrors lists only the ADDITIONAL
// ones. It deliberately does not go through the /repos directory that the
// GitHub path uses — only this endpoint carries stage and lastError, the two
// fields that say anything useful about a placement that is stuck.
func runNativeMirrorGet(cmd *cobra.Command, ref mirrorRepoRef) error {
	name := nativeRefOf(ref)
	return runCore(cmd, func(ctx context.Context, c *coreapi.Client) error {
		repo, clusters, err := loadNativeRepo(ctx, c, ref)
		if err != nil {
			return err
		}
		mirrors, err := listNativeMirrors(ctx, c, repo.ID)
		if err != nil {
			return err
		}
		// A plain repo read leaves `state` unset, which would dash the one cell
		// in this table that says whether the primary is usable — and a dashed
		// primary next to a "ready" mirror reads as broken. The authoritative
		// read is the one that answers it (the same flag `repo view` exposes).
		//
		// Only `state` is taken from it, never the whole repo: an authoritative
		// lifecycle response can omit clusterSlug and path (see the fixture in
		// repo_readiness_test.go), and swapping the object wholesale would drop
		// the primary placement and every clone URL this view exists to show.
		// Best-effort for the same reason it is narrow — a registry-only
		// fallback cannot answer the readiness question, and a dash is a better
		// trade than losing the table.
		if authoritative, aerr := c.GetRepo(ctx, coreapi.GetRepoParams{
			RepoId:        repo.ID,
			Authoritative: coreapi.NewOptBool(true),
		}); aerr == nil {
			if state, ok := authoritative.State.Get(); ok {
				repo.State = coreapi.NewOptString(state)
			}
		}
		row := nativeRepoDetailRow(name, repo, mirrors, clusters)
		if jsonRequested(cmd) {
			return printJSON(cmd.OutOrStdout(), row)
		}
		renderRepoDetail(cmd.OutOrStdout(), row)
		reportNativeMirrorNotes(cmd.ErrOrStderr(), mirrors)
		return nil
	})
}

// nativeRepoDetailRow shapes a native repo and its mirrors into the same row
// the GitHub detail view renders, so both forges produce one table and one
// --json shape. The primary comes first; the mirrors follow in slug order.
func nativeRepoDetailRow(name string, repo *coreapi.Repo, mirrors []coreapi.NativeMirrorPlacement, clusters []coreapi.Cluster) repoDirRow {
	hostBySlug := clusterHostBySlug(clusters)
	cloneURL := func(slug string) string {
		host, path := hostBySlug[slug], strings.TrimSpace(repo.Path.Or(""))
		if host == "" || path == "" {
			return ""
		}
		return entireCloneURLScheme + host + "/" + strings.TrimPrefix(path, "/")
	}

	placements := make([]repoDirPlacement, 0, len(mirrors)+1)
	if primary := repo.ClusterSlug.Or(""); primary != "" {
		// The primary has no placement record of its own here, so its status is
		// the repo's provisioning state — the same question, answered by the
		// only field that answers it.
		placements = append(placements, repoDirPlacement{
			Cluster:  primary,
			Status:   repo.State.Or("-"),
			Role:     placementRolePrimary,
			CloneURL: cloneURL(primary),
		})
	}
	sorted := slices.Clone(mirrors)
	slices.SortFunc(sorted, func(a, b coreapi.NativeMirrorPlacement) int {
		return strings.Compare(a.ClusterSlug, b.ClusterSlug)
	})
	for _, m := range sorted {
		p := repoDirPlacement{
			Cluster:  m.ClusterSlug,
			Status:   string(m.Status),
			Role:     placementRoleNativeMirror,
			CloneURL: cloneURL(m.ClusterSlug),
		}
		if m.Status == coreapi.NativeMirrorPlacementStatusProcessing {
			p.Stage = string(m.Stage)
		}
		p.Removing = m.DesiredState == coreapi.NativeMirrorPlacementDesiredStateDeleted
		placements = append(placements, p)
	}
	return repoDirRow{
		Repo:       name,
		Private:    strings.EqualFold(repo.Visibility.Or(""), "private"),
		Placements: placements,
	}
}

// reportNativeMirrorNotes writes what the table has no column for: the server's
// own reason for a placement that is not healthy. It goes to stderr so a piped
// table or --json stays clean, and names the cluster so a multi-placement repo
// stays legible.
func reportNativeMirrorNotes(w io.Writer, mirrors []coreapi.NativeMirrorPlacement) {
	for _, m := range mirrors {
		if detail := strings.TrimSpace(m.LastError.Or("")); detail != "" {
			fmt.Fprintf(w, "%s: %s\n", m.ClusterSlug, detail)
		}
	}
}

// nativeUsePlacements lists the clusters `repo remote use` may point a git
// remote at: the repo's primary, plus every mirror that is actually readable.
//
// A placement that is still seeding, failed or suspended serves nothing, so
// offering it would hand the user a remote that cannot fetch. The primary is
// always offered — it is where the repo lives, and the only placement that
// accepts a push.
//
// The result is coreapi.ResolvedPlacement, the shape the GitHub path resolves
// from the server, so both forges feed one picker (selectPlacement) instead of
// growing a second. Cell carries the slug, which is what the picker labels and
// what --cluster names.
func nativeUsePlacements(repo *coreapi.Repo, mirrors []coreapi.NativeMirrorPlacement, clusters []coreapi.Cluster) []coreapi.ResolvedPlacement {
	hostBySlug := clusterHostBySlug(clusters)
	out := make([]coreapi.ResolvedPlacement, 0, len(mirrors)+1)
	add := func(slug string) {
		host := hostBySlug[slug]
		if host == "" {
			return // unresolvable or unsafe public URL: never offer a spoofable remote
		}
		p := coreapi.ResolvedPlacement{ClusterHost: host, Cell: coreapi.NewOptString(slug)}
		if cl, ok := clusterBySlug(clusters, slug); ok && cl.Jurisdiction != "" {
			p.Jurisdiction = coreapi.NewOptString(cl.Jurisdiction)
		}
		out = append(out, p)
	}
	if primary := repo.ClusterSlug.Or(""); primary != "" {
		add(primary)
	}
	for _, m := range mirrors {
		if m.Status != coreapi.NativeMirrorPlacementStatusReady ||
			m.DesiredState == coreapi.NativeMirrorPlacementDesiredStateDeleted {
			continue
		}
		add(m.ClusterSlug)
	}
	return out
}

// nativeRepoURLAt builds the entire:// URL for this repo on host, from the
// server's own `path`. Empty when the repo has no path yet (still provisioning)
// or the host could not be validated.
func nativeRepoURLAt(repo *coreapi.Repo, host string) string {
	path := strings.TrimSpace(repo.Path.Or(""))
	if host == "" || path == "" {
		return ""
	}
	return entireCloneURLScheme + host + "/" + strings.TrimPrefix(path, "/")
}
