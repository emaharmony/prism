// Package version provides Prism's semantic release version.
package version

// Version is the semantic version for development builds. Release builds
// override it with -ldflags "-X github.com/emaharmony/prism/internal/version.Version=<tag>".
var Version = "0.1.0"

// String returns the user-facing CLI version.
func String() string {
	return "prism v" + Version
}
