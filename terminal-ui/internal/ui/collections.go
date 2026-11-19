package ui

import (
	"strings"

	"midnight-royale/internal/art"
)

// CollectionID is the stable machine identifier for a collection. It is never
// derived from a display name or an array position, and it must not change
// after a collection ships (saves, purchases and entitlement reference it).
// Indexes are used only for temporary on-screen navigation.
type CollectionID string

const (
	CollectionGrandRivieraHotel CollectionID = "grand-riviera-hotel"
	CollectionAzurePalmsResort  CollectionID = "azure-palms-resort"
	CollectionStarlightLodge    CollectionID = "starlight-lodge"
)

// DefaultCollectionID is the single backward-compatible default for deep links,
// tests and internal entry points that reach the opponent selector directly.
const DefaultCollectionID = CollectionGrandRivieraHotel

// progressNamespace maps a collection to its isolated save/gallery namespace.
// The progress save is keyed by collection, so each collection’s earned
// artwork stays separate from the others. It is the string form of the stable
// collection ID (equal to progress.GrandRivieraHotel for Grand Riviera).
func progressNamespace(c CollectionID) string { return string(c) }

// ReleaseState is a typed release state, deliberately separate from purchase
// entitlement. A ComingSoon collection cannot be entered regardless of
// purchases; ownership of opponents inside a released collection remains a
// separate concern handled by the existing entitlement system.
type ReleaseState int

const (
	ReleaseAvailable ReleaseState = iota
	ReleaseComingSoon
)

// Collection is one entry in the collection catalog, the single source of
// truth for collection selection. Opponent membership is not listed here; it
// lives on the opponent definition (Character.Collection) so membership is
// never duplicated across registries.
type Collection struct {
	ID    CollectionID
	Name  string // display name, including "The" where applicable
	State ReleaseState
	// AvailableArt / DisabledArt are .ans asset keys (filename without the
	// .ans suffix) selected by release state. A collection with no separate
	// disabled asset leaves DisabledArt empty and keeps its available artwork.
	AvailableArt string
	DisabledArt  string
}

// Collections returns the collection catalog in display order.
func Collections() []Collection {
	return []Collection{
		{ID: CollectionGrandRivieraHotel, Name: "Grand Riviera Hotel Collection", State: ReleaseAvailable, AvailableArt: "grand-riviera-hotel-collection"},
		{ID: CollectionAzurePalmsResort, Name: "The Azure Palms Resort Collection", State: ReleaseAvailable, AvailableArt: "azure-palms-resort-collection-available", DisabledArt: "azure-palms-resort-collection-disabled"},
		{ID: CollectionStarlightLodge, Name: "The Starlight Lodge Collection", State: ReleaseAvailable, AvailableArt: "starlight-lodge-collection-available", DisabledArt: "starlight-lodge-collection-disabled"},
	}
}

// defaultAccent is the canonical warm gold shared by every collection. It is
// both the Grand Riviera accent and the fallback for any unknown collection,
// so screens without an active collection keep the established gold theme.
const defaultAccent = "#DDB979"

// collectionAccent returns the per-collection accent colour keyed by stable
// collection ID. Gold is the fallback. Keep these readable against the dark
// background and distinct from the semantic Hot/Good/Bad colours.
func collectionAccent(id CollectionID) string {
	switch id {
	case CollectionAzurePalmsResort:
		return "#7CD8E8" // light cyan
	case CollectionStarlightLodge:
		return "#90DFA0" // soft light green
	default:
		return defaultAccent // warm gold
	}
}

// ArtworkKey returns the .ans asset key for the collection's current release
// state: the disabled artwork when coming soon, otherwise the available
// artwork.
func (c Collection) ArtworkKey() string {
	if c.State == ReleaseComingSoon && c.DisabledArt != "" {
		return c.DisabledArt
	}
	return c.AvailableArt
}

// PlaqueTitle returns the two-line title shown on the collection nameplate:
// the hotel name (the display name without its trailing " Collection") and the
// literal "Collection" subtitle. The split is intentional, never a word-wrap
// fallback, so the break stays stable for every collection.
func (c Collection) PlaqueTitle() (string, string) {
	return strings.TrimSuffix(c.Name, " Collection"), "Collection"
}

// isAvailable reports whether the collection may be entered.
func (c Collection) isAvailable() bool { return c.State == ReleaseAvailable }

// collectionIndex returns the catalog index of the selected collection.
func (m *Model) collectionIndex() int {
	cols := Collections()
	for i, c := range cols {
		if c.ID == m.SelectedCollection {
			return i
		}
	}
	return 0
}

// collection returns the selected collection catalog entry.
func (m *Model) collection() Collection { return Collections()[m.collectionIndex()] }

// selectCollection moves the selected collection by delta (wrapping), matching
// the opponent selector's wrapping behavior.
func (m *Model) selectCollection(delta int) {
	cols := Collections()
	if len(cols) == 0 {
		return
	}
	i := (m.collectionIndex() + delta + len(cols)) % len(cols)
	if cols[i].ID != m.SelectedCollection {
		m.SelectedCollection = cols[i].ID
		m.Selected = 0
	}
}

// collectionKey handles keyboard input on the collection-selection screen.
// Enter opens an available collection and is a no-op on coming-soon entries;
// Esc returns to the title screen.
func (m *Model) collectionKey(k string) {
	switch k {
	case "left":
		m.selectCollection(-1)
	case "right":
		m.selectCollection(+1)
	case "enter":
		if m.collection().isAvailable() {
			m.goScreen("select")
		}
	case "esc":
		m.goScreen("opening")
	}
}

// collectionArt returns the framed collection artwork for the given asset key,
// or nil when it is unavailable.
func (m *Model) collectionArt(key string) *art.Art {
	if m.Assets == nil || m.Assets.Collections == nil {
		return nil
	}
	return m.Assets.Collections[key]
}

// collectionColumns / collectionRows are the fixed, validated dimensions of
// every framed collection asset: 94 columns x 30 rows (92x28 inner image
// inside a one-character gold frame).
const (
	collectionColumns = 94
	collectionRows    = 30
)
