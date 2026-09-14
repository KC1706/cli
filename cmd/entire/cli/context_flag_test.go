package cli

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/entireio/cli/internal/entireclient/contexts"
)

// These tests mutate the process-wide --context override and the process
// environment, so none of them may call t.Parallel().

// TestContextFlag_ExportsToChildEnvironment: parsing --context publishes the
// selection as ENTIRE_CONTEXT, the only channel git's `entire` remote helper
// (and any other child that resolves a login itself) reads (COR-1630). The
// flag outranks an inherited value, matching in-process precedence.
func TestContextFlag_ExportsToChildEnvironment(t *testing.T) {
	t.Setenv(contexts.EnvContextVar, "eu.auth.partial.to")
	contexts.SetFlagOverrideForTest(t, "")

	require.NoError(t, (&contextFlagValue{}).Set("us.auth.partial.to"))

	assert.Equal(t, "us.auth.partial.to", os.Getenv(contexts.EnvContextVar), "children must see the flag's choice")
	sel, err := (&contexts.File{Contexts: []*contexts.Context{{Name: "us.auth.partial.to", CoreURL: "https://us.auth.partial.to"}}}).Active()
	require.NoError(t, err)
	assert.Equal(t, "--context", sel.Source, "in-process resolution still attributes the choice to the flag")
}

// TestContextFlag_BlankLeavesEnvironmentAlone: a blank --context clears the
// in-process override and exports nothing, so an ENTIRE_CONTEXT the user set
// still reaches children untouched.
func TestContextFlag_BlankLeavesEnvironmentAlone(t *testing.T) {
	t.Setenv(contexts.EnvContextVar, "eu.auth.partial.to")
	contexts.SetFlagOverrideForTest(t, "")

	require.NoError(t, (&contextFlagValue{}).Set("  "))

	assert.Equal(t, "eu.auth.partial.to", os.Getenv(contexts.EnvContextVar))
}
