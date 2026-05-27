// Package assets embeds static files (currently the application icon) so they
// ship inside the single Helena binary.
package assets

import _ "embed"

// AppIcon is the PNG application/window icon (the Helena cat mascot).
//
//go:embed app_icon.png
var AppIcon []byte
