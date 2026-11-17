# Get Started

This document gives a brief introduction to installing Hermit and using a
newly created environment.

## Installing Hermit

Installing Hermit is straightforward:

```shell
curl -fsSL https://github.com/cashapp/hermit/releases/download/stable/install.sh | /bin/bash
```

This will download and install `hermit` into `~/bin`. You should add this to your `$PATH` if it isn't already.


!!! note

    Also consider installing the [shell hooks](../shell) to get automatic
    environment activation/deactivation when changing directories.




## Initialising a Project

Change into a project directory and run the following:

```shell
~$ cd ~/project
~/project$ hermit init
info: Creating new Hermit environment in /home/user/project
...

```

At this point the Hermit environment should be initialised and the `./bin`
directory should contain something like the following:

```shell
README.hermit.md
activate-hermit
hermit
hermit.hcl
```

### Migrating from asdf or mise

If the directory contains a `.tool-versions` file, `hermit init` installs the
tools it pins into the new environment:

```text
nodejs 20.11.0
golang 1.22.1
python 3.12.2 3.11.8
java temurin-17.0.2+8
```

Each line is mapped to a Hermit package:

* Common asdf plugin names are translated (`nodejs` to `node`, `golang` to
  `go`, `python` to `python3`, `dotnet-core` to `dotnet`, `aws-cli` to
  `awscli`). Other names are used as-is.
* An exact version is used if Hermit has it, otherwise the newest version
  starting with it (so `20.11` selects the latest `20.11.x`). `latest` and
  `latest:<prefix>` are also supported.
* If a line lists several versions, the first one Hermit can resolve is
  installed, since a Hermit environment has one version of each package.
* Java versions with a `temurin-`, `adoptopenjdk-` or `openjdk-` prefix map
  to `openjdk`, and `corretto-` maps to `corretto`.

Lines that can't be mapped, such as `system`, `ref:` or `path:` versions, are
skipped with a warning. Pass `--no-tool-versions` to ignore the file.

## Activating an Environment

Activating an environment will add its `bin` directory to your `$PATH`, as
well as setting any [environment variables](../envars) managed by Hermit.

To activate a Hermit environment source the `activate-hermit` script:

```shell
~/project$ . ./bin/activate-hermit
Hermit environment /home/user/project activated
project🐚~/project$
```

Once activated the shell prompt will change to include the prefix `<environment>🐚`.

At this point you can [use and manage](../management) packages in this environment.

## Searching for packages

Once your environment is activated, use `hermit search` to search for
packages. Without arguments it will list all known packages, or you may pass
a term to search in package names and descriptions.

```shell
project🐚~/project$ hermit search openjdk
openjdk (11.0.11_9-zulu11.48.21, @stable, @zulu, 11.0.8_10, 11.0.9_11, 11.0.10_9)
  Java is a class-based, object-oriented programming language.
```

## Installing a package

After you've selected the package you'd like to install into your environment,
use `hermit install` to install packages:

```shell
project🐚~/project$ hermit install go-1.16.3
project🐚~/project$ go version
go version go1.16.3 darwin/amd64
```

Refer to the [package management](../management) documentation for more
details, including how to uninstall, information about channels, etc.

## Deactivating an Environment

When an environment is activated, Hermit inserts a shell function
`deactivate-hermit`. Call this to deactivate the current environment.

```shell
project🐚~/project$ deactivate-hermit
Hermit environment /home/user/project deactivated
~/project$
```