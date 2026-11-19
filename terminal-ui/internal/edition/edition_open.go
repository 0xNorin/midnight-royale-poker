package edition

const (
	// Current is the default 2.1.0 build: the Open edition.
	Current ID = Open
	// Version is the compile-time fallback for unstamped development builds.
	// The authoritative release version is the embedded VERSION file.
	Version = "2.1.1"
)
