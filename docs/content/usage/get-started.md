+++
title = "Get Started"
weight = 101
+++

This document gives a brief introduction to installing Hermit and using a
newly created environment.

## Installing Hermit

Installing Hermit is straightforward:

```text
curl -fsSL https://github.com/cashapp/hermit/releases/download/stable/install.sh | /bin/bash
```

This will download and install `hermit` into `~/bin`. You should add this to your `$PATH` if it isn't already.

{{< hint ok >}}

Also consider installing the [shell hooks](../shell) to get automatic
environment activation/deactivation when changing directories.

{{< /hint >}}

## Initialising a Project

Change into a project directory and run the following:

```text
~$ cd ~/project
~/project$ hermit init
info: Creating new Hermit environment in /home/user/project
...

```

At this point the Hermit environment should be initialised and the `./bin`
directory should contain something like the following:

```text
README.hermit.md
activate-hermit
hermit
hermit.hcl
```

## Activating an Environment

Activating an environment will add its `bin` directory to your `$PATH`, as
well as setting any [environment variables](../envars) managed by Hermit.

To activate a Hermit environment source the `activate-hermit` script:

```text
~/project$ . ./bin/activate-hermit
Hermit environment /home/user/project activated
project🐚~/project$
```

Once activated the shell prompt will change to include the prefix `<environment>🐚`.

At this point you can [use and manage](../management) packages in this environment.

## Installing a package

One your environment is activated, use `hermit install` to install packages:

```text
project🐚~/project$ hermit install go-1.16.3
project🐚~/project$ go version
go version go1.16.3 darwin/amd64
```

Refer to the [package management](../management) documentation for more
details, including how to uninstall, information about channels, etc.

## Deactivating an Environment

When an environment is activated, Hermit inserts a shell function
`deactivate-hermit`. Call this to deactivate the current environment.

```text
project🐚~/project$ deactivate-hermit
Hermit environment /home/user/project deactivated
~/project$
```