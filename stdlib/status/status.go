package status

import (
	_ "embed"
)

//go:embed status.css
var statusCSS string

//go:embed status.template
var statusTemplateContents string
