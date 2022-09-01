+++
title = "Configuration"
weight = 106
+++

Each Hermit environment contains a `bin/hermit.hcl` file that can be used to
customise that Hermit environment.

## Attributes

| Attribute | Type | Description |
|-----------|------|-------------|
| `env` | `{string:string}?` | Extra environment variables. |
| `sources` | `[string]?` | Package manifest sources in order of preference. |
| `manage-git` | `bool?` | Whether Hermit should manage Git. |

## Per-environment Sources

Hermit supports three different manifest sources:

1. Git repositories; any cloneable URI ending with `.git`, eg.<br/>`https://github.com/cashapp/hermit-packages.git`. An optional `#<tag>` suffix can be added to checkout a specific tag.
2. Local filesystem, eg. `file:///home/user/my-packages`.<br/>This is mostly only useful for local development and testing.
3. Environment relative, eg. `env:///my-packages`.<br/>This will search for package manifests in the directory `${HERMIT_ENV}/my-packages`. Useful for local overrides.
4. Data URI encode sources, eg. `data:application/json;base64,[BASE64 ENCODED JSON MAP]`.<br/>This will decode the data source as a JSON map of key binary name to value content. Useful for local overrides.
