package ui

import (
	"fmt"

	"midnight-royale/internal/art"
	"midnight-royale/internal/progress"
)

type galleryEntry struct {
	CharacterID string
	Image       int
}

func (m *Model) galleryEntries() []galleryEntry {
	var entries []galleryEntry
	for _, ch := range m.roster() {
		cp := m.Save.Character(progressNamespace(ch.Collection), ch.ID)
		for _, image := range cp.EarnedImages {
			if image >= progress.FirstEarnedArtworkImage && image <= progress.FinalArtworkImage {
				entries = append(entries, galleryEntry{CharacterID: ch.ID, Image: image})
			}
		}
	}
	return entries
}

func (m *Model) enterGallery() {
	m.GalleryDetail = false
	m.GalleryScroll = 0
	m.clampGalleryCursor()
	if len(m.galleryEntries()) == 0 {
		m.GalleryFocus = 1
	} else {
		m.GalleryFocus = 0
	}
}

func (m *Model) clampGalleryCursor() {
	entries := m.galleryEntries()
	if len(entries) == 0 {
		m.GalleryCursor = 0
		return
	}
	m.GalleryCursor = max(0, min(m.GalleryCursor, len(entries)-1))
}

func (m *Model) selectedGalleryEntry() (galleryEntry, bool) {
	entries := m.galleryEntries()
	if len(entries) == 0 {
		return galleryEntry{}, false
	}
	m.GalleryCursor = (m.GalleryCursor%len(entries) + len(entries)) % len(entries)
	return entries[m.GalleryCursor], true
}

func (m *Model) galleryMove(delta int) {
	entries := m.galleryEntries()
	if len(entries) == 0 {
		return
	}
	m.GalleryCursor = (m.GalleryCursor + delta + len(entries)) % len(entries)
	m.GalleryScroll = 0
}

func (m *Model) galleryMoveCharacter(delta int) {
	current, ok := m.selectedGalleryEntry()
	if !ok {
		return
	}
	entries := m.galleryEntries()
	for step := 1; step <= len(entries); step++ {
		i := (m.GalleryCursor + delta*step + len(entries)*2) % len(entries)
		if entries[i].CharacterID != current.CharacterID {
			m.GalleryCursor = i
			m.GalleryScroll = 0
			return
		}
	}
}

func (m *Model) galleryKey(k string) {
	if m.GalleryDetail {
		m.galleryDetailKey(k)
		return
	}
	entries := m.galleryEntries()
	switch k {
	case "esc":
		m.goScreen("select")
	case "tab", "shift+tab":
		if len(entries) == 0 {
			m.GalleryFocus = 1
		} else {
			m.GalleryFocus = 1 - m.GalleryFocus
		}
	case "left":
		if m.GalleryFocus == 0 {
			m.galleryMove(-1)
		}
	case "right":
		if m.GalleryFocus == 0 {
			m.galleryMove(1)
		}
	case "up":
		if m.GalleryFocus == 1 && len(entries) > 0 {
			m.GalleryFocus = 0
		} else if m.GalleryFocus == 0 {
			m.galleryMoveCharacter(-1)
		}
	case "down":
		if m.GalleryFocus == 0 {
			m.galleryMoveCharacter(1)
		}
	case "enter", "space":
		if m.GalleryFocus == 1 || len(entries) == 0 {
			m.goScreen("select")
		} else {
			m.GalleryDetail = true
			m.GalleryScroll = 0
			m.GalleryFocus = 0
		}
	}
}

func (m *Model) galleryDetailKey(k string) {
	switch k {
	case "esc":
		m.GalleryDetail = false
		m.GalleryScroll = 0
		m.GalleryFocus = 0
	case "tab", "shift+tab":
		m.GalleryFocus = 1 - m.GalleryFocus
	case "enter", "space":
		if m.GalleryFocus == 1 {
			m.GalleryDetail = false
			m.GalleryScroll = 0
			m.GalleryFocus = 0
		}
	case "left":
		m.galleryMove(-1)
		m.GalleryFocus = 0
	case "right":
		m.galleryMove(1)
		m.GalleryFocus = 0
	case "up":
		m.GalleryScroll--
	case "down":
		m.GalleryScroll++
	case "pgup":
		m.GalleryScroll -= max(1, m.galleryGeometry().ArtH-2)
	case "pgdown":
		m.GalleryScroll += max(1, m.galleryGeometry().ArtH-2)
	case "home":
		m.GalleryScroll = 0
	case "end":
		m.GalleryScroll = 1 << 30
	}
	m.clampGalleryScroll()
}

func (m *Model) galleryGeometry() Geometry {
	return Geometry{ArtY: 3, ArtH: max(0, m.H-5), ArtX: (m.W - 96) / 2}
}

func (m *Model) galleryArtwork(entry galleryEntry) (*art.Art, bool) {
	ch, ok := m.characterByID(entry.CharacterID)
	if !ok || m.Assets == nil {
		return nil, false
	}
	image := m.Assets.Art[artKey(ch.ID, entry.Image)]
	return image, image != nil
}

