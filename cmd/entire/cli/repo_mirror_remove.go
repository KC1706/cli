package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"charm.land/huh/v2"
	"github.com/spf13/cobra"

	"github.com/entireio/cli/cmd/entire/cli/interactive"
	"github.com/entireio/cli/cmd/entire/cli/uiform"
	"github.com/entireio/cli/internal/coreapi"
)

// mirrorPlacement is one cluster a repo is currently mirrored on, in the terms
// `mirror remove` acts in. Both forges resolve to this so the picker, the
// parallel removal and the result table are written once.
type mirrorPlacement struct {
	region regionChoice
	status string
	// primary marks a native repo's home cluster. It is listed so the picker
	// shows where the repo actually lives, but it is never removable here:
	// dropping it is `entire repo delete`.
	primary bool
}

// listMirrorPlacements reports where a repo is mirrored today. A GitHub repo's
// placements come from the pull-gated resolver (anything you could clone), a
// native repo's from its own primary plus its native-mirror list — the primary
// is not in that list, so it is joined on.
func listMirrorPlacements(ctx context.Context, c *coreapi.Client, ref mirrorRepoRef, regions []regionChoice) ([]mirrorPlacement, error) {
	if ref.forge == nativeCloneForge {
		repo, err := resolveNativeRepo(ctx, c, ref.owner, ref.repo)
		if err != nil {
			return nil, err
		}
		mirrors, err := listNativeMirrors(ctx, c, repo.ID)
		if err != nil {
			return nil, err
		}
		out := make([]mirrorPlacement, 0, len(mirrors)+1)
		if primary := repo.ClusterSlug.Or(""); primary != "" {
			if region, ok := regionBySlug(regions, primary); ok {
				out = append(out, mirrorPlacement{region: region, status: repo.State.Or("-"), primary: true})
			}
		}
		for _, m := range mirrors {
			if region, ok := regionBySlug(regions, m.ClusterSlug); ok {
				out = append(out, mirrorPlacement{region: region, status: string(m.Status)})
			}
		}
		return out, nil
	}

	placements, err := resolvePullablePlacements(ctx, c, ref.owner, ref.repo)
	if err != nil {
		return nil, err
	}
	out := make([]mirrorPlacement, 0, len(placements))
	for _, p := range placements {
		for _, region := range regions {
			if strings.EqualFold(region.host, p.ClusterHost) {
				out = append(out, mirrorPlacement{region: region})
				break
			}
		}
	}
	return out, nil
}

// removableMirrorPlacements drops the ones `mirror remove` must not act on.
func removableMirrorPlacements(placements []mirrorPlacement) []mirrorPlacement {
	out := make([]mirrorPlacement, 0, len(placements))
	for _, p := range placements {
		if p.primary {
			continue
		}
		out = append(out, p)
	}
	return out
}

func mirrorPlacementRegions(placements []mirrorPlacement) []regionChoice {
	out := make([]regionChoice, 0, len(placements))
	for _, p := range placements {
		out = append(out, p.region)
	}
	return out
}

// chooseMirrorRemoveRegions turns --cluster (or the absence of it) into the
// placements a remove will drop.
//
// Unlike `add`, there is no default: removing is destructive, and the set of
// clusters is a property of the repo rather than of the catalog, so guessing
// one would delete a copy the caller never named. A terminal gets a
// multi-select over what the repo actually has — which is also the
// confirmation, since nothing is removed that was not ticked.
func chooseMirrorRemoveRegions(cmd *cobra.Command, ref mirrorRepoRef, placements []mirrorPlacement, slugs []string) ([]regionChoice, error) {
	byPrimary := map[string]bool{}
	for _, p := range placements {
		if p.primary {
			byPrimary[strings.ToLower(p.region.slug)] = true
		}
	}
	removable := removableMirrorPlacements(placements)

	if len(slugs) > 0 {
		chosen := make([]regionChoice, 0, len(slugs))
		seen := map[string]bool{}
		for _, slug := range slugs {
			if byPrimary[strings.ToLower(slug)] {
				return nil, fmt.Errorf("%s lives on %s: that is its primary, not a mirror, and removing it is `entire repo delete %s`", ref.qualified(), slug, ref.qualified())
			}
			region, ok := regionBySlug(mirrorPlacementRegions(removable), slug)
			if !ok {
				if len(removable) == 0 {
					return nil, fmt.Errorf("%s has no mirrors to remove", ref.qualified())
				}
				return nil, fmt.Errorf("%s is not mirrored on %q; it is on: %s", ref.qualified(), slug, strings.Join(regionSlugs(mirrorPlacementRegions(removable)), ", "))
			}
			if seen[region.slug] {
				continue
			}
			seen[region.slug] = true
			chosen = append(chosen, region)
		}
		return chosen, nil
	}

	if len(removable) == 0 {
		return nil, fmt.Errorf("%s has no mirrors to remove", ref.qualified())
	}
	if !interactive.CanPromptInteractively() {
		return nil, fmt.Errorf("pass --cluster to say which mirror of %s to remove; it is on: %s",
			ref.qualified(), strings.Join(regionSlugs(mirrorPlacementRegions(removable)), ", "))
	}
	return pickRemoveRegions(cmd.Context(), cmd.ErrOrStderr(), removable)
}

