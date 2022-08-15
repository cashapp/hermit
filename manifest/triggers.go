package manifest

import (
	"sort"

	"github.com/cashapp/hermit/errors"
)

// Event in the lifecycle of a package.
type Event string

func (e *Event) UnmarshalText(text []byte) error {
	event := Event(text)
	_, ok := eventMap[event]
	if !ok {
		return errors.Errorf("invalid event %q", event)
	}
	*e = event
	return nil
}

// Lifecycle events.
const (
	// Package specific events
	EventUnpack    Event = "unpack"
	EventInstall   Event = "install"
	EventUninstall Event = "uninstall"
	EventExec      Event = "exec"
	// Environment specific events
	EventEnvActivate Event = "activate"
)

// Valid events.
var eventMap = map[Event]bool{
	EventUnpack:      true,
	EventInstall:     true,
	EventUninstall:   true,
	EventExec:        true,
	EventEnvActivate: true,
}

// A Trigger applied when a lifecycle event occurs.
type Trigger struct {
	Event Event `hcl:"event,label" help:"Event to Trigger (unpack, install, activate)."`

	Run     []*RunAction     `hcl:"run,block" help:"A command to run when the event is triggered."`
	Copy    []*CopyAction    `hcl:"copy,block" help:"A file to copy when the event is triggered."`
	Chmod   []*ChmodAction   `hcl:"chmod,block" help:"Change a files mode."`
	Rename  []*RenameAction  `hcl:"rename,block" help:"Rename a file."`
	Delete  []*DeleteAction  `hcl:"delete,block" help:"Delete files."`
	Message []*MessageAction `hcl:"message,block" help:"Display a message to the user."`
}

// Ordered list of actions.
func (a *Trigger) Ordered() []Action {
	var out []Action
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
	for _, action := range a.Delete {
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
