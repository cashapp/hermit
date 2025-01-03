---
title: "Shell Integration"
---

## Tracking Environment Variables

When a Hermit environment is activated, Hermit will install a shell hook to
keep your shell's environment variables synchronized with Hermit's
environment variables as you add and remove packages. This hook executes
prior to each command.

## Shell Hooks

Hermit can also install shell integration hooks to provide
 * Automatic environment activation / deactivation of Hermit environments as you change directories in
   your terminal.
 * Shell completion for the Hermit commands and packages

### Zsh

This will install Hermit hooks into your `~/.zshrc` file. Restart your shell
in order for the changes to take effect.

```shell
hermit shell-hooks --zsh
```

To enable the ZSH command completion, you also need to manually initialise the completion system.
A simple example on how to do this is to add this to your `~/.zshrc` file, before the Hermit hooks:
```shell
autoload -U compinit && compinit -i
```
See the [ZSH Documentation](https://zsh.sourceforge.io/Doc/Release/Completion-System.html) for more information

### Bash

This will install Hermit hooks into your `~/.bashrc` file. Restart your shell
in order for the changes to take effect.


```shell
hermit shell-hooks --bash
```

### Fish

This will install Hermit hooks into `~/.config/fish/conf.d/hermit.fish`. Restart your shell
in order for the changes to take effect.

```shell
hermit shell-hooks --fish
```
