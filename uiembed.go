package courtyard

import "embed"

// UIFS contains the embedded UI files from the ui/ directory.
//
//go:embed all:ui
var UIFS embed.FS
