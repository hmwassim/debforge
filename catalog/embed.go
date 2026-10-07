// Package catalogdata embeds the default package catalog and setup profile
// into the debforge binary, so definitions always match the code.
package catalogdata

import "embed"

// FS holds packages/*.yaml, files/<package>/* and profiles/*.yaml.
//
//go:embed packages files profiles
var FS embed.FS
