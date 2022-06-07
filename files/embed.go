package files

import _ "embed"

//go:embed "install.sh.tmpl"
var InstallerTemplateSource string
