package app

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/cashapp/hermit"
	"github.com/cashapp/hermit/ui"
	"github.com/pkg/errors"
	"mvdan.cc/sh/v3/syntax"
)

var (
	builtinCommands = func() map[string]bool {
		cmds := []string{
			// POSIX utilites (http://pubs.opengroup.org/onlinepubs/9699919799/idx/utilities.html)
			"admin", "alias", "ar", "asa", "at", "awk", "basename", "batch", "bc",
			"bg", "c99", "cal", "cat", "cd", "cflow", "chgrp", "chmod", "chown",
			"cksum", "cmp", "comm", "command", "compress", "cp", "crontab", "csplit",
			"ctags", "cut", "cxref", "date", "dd", "delta", "df", "diff", "dirname",
			"du", "echo", "ed", "env", "ex", "expand", "expr", "false", "fc",
			"fg", "file", "find", "fold", "fort77", "fuser", "gencat", "get",
			"getconf", "getopts", "grep", "hash", "head", "iconv", "id", "ipcrm",
			"ipcs", "jobs", "join", "kill", "lex", "link", "ln", "locale", "localedef",
			"logger", "logname", "lp", "ls", "m4", "mailx", "make", "man", "mesg",
			"mkdir", "mkfifo", "more", "mv", "newgrp", "nice", "nl",
			"nm", "nohup", "od", "paste", "patch", "pathchk", "pax", "pr", "printf",
			"prs", "ps", "pwd", "read", "renice", "rm", "rmdel", "rmdir", "sact",
			"sccs", "sed", "sh", "sleep", "sort", "split", "strings", "strip",
			"stty", "tabs", "tail", "talk", "tee", "test", "time", "touch", "tput",
			"tr", "true", "tsort", "tty", "type", "ulimit", "umask", "unalias",
			"uname", "uncompress", "unexpand", "unget", "uniq", "unlink", "uucp",
			"uudecode", "uuencode", "uustat", "uux", "val", "vi", "wait", "wc",
			"what", "who", "write", "xargs", "yacc",
			// Bash builtins
			":", ".", "[", "alias", "bg", "bind", "break", "builtin", "case", "cd",
			"command", "compgen", "complete", "continue", "declare", "dirs", "disown",
			"echo", "enable", "eval", "exec", "exit", "export", "fc", "fg", "getopts",
			"hash", "help", "history", "if", "jobs", "kill", "let", "local",
			"logout", "popd", "printf", "pushd", "pwd", "read", "readonly", "return",
			"set", "shift", "shopt", "source", "suspend", "test", "times", "trap",
			"type", "typeset", "ulimit", "umask", "unalias", "unset", "until",
			"wait", "while",
			// Other
			"bash", "mktemp",
		}
		out := make(map[string]bool, len(cmds))
		for _, cmd := range cmds {
			out[cmd] = true
		}
		return out
	}()
)

type validateScriptCmd struct {
	Allow  []string `short:"a" enum:"none,relative,var-relative" help:"Enable optional features (${enum})." default:"none"`
	Cmds   []string `short:"c" placeholder:"CMD" help:"Extra commands to allow."`
	Script []string `arg:"" placeholder:"SCRIPT" type:"existingfile" help:"Bourne-compatible shell scripts to validate."`
}

func (v *validateScriptCmd) Help() string {
	return `
Builtin commands:

	` + strings.ReplaceAll(builtins(70), "\n", "\n  ")
}

