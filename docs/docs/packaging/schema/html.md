---
title: "auto-version &gt; html"
---

Extract version information from a HTML URL using XPath.

Used by: [auto-version](../auto-version#blocks)


## Attributes

| Attribute | Type | Description |
|-----------|------|-------------|
| `css` | `string?` | CSS selector for selecting versions from HTML (see https://github.com/andybalholm/cascadia). Only one of xpath or css can be specified. |
| `url` | `string` | URL to retrieve HTML from. |
| `xpath` | `string?` | XPath for selecting versions from HTML (see https://github.com/antchfx/htmlquery) - use version-pattern to extract substrings |
