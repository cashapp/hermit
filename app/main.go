package app

import (
	"bufio"
	"log"
	"net"
	"net/http"
	"os"
	"runtime"
	"runtime/pprof"
	"strings"
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

// ScriptSHAs contains the default known valid SHA256 sums for bin/activate-hermit and bin/hermit.
var ScriptSHAs = []string{
	"020657425d0ba9f42fd3536d88fb3e80e85eaeae2daa1f7005b0b48dc270a084",
	"3ec9e59a260a2befeb83a94952337dddcac1fb4f7dcc1200a2964bfb336f26c3",
	"5ba24eaadfe620ad7c78a5c1f860d9845bc20077a3f9c766936485d912b75b60",
	"60c8e1787b16b6bd02c0cf6562671b0f60fb8d867b6d5140afd96bd2521e2f68",
	"6e1e6a687dc1f43c8187fb6c11b2a3ad1b1cfc93cda0b5ef307710dcfafa0dd4",
	"7a2b479e582d39826ef3e47d9930c7e8ff21275fba53efdc8204fe160742b56c",
	"04f065a430d1d99bc99f19e82a6465ab6823467d9c6b5ec3f751befa7a3b30a8",
	"57697ee9f19658d1872fc5877e2a38ba132a2df85e4416802a4c33968e00c716",
	"75abcf121df40b25cd0c7bab908c43dbf536bc6f4552a2f6e825ac90c8fff994",
	"7c64aa474afa3202305953e9b2ac96852f4bf65ddb417dee2cfa20ad58986834",
	"b42be79b29ac118ba05b8f5b6bd46faa2232db945453b1b10afc1a6e031ca068",
	"c419082d4cf1e2e9ac33382089c64b532c88d2399bae8b07c414b1d205bea74e",
	"d575eda7d5d988f6f3c233ceaa42fae61f819d863145aec7a58f4f1519db31ad",
	"ec14f88a38560d4524a8679f36fdfb2fb46ccd13bc399c3cddf3ca9f441952ec",
}

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
	config.LogLevel = ui.AutoLevel(config.LogLevel)
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

	if len(config.SHA256Sums) == 0 {
		config.SHA256Sums = ScriptSHAs
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
		cli = &activated{unactivated: unactivated{Plugins: config.KongPlugins}}
	} else {
		envPath, err = os.Getwd()
		if err != nil {
			log.Fatalf("couldn't get working directory: %s", err)
		}
		cli = &unactivated{Plugins: config.KongPlugins}
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

	hermitHelp := help
	hermitHelp += "\n\nConfiguration format for ~/.hermit.hcl:\n"
	hermitHelp += "    " + strings.Join(strings.Split(userConfigSchema, "\n"), "\n    ")
	hermitHelp += "\nGITHUB_TOKEN can be set to retrieve private GitHub release assets."

	kongOptions := []kong.Option{
		kong.Groups{
			"env":    "Environment:\nCommands for creating and managing environments.",
			"global": "Global:\nCommands for interacting with the shared global Hermit state.",
		},
		kong.Resolvers(UserConfigResolver(userConfig)),
		kong.UsageOnError(),
		kong.Description(hermitHelp),
		kong.BindTo(cli, (*cliCommon)(nil)),
		kong.Bind(userConfig, config),
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
	if githubToken != "" {
		getSource = cache.GitHubSourceSelector(getSource, ghClient)
	}

	cache, err := cache.Open(hermit.UserStateDir, getSource, defaultHTTPClient, config.fastHTTPClient(p))
	if err != nil {
		log.Fatalf("failed to open cache: %s", err)
	}
	sta, err = state.Open(hermit.UserStateDir, config.State, cache)
	if err != nil {
		log.Fatalf("failed to open state: %s", err)
	}

	if isActivated {
		env, err = hermit.OpenEnv(envPath, sta, cache.GetSource, cli.getGlobalState().Env, defaultHTTPClient)
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
	)

	ctx, err := parser.Parse(os.Args[1:])
	parser.FatalIfErrorf(err)
	configureLogging(cli, ctx.Command(), p)

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