func (v *validateScriptCmd) Run(l *ui.UI, env *hermit.Env) error {
	validCommands := make(map[string]bool, len(builtinCommands)+len(v.Cmds))
	if env != nil {
		validCommands["hermit"] = true
		validCommands["activate-hermit"] = true
		pkgs, err := env.ListInstalled(l)
		if err != nil {
			return errors.WithStack(err)
		}
		for _, pkg := range pkgs {
			bins, err := env.LinkedBinaries(pkg)
			if err != nil {
				return errors.WithStack(err)
			}
			for _, bin := range bins {
				validCommands[filepath.Base(bin)] = true
			}
		}
	}

	pwd, err := os.Getwd()
	if err != nil {
		return errors.WithStack(err)
	}
	for k, v := range builtinCommands {
		validCommands[k] = v
	}
	for _, cmd := range v.Cmds {
		validCommands[cmd] = true
	}
	allow := map[string]bool{}
	for _, feature := range v.Allow {
		if feature == "none" {
			allow = map[string]bool{}
		} else {
			allow[feature] = true
		}
	}
	parser := syntax.NewParser()
	var issues []issue
	for _, path := range v.Script {
		pissues, err := check(parser, validCommands, allow, path)
		if err != nil {
			return errors.Wrapf(err, "failed to check %s", path)
		}
		issues = append(issues, pissues...)
	}

	for _, issue := range issues {
		path, err := filepath.Rel(pwd, issue.path)
		if err != nil {
			return errors.WithStack(err)
		}
		fmt.Fprintf(os.Stderr, "%s:%s: %s\n", path, issue.pos, issue.message)
	}
	if len(issues) != 0 {
		return fmt.Errorf("%d errors encountered", len(issues))
	}
	return nil
}

type issue struct {
	path    string
	pos     syntax.Pos
	message string
}

func check(parser *syntax.Parser, validCommands map[string]bool, allow map[string]bool, path string) ([]issue, error) {
	var issues []issue
	r, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("could not open %q: %w", path, err)
	}
	defer r.Close() // nolint
	ast, err := parser.Parse(r, path)
	if err != nil {
		return nil, fmt.Errorf("failed to parse %q: %w", path, err)
	}
	localFunctions := map[string]bool{}

	// Collection forward declarations - this is probably not 100% accurate due to nested functions
	syntax.Walk(ast, func(node syntax.Node) bool {
		if node, ok := node.(*syntax.FuncDecl); ok {
			localFunctions[node.Name.Value] = true
		}
		return true
	})
	cmds := map[string]syntax.Pos{}
	syntax.Walk(ast, func(node syntax.Node) bool {
		switch node := node.(type) {
		case *syntax.CallExpr:
			if len(node.Args) == 0 {
				break
			}
			cmd := stringify(node.Args[0])
			if strings.HasPrefix(cmd, "\"") {
				uqcmd, err := strconv.Unquote(cmd) // FIXME: this is a hack
				if err == nil {
					cmd = uqcmd
				}
			}
			cmds[cmd] = node.Pos()
		}
		return true
	})

	for cmd, pos := range cmds {
		if allow["var-relative"] && strings.HasPrefix(cmd, "$") {
			continue
		}
		if allow["relative"] && !filepath.IsAbs(cmd) && strings.Contains(cmd, "/") {
			continue
		}
		if validCommands[cmd] || localFunctions[cmd] {
			continue
		}
		issues = append(issues, issue{
			path:    ast.Name,
			pos:     pos,
			message: fmt.Sprintf("%s: unsupported external command: %s", pos, cmd),
		})
	}
	return issues, nil
}

func stringify(node syntax.Node) string {
	out := &strings.Builder{}
	syntax.NewPrinter().Print(out, node)
	return out.String()
}

func builtins(maxWidth int) string {
	w := &strings.Builder{}
	cmds := make([]string, 0, len(builtinCommands))
	for cmd := range builtinCommands {
		cmds = append(cmds, cmd)
	}
	sort.Strings(cmds)
	width := 0
	for _, cmd := range cmds {
		if width > 0 && width+1+len(cmd) > maxWidth {
			fmt.Fprintln(w)
			width = 0
		}
		if width > 0 {
			fmt.Fprint(w, " ")
			width++
		}
		fmt.Fprint(w, cmd)
		width += len(cmd)
	}
	return w.String()
}
