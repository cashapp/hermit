---
name: hermit
description: Use when working in repositories that contain ./bin/activate-hermit. Covers detecting Hermit environments with that definitive check, activating Hermit before running commands, preferring Hermit-managed tools over global tools, and contributing missing packages to hermit-packages.
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

## Modify Toolchains

Use Hermit commands to change tools in a Hermit-managed repository:

```sh
./bin/hermit install <package>
./bin/hermit uninstall <package>
```

After changing packages, run the repository's tests through the Hermit
environment.

## Configure Environment Variables

Use `hermit env` to set persistent environment variables for the Hermit
environment:

```sh
./bin/hermit env <name> <value>
```

After changing environment variables, run the repository's tests through the
Hermit environment.

## Contribute Missing Packages

If a required tool is not available from Hermit, contribute it to
`cashapp/hermit-packages` when it is generally useful. Follow the packaging
tutorial at [cashapp.github.io/hermit/packaging/tutorial](https://cashapp.github.io/hermit/packaging/tutorial/).

1. Clone `https://github.com/cashapp/hermit-packages`.
2. Activate that repository with `. ./bin/activate-hermit`.
3. Add or update the package manifest.
4. Run `hermit add-digests <manifest>.hcl` when download digests are needed.
5. Run `hermit test <package> --trace`.
6. Open a pull request to `cashapp/hermit-packages`.

For project-private tools, prefer adding a private package source in
`bin/hermit.hcl` instead of depending on global machine state.
