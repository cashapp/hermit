package hermit

import (
	"testing"

	"github.com/alecthomas/assert/v2"
	"github.com/cashapp/hermit/shell"
)

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
