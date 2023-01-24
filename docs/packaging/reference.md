---
title: "Reference"
---

## Update Policy

Hermit syncs manifest sources every 24 hours from HEAD. Because any changes
are then immediately reflected in active environments, care _must_ be taken
to maintain backwards compatibility.

In particular this means:

- _Never_ delete or rename versions.
- Take care when updating environment variables.

And in general think carefully about what impact your change will have if it
is applied to an active environment.

## Manifests

Hermit manifests (package definitions) are [HCL](https://github.com/alecthomas/hcl) 
configuration files defining where to download packages from and how to install them.

Refer to the [schema documentation](../schema) for details.

Here's an example manifest for Rust:

```terraform
description = "A language empowering everyone to build reliable and efficient software."
binaries = ["bin/*"]
strip = 2

darwin {
  source = "https://static.rust-lang.org/dist/rust-${version}-x86_64-apple-darwin.tar.xz"
}

linux {
  source = "https://static.rust-lang.org/dist/rust-${version}-x86_64-unknown-linux-musl.tar.xz"
}

version "1.51.0" {}

channel nightly {
  update = "24h"
  darwin {
    source = "https://static.rust-lang.org/dist/rust-nightly-x86_64-apple-darwin.tar.xz"
  }

  linux {
    source = "https://static.rust-lang.org/dist/rust-nightly-x86_64-unknown-linux-musl.tar.xz"
  }
}
```

Package source can refer to a remote archive file by using `http://` or `https://` prefixes, to a local file by using `file://` prefix, or to a Git repository by using `.git` suffix. 
If the source points to an archive file, it is extracted at installation. Git repositories are cloned from the default branch and used as is.

## Sources

A manifest source is a location where a set of manifests are stored. Hermit
supports manifest sources in Git repositories, local filesystems (useful for
temporary overrides while testing packages), and environment-relative.

Multiple sources can be specified globally by Hermit or per-project, allowing
fine-grained control over which package definitions will be used.

## Versions

[Version](../schema/version) blocks are explicitly defined versions of a particular package.

## Channels

[Channels](../schema/channel) define a download source that will be automatically checked for
updates periodically. Hermit will check the URL's ETag and update the package
if there is a newer version.

Additionally, Hermit will create several synthetic channels which are checked for updates every 24h:

1. A `@latest` channel pointing at the most recent non-pre-release version.
2. A `@<MAJOR>` channel for every major version.
3. A `@<MAJOR>.<MINOR>` channel for every major+minor version.

This allows projects to pin to stable releases.

## Dependencies

Hermit supports two kinds of dependencies between packages, direct dependencies and runtime dependencies.

### Direct dependencies

Direct dependencies are declared in the manifest files using a `requires` definition in the manifest file. 
This lists the packages that are needed in the environment to use the given package. 
The package references in the `requires` list can either refer to an explicit package, or to a value defined in the `provides` definition of the dependency.
For example, `requires = ["jre"]` would work with any package defining `provides = ["jre"]` in its definition.

When a package with `requires` definition is installed, all its dependencies are installed to the target environment as well.

### Runtime dependencies

Runtime dependencies are package dependencies that are not installed into the target environment.
These are defined using a `runtime-dependencies` definition in the manifest.

Hermit makes sure the runtime dependencies are on the system when a binary from the package is executed, and injects the environment variables from the runtime dependencies to the binary when executed.
This is a good way on depending on binaries and env variables from other packages in your package without exposing them to the target environment.

## Variable Interpolation

Hermit manifests support basic variable interpolation to simplify
configuration. It's not necessary to utilise them, but they can make life
simpler in many cases.

The available variables are:

| Variable     | Description |
|--------------|-------------|
| `name`       | The name of the current package. |
| `version`    | The version selected by the user. Does not apply when installing a channel. |
| `dest`       | The directory where the archive will be extracted.<br/> Defaults to `<hermit-state>/pkg/<pkg-selector>`. |
| `root`       | Directory considered the package root. Defaults to `${dest}`.
| `os`         | The system's [OS](https://github.com/golang/go/blob/master/src/go/build/syslist.go) as reported by Go. |
| `arch`       | The system's [CPU](https://github.com/golang/go/blob/master/src/go/build/syslist.go) architecture as reported by Go. |
| `xarch`      | An alternate mapping of `${arch}` where `amd64`=>`x86_64`,  `i386`=>`386`, and `arm64`=>`aarch64`. |
| `HERMIT_ENV` | Path to the active Hermit environment. |
| `HERMIT_BIN` | Path to the active Hermit environment's `bin` directory. |
| `HOME`       | The user's home directory. |

## Triggers and Actions

Hermit supports the concept of [triggers](../schema/on) and actions which can
be applied when certain events occur in the package lifecycle. Supported events are:

| Event         | Description |
|---------------|-------------|
| `unpack`      | Triggered when a package is unpacked into the Hermit cache. |
| `install`     | Triggered when a package is installed into an environment. |
| `activate`    | Triggered when the environment the package is installed in is activated. |

More triggers may be added in the future.