// runMirrorRemove is the `repo mirror remove <repo>` body: find where the repo
// is mirrored, choose which of those to drop, then remove them in parallel.
func runMirrorRemove(cmd *cobra.Command, repoRef string, clusterSlugs []string) error {
	cmd.SilenceUsage = true
	ref, err := parseMirrorRepoRef(repoRef)
	if err != nil {
		return err
	}
	for _, slug := range clusterSlugs {
		if err := validateClusterSlug(slug); err != nil {
			return fmt.Errorf("invalid --cluster: %w", err)
		}
	}

	var (
		regions    []regionChoice
		placements []mirrorPlacement
		nativeRepo *coreapi.Repo
	)
	if err := runCore(cmd, func(ctx context.Context, c *coreapi.Client) error {
		out, lerr := c.ListClusters(ctx)
		if lerr != nil {
			return lerr
		}
		regions = clustersToRegions(out.Clusters)
		if placements, lerr = listMirrorPlacements(ctx, c, ref, regions); lerr != nil {
			return lerr
		}
		if ref.forge == nativeCloneForge {
			nativeRepo, lerr = resolveNativeRepo(ctx, c, ref.owner, ref.repo)
		}
		return lerr
	}); err != nil {
		return err
	}

	chosen, err := chooseMirrorRemoveRegions(cmd, ref, placements, clusterSlugs)
	if err != nil {
		return err
	}
	if len(chosen) == 0 {
		// A cancelled picker reports itself and returns no selection (see
		// handleFormCancellation). Stop here rather than running an empty batch,
		// which would print a progress block and a headerless table for nothing.
		return nil
	}
	results := removeMirrors(cmd.Context(), cmd.ErrOrStderr(), oneRepoTargets(ref, nativeRepo, chosen))
	return reportMirrorRemoveResults(cmd.OutOrStdout(), cmd.ErrOrStderr(), results)
}

// pickRemoveRegions is the remove verb's multi-select. Nothing starts checked —
// unlike `add`, where a default region is a convenience, a pre-ticked box here
// would be a copy deleted by pressing enter. The status of each placement is
// shown because it is often the reason one is being removed.
func pickRemoveRegions(ctx context.Context, w io.Writer, placements []mirrorPlacement) ([]regionChoice, error) {
	opts := make([]huh.Option[string], 0, len(placements))
	bySlug := make(map[string]regionChoice, len(placements))
	for _, p := range placements {
		label := regionLabel(p.region)
		if p.status != "" {
			label += " — " + p.status
		}
		opts = append(opts, huh.NewOption(label, p.region.slug))
		bySlug[p.region.slug] = p.region
	}

	var selected []string
	form := NewAccessibleForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Select the mirrors to remove").
				Description("Only the placements you tick are removed; the repo itself is untouched.").
				Options(opts...).
				Height(uiform.SingleLineMultiSelectHeight(len(opts))).
				Validate(func(s []string) error {
					if len(s) == 0 {
						return errors.New("select at least one mirror")
					}
					return nil
				}).
				Value(&selected),
		),
	)
	if err := form.RunWithContext(ctx); err != nil {
		return nil, handleFormCancellation(w, "Mirror remove", err)
	}
	chosen := make([]regionChoice, 0, len(selected))
	for _, slug := range selected {
		if r, ok := bySlug[slug]; ok {
			chosen = append(chosen, r)
		}
	}
	return chosen, nil
}

