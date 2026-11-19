// Package edition declares the compile-time build edition for Midnight Royale.
//
// Midnight Royale 2.1.0 is a free, MIT-licensed game. The default build (no
// build tag) is the Open edition: every collection ships as local, unencrypted
// content and no commerce, checkout, account, gift, recovery or updater surface
// is compiled into the binary.
//
// One transitional tag remains:
//
//	-tags norin -> the retired 0xNorin direct-commerce edition (v1.x layout)
//
// The norin edition exists only so the archived v1.x release line can still be
// rebuilt while the commerce code is removed from the source tree. It is not a
// public build.
package edition

// ID names a Midnight Royale build edition.
type ID string

const (
	// Open is the canonical 2.1.0 edition: all 36 characters and all three
	// collections are embedded as plain artwork. The game runs fully offline,
	// grants the complete roster at startup and contacts no server.
	Open ID = "open"
	// Norin is the retired 0xNorin direct-commerce edition (v1.x): direct
	// catalogue, checkout, entitlements, gift codes, recovery, the signed
	// updater and the isolated ETN-testnet flow. TRANSITIONAL ONLY.
	Norin ID = "0xnorin"
)

func (e ID) String() string { return string(e) }

// DirectCommerce reports whether the build exposes the 0xNorin direct
// catalogue, checkout, gift/recovery and other server commerce screens.
func (e ID) DirectCommerce() bool { return e == Norin }

// DirectUpdater reports whether the build exposes the direct-distribution
// signed release/update check (Settings > Check Updates).
func (e ID) DirectUpdater() bool { return e == Norin }

// PreinstalledContent reports whether the build ships playable content as
// local embedded data instead of server-issued signed packages.
func (e ID) PreinstalledContent() bool { return e == Open }

// SelfContained reports whether the build runs fully offline from embedded
// content with no server dependency of any kind.
func (e ID) SelfContained() bool { return e == Open }

// OwnNamespace reports whether the build persists user data in a dedicated
// edition namespace instead of the canonical production namespace. The Open
// edition deliberately uses the canonical namespace so players who owned or
// tested v1.x keep their progress, settings and adult-content consent.
func (e ID) OwnNamespace() bool { return false }
