package hermit

import (
	"strings"
	"testing"

	"github.com/alecthomas/assert/v2"
	"github.com/cashapp/hermit/shell"
)

func TestHermitLauncherUsesTrustedSystemHelpers(t *testing.T) {
	// Regression test for DX-27: the launcher can run after the environment's
	// bin directory has been prepended to PATH.
	script, err := files.ReadFile("files/hermit")
	assert.NoError(t, err)

	calls := 0
	for line := range strings.SplitSeq(string(script), "\n") {
		if strings.Contains(line, "uname -s") {
			assert.Contains(t, line, "/usr/bin/uname -s")
			calls++
		}
		if strings.Contains(line, "basename ") {
			assert.Contains(t, line, "/usr/bin/basename ")
			calls++
		}
	}
	assert.Equal(t, 2, calls)
}

func TestActivationCommand(t *testing.T) {
	const env = "/path/to/env"

	tests := []struct {
		name     string
		shell    shell.Shell
		expected string
	}{
		{
			name:     "fish",
			shell:    &shell.Fish{},
			expected: ". /path/to/env/bin/activate-hermit.fish",
		},
		{
			name:     "bash",
			shell:    &shell.Bash{},
			expected: ". /path/to/env/bin/activate-hermit",
		},
		{
			name:     "zsh",
			shell:    &shell.Zsh{},
			expected: ". /path/to/env/bin/activate-hermit",
		},
		{
			name:     "detection failure",
			shell:    nil,
			expected: ". /path/to/env/bin/activate-hermit",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, activationCommand(env, tt.shell))
		})
	}
}
