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

	"github.com/square/exit"

	"github.com/alecthomas/kong"
	"github.com/mattn/go-isatty"
	"github.com/posener/complete"
	"github.com/willabides/kongplete"

	"github.com/cashapp/hermit"
	"github.com/cashapp/hermit/cache"
	"github.com/cashapp/hermit/github"
	"github.com/cashapp/hermit/redact"
	"github.com/cashapp/hermit/sources"
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
	// URL for Cachew proxy server. If set, all HTTP/HTTPS downloads will be proxied through Cachew.
	CachewURL string
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
		p = ui.New(config.LogLevel, &bufioSyncer{stdout}, &bufioSyncer{stderr}, stdoutIsTTY, stderrIsTTY)
		go func() {
			for {
				time.Sleep(time.Millisecond * 100)
				if err := p.Sync(); err != nil {
					break
				}
			}
		}()
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
		log.Fatalf("couldn't get working directory: %s", err) //nolint: gocritic
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

	githubToken, githubTokenSource, err := githubTokenForHost(hermit.GitHubTokenAuthConfig{Host: "github.com"})
	if err != nil {
		log.Fatalf("failed to retrieve GitHub token: %s", err)
	}
	if githubTokenSource != "" {
		p.Tracef("GitHub token for github.com set from %s", githubTokenSource)
	}

	kongOptions := []kong.Option{
		kong.Groups{
			"env":    "Environment:\nCommands for creating and managing environments.",
			"global": "Global:\nCommands for interacting with the shared global Hermit state.",
		},
		kong.UsageOnError(),
		kong.Description(help),
		kong.BindTo(cli, (*cliInterface)(nil)),
		kong.Bind(config),
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

	var envInfo *hermit.EnvInfo
	if isActivated {
		envInfo, err = hermit.LoadEnvInfo(envPath)
		if err != nil {
			log.Fatalf("failed to load environment info: %s", err)
		}
	}

	getSource := config.PackageSourceSelector
	if config.PackageSourceSelector == nil {
		getSource = cache.GetSource
	}
	defaultHTTPClient := config.defaultHTTPClient(p)
	githubAuths, err := configuredGitHubAuths(p, envInfo)
	if err != nil {
		log.Fatalf("Environment configuration has a bad github-auth-token.match: %v", err)
	}
	githubHosts := make([]github.HostConfig, 0, len(githubAuths)+1)
	githubHosts = append(githubHosts, github.HostConfig{WebHost: "github.com", Token: githubToken})
	for _, auth := range githubAuths {
		githubHosts = append(githubHosts, auth.host)
	}
	ghClient := github.NewWithHosts(defaultHTTPClient, githubHosts)

	var githubHostMatchers []cache.GitHubHostMatcher
	for _, auth := range githubAuths {
		githubHostMatchers = append(githubHostMatchers, cache.GitHubHostMatcher{Host: auth.host.WebHost, Match: auth.matcher})
	}
	getSource = cache.GitHubSourceSelectorForHosts(getSource, ghClient, githubHostMatchers)

	// Add Cachew source selector if configured
	if config.CachewURL != "" {
		getSource = cache.CachewSourceSelector(getSource, config.CachewURL)
	}

	cache, err := cache.Open(hermit.UserStateDir, getSource, defaultHTTPClient, config.fastHTTPClient(p))
	if err != nil {
		log.Fatalf("failed to open cache: %s", err)
	}

	ctx, err := parser.Parse(os.Args[1:])
	parser.FatalIfErrorf(err)
	configureLogging(cli, p)

	userConfig := NewUserConfigWithDefaults()
	userConfigPath := cli.getUserConfigFile()

	if IsUserConfigExists(userConfigPath) {
		p.Tracef("Loading user config from: %s", userConfigPath)
		userConfig, err = LoadUserConfig(userConfigPath)
		if err != nil {
			log.Printf("%s: %s", userConfigPath, err)
		}
	} else {
		p.Tracef("No user config found at: %s", userConfigPath)
	}

	config.State.LockTimeout = cli.getLockTimeout()
	sta, err = state.Open(hermit.UserStateDir, config.State, cache)
	if err != nil {
		log.Fatalf("failed to open state: %s", err)
	}

	var sourceRewriters []sources.URLRewriter
	if isActivated {
		for _, auth := range githubAuths {
			sourceRewriters = append(sourceRewriters, github.AuthenticatedURLRewriter(auth.host.WebHost, auth.host.Token, auth.matcher))
		}

		env, err = hermit.OpenEnv(envInfo, sta, cache.GetSource, cli.getGlobalState().Env, defaultHTTPClient, config.SHA256Sums, sourceRewriters...)
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
		fatalIfError(p, ctx, err)
		defer f.Close()
		err = pprof.StartCPUProfile(f)
		fatalIfError(p, ctx, err)
		defer pprof.StopCPUProfile()

	}
	if pprofPath := cli.getMemProfile(); pprofPath != "" {
		f, err := os.Create(pprofPath)
		fatalIfError(p, ctx, err)
		defer f.Close()
		defer func() {
			runtime.GC() // get up-to-date statistics
			err = pprof.Lookup("allocs").WriteTo(f, 0)
			fatalIfError(p, ctx, err)
		}()
	}
	err = ctx.Run(env, p, sta, config, cli.getGlobalState(), ghClient, defaultHTTPClient, cache, userConfig, sourceRewriters)
	fatalIfError(p, ctx, err)
}

