---
title:  "Configuration"
---

Each Hermit environment contains a `bin/hermit.hcl` file that can be used to
customise that Hermit environment.

```hcl
// Extra environment variables to be added to the Hermit environment.
//
// Can prepend/append to existing variables, eg. "$PATH:${HERMIT_ENV}/scripts"
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
```