// removeMirrors tears down every target in parallel, one result per target in
// input order — the same shape createMirrors uses, so a batch remove reports
// like a batch add.
func removeMirrors(ctx context.Context, errW io.Writer, targets []mirrorTarget) []mirrorResult {
	clientByHost := make(map[string]*coreapi.Client)
	clientErrByHost := make(map[string]error)
	for _, t := range targets {
		host := t.clientHost()
		if _, seen := clientByHost[host]; seen {
			continue
		}
		if _, seen := clientErrByHost[host]; seen {
			continue
		}
		c, err := mirrorTargetClient(ctx, host)
		if err != nil {
			clientErrByHost[host] = err
		} else {
			clientByHost[host] = c
		}
	}

	labels := make([]string, len(targets))
	for i, t := range targets {
		labels[i] = t.ref() + " @ " + t.region.slug
	}
	prog := newMirrorProgress(errW, labels)
	prog.start()

	results := make([]mirrorResult, len(targets))
	var wg sync.WaitGroup
	for i, t := range targets {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = removeOneMirror(ctx, t, clientByHost[t.clientHost()], clientErrByHost[t.clientHost()],
				func(status string, final, ok bool) { prog.set(i, status, final, ok) })
		}()
	}
	wg.Wait()
	prog.stop()
	return results
}

// removeOneMirror tears down a single placement. Like its create counterpart it
// never returns an error: every outcome folds into the result so one failure
// cannot sink the batch.
func removeOneMirror(ctx context.Context, t mirrorTarget, c *coreapi.Client, clientErr error, report func(status string, final, ok bool)) mirrorResult {
	// report may be nil, as in createOneMirror: a caller that wants the outcome
	// but not the live progress should not have to supply a no-op.
	if report == nil {
		report = func(string, bool, bool) {}
	}
	res := mirrorResult{forge: t.forge, owner: t.owner, repo: t.repo, regionLabel: regionLabel(t.region)}
	if clientErr != nil {
		res.status, res.err = mirrorStatusError, clientErr
		report(mirrorStatusError, true, false)
		return res
	}
	report(mirrorStatusRemoving, false, false)

	if t.forge == nativeCloneForge {
		if _, err := c.DeleteNativeMirror(ctx, coreapi.DeleteNativeMirrorParams{
			RepoId: t.nativeRepo.ID, ClusterSlug: t.region.slug,
		}); err != nil {
			return failedRemoval(&res, err, report)
		}
		// Teardown is asynchronous: the delete records the intent and the
		// placement disappears later, so the wait is what makes "removed" true.
		if err := awaitNativeMirrorRemoved(ctx, c, t.nativeRepo.ID, t.region.slug); err != nil {
			return failedRemoval(&res, err, report)
		}
		res.status = mirrorStatusRemoved
		report(res.status, true, true)
		return res
	}

	if err := c.DeleteMirror(ctx, coreapi.DeleteMirrorParams{
		Provider:    coreapi.DeleteMirrorProviderGithub,
		Owner:       t.owner,
		Repo:        t.repo,
		ClusterHost: t.region.host,
	}); err != nil {
		return failedRemoval(&res, err, report)
	}
	res.status = mirrorStatusRemoved
	report(res.status, true, true)
	return res
}

func failedRemoval(res *mirrorResult, err error, report func(status string, final, ok bool)) mirrorResult {
	res.status, res.err = mirrorStatusError, renderCoreError(err)
	report(mirrorStatusError, true, false)
	return *res
}

var mirrorRemoveResultColumns = []string{colHeaderRepo, colHeaderRegion, colHeaderStatus}

func mirrorRemoveResultRow(r mirrorResult) []string {
	return []string{r.ref(), r.regionLabel, r.status}
}

// reportMirrorRemoveResults prints the summary table and fails the command when
// any placement survived, naming each one.
func reportMirrorRemoveResults(outW, errW io.Writer, results []mirrorResult) error {
	if len(results) == 0 {
		return nil
	}
	fmt.Fprintln(outW)
	if err := printTable(outW, mirrorRemoveResultColumns, results, mirrorRemoveResultRow); err != nil {
		return err
	}
	failures := 0
	for _, r := range results {
		if r.err != nil {
			failures++
			fmt.Fprintf(errW, "%s @ %s: %v\n", r.ref(), r.regionLabel, r.err)
		}
	}
	if failures > 0 {
		return NewSilentError(fmt.Errorf("%d mirror(s) could not be removed", failures))
	}
	return nil
}
