+++
title = "version > auto-version"
weight = 414
+++

Automatically update versions.

Used by: [version](../version#blocks)


## Attributes

| Attribute | Type | Description |
|-----------|------|-------------|
| `github-release` | `string` | GitHub &lt;user&gt;/&lt;repo&gt; to retrieve and update versions from the releases API. |
| `version-pattern` | `string?` | Regex with one capture group to extract the version number from the origin. |
