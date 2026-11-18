---
name: hermit
description: Use when working in repositories that contain ./bin/activate-hermit. Covers detecting Hermit environments with that definitive check, activating Hermit before running commands, preferring Hermit-managed tools over global tools, and managing packages and environment variables with Hermit commands.
---

# Hermit

Use Hermit-managed tools whenever a repository contains a Hermit activation
script.

## Detect Hermit

Treat a repository as Hermit-managed when it contains `bin/activate-hermit`.
Related files commonly include `bin/hermit`, `bin/hermit.hcl`, and
`bin/README.hermit.md`.

Before running project commands, check for Hermit from the repository root:

```sh
test -f ./bin/activate-hermit
```

## Run Commands

Prefer the repository's Hermit environment over globally installed tools.
For agent command runners and other non-interactive POSIX shells, apply the
Hermit environment in the same shell invocation as the command:

```sh
eval "$(./bin/hermit env --activate)" && go test ./...
```

For interactive POSIX shells, source the activation script:

```sh
. ./bin/activate-hermit && go test ./...
```

For fish shells, use:

```fish
source ./bin/activate-hermit.fish; go test ./...
```

Do not install missing tools with another package manager until checking
whether the project expects Hermit to provide them. If a tool is unavailable,
run `hermit search <tool>` from the activated environment.

When automation needs environment variables without sourcing shell hooks, use:

```sh
./bin/hermit env --raw
```

## Manage Packages and Environment Variables

Always use Hermit commands to manage packages and environment variables. Never
create, edit, rename, or delete files in `bin/` by hand for these tasks. This
includes `.<package>-<version>.pkg` markers, binary symlinks, and the `env`
block in `bin/hermit.hcl`. Hermit keeps these files consistent with each other,
and manual edits can leave the environment broken or out of sync.

## Modify Toolchains

Use Hermit commands to change tools in a Hermit-managed repository:

```sh
./bin/hermit install <package>
./bin/hermit install <package>-<version>
./bin/hermit upgrade <package>
./bin/hermit uninstall <package>
```

Do not install packages by channel, such as `<package>@latest` or
`<package>@nightly`, unless the user explicitly requests a channel. Channels
update automatically and make the environment non-reproducible. Install a bare
package name or a specific `<package>-<version>` instead. If `hermit search`
shows that a package provides only channels, ask the user before installing it.

After changing packages, run the repository's tests through the Hermit
environment.

## Configure Environment Variables

Use `hermit env` to set or unset persistent environment variables for the
Hermit environment:

```sh
./bin/hermit env <name> <value>
./bin/hermit env --unset <name>
```

After changing environment variables, run the repository's tests through the
Hermit environment.