func configureLogging(cli cliInterface, p *ui.UI) {
	switch {
	case cli.getTrace():
		p.SetLevel(ui.LevelTrace)
	case cli.getDebug():
		p.SetLevel(ui.LevelDebug)
	case cli.getQuiet():
		p.SetLevel(ui.LevelFatal)
	default:
		p.SetLevel(cli.getLevel())
	}

	if cli.getQuiet() {
		p.SetProgressBarEnabled(false)
	}
}

// Makes bufio conform to Sync() so the logger can flush it after each line.
type bufioSyncer struct{ *bufio.Writer }

func (b *bufioSyncer) Sync() error { return b.Flush() }

func fatalIfError(logger *ui.UI, ctx *kong.Context, err error) {
	if err != nil {
		logger.Task("hermit").Fatalf("%s", err)
		ctx.Exit(exit.FromError(err))
	}
}

type gitHubAuth struct {
	host    github.HostConfig
	matcher github.RepoMatcher
}

func configuredGitHubAuths(p *ui.UI, envInfo *hermit.EnvInfo) ([]gitHubAuth, error) {
	if envInfo == nil {
		return nil, nil
	}

	var auths []gitHubAuth
	for _, ghTokenAuth := range envInfo.Config.GitHubTokenAuth {
		if len(ghTokenAuth.Match) == 0 {
			continue
		}

		matcher, err := github.GlobRepoMatcher(ghTokenAuth.Match)
		if err != nil {
			return nil, err
		}

		token, source, err := githubTokenForHost(ghTokenAuth)
		if err != nil {
			return nil, err
		}
		host := github.HostConfig{
			WebHost: ghTokenAuth.Host,
			Token:   token,
		}
		host.WebHost = github.NormalizeHost(host.WebHost)
		if source != "" {
			p.Tracef("GitHub token for %s set from %s", host.WebHost, source)
		}
		auths = append(auths, gitHubAuth{host: host, matcher: matcher})
	}
	return auths, nil
}

var githubTokenFromCLI = github.TokenFromCLI

func githubTokenForHost(config hermit.GitHubTokenAuthConfig) (redact.Secret, string, error) {
	if config.TokenEnv != "" {
		if token := os.Getenv(config.TokenEnv); token != "" {
			return redact.Secret(token), config.TokenEnv, nil
		}
	}

	if host := github.NormalizeHost(config.Host); host != "github.com" {
		token, err := githubTokenFromCLI(host)
		return token, "gh auth token", err
	}
	for _, candidate := range []string{"HERMIT_GITHUB_TOKEN", "GITHUB_TOKEN"} {
		if token := os.Getenv(candidate); token != "" {
			return redact.Secret(token), candidate, nil
		}
	}
	return "", "", nil
}
