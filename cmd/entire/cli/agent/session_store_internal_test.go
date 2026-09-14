package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/entireio/cli/cmd/entire/cli/osroot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionStore_OpenRootForWriteRejectsAncestorReplacedAfterPreflight(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	parent := filepath.Join(base, "parent")
	require.NoError(t, os.Mkdir(parent, 0o750))
	store := &SessionStore{dir: filepath.Join(parent, "store")}
	require.NoError(t, store.ValidateWritePath("session.jsonl"))

	originalParent := filepath.Join(base, "original-parent")
	require.NoError(t, os.Rename(parent, originalParent))
	outside := t.TempDir()
	if err := os.Symlink(outside, parent); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	root, err := store.openRootForWrite()
	if err == nil {
		assert.NoError(t, osroot.WriteFile(root, "session.jsonl", []byte("outside\n"), 0o600))
		assert.NoError(t, root.Close())
	}
	require.ErrorIs(t, err, osroot.ErrSymlinkedPath)

	_, statErr := os.Stat(filepath.Join(outside, "store", "session.jsonl"))
	assert.True(t, os.IsNotExist(statErr), "write must not follow an ancestor replaced after preflight")
}
