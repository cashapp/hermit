---
title: "Recipes / Patterns"
---

Patterns for solving common problems.

## Pin to a major or minor version

Each Hermit package includes a `@latest` channel, which is the latest stable
version. One downside to this is that if a new major version is released,
potentially including breaking changes, Hermit will use that as the version.

To solve this problem Hermit also creates channels for each `(major)` and `
(major, minor)` version tuple. For example:

```shell
$ hermit search -s '^go$' | grep 'go@[0-9]'
go@1
go@1.13
go@1.14
go@1.15
go@1.16
go@1.17
```

In Go's case, a minor (1.17 -> 1.18) version bump can be a bit rocky, so we
might want to pin to the current stable minor version:

```shell
$ hermit install go@1.17
```

This will track the latest point release of Go 1.17.

## Reusing shell scripts across multiple projects

In large multi-repo environments, it's common to have sets of shell scripts
that are shared across projects. Anything from setting up and pushing Docker
images to ECR, to linting in a consistent way, to validating `.proto` files,
and so on.

One solution to this is to have a git repository containing the scripts,
cloned down by each project. The main issue here is versioning. Either the
mainline branch is used, which exposes users directly to bugs, or a specific
tagged version is be used, which relies on consumers updating those tags as
the scripts are updated.

Hermit can solve this problem nicely with its support for git cloned packages:

```terraform
description = "Our common scripts"
binaries = ["*.sh"]

source = "https://github.com/my-org/common-scripts.git#v${version}"
version "0.1.0", "0.1.1", "0.2.0" {}

channel tip {
	update = "1h"
	source = "https://github.com/my-org/common-scripts.git"
}
```

This provides semantic versioning for our library of scripts. Consumers can
[pin to a major or minor version](#pin-to-a-major-or-minor-version) to get
stability, but test repositories can opt in to the "tip" channel to get the
bleeding edge.

## Shell script "libraries"

With a bit of creativity, Hermit can help share libraries of scripts to be
used by other scripts. Add a `my-script-lib-prefix` to your library to report
its install directory. This is somewhat akin to `pkg-config`.

```bash
#!/usr/bin/env bash
echo "$(dirname $0)"
```

Then expose this as a binary in the Hermit package:

```terraform
description = "My script lib"
binaries = ["my-script-lib-prefix"]

source = "https://github.com/my-org/my-script-lib.git#v${version}"
version "0.1.0", "0.1.1", "0.2.0" {}
```

Then to source any of the "library" scripts just install the package and:

```bash
#!/usr/bin/env bash

. $(my-script-lib-prefix)/lib1.sh

# Use definitions from lib1.sh`
```
