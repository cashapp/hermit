# Modify prompt to include hermit environment (env), just an icon (short) or nothing (none)
# enum: env,short,none
# default: env
prompt = string # (optional)
# If true use a short prompt when an environment is activated.
short-prompt = boolean # (optional)
# If true Hermit will never add/remove files from Git automatically.
no-git = boolean # (optional)
# If true Hermit will try to add the IntelliJ IDEA plugin automatically.
idea = boolean # (optional)

# Default configuration values for new Hermit environments.
defaults {
  # Extra environment variables.
  env = {
    string: string,
  } # (optional)
  # Package manifest sources.
  sources = [string] # (optional)
  # Whether Hermit should automatically 'git add' new packages.
  # default: true
  manage-git = boolean # (optional)
  # Whether this environment inherits a potential parent environment from one of the parent directories
  # default: false
  inherit-parent = boolean # (optional)
  # Whether Hermit should automatically add the IntelliJ IDEA plugin.
  # default: false
  idea = boolean # (optional)

  # When to use GitHub token authentication.
  github-token-auth {
    # One or more glob patterns. If any of these match the 'owner/repo' pair of a GitHub repository, the GitHub token from the current environment will be used to fetch their artifacts.
    match = [string] # (optional)
  }
}
