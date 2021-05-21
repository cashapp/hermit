package manifest

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alecthomas/hcl"
	"github.com/kballard/go-shellquote"
	"github.com/pkg/errors"

	"github.com/cashapp/hermit/shell"
	"github.com/cashapp/hermit/vfs"
)

func (p *Package) triggerRename(action *RenameAction) error {
	return os.Rename(action.From, action.To)
}

func (p *Package) triggerChmod(action *ChmodAction) error {
	return os.Chmod(action.File, os.FileMode(action.Mode))
}

func (p *Package) triggerCopy(action *CopyAction) error {
	if filepath.IsAbs(action.From) {
		return vfs.CopyFile(os.DirFS("/"), action.From, action.To)
	}
	return vfs.CopyFile(p.FS, action.From, action.To)
}

func (p *Package) triggerRun(trigger *RunAction) error {
	cmd := exec.Command(trigger.Command, trigger.Args...)
	cmd.Env = trigger.Env
	if trigger.Dir == "" {
		cmd.Dir = p.Root
	} else {
		cmd.Dir = trigger.Dir
	}
	if trigger.Stdin != "" {
		cmd.Stdin = strings.NewReader(trigger.Stdin)
	}

	out, err := cmd.Output()
	if err != nil {
		return errors.Wrapf(err, "%s: failed to execute %q: %s", p, trigger.Command, string(out))
	}
	return nil
}

// go-sumtype:decl Action EnvOp

// Action interface implemented by all lifecycle trigger actions.
type Action interface {
	position() hcl.Position
	String() string
}

// MessageAction displays a message to the user.
type MessageAction struct {
	Pos hcl.Position `hcl:"-"`

	Text string `hcl:"text" help:"Message text to display to user."`
}

func (m *MessageAction) position() hcl.Position { return m.Pos }
func (m *MessageAction) String() string         { return fmt.Sprintf("echo %s", shell.Quote(m.Text)) }

// RenameAction renames a file.
type RenameAction struct {
	Pos hcl.Position `hcl:"-"`

	From string `hcl:"from" help:"Source path to rename."`
	To   string `hcl:"to" help:"Destination path to rename to."`
}

func (r *RenameAction) position() hcl.Position { return r.Pos }
func (r *RenameAction) String() string {
	return fmt.Sprintf("mv %s %s", shell.Quote(r.From), shell.Quote(r.To))
}

// ChmodAction changes the file mode on a file.
type ChmodAction struct {
	Pos hcl.Position `hcl:"-"`

	Mode int    `hcl:"mode" help:"File mode to set."`
	File string `hcl:"file" help:"File to set mode on."`
}

func (c *ChmodAction) position() hcl.Position { return c.Pos }
func (c *ChmodAction) String() string         { return fmt.Sprintf("chmod %o %s", c.Mode, shell.Quote(c.File)) }

// RunAction executes a command when a lifecycle event occurs
type RunAction struct {
	Pos hcl.Position `hcl:"-"`

	Command string   `hcl:"cmd" help:"The command to execute"`
	Dir     string   `hcl:"dir,optional" help:"The directory where the command is run in. Defaults to the root directory."`
	Args    []string `hcl:"args,optional" help:"The arguments to the binary"`
	Env     []string `hcl:"env,optional" help:"The environment variables for the execution"`
	Stdin   string   `hcl:"stdin,optional" help:"Optional string to be used as the stdin for the command"`
}

func (c *RunAction) position() hcl.Position { return c.Pos }
func (c *RunAction) String() string {
	return fmt.Sprintf("%s %s", c.Command, shellquote.Join(c.Args...))
}

// CopyAction is an action for copying
type CopyAction struct {
	Pos hcl.Position `hcl:"-"`

	From string `hcl:"from" help:"The source file to copy from. Absolute paths reference the file system while relative paths are against the manifest source bundle."`
	To   string `hcl:"to" help:"The relative destination to copy to, based on the context."`
}

func (c *CopyAction) position() hcl.Position { return c.Pos }
func (c *CopyAction) String() string {
	return fmt.Sprintf("cp %s %s", shell.Quote(c.From), shell.Quote(c.To))
}

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
