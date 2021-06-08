package manifest

import (
	"sort"
)

// Event in the lifecycle of a package.
type Event string

// Lifecycle events.
const (
	// Package specific events
	EventUnpack    Event = "unpack"
	EventInstall   Event = "install"
	EventUninstall Event = "uninstall"
	// Environment specific events
	EventEnvActivate   Event = "activate"
	EventEnvDeactivate Event = "deactivate"
)

// A Trigger applied when a lifecycle event occurs.
type Trigger struct {
	Event Event `hcl:"event,label" help:"Event to Trigger (unpack, install, activate)."`

	Run     []*RunAction     `hcl:"run,block" help:"A command to run when the event is triggered."`
	Copy    []*CopyAction    `hcl:"copy,block" help:"A file to copy when the event is triggered."`
	Chmod   []*ChmodAction   `hcl:"chmod,block" help:"Change a files mode."`
	Rename  []*RenameAction  `hcl:"rename,block" help:"Rename a file."`
	Message []*MessageAction `hcl:"message,block" help:"Display a message to the user."`
}

// Ordered list of actions.
func (a *Trigger) Ordered() []Action {
	out := make([]Action, 0, len(a.Run)+len(a.Copy)+len(a.Chmod)+len(a.Rename))
	for _, action := range a.Run {
		out = append(out, action)
	}
	for _, action := range a.Copy {
		out = append(out, action)
	}
	for _, action := range a.Chmod {
		out = append(out, action)
	}
	for _, action := range a.Rename {
		out = append(out, action)
	}
	for _, action := range a.Message {
		out = append(out, action)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].position().Line < out[j].position().Line
	})
	return out
}
