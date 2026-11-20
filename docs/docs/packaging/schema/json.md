---
title: "auto-version &gt; json"
---

Extract version information from a JSON URL using jq.

Used by: [auto-version](../auto-version#blocks)


## Attributes

| Attribute | Type | Description |
|-----------|------|-------------|
| `jq` | `string` | jq expression to extract version strings from JSON (see https://github.com/itchyny/gojq). |
| `url` | `string` | URL to retrieve JSON from. |
