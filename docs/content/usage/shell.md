+++
title = "Shell Integration"
weight = 105
+++

## Tracking Environment Variables

When a Hermit environment is activated, Hermit will install a shell hook to
keep your shell's environment variables synchronised with Hermit's
environment variables as you add and remove packages. This hook executes
prior to each command.

## Automatic Environment Activation / Deactivation

Hermit can also install shell integration hooks to automate
activation/deactivation of Hermit environments as you change directories in
your terminal.

### Zsh

This will install Hermit hooks into your `~/.zshrc` file. Restart your shell
in order for the changes to take effect.

```text
hermit shell-hooks --zsh
```

### Bash

This will install Hermit hooks into your `~/.bashrc` file. Restart your shell
in order for the changes to take effect.


```text
hermit shell-hooks --bash
```
