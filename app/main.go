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
	"github.com/mattn/go-isatty"
	"github.com/posener/complete"
	"github.com/willabides/kongplete"

	"github.com/cashapp/hermit"
	"github.com/cashapp/hermit/cache"
	"github.com/cashapp/hermit/github"
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
	// Possible system-wide installation paths
	InstallPaths []string
	// SHA256 checksums for all known versions of per-environment scripts.
	// If empty shell.ScriptSHAs will be used.
	SHA256Sums  []string
	HTTP        func(HTTPTransportConfig) *http.Client
	State       state.Config
	KongOptions []kong.Option
	KongPlugins kong.Plugins
	// Defaults to cache.GetSource if nil.
	PackageSourceSelector cache.PackageSourceSelector
	// True if we're running in CI - disables progress bar.
	CI bool
	// True if you want hermit to check digests for every package.
	RequireDigests bool
}

type loggingHTTPTransport struct {
	logger ui.Logger
	next   http.RoundTripper
}

func (l *loggingHTTPTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	l.logger.Tracef("%s %s", r.Method, r.URL)
	return l.next.RoundTrip(r)
}

// Make a HTTP client.
func (c Config) makeHTTPClient(logger ui.Logger, config HTTPTransportConfig) *http.Client {
	client := c.HTTP(config)
	if debug.Flags.FailHTTP {
		client.Timeout = time.Millisecond
	}
	client.Transport = &loggingHTTPTransport{logger, client.Transport}
	return client
}

// Make a HTTP client with very short timeouts for issuing optional requests.
func (c Config) fastHTTPClient(logger ui.Logger) *http.Client {
	return c.makeHTTPClient(logger, HTTPTransportConfig{
		ResponseHeaderTimeout: time.Second * 5,
		DialTimeout:           time.Second,
		KeepAlive:             30 * time.Second,
	})
}

func (c Config) defaultHTTPClient(logger ui.Logger) *http.Client {
	return c.makeHTTPClient(logger, HTTPTransportConfig{})
}

// Main runs the Hermit command-line application with the given config.
func Main(config Config) {
	if len(config.InstallPaths) == 0 {
		config.InstallPaths = []string{
			"${HOME}/bin",
			"/opt/homebrew/bin",
			"/usr/local/bin",
		}
	}
	config.LogLevel = ui.AutoLevel(config.LogLevel)
	if config.HTTP == nil {
		config.HTTP = func(config HTTPTransportConfig) *http.Client {
			transport := &http.Transport{
				ResponseHeaderTimeout: config.ResponseHeaderTimeout,
				Proxy:                 http.ProxyFromEnvironment,
				DialContext: (&net.Dialer{
					Timeout:   config.DialTimeout,
					KeepAlive: config.KeepAlive,
				}).DialContext,
			}
			return &http.Client{Transport: transport}
		}
	}

	if len(config.SHA256Sums) == 0 {
		config.SHA256Sums = hermit.ScriptSHAs
	}
	var (
		err         error
		p           *ui.UI
		stdoutIsTTY = isatty.IsTerminal(os.Stdout.Fd())
		stderrIsTTY = isatty.IsTerminal(os.Stderr.Fd())
	)
	if stdoutIsTTY {
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
	p.SetProgressBarEnabled(!config.CI)
	defer func() {
		err := recover()
		p.Clear()
		if err != nil {
			panic(err)
		}
	}()

	var (
		cli cliInterface
		env *hermit.Env
		sta *state.State
	)
	// By default, we assume Hermit will run in an unactivated state
	isActivated := false
	envPath, err := os.Getwd()
	if err != nil {
		log.Fatalf("couldn't get working directory: %s", err) // nolint: gocritic
	}
	common := cliBase{Plugins: config.KongPlugins}

	// But we activate any environment we find
	if envDir, err := hermit.FindEnvDir(os.Args[0]); err == nil {
		envPath = envDir
		isActivated = true
		cli = &activated{cliBase: common}
	} else {
		cli = &unactivated{cliBase: common}
	}

	userConfig, err := LoadUserConfig()
	if err != nil {
		log.Printf("%s: %s", userConfigPath, err)
	}

	githubToken := os.Getenv("HERMIT_GITHUB_TOKEN")
	if githubToken == "" {
		githubToken = os.Getenv("GITHUB_TOKEN")
		p.Tracef("GitHub token set from GITHUB_TOKEN")
	} else {
		p.Tracef("GitHub token set from HERMIT_GITHUB_TOKEN")
	}

	kongOptions := []kong.Option{
		kong.Groups{
			"env":    "Environment:\nCommands for creating and managing environments.",
			"global": "Global:\nCommands for interacting with the shared global Hermit state.",
		},
		kong.Resolvers(UserConfigResolver(userConfig)),
		kong.UsageOnError(),
		kong.Description(help),
		kong.BindTo(cli, (*cliInterface)(nil)),
		kong.Bind(userConfig, config),
		kong.AutoGroup(func(parent kong.Visitable, flag *kong.Flag) *kong.Group {
			node, ok := parent.(*kong.Command)
			if !ok {
				return nil
			}
			return &kong.Group{
				Key:   node.Name,
				Title: "Command flags:",
			}
		}),
		kong.Vars{
			"version": config.Version,
			"env":     envPath,
		},
		kong.HelpOptions{
			Compact: true,
		},
	}
	kongOptions = append(kongOptions, config.KongOptions...)

	parser, err := kong.New(cli, kongOptions...)
	if err != nil {
		log.Fatalf("failed to initialise CLI: %s", err)
	}

	getSource := config.PackageSourceSelector
	if config.PackageSourceSelector == nil {
		getSource = cache.GetSource
	}
	defaultHTTPClient := config.defaultHTTPClient(p)

	ghClient := github.New(defaultHTTPClient, githubToken)
	cache, err := cache.Open(hermit.UserStateDir, getSource, defaultHTTPClient, config.fastHTTPClient(p))
	if err != nil {
		log.Fatalf("failed to open cache: %s", err)
	}

	ctx, err := parser.Parse(os.Args[1:])
	parser.FatalIfErrorf(err)
	configureLogging(cli, ctx.Command(), p)

	config.State.LockTimeout = cli.getLockTimeout()
	sta, err = state.Open(hermit.UserStateDir, config.State, cache)
	if err != nil {
		log.Fatalf("failed to open state: %s", err)
	}

	if isActivated {
		env, err = hermit.OpenEnv(envPath, sta, cache.GetSource, cli.getGlobalState().Env, defaultHTTPClient, config.SHA256Sums)
		if err != nil {
			log.Fatalf("failed to open environment: %s", err)
		}
	}

	packagePredictor := hermit.NewPackagePredictor(sta, env, p)
	installedPredictor := hermit.NewInstalledPackagePredictor(env, p)
	kongplete.Complete(parser,
		kongplete.WithPredictor("package", packagePredictor),
		kongplete.WithPredictor("installed-package", installedPredictor),
		kongplete.WithPredictor("dir", complete.PredictDirs("*")),
		kongplete.WithPredictor("hclfile", complete.PredictFiles("*.hcl")),
		kongplete.WithPredictor("file", complete.PredictFiles("*")),
	)

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
	err = ctx.Run(env, p, sta, config, cli.getGlobalState(), ghClient, defaultHTTPClient, cache)
	if err != nil && p.WillLog(ui.LevelDebug) {
		p.Fatalf("%+v", err)
	} else {
		fatalIfError(p, err)
	}
}

func configureLogging(cli cliInterface, cmd string, p *ui.UI) {
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
