---
title: "Configuration"
---

Each Hermit environment contains a `bin/hermit.hcl` file that can be used to customise that Hermit environment.

```hcl
// Extra environment variables to be added to the Hermit environment.
//
// Other variables can be expanded, allowing you to prepend/append to
// existing variables, eg. "$PATH:${HERMIT_ENV}/scripts". To prevent
// undesired expansion, escape using $$, eg. "TEMPLATE": "foo-$${bar}".
//
// These values are managed by the `hermit env` command.
env = {
  "ENVAR": "VALUE",
}

// Hermit supports three different manifest sources:
//
// 1. Git repositories; any cloneable URI ending with `.git`.
//    eg. `https://github.com/cashapp/hermit-packages.git`.
//    An optional `#<tag>` suffix can be added to checkout a specific tag.
// 2. Local filesystem, eg. `file:///home/user/my-packages`.
//    This is mostly only useful for local development and testing.
// 3. Environment relative, eg. `env:///my-packages`.
//    This will search for package manifests in the directory `${HERMIT_ENV}/my-packages`.
//    Useful for local overrides.
sources = ["SOURCE"]

// Whether Hermit should automatically add/remove files from Git.
manage-git = false

// Whether this Hermit environment should inherit an environment from a parent directory.
inherit-parent = false

// Configures when to use GitHub token authentication.
github-token-auth {
  // A list of globs to match against GitHub repositories.
  match = ["ORG/REPO", "ORG/*"]
}

// Configure a GitHub Enterprise Cloud data residency host.
github-token-auth {
  host = "mycompany.ghe.com"
  token-env = "HERMIT_GITHUB_TOKEN_MYCOMPANY_GHE_COM"
  match = ["ORG/REPO", "ORG/*"]
}
```

## Attributes

| Attribute          | Type               | Description                                                                                          |
|------------------|--------------------|------------------------------------------------------------------------------------------------------|
| `env`            | `{string:string}?` | Extra environment variables.                                                                         |
| `sources`        | `[string]?`        | Package manifest sources in order of preference.                                                     |
| `manage-git`     | `bool?`            | Whether Hermit should manage Git.                                                                    |
| `inherit-parent` | `bool?`            | Whether this Hermit environment should inherit an environment from a parent directory.             |
| `github-token-auth` | `[GitHubTokenAuthConfig]?` | When to use GitHub token authentication. |
| `idea`           | `bool?`            | Whether Hermit should automatically add the IntelliJ IDEA plugin. |

### GitHubTokenAuthConfig

| Attribute | Type     | Description                                                                                                           |
|-----------|----------|-----------------------------------------------------------------------------------------------------------------------|
| `host` | `string?` | GitHub web host for matched repositories. Defaults to `github.com`. For GitHub Enterprise Cloud with data residency this is typically `<subdomain>.ghe.com`. |
| `token-env` | `string?` | Environment variable containing the token for this host. Defaults to `HERMIT_GITHUB_TOKEN`/`GITHUB_TOKEN` for `github.com`. Non-`github.com` hosts use `gh auth token -h <host>` if this is unset or empty. |
| `match` | `[string]?` | One or more glob patterns. If any of these match the 'owner/repo' pair of a GitHub repository on the configured host, that host's GitHub token from the current environment will be used to fetch their artifacts. |

## Per-environment Sources

Hermit supports three different manifest sources:

1. Git repositories; any cloneable URI ending with `.git`, eg. `https://github.com/cashapp/hermit-packages.git`. An optional `#<tag>` suffix can be added to checkout a specific tag.
2. Local filesystem, eg. `file:///home/user/my-packages`. This is mostly only useful for local development and testing.
3. Environment relative, eg. `env:///my-packages`. This will search for package manifests in the directory `${HERMIT_ENV}/my-packages`. Useful for local overrides.
