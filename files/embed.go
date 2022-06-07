package files

// Embed installer template
import _ "embed"

// InstallerTemplateSource is a string containing the installer template
//go:embed "install.sh.tmpl"
var InstallerTemplateSource string
