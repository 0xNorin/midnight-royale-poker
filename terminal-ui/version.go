package main

import _ "embed"

// embeddedVersion is the authoritative release version, read from the VERSION
// file at the repository root. The release pipeline stamps
// -X main.versionOverride=... for tagged releases; otherwise the Open edition
// reports this value.
//
// It lives in its own file so every build-tagged bundle variant (1.2.x and the
// 2.0.0 development build) shares one version source.
//
//go:embed VERSION
var embeddedVersion string
