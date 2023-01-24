---
title: "version > auto-version"
---

Automatically update versions.

Used by: [version](../version#blocks)


## Blocks

| Block  | Description |
|--------|-------------|
| [`html { … }`](../html) | Extract version information from a HTML URL using XPath. |

## Attributes

| Attribute | Type | Description |
|-----------|------|-------------|
| `github-release` | `string?` | GitHub &lt;user&gt;/&lt;repo&gt; to retrieve and update versions from the releases API. |
| `ignore-invalid-versions` | `boolean?` | Ignore tags that don&#39;t match the versin-pattern instead of failing. Does not apply to versions extracted using HTML URL |
| `version-pattern` | `string?` | Regex with one capture group to extract the version number from the origin. default: v?(.*) |
