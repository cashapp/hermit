---
title: "auto-version > json"
---

Extract version information from a JSON URL using gjson path syntax.

Used by: [auto-version](../auto-version#blocks)


## Attributes

| Attribute | Type | Description |
|-----------|------|-------------|
| `headers` | `{string: string}?` | HTTP headers to send with the request. |
| `path` | `string` | gjson path expression for selecting versions from JSON (see https://github.com/tidwall/gjson) - use version-pattern to extract substrings |
| `sha256-path` | `string?` | gjson path expression for extracting SHA256 checksum. |
| `url` | `string` | URL to retrieve JSON from. |
| `vars` | `{string: string}?` | Additional variables to extract from JSON using gjson path expressions. |