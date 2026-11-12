package ui

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestProgressLinesFitTerminalWidth(t *testing.T) {
	ui, buf := NewForTesting()
	ui.width = 60
	for i := range 1000 {
		ui.Progress(fmt.Sprintf("a-long-task-label-%d", i), 1)
	}
	escapes := regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]|\r`)
	for _, line := range strings.Split(buf.String(), "\n") {
		line = escapes.ReplaceAllString(line, "")
		if n := utf8.RuneCountInString(line); n >= ui.width {
			t.Fatalf("progress line of %d columns would wrap a %d column terminal: %q", n, ui.width, line)
		}
	}
}
