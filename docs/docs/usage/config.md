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

// Configures when to use GitHub token authentication from $GITHUB_TOKEN.
github-token-auth {
  // A list of globs to match against GitHub repositories.
  match = ["ORG/REPO", "ORG/*"]
}

// Git repositories providing agent skills to link into the environment
// on activation. Skills are resolved to immutable snapshots under the
// Hermit state directory and symlinked into `.agents/skills/` and
// `.claude/skills/`. Add both directories to .gitignore.
skill-repo "https://github.com/ORG/REPO.git" {
  // Subdirectory within the repository containing the skill directories.
  path = "skills"
  // Names of the skill directories to link into the environment.
  skills = ["SKILL"]
  // Optional full commit SHA to pin to. When omitted the remote HEAD is
  // used, re-checked at most every 15 minutes.
  // ref = "0123456789abcdef0123456789abcdef01234567"
}
```

## Attributes

| Attribute          | Type               | Description                                                                                          |
|------------------|--------------------|------------------------------------------------------------------------------------------------------|
| `env`            | `{string:string}?` | Extra environment variables.                                                                         |
| `sources`        | `[string]?`        | Package manifest sources in order of preference.                                                     |
| `manage-git`     | `bool?`            | Whether Hermit should manage Git.                                                                    |
| `inherit-parent` | `bool?`            | Whether this Hermit environment should inherit an environment from a parent directory.             |
| `github-token-auth` | `GitHubTokenAuthConfig?` | When to use GitHub token authentication. |
| `idea`           | `bool?`            | Whether Hermit should automatically add the IntelliJ IDEA plugin. |
| `skill-repo`     | `[SkillRepo]?`     | Git repositories providing agent skills to link into the environment on activation. |

### GitHubTokenAuthConfig

| Attribute | Type     | Description                                                                                                           |
|-----------|----------|-----------------------------------------------------------------------------------------------------------------------|
| match     | `[string]?` | One or more glob patterns. If any of these match the 'owner/repo' pair of a GitHub repository, the GitHub token from the current environment will be used to fetch their artifacts. |

### SkillRepo

| Attribute | Type        | Description                                                                                             |
|-----------|-------------|---------------------------------------------------------------------------------------------------------|
| `url`     | `string`    | Git repository URL providing agent skills (block label).                                                |
| `path`    | `string?`   | Subdirectory within the repository containing the skill directories.                                   |
| `skills`  | `[string]`  | Names of the skill directories to link into the environment. Each must contain a `SKILL.md`.           |
| `ref`     | `string?`   | Full commit SHA to pin to. When omitted the remote HEAD is used, re-checked at most every 15 minutes.  |

## Agent Skills

Environments can declare agent skills — `SKILL.md` directories used by AI
coding agents — that Hermit links into the project on activation, the same
way it provides toolchains.
On activation Hermit resolves each `skill-repo` to a commit, materialises an
immutable snapshot of each declared skill under the Hermit state directory,
and symlinks it into `.agents/skills/<name>` and `.claude/skills/<name>`.

Skill content is never written into the project itself and must not be
committed: add `.agents/skills/` and `.claude/skills/` to `.gitignore`.
A skill directory committed directly to the repository always takes
precedence over a Hermit-managed one with the same name.

When offline, activation falls back to the last good snapshot. Removing a
skill from the configuration removes its links on the next activation.

## Per-environment Sources

Hermit supports three different manifest sources:

1. Git repositories; any cloneable URI ending with `.git`, eg. `https://github.com/cashapp/hermit-packages.git`. An optional `#<tag>` suffix can be added to checkout a specific tag.
2. Local filesystem, eg. `file:///home/user/my-packages`. This is mostly only useful for local development and testing.
3. Environment relative, eg. `env:///my-packages`. This will search for package manifests in the directory `${HERMIT_ENV}/my-packages`. Useful for local overrides.
