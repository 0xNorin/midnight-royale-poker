package ui

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"midnight-royale/internal/art"
)

type metadata struct {
	Preset     string `json:"preset"`
	Dimensions struct {
		Columns int `json:"columns"`
		Rows    int `json:"rows"`
	} `json:"terminal_dimensions"`
	Samples struct {
		Width  int `json:"width"`
		Height int `json:"height"`
	} `json:"sample_dimensions"`
}

// Face cell dimensions for the enlarged character-selector thumbnails.
const (
	faceColumns = 28
	faceRows    = 14
	introPath   = "assets/midnight-royale-neon/midnight-royale.ans"
)

// LoadAssets validates the intentionally public bundle before the terminal
// enters raw mode: free all-five, paid scene-01 previews, and every face.
func LoadAssets(files fs.FS) (*Assets, error) {
	a := &Assets{Art: map[string]*art.Art{}, Faces: map[string]*art.Art{}, Collections: map[string]*art.Art{}}
	for _, c := range ShippedCharacters() {
		for stage := 1; stage <= 5; stage++ {
			key := artKey(c.ID, stage)
			path := "assets/art/" + key
			raw, e := fs.ReadFile(files, path+".metadata.json")
			if e != nil {
				return nil, fmt.Errorf("%s metadata: %w", key, e)
			}
			var md metadata
			if e = json.Unmarshal(raw, &md); e != nil {
				return nil, e
			}
			if md.Preset != "candidate-lanczos-96" || md.Dimensions.Columns != 96 || md.Dimensions.Rows != 72 || md.Samples.Width != 96 || md.Samples.Height != 144 {
				return nil, fmt.Errorf("%s is not approved 96x72 artwork", key)
			}
			raw, e = fs.ReadFile(files, path+".ans")
			if e != nil {
				return nil, e
			}
			if len(raw) > 4*1024*1024 {
				return nil, fmt.Errorf("%s exceeds asset size limit", key)
			}
			img, e := art.Parse(string(raw))
			if e != nil {
				return nil, fmt.Errorf("%s: %w", key, e)
			}
			if img.Width != 96 || len(img.Rows) != 72 {
				return nil, fmt.Errorf("%s content does not match metadata", key)
			}
			a.Art[key] = img
		}
		raw, e := fs.ReadFile(files, "assets/faces/"+c.ID+".ans")
		if e != nil {
			return nil, e
		}
		img, e := art.Parse(string(raw))
		if e != nil {
			return nil, e
		}
		if img.Width != faceColumns || len(img.Rows) != faceRows {
			return nil, fmt.Errorf("invalid face dimensions: %s", c.ID)
		}
		a.Faces[c.ID] = img
	}
	for _, col := range Collections() {
		for _, key := range []string{col.AvailableArt, col.DisabledArt} {
			if key == "" {
				continue
			}
			if _, ok := a.Collections[key]; ok {
				continue
			}
			raw, e := fs.ReadFile(files, "assets/collections/"+key+".ans")
			if e != nil {
				return nil, fmt.Errorf("collection %s artwork: %w", key, e)
			}
			if len(raw) > 4*1024*1024 {
				return nil, fmt.Errorf("collection %s exceeds asset size limit", key)
			}
			img, e := art.Parse(string(raw))
			if e != nil {
				return nil, fmt.Errorf("collection %s: %w", key, e)
			}
			if img.Width != collectionColumns || len(img.Rows) != collectionRows {
				return nil, fmt.Errorf("collection %s is not approved %dx%d artwork", key, collectionColumns, collectionRows)
			}
			a.Collections[key] = img
		}
	}
	intro, err := loadIntro(files)
	if err != nil {
		return nil, err
	}
	a.Intro = intro
	return a, nil
}

// AddSceneBytes parses and validates one approved 96x72 scene and installs it
// into the assets map. Preinstalled-content editions use it to inject paid
// scenes without touching the commerce install path.
func AddSceneBytes(a *Assets, key string, raw []byte) error {
	if a == nil || a.Art == nil {
		return fmt.Errorf("assets are not initialized")
	}
	if len(raw) == 0 || len(raw) > 4*1024*1024 {
		return fmt.Errorf("%s exceeds asset size limit", key)
	}
	image, err := art.Parse(string(raw))
	if err != nil {
		return fmt.Errorf("%s: %w", key, err)
	}
	if image.Width != 96 || len(image.Rows) != 72 {
		return fmt.Errorf("%s content does not match metadata", key)
	}
	a.Art[key] = image
	return nil
}

// loadIntro reads the approved neon cracktro. The authored file ends with a bare
// background-reset line of zero visible width; it is trimmed before parsing.
func loadIntro(files fs.FS) (*art.Art, error) {
	raw, err := fs.ReadFile(files, introPath)
	if err != nil {
		return nil, err
	}
	if len(raw) > 4*1024*1024 {
		return nil, fmt.Errorf("intro exceeds asset size limit")
	}
	lines := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
	for len(lines) > 0 && ansi.StringWidth(lines[len(lines)-1]) == 0 {
		lines = lines[:len(lines)-1]
	}
	img, err := art.Parse(strings.Join(lines, "\n") + "\n")
	if err != nil {
		return nil, fmt.Errorf("intro: %w", err)
	}
	if img.Width != 58 || len(img.Rows) != 31 {
		return nil, fmt.Errorf("unexpected intro dimensions %dx%d", img.Width, len(img.Rows))
	}
	return img, nil
}
