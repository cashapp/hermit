+++
title = "darwin"
weight = 402
+++

Darwin-specific configuration.

Used by: [channel](../channel#blocks) [linux](../linux#blocks) [&lt;manifest>](../manifest#blocks) [version](../version#blocks)


## Blocks

| Block  | Description |
|--------|-------------|
| [`darwin { … }`](../darwin) | Darwin-specific configuration. |
| [`linux { … }`](../linux) | Linux-specific configuration. |
| [`on <event> { … }`](../on) | Triggers to run on lifecycle events. |

## Attributes

| Attribute | Type | Description |
|-----------|------|-------------|
| `apps` | `[string]?` | Relative paths to Mac .app packages to install. |
| `arch` | `string?` | CPU architecture to match (amd64, 386, arm, etc.). |
| `binaries` | `[string]?` | Relative glob from $root to individual terminal binaries. |
| `dest` | `string?` | Override archive extraction destination for package. |
| `env` | `{string: string}?` | Environment variables to export. |
| `files` | `{string: string}?` | Files to load strings from to be used in the manifest. |
| `mirrors` | `[string]?` | Mirrors to use if the primary source is unavailable. |
| `provides` | `[string]?` | This package provides the given virtual packages. |
| `rename` | `{string: string}?` | Rename files after unpacking to ${root}. |
| `requires` | `[string]?` | Packages this one requires. |
| `root` | `string?` | Override root for package. |
| `sha256` | `string?` | SHA256 of source package for verification. |
| `source` | `string?` | URL for source package. |
| `strip` | `number?` | Number of path prefix elements to strip. |
| `test` | `string?` | Command that will test the package is operational. |