func (m *Model) clampGalleryScroll() {
	entry, ok := m.selectedGalleryEntry()
	if !ok {
		m.GalleryScroll = 0
		return
	}
	image, available := m.galleryArtwork(entry)
	if !available {
		m.GalleryScroll = 0
		return
	}
	m.GalleryScroll = max(0, min(m.GalleryScroll, max(0, len(image.Rows)-m.galleryGeometry().ArtH)))
}

func (m *Model) gallery(c *Canvas, t Theme) {
	if m.GalleryDetail {
		m.galleryDetail(c, t)
		return
	}
	m.galleryOverview(c, t)
}

func (m *Model) galleryOverview(c *Canvas, t Theme) {
	c.center(2, paint("GALLERY", t.Accent, t.BG, true))
	c.center(3, paint("Earned artwork", t.Muted, t.BG, false))
	entries := m.galleryEntries()
	if len(entries) == 0 {
		c.center(10, paint("Your collection starts here.", t.Accent, t.BG, true))
		c.center(12, paint("Win rounds to reveal character artwork and revisit it here.", t.Text, t.BG, false))
		label := "Back to Characters"
		c.button((m.W-len(label)-2)/2, 16, label, "gallery-back", true, t)
		c.put(2, m.H-1, paint("Tab: Actions · Enter: Select · Esc: Back", t.Muted, t.BG, false))
		return
	}

	entryIndex := make(map[string]int, len(entries))
	for i, entry := range entries {
		entryIndex[fmt.Sprintf("%s:%d", entry.CharacterID, entry.Image)] = i
	}
	y := 5
	for _, ch := range m.roster() {
		cp := m.Save.Character(progressNamespace(ch.Collection), ch.ID)
		if len(cp.EarnedImages) == 0 {
			continue
		}
		c.put(3, y, paint(fmt.Sprintf("%-12s %d/4", ch.Name, len(cp.EarnedImages)), t.Text, t.BG, true))
		for image := progress.FirstEarnedArtworkImage; image <= progress.FinalArtworkImage; image++ {
			x := 24 + (image-progress.FirstEarnedArtworkImage)*17
			idx, earned := entryIndex[fmt.Sprintf("%s:%d", ch.ID, image)]
			if !earned {
				c.put(x, y, paint(" NOT YET ", t.Muted, t.BG, false))
				continue
			}
			label := fmt.Sprintf("IMAGE %d", image)
			c.button(x, y, label, fmt.Sprintf("gallery-item:%d", idx), m.GalleryFocus == 0 && idx == m.GalleryCursor, t)
		}
		y += 2
	}
	controlsY := m.H - 3
	c.button(3, controlsY, "View earned image", "gallery-view", m.GalleryFocus == 0, t)
	c.button(25, controlsY, "Back to Characters", "gallery-back", m.GalleryFocus == 1, t)
	c.put(2, m.H-1, paint("←/→: Earned images · ↑/↓: Characters · Enter: Select · Tab: Actions · Esc: Back", t.Muted, t.BG, false))
}

func (m *Model) galleryDetail(c *Canvas, t Theme) {
	entry, ok := m.selectedGalleryEntry()
	if !ok {
		m.GalleryDetail = false
		m.galleryOverview(c, t)
		return
	}
	ch, ok := m.characterByID(entry.CharacterID)
	if !ok {
		m.GalleryDetail = false
		m.galleryOverview(c, t)
		return
	}
	stage := fmt.Sprintf("STAGE %d", entry.Image)
	if entry.Image == progress.FinalArtworkImage {
		stage = "VICTORY"
	}
	c.center(2, paint(fmt.Sprintf("%s · IMAGE %02d · %s", ch.Name, entry.Image, stage), t.Accent, t.BG, true))
	if image, available := m.galleryArtwork(entry); available {
		m.paintPortrait(c, t, image, m.GalleryScroll, m.galleryGeometry())
	} else {
		c.center(13, paint("Artwork unavailable", t.Accent, t.BG, true))
		c.center(15, paint("Artwork is not available yet.", t.Text, t.BG, false))
		c.center(17, paint("Your earned progress is safe.", t.Muted, t.BG, false))
	}
	c.put(2, m.H-1, paint("←/→: Previous/next earned · ↑/↓: Scroll · Esc: Overview", t.Muted, t.BG, false))
	c.button(67, m.H-1, "Tab: Overview", "gallery-overview", m.GalleryFocus == 1, t)
}

func (m *Model) galleryHit(id string) bool {
	var index int
	switch id {
	case "gallery-back":
		m.goScreen("select")
		return true
	case "gallery-view":
		if len(m.galleryEntries()) > 0 {
			m.GalleryDetail = true
			m.GalleryScroll = 0
			m.GalleryFocus = 0
		}
		return true
	case "gallery-overview":
		m.GalleryDetail = false
		m.GalleryScroll = 0
		m.GalleryFocus = 0
		return true
	}
	if _, err := fmt.Sscanf(id, "gallery-item:%d", &index); err == nil {
		if index >= 0 && index < len(m.galleryEntries()) {
			m.GalleryCursor = index
			m.GalleryDetail = true
			m.GalleryScroll = 0
			m.GalleryFocus = 0
		}
		return true
	}
	return false
}
