+++
title = "Editor / IDE Integration"
weight = 105
+++

## JetBrains - IntelliJ, GoLand

{{< hint "warning" >}}
Due to the way JetBrains IDE plugin APIs are designed, specific support for
each language has to be built into the plugin. Currently, only the JDK and Go
are supported.
{{< /hint >}}

To install the plugin, search for the "Hermit" plugin from the Plugin Marketplace in the IDE `Preferences > Plugin` view, and install it.

When you open a Hermit managed project, a dialog is shown asking if you want to enable the plugin for the project.
If you select "yes", the plugin is configured for your project.

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

{{< hint "warning" >}}
Note that if you add/remove packages from your Hermit environment you will
need to reconfigure your IDE to pick up any changes to environment variable.
{{< /hint >}}
