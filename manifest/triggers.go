package manifest

import (
	"sort"

	"github.com/cashapp/hermit/errors"
	"github.com/qdm12/reprint"
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
	EventExec      Event = "exec" // Triggered when a binary in a package is executed.
	// Environment specific events
	EventEnvActivate Event = "activate"
)

// Valid events.
var eventMap = map[Event]bool{
	EventUnpack:      true,
	EventInstall:     true,
	EventUninstall:   true,
	EventEnvActivate: true,
	EventExec:        true,
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
	Mkdir   []*MkdirAction   `hcl:"mkdir,block" help:"Create a directory and any missing parents."`
	Symlink []*SymlinkAction `hcl:"symlink,block" help:"Create a symbolic link."`
}

// Ordered list of actions.
func (a *Trigger) Ordered() []Action {
	var out []Action
	for _, action := range a.Run {
		out = append(out, reprint.This(action).(Action))
	}
	for _, action := range a.Copy {
		out = append(out, reprint.This(action).(Action))
	}
	for _, action := range a.Chmod {
		out = append(out, reprint.This(action).(Action))
	}
	for _, action := range a.Rename {
		out = append(out, reprint.This(action).(Action))
	}
	for _, action := range a.Delete {
		out = append(out, reprint.This(action).(Action))
	}
	for _, action := range a.Message {
		out = append(out, reprint.This(action).(Action))
	}
	for _, action := range a.Mkdir {
		out = append(out, reprint.This(action).(Action))
	}
	for _, action := range a.Symlink {
		out = append(out, reprint.This(action).(Action))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].position().Line < out[j].position().Line
	})
	return out
}
