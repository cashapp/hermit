package hermit

import (
	"testing"

	"github.com/alecthomas/assert/v2"
	"github.com/cashapp/hermit/shell"
)

func TestHermitLauncherUsesTrustedSystemHelpers(t *testing.T) {
	// Regression test for DX-27: the launcher can run after the environment's
	// bin directory has been prepended to PATH.
	script, err := files.ReadFile("files/hermit")
	assert.NoError(t, err)

	assert.Contains(t, string(script), `case "${OSTYPE}" in`)
	assert.NotContains(t, string(script), "uname")
	assert.Contains(t, string(script), `/usr/bin/basename `)
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
