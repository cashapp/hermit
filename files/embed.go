package files

import (
	// Embed installer template
	_ "embed"
)

// InstallerTemplateSource is a string containing the installer template
//go:embed "install.sh.tmpl"
var InstallerTemplateSource string
