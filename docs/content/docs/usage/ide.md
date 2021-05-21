+++
title = "Editor / IDE Integration"
weight = 104
+++

IDE integration is currently quite sparse, natively supporting only JetBrains
IDEs. There are workarounds described below for other editors.

## JetBrains - IntelliJ, GoLand

{{< tip "warning" >}}
Due to the way JetBrains IDE plugin APIs are designed, specific support for
each language has to be built into the plugin. Currently only the JDK and Go
are supported.
{{< /tip >}}

Add the following URL to your IDE via the
[Custom Plugin Repositories](https://www.jetbrains.com/help/idea/custom-plugin-repositories.html)
dialog:

```text
https://github.com/cashapp/hermit/releases/download/stable/updatePlugins.xml
```

Then search for the "Hermit" plugin and install it. You will need to restart your IDE.

The plugin will automatically configure Go and Java SDKs to work with the IDE,
including Run Configurations, tests, and the builtin terminal.

## Terminal-based Editors

Terminal based editors should Just Work™️ if launched after a Hermit
environment is activated.

## Mac GUI Editors (Workaround)

For other editors and IDEs, the best solution in lieu of native plugins is to
open up a terminal, activate the Hermit environment, then launch the editor
from the terminal. This is not ideal, but does work until a plugin is
available.

1. Close your editor.
2. From a terminal activate your Hermit environment: `. ./bin/activate-hermit`
3. Launch your editor from the terminal:

	| Editor     | Launch command |
	|------------|----------------|
	| [Sublime](https://www.sublimetext.com/docs/3/osx_command_line.html)  | `subl -nd .`   |
	| [Visual Studio Code](https://code.visualstudio.com/docs/setup/mac)    | `code .`   |

At this point your editor should be running with environment variables from
the Hermit environment.

## Other

Some IDEs/editors have support for configuring environment variables
explicitly. In this case you can use `hermit env` to dump a machine-readable
list of the environment variables Hermit manages. This can then be configured
in your IDE.

{{< tip "warning" >}}
Note that if you add/remove packages from your Hermit environment you will
need to reconfigure your IDE to pick up any changes to environment variable.
{{< /tip >}}
