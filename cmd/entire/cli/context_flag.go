package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/entireio/cli/internal/entireclient/contexts"
	"github.com/spf13/cobra"
)

// contextFlagValue applies --context to the process-wide selection as pflag
// parses it.
//
// Binding through a pflag.Value rather than a PersistentPreRun is deliberate:
// Set() runs during flag parsing — before any PreRun, before RunE, and before
// anything resolves a token — so there is no ordering to get wrong. That
// ordering is the only reason: root.go sets cobra.EnableTraverseRunHooks, so a
// root PersistentPreRun is not shadowed by the subtrees that define their own
// (agent_group.go, the per-agent hooks commands) and would work too, just
// resolving the identity later than flag parsing does, for no gain.
type contextFlagValue struct{ name string }

func (v *contextFlagValue) String() string { return v.name }
func (v *contextFlagValue) Type() string   { return "string" }

func (v *contextFlagValue) Set(name string) error {
	v.name = name
	contexts.SetFlagOverride(name)
	return exportContextToChildren(name)
}

// exportContextToChildren publishes the --context selection as ENTIRE_CONTEXT in
// this process's own environment, so every child inherits it.
//
// The in-process override alone does not reach a child, and several commands
// spawn one that selects a login by itself: `repo clone` execs `git clone
// entire://…`, and git runs the `entire` remote helper as a separate process
// that resolves credentials from the saved contexts; `resume`, `explain`,
// `trail create` and checkpoint-policy fetch or push through the same helper
// whenever origin is an entire:// URL. Without the export the helper sees only
// the active context and, with several logins eligible for the cluster, fails
// with the ambiguity error the flag exists to avoid (COR-1630). ENTIRE_CONTEXT
// is the channel the helper honours for `ENTIRE_CONTEXT=… git push`, and it is
// set here rather than on each exec for the same reason the flag is global
// rather than per-command: a new spawn site would otherwise silently drop it.
//
// Mutating the process environment is deliberate and in scope: flagOverride is
// already process-global on the grounds that one CLI invocation acts as one
// identity, and this is that same identity, made visible to the children of
// that same invocation. The flag outranks an inherited ENTIRE_CONTEXT in
// process (contexts.requestedContext), so overwriting it here keeps parent and
// children agreeing. A blank name clears the in-process override and is not
// exported, leaving whatever the environment already said.
func exportContextToChildren(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	if err := os.Setenv(contexts.EnvContextVar, name); err != nil {
		return fmt.Errorf("export --context to child processes: %w", err)
	}
	return nil
}

// addContextFlag registers --context as a persistent flag on the root command,
// so every subcommand that authenticates inherits it without per-command
// plumbing.
//
// It is global rather than per-command because it selects an identity, and every
// path that resolves one honours it (git, control plane, data API, cell routing,
// status, logout). Registering it only on the commands we think authenticate
// today is how it would drift out of sync: a new authenticating command would
// silently ignore it. The cost is that it also parses on commands with no
// identity to select, like `version` — harmless, and the same tradeoff kubectl
// makes with its own global --context.
func addContextFlag(cmd *cobra.Command) {
	// The back-quoted word is pflag's value placeholder, so this renders as
	// `--context name`. Any other back-quoted span here (e.g. around a command to
	// run) would be silently hijacked as the placeholder instead.
	cmd.PersistentFlags().Var(&contextFlagValue{}, "context",
		"Act as this saved login `name` for this command only, instead of the active context (entire auth contexts lists them)")
	if err := cmd.RegisterFlagCompletionFunc("context", completeContextFlag); err != nil {
		panic("register --context completion: " + err.Error())
	}
}

// completeContextFlag completes saved context names for --context. It reuses the
// same listing `auth use` completes against, so both offer the same names with
// the same descriptions.
func completeContextFlag(cmd *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// completeContextNames takes the positional args of `auth use <context>`; pass
	// none so it treats this as completing the first (and only) value.
	return completeContextNames(cmd, nil, toComplete)
}
