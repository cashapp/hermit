+++
title = "channel <name>"
weight = 405
+++

Definition of and configuration for an auto-update channel.

Used by: [&lt;manifest>](../manifest#blocks)


## Blocks

| Block  | Description |
|--------|-------------|
| [`darwin { … }`](../darwin) | Darwin-specific configuration. |
| [`linux { … }`](../linux) | Linux-specific configuration. |
| [`on <event> { … }`](../on) | Triggers to run on lifecycle events. |
| [`platform <attr> { … }`](../platform) | Platform-specific configuration. &lt;attr&gt; is a set regexes that must all match against one of CPU, OS, etc.. |

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
| `runtime-dependencies` | `[string]?` | Packages used internally by this package, but not installed to the target environment |
| `sha256` | `string?` | SHA256 of source package for verification. |
| `source` | `string?` | URL for source package. Valid URLs are Git repositories (using .git[#&lt;tag&gt;] suffix), Local Files (using file:// prefix), and Remote Files (using http:// or https:// prefix) |
| `strip` | `number?` | Number of path prefix elements to strip. |
| `test` | `string?` | Command that will test the package is operational. |
| `update` | `string` | Update frequency for this channel. |
| `vars` | `{string: string}?` | Set local variables used during manifest evaluation. |
| `version` | `string?` | Use the latest version matching this version glob as the source of this channel. Empty string matches all versions |
