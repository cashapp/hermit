+++
title = "FAQ"
weight = 200
+++

{{< toc >}}

## Which Shells Does Hermit work with?

Hermit currently works with ZSH and BASH, but we would welcome 
[contributions](https://github.com/cashapp/hermit/pulls) to support other shells.

### powerlevel10k support
If you would like powerlevel10k to support hermit, all that is needed is to add the following to your `~/.p10k.zsh`

```zsh
function prompt_hermit() {
  if [[ -n $HERMIT_ENV ]]; then
    p10k segment -t "${${HERMIT_ENV:t}//\%/%%} 🐚"  -f blue
  fi
}
```
Then you can add the hermit segment to any location. For example:

```zsh
  typeset -g POWERLEVEL9K_LEFT_PROMPT_ELEMENTS=(
    # =========================[ Line #1 ]=========================
    os_icon                 # os identifier
    dir                     # current directory
    vcs                     # git status
    # =========================[ Line #2 ]=========================
    newline                 # \n
    hermit
    prompt_char             # prompt symbol
  )
```


## Does Hermit Manage Libaries?

No, Hermit is deliberately not in the business of installing libraries. Hermit
is designed to manage development _tools_ only, not be a general purpose
package manager. Consider [Nix](https://nixos.org) if you need this kind of functionality.

## Is Python supported?

[Yes!](https://github.com/cashapp/hermit-packages/blob/master/python3.hcl)

Hermit sets [`PYTHONUSERBASE`](https://docs.python.org/3/using/cmdline.html#envvar-PYTHONUSERBASE) to
`${HERMIT_ENV}/.hermit/python` and adds `${PYTHONUSERBASE}/bin` to the
`${PATH}` when in an activated environment. This results in packages installed
within the environment being mostly (completely?) isolated similar to how virtualenv works.

## Is Ruby supported?

Not yet. Hermit only supports static/relocatable binary packages and there are
no recent Ruby versions compiled in this way. We would love contributions to
support Ruby.

## Why Doesn't Hermit Have a Package for ...?

There could be a number of reasons why a package isn't present in Hermit. 

- The package may not be conducive to self-contained packaging (eg. Python).
- The community might not have needed one (yet) - please [contribute one](https://github.com/cashapp/hermit-packages/pulls)!

## Does the Hermit Project Build and Host its own Packages?

Yes and no. Mostly no, but some existing upstream binary packages require some
level of pre-processing (eg. Python). These are hosted at [cashapp/hermit-build](https://github.com/cashapp/hermit-build).

## How is Hermit different to ...?

### asdf

Hermit is probably most similar to [asdf](https://asdf-vm.com/), but their goals
differ. Hermit's goal is to make isolated cross-platform tooling consistent,
self-bootstrapping, and reproducible at the project level. asdf's primary
goal is to allow developers to install and switch between multiple versions
of languages and tooling.

|                | Hermit                                | asdf                  | Compare                           |
|----------------|---------------------------------------|-----------------------|-----------------------------------|
| **Packaging**      | HCL [manifest](../packaging)          | Shell script-based plugin [API](https://asdf-vm.com/#/plugins-create) | [Java in asdf](https://github.com/halcyon/asdf-java) / [Java in Hermit](https://github.com/cashapp/hermit-packages/blob/master/openjdk.hcl).
| **Packages**       | Binary only.                          | Compile from source, binary, wrappers around pyenv, rbenv, etc. | [Python in asdf](https://github.com/danhper/asdf-python/blob/master/bin/install#L7) / [Python in Hermit](https://github.com/cashapp/hermit-packages/blob/master/python3.hcl)

Limiting Hermit to installing only binary packages has pros and cons:

|     | Feature              | Explanation
|-----|----------------------|-------------------
| Pro | Faster               | Binary packages don't require compilation, just downloading and unpacking.
| Con | Less choice          | There are typically less relocatable/static binary packages available.
| Con | Relocatable packages | Relocatable/static binary packages can be difficult to build.
| Pro | Less fragile         | Source installations fail frequently due to missing dependencies, missing tools, and so on.
| Pro | Less requirements    | Source installations generally require a functional compiler toolchain be already present on your system, such as GCC, clang, etc.


### Bazel

While not really in the same space as Hermit,
[Bazel](https://bazel.build/) does provide build isolation and
opt-in hermetic builds. However Bazel also:

- Requires going all-in on Bazel as a build system, whereas Hermit is
  explicitly _not_ a build system but rather integrates into existing
  toolchains.
- Requires completely separate tooling, editor/IDE integration and so on.

### Docker

[Docker](https://www.docker.com/) has a very large community and provides isolation, both of which are
appealing. Unfortunately it has several shortcomings which in our view
preclude it from use as a day to day _development tooling_ system.

- Filesystem mapping on OSX is [very slow](https://github.com/docker/roadmap/issues/7).
- It does not support OSX binaries inside Docker (though see [Docker-OSX](https://github.com/sickcodes/Docker-OSX)).
- Poor integration with host editors/IDEs (though there is [some movement](https://code.visualstudio.com/docs/remote/containers)).

### GoFish

[GoFish](https://gofi.sh/)'s package definitions are quite similar to
Hermit's, but GoFish itself:

- Does not support multiple versions of the same package.
- Requires root for system wide installation.
- Does not support the concept of "environments".

### Homebrew

[Homebrew](https://brew.sh/) is a full package build system but also:

- Is a system wide package manager.
- Is largely OSX specific.
- Does not support concurrent installation of different versions of the same package well.

### Nix

[Nix](https://github.com/NixOS/nix) is the package manager for an entire OS and thus provides vastly more
functionality than Hermit, including a full package build system. This
naturally also comes with a corresponding increase in complexity. Hermit is
deliberately designed to be narrow in scope, limited to just _installing_
existing packages.
