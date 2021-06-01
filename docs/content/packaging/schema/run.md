+++
title = "on > run"
weight = 411
+++

A command to run when the event is triggered.

Used by: [on](../on#blocks)


## Attributes

| Attribute | Type | Description |
|-----------|------|-------------|
| `args` | `[string]?` | The arguments to the binary. |
| `cmd` | `string` | The command to execute, split by shellquote. |
| `dir` | `string?` | The directory where the command is run. Defaults to the ${root} directory. |
| `env` | `[string]?` | The environment variables for the execution. |
| `stdin` | `string?` | Optional string to be used as the stdin for the command. |
