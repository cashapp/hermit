+++
title = "Environment variables"
weight = 102
+++

When a Hermit environment is active, environment variables defined by Hermit itself,
installed packages, and the active environment will be set.

Hermit includes a `hermit env` command to view and set environment variables:

```text
Usage: hermit env [<name>] [<value>]

Manage environment variables.

Arguments:
  [<name>]     Name of the environment variable.
  [<value>]    Value to set the variable to.

Flags:
  -r, --raw           Output raw values without shell quoting.
  -i, --inherit       Inherit variables from parent environment.
  -n, --names         Show only names.
  -u, --unset         Unset the specified environment variable.
```

## Hermit

Hermit itself requires several environment variables to operate correctly. An
empty environment might look something like the following:

```text
project🐚~/project$ hermit env
HERMIT_BIN=/home/user/project/bin
HERMIT_ENV=/home/user/project
PATH=/home/user/project/bin:/home/user/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin:/opt/local/bin
```

## Package

Packages may export environment variables for convenience or in order to
operate correctly. For example, the `go` package sets the `GOROOT` to the
location of the installed Go SDK:

```text
project🐚~/project$ hermit install go
project🐚~/project$ hermit env       
GOROOT=/home/user/.cache/hermit/pkg/go-1.16
...
```

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

{{< tip "warning" >}}

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
{{< /tip >}}

