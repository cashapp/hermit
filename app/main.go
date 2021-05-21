package app

import (
	"bufio"
	"log"
	"net"
	"net/http"
	"os"
	"runtime"
	"runtime/pprof"
	"time"

	"github.com/alecthomas/kong"
	konghcl "github.com/alecthomas/kong-hcl"
	"github.com/mattn/go-isatty"

	"github.com/cashapp/hermit"
	"github.com/cashapp/hermit/state"
	"github.com/cashapp/hermit/ui"
	"github.com/cashapp/hermit/util/debug"
)

const help = `🐚 Hermit is a hermetic binary package manager.`

// HTTPTransportConfig defines the configuration for HTTP transports used by Hermit.
type HTTPTransportConfig struct {
	ResponseHeaderTimeout time.Duration
	DialTimeout           time.Duration
	KeepAlive             time.Duration
}

// Config for the main Hermit application.
type Config struct {
	Version     string
	LogLevel    ui.Level
	BaseDistURL string
	HTTP        func(HTTPTransportConfig) *http.Client
	State       state.Config
	// True if we're running in CI.
	CI bool
}

// Main runs the Hermit command-line application with the given config.
func Main(config Config) {
	if config.HTTP == nil {
		config.HTTP = func(config HTTPTransportConfig) *http.Client {
			transport := &http.Transport{
				ResponseHeaderTimeout: config.ResponseHeaderTimeout,
				DialContext: (&net.Dialer{
					Timeout:   config.DialTimeout,
					KeepAlive: config.KeepAlive,
				}).DialContext,
			}
			return &http.Client{Transport: transport}
		}
	}
	var (
		err         error
		p           *ui.UI
		stdoutIsTTY = !config.CI && isatty.IsTerminal(os.Stdout.Fd())
		stderrIsTTY = !config.CI && isatty.IsTerminal(os.Stderr.Fd())
	)
	if isatty.IsTerminal(os.Stdout.Fd()) && !config.CI {
		// This is necessary because stdout/stderr are unbuffered and thus _very_ slow.
		stdout := bufio.NewWriter(os.Stdout)
		stderr := bufio.NewWriter(os.Stderr)
		go func() {
			for {
				time.Sleep(time.Millisecond * 100)
				err = stdout.Flush()
				if err != nil {
					break
				}
				err = stderr.Flush()
				if err != nil {
					break
				}
			}
		}()
		p = ui.New(config.LogLevel, &bufioSyncer{stdout}, &bufioSyncer{stderr}, stdoutIsTTY, stderrIsTTY)
		defer stdout.Flush()
		defer stderr.Flush()
	} else {
		p = ui.New(config.LogLevel, os.Stdout, os.Stderr, stdoutIsTTY, stderrIsTTY)
	}
	defer func() {
		err := recover()
		p.Clear()
		if err != nil {
			panic(err)
		}
	}()

	var (
		cli cliCommon
		env *hermit.Env
		sta *state.State
	)
	isActivated := false
	envPath := os.Getenv("HERMIT_ENV")
	if err != nil {
		log.Fatalf("failed to open state: %s", err) // nolint: gocritic
	}
	if envPath != "" {
		isActivated = true
		cli = &activated{}
	} else {
		envPath, err = os.Getwd()
		if err != nil {
			log.Fatalf("couldn't get working directory: %s", err)
		}
		cli = &unactivated{}
	}

	parser, err := kong.New(cli,
		kong.Groups{
			"env":    "Environment:\nCommands for creating and managing environments.",
			"global": "Global:\nCommands for interacting with the shared global Hermit state.",
		},
		kong.Configuration(konghcl.Loader, "~/.hermit.hcl"),
		kong.UsageOnError(),
		kong.Description(help),
		kong.BindTo(cli, (*cliCommon)(nil)),
		kong.Bind(p),
		kong.Vars{
			"version": config.Version,
			"env":     envPath,
		},
		kong.HelpOptions{
			Compact: true,
		})
	if err != nil {
		log.Fatalf("failed to initialise CLI: %s", err)
	}
	ctx, err := parser.Parse(os.Args[1:])
	parser.FatalIfErrorf(err)

	configureLogging(cli, ctx.Command(), p)

	sta, err = openState(p, config.State, config.HTTP)
	if err != nil {
		log.Fatalf("failed to open state: %s", err)
	}

	if isActivated {
		env, err = hermit.OpenEnv(p, envPath, sta, cli.getEnv())
		if err != nil {
			log.Fatalf("failed to open environment: %s", err)
		}
	}

	if pprofPath := cli.getCPUProfile(); pprofPath != "" {
		f, err := os.Create(pprofPath)
		fatalIfError(p, err)
		defer f.Close() // nolint: gosec
		err = pprof.StartCPUProfile(f)
		fatalIfError(p, err)
		defer pprof.StopCPUProfile()

	}
	if pprofPath := cli.getMemProfile(); pprofPath != "" {
		f, err := os.Create(pprofPath)
		fatalIfError(p, err)
		defer f.Close() // nolint: gosec
		runtime.GC()    // get up-to-date statistics
		err = pprof.WriteHeapProfile(f)
		fatalIfError(p, err)
	}
	err = ctx.Run(env, p, sta, config, cli.getEnv())
	if err != nil && p.WillLog(ui.LevelDebug) {
		p.Fatalf("%+v", err)
	} else {
		fatalIfError(p, err)
	}
}

func openState(p *ui.UI, config state.Config, newHTTPClient func(HTTPTransportConfig) *http.Client) (*state.State, error) {
	client := newHTTPClient(HTTPTransportConfig{})
	fastFailClient := newHTTPClient(HTTPTransportConfig{
		ResponseHeaderTimeout: time.Second * 5,
		DialTimeout:           time.Second,
		KeepAlive:             30 * time.Second,
	})
	if debug.Flags.FailHTTP {
		client.Timeout = time.Millisecond
		fastFailClient.Timeout = time.Millisecond
	}
	return state.Open(hermit.UserStateDir, config, client, fastFailClient, p)
}

func configureLogging(cli cliCommon, cmd string, p *ui.UI) {
	// This is set to avoid logging in environments where quiet flag is not used
	// in the "hermit" script. This is fragile, and should be removed when we know that all the
	// environments are using a script with executions done with --quiet
	isExecution := cmd == "exec <binary>"

	switch {
	case cli.getTrace():
		p.SetLevel(ui.LevelTrace)
	case cli.getDebug():
		p.SetLevel(ui.LevelDebug)
	case cli.getQuiet():
		p.SetLevel(ui.LevelFatal)
	default:
		if isExecution {
			p.SetLevel(ui.LevelFatal)
		} else {
			p.SetLevel(cli.getLevel())
		}
	}

	if cli.getQuiet() {
		p.SetProgressBarEnabled(false)
	}
}

// Makes bufio conform to Sync() so the logger can flush it after each line.
type bufioSyncer struct{ *bufio.Writer }

func (b *bufioSyncer) Sync() error { return b.Flush() }

func fatalIfError(logger *ui.UI, err error) {
	if err != nil {
		logger.Task("hermit").Fatalf("%s", err)
	}
}
