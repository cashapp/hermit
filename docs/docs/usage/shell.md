---
title: "Shell Integration"
---

## Tracking Environment Variables

When a Hermit environment is activated, Hermit will install a shell hook to
keep your shell's environment variables synchronized with Hermit's
environment variables as you add and remove packages. This hook executes
prior to each command.

## Custom Shell Hooks

You can define custom functions that are called when Hermit environments are activated or deactivated.

### `hermit_on_activate`

Called after a Hermit environment is activated. Has access to `$HERMIT_ENV` and other Hermit variables.

**Bash/Zsh example:**
```bash
hermit_on_activate() {
  export MY_VAR="value"
  PS1="🐚 $PS1"  # Customize prompt when prompt = "none" in bin/hermit.hcl
}
```

**Fish example:**
```fish
function hermit_on_activate
  set -gx MY_VAR "value"
end
```

### `hermit_on_deactivate`

Called when deactivating an environment (via `deactivate-hermit` or when switching environments).

**Bash/Zsh example:**
```bash
hermit_on_deactivate() {
  unset MY_VAR
  PS1=${PS1#"🐚 "} # Remove the shell when deactivating
}
```

**Fish example:**
```fish
function hermit_on_deactivate
  set -e MY_VAR
end
```

Define these functions in your shell's RC file (`~/.bashrc`, `~/.zshrc`, or `~/.config/fish/config.fish`). They are optional.

**Tip:** Set `prompt = "none"` in your `bin/hermit.hcl` to disable Hermit's default prompt modification and use `hermit_on_activate` to customize `PS1` instead.

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
