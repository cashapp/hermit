+++
title = "Renovate"
weight = 110
+++

[Renovate](https://docs.renovatebot.com/) is an open source dependency update tools. It supports the followings:

* Update Hermit Packages
* Using Hermit as a source of Binaries

## Update Hermit Packages

Package update with Renovate ensures updates are done explicitly to the code repository via code commits. Together with proper default branch protection setup & CI pipeline steps, it can prevent breaking hermit package update flows into the repository, which is always a problem in the implicit package update using [Channel](../updates).

### Enable Hermit Manager

To start using Renovate for Hermit package updates, you will need to add `hermit` to the [`enabledManagers`](https://docs.renovatebot.com/configuration-options/#enabledmanagers) option in the Renovate repository config.

```json5
{
  enabledManagers: ["hermit"]
}

```

### Private Packags

If you are using [Private Packages](../../packaging/private), You will need to configure the followings:

* [Datasource Registry Url](https://docs.renovatebot.com/modules/datasource/#hermit-datasource) to make Hermit in Renovate to use the correct sources of packages.
* [Github Token for Hermit](https://docs.renovatebot.com/modules/manager/hermit/#additional-information) to make sure Hermit has proper access when downloading packages.

## Hermit as Binary Source
Renovate provides [different ways to specify the source of binaries](https://docs.renovatebot.com/self-hosted-configuration/#binarysource) of the package managers, which satisfies different needs across the Renovate community. 

With the ability to specify `hermit` as a binary source for Renovate, an extra level of flexibility is provided to Renovate. The benefits are listed as follows:

* Use exact version of package manager in a repository. (as opposite to the [Go binary version](https://docs.renovatebot.com/golang/#go-binary-version))
* Allows different package manager versions across all repositories managed by the given Renovate instance
* Supports package managers outside the [listing for `binarySource=install`](https://docs.renovatebot.com/self-hosted-configuration/#binarysource)


***Note***: This feature only supports Self-hosting Renovate environment where user have control over the `binarySource` attribute.

