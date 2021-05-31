package sandbox

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEval(t *testing.T) {
	root := t.TempDir()
	sandbox, err := New(root)
	require.NoError(t, err)
	err = sandbox.Eval(`
		set -euo pipefail

		ls --help
		rm --help
		ls --foo || true
		ls asdfa || true
		echo > .t
		echo "Hello world" > t
		mkdir test
		echo hi > test/foo
		echo CAT; cat test/foo
		(cd test && ls -l)
		ls
		ls -la | cat
		ls -la | grep test
		ls -l t
		rm -rf *
		echo empty
		ls -l
		cd /
	`)
	require.NoError(t, err)
}
