---
title: "version &gt; auto-version"
---

Automatically update versions.

Used by: [version](../version#blocks)


## Blocks

| Block  | Description |
|--------|-------------|
| [`html { … }`](../html) | Extract version information from a HTML URL using XPath. |
| [`json { … }`](../json) | Extract version information from a JSON URL using gjson path syntax. |

## Attributes

| Attribute | Type | Description |
|-----------|------|-------------|
| `git-tags` | `string?` | Git remote URL to fetch git tags for version extraction. |
| `github-release` | `string?` | GitHub &lt;user&gt;/&lt;repo&gt; to retrieve and update versions from the releases API. |
| `ignore-invalid-versions` | `boolean?` | Ignore tags that don&#39;t match the versin-pattern instead of failing. Does not apply to versions extracted using HTML URL |
| `version-pattern` | `string?` | Regex with one capture group to extract the version number from the origin. default: v?(.*) |
