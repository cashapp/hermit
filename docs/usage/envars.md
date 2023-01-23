+++
title = "Environment variables"
weight = 102
+++

When a Hermit environment is active, environment variables will be set by
[Hermit](#hermit) itself, the [command-line](#command-line), the
[active environment](#active-environment), and
[installed packages](#installed-packages), in that order.

## Hermit

Hermit prefixes all of its own variables with `HERMIT_` or `_HERMIT_`. While
it uses a bunch of variables internally, two you can rely on to always be
present in an active environment are:

| Name | Description |
|-----------|------|-------------|
| `HERMIT_ENV` | Path to the active Hermit environment. |
| `HERMIT_BIN` | Path to the active Hermit environment `bin` directory. |

An empty environment might look something like the following:

```text
project🐚~/project$ hermit env
HERMIT_BIN=/home/user/project/bin
HERMIT_ENV=/home/user/project
PATH=/home/user/project/bin:/home/user/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin:/opt/local/bin
```

## Command-line

Use the flag `--env=NAME=value` to set per-invocation environment variables.

## Active Environment

The active environment may define additional environment variables in
`bin/hermit.hcl`. These can be managed with the `hermit env` command, or by
directly editing the configuration file.

For example, to set `GOBIN` to a `build` directory within the environment:

```text
project🐚~/project$ hermit env GOBIN '${HERMIT_ENV}/build'
project🐚~/project$ hermit env       
GOBIN=/home/user/project/build
GOROOT=/home/user/.cache/hermit/pkg/go-1.16
...
project🐚~/project$ echo $GOBIN
/home/user/project/build
```

The `bin/hermit.hcl` file will contain:

```hcl
# Extra environment variables.
env = {
  "GOBIN": "${HERMIT_ENV}/build",
}
```

Use the `hermit env` command to view and set per-environment variables:

```text
Usage: hermit env [<name>] [<value>]

Manage environment variables.

Without arguments the "env" command will display environment variables for the
active Hermit environment.

Passing "<name>" will print the value for that environment variable.

Passing "<name> <value>" will set the value for an environment variable in the
active Hermit environment."

Arguments:
  [<name>]     Name of the environment variable.
  [<value>]    Value to set the variable to.

Flags:
  -r, --raw           Output raw values without shell quoting.
      --activate      Prints the commands needed to set the environment to the
                      activated state
      --deactivate    Prints the commands needed to reset the environment to the
                      deactivated state
  -i, --inherit       Inherit variables from parent environment.
  -n, --names         Show only names.
  -u, --unset         Unset the specified environment variable.
```


{{< hint warning >}}

Take care to _only_ use single quotes (`'`) when setting values so that the shell
doesn't interpolate environment variables before Hermit. ie. **Do not do this**:

```text
project🐚~/project$ hermit env GOBIN "${HERMIT_ENV}/build"
```

as it will result in this `bin/hermit.hcl`:

```hcl
# Extra environment variables.
env = {
  "GOBIN": "/home/user/project/build",
}
```

This will of course work fine for the local user, but will fail tragically for anyone else.
{{< /hint >}}

## Installed Packages

Packages may export environment variables for convenience or in order to
operate correctly. For example, the `go` package sets the `GOROOT` to the
location of the installed Go SDK:

```text
project🐚~/project$ hermit install go
project🐚~/project$ hermit env
GOROOT=/home/user/.cache/hermit/pkg/go-1.16
...
```


