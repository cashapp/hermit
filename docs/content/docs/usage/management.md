+++
title = "Package management"
weight = 103
+++

This document describes how packages within a Hermit environment can be found,
installed, and managed. [Packages are defined](../../packaging/overview) in
configuration files called _manifests_ which are retrieved from collections
of manifests called _manifest sources_ which in turn are commonly (but not
always) Git repositories.

## Keeping up to Date

Hermit retrieves package manifests from various locations, including Git
repositories. It will periodically sync these repositories to your system to
ensure you have the most up to date manifests, but you may also force this by running:

```text
project🐚~/project$ hermit sync
```

## Searching for Packages

Search for packages with the `search` command, optionally passing a substring
to match within the package name or description:

```text
project🐚~/project$ hermit search rust
rust (@nightly, 1.51.0)
  A language empowering everyone to build reliable and efficient software.
```

## Selecting Packages

Packages can be selected in one of three ways:

1. **By version - `<package>-<version>`**

	A specific version of a package can be installed by specifying
	`<package>-<version>`. eg. `hermit install rustc-1.51.0`

2. **By channel - `<package>@<channel>`**

	Channels can be explicitly selected with `<package>@<channel>`, eg.
	`hermit install rustc@nightly`. Channels are automatically updated at
	a frequency defined by the package manifest.

3. **By preferred version - `<package>`**

	When specifying just a package name, ie. `<package>`, the _preferred version_
	will be installed. The _preferred version_ is, in order of priority:

	1. The version specified as the `default` in the manifest.
	2. The latest stable version.
	3. The latest unstable version.
	4. The last channel, alphabetically.

## Installing Packages

To install the latest stable version of `protoc` and the `nightly` channel of
`rust`:

```text
project🐚~/project$ hermit install rust@nightly protoc
```

At this point if you `ls bin` you will see something like the following:

```text
project🐚~/project$ ls bin
README.hermit.md  clippy-driver@    rust-analyzer@    rustc@
activate-hermit*  hermit*           rust-demangler@   rustdoc@
cargo@            hermit.hcl        rust-gdb@
cargo-clippy@     miri@             rust-gdbgui@
cargo-miri@       protoc@           rust-lldb@
```

## List Installed Packages

To list packages installed in the active environment:

```text
project🐚~/project$ hermit list
protoc (3.14.0)
  protoc is a compiler for protocol buffers definitions files.
rust (@nightly)
  A language empowering everyone to build reliable and efficient software.
```

## Package Information

You can obtain more detailed package information with `hermit info <package>`, eg.

```text
project🐚~/project$ hermit info rust
hermit info rust@nightly
Name: rust
Channel: nightly
Description: A language empowering everyone to build reliable and efficient software.
State: installed
Last used: 3m36.889138s ago
Source: https://static.rust-lang.org/dist/rust-nightly-x86_64-apple-darwin.tar.xz
Root: /home/user/.cache/hermit/pkg/rust@nightly
Binaries: cargo cargo-clippy clippy-driver cargo-miri miri rust-analyzer rust-demangler rust-gdb rust-gdbgui rust-lldb rustc rustdoc
```

## Upgrading Packages

For package channels or versions that adhere to semantic versioning, Hermit
will automatically upgrade to the latest minor version using the
`hermit upgrade` command:

```text
project🐚~/project$ hermit upgrade rust
project🐚~/project$ rustc --version
rustc 1.51.0 (2fd73fabe 2021-03-23)
```

## Downgrading / Changing Versions

To downgrade or switch to a specific version, use `hermit install` to
explicitly specify the version. eg.

```text
project🐚~/project$ hermit install rust-1.50.0
project🐚~/project$ rustc --version
rustc 1.50.0 (940f2a77 2021-01-02)
```

## Uninstalling Packages

Use `hermit uninstall`:

```text
project🐚~/project$ hermit uninstall rust
```

