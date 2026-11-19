// Package progress implements Midnight Royale's local earned-artwork and
// preference persistence. Purchase authority, the active level, chips, hand,
// and match state deliberately live elsewhere.
package progress

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"midnight-royale/internal/appdata"
	"os"
	"path/filepath"
	"slices"
)

// CurrentVersion is the save schema version. It was bumped 4 -> 5 in 1.2.1
// when Royal Score fields (best_score / score_version) were added, so a
// 1.2.0 client (which supports only up to version 4) refuses to load a scored
// save instead of silently dropping the new fields on write.
const (
	CurrentVersion          = 5
	PreScoreVersion         = 4 // 1.2.0 and earlier: no best_score / score_version
	GrandRivieraHotel       = "grand-riviera-hotel"
	FirstEarnedArtworkImage = 2
	FinalArtworkImage       = 5
	RoyalScoreVersion       = 1
)

var (
	ErrUnsafeOverwrite         = errors.New("progress: refusing to overwrite after a failed load")
	ErrUnsupportedVersion      = errors.New("progress: unsupported save version")
	ErrUnsupportedScoreVersion = errors.New("progress: unsupported scoring-rule version")
	ErrUnversionedScore        = errors.New("progress: best_score present without score_version")
)

// CharacterProgress stores only additive, game-earned artwork. Installed
// content and entitlements are intentionally absent: they can make earned art
// renderable, but can never earn it.
type CharacterProgress struct {
	EarnedImages []int `json:"earned_images,omitempty"`
	Completed    bool  `json:"completed"`
	BestScore    *int  `json:"best_score,omitempty"`
}

type CollectionProgress struct {
	Characters map[string]CharacterProgress `json:"characters"`
}

type Preferences struct {
	ReducedMotion bool   `json:"reduced_motion"`
	ASCII         bool   `json:"ascii"`
	ScrollStep    int    `json:"scroll_step"`
	SoundEffects  *bool  `json:"sound_effects,omitempty"`
	UpdateChannel string `json:"update_channel,omitempty"`
}

// SoundOn reports whether sound effects are enabled, defaulting to true when
// the preference is absent (older saves).
func (p Preferences) SoundOn() bool {
	return p.SoundEffects == nil || *p.SoundEffects
}

type Data struct {
	Version      int                           `json:"version"`
	ScoreVersion int                           `json:"score_version,omitempty"`
	Collections  map[string]CollectionProgress `json:"collections"`
	Preferences  Preferences                   `json:"preferences"`
}

func NewData() Data {
	return Data{Version: CurrentVersion, Collections: make(map[string]CollectionProgress), Preferences: Preferences{ScrollStep: 3}}
}

// Character returns a copy of one collection character's earned progress.
func (d Data) Character(collectionID, characterID string) CharacterProgress {
	collection, ok := d.Collections[collectionID]
	if !ok {
		return CharacterProgress{}
	}
	cp := collection.Characters[characterID]
	cp.EarnedImages = slices.Clone(cp.EarnedImages)
	return cp
}

// Earn records one proven stage/image reach. It is additive and returns true
// only when durable state changed. Image 5 is the completion milestone.
func (d *Data) Earn(collectionID, characterID string, image int) bool {
	if d == nil || collectionID == "" || characterID == "" || image < FirstEarnedArtworkImage || image > FinalArtworkImage {
		return false
	}
	if d.Collections == nil {
		d.Collections = make(map[string]CollectionProgress)
	}
	collection := d.Collections[collectionID]
	if collection.Characters == nil {
		collection.Characters = make(map[string]CharacterProgress)
	}
	cp := collection.Characters[characterID]
	if image == FinalArtworkImage {
		changed := !cp.Completed || !slices.Equal(cp.EarnedImages, []int{2, 3, 4, 5})
		cp.Completed = true
		cp.EarnedImages = []int{2, 3, 4, 5}
		collection.Characters[characterID] = cp
		d.Collections[collectionID] = collection
		return changed
	}
	if slices.Contains(cp.EarnedImages, image) {
		return false
	}
	cp.EarnedImages = append(cp.EarnedImages, image)
	slices.Sort(cp.EarnedImages)
	collection.Characters[characterID] = cp
	d.Collections[collectionID] = collection
	return true
}

// Best returns the recorded best Royal Score for a character and whether one
// is recorded. A recorded 0 is a genuine result (cleared with no deductions
// left standing), so the boolean - not the value - distinguishes "no result"
// from "score 0".
func (d Data) Best(collectionID, characterID string) (int, bool) {
	collection, ok := d.Collections[collectionID]
	if !ok {
		return 0, false
	}
	cp := collection.Characters[characterID]
	if cp.BestScore == nil {
		return 0, false
	}
	return *cp.BestScore, true
}

// RecordBest records a completed run's Royal Score as the character's best,
// keeping only the highest. It returns true when durable state changed (a
// first result or a strictly higher one). A weaker replay never lowers the
// record.
func (d *Data) RecordBest(collectionID, characterID string, score int) bool {
	if d == nil || collectionID == "" || characterID == "" || score < 0 {
		return false
	}
	if d.Collections == nil {
		d.Collections = make(map[string]CollectionProgress)
	}
	collection := d.Collections[collectionID]
	if collection.Characters == nil {
		collection.Characters = make(map[string]CharacterProgress)
	}
	cp := collection.Characters[characterID]
	if cp.BestScore != nil && *cp.BestScore >= score {
		return false
	}
	v := score
	cp.BestScore = &v
	collection.Characters[characterID] = cp
	d.Collections[collectionID] = collection
	if d.ScoreVersion == 0 {
		d.ScoreVersion = RoyalScoreVersion
	}
	return true
}

// CollectionScore sums the recorded bests for a collection, returning the sum
// and the number of characters with a recorded result.
func (d Data) CollectionScore(collectionID string) (sum, recorded int) {
	collection, ok := d.Collections[collectionID]
	if !ok {
		return 0, 0
	}
	for _, cp := range collection.Characters {
		if cp.BestScore != nil {
			sum += *cp.BestScore
			recorded++
		}
	}
	return sum, recorded
}

// CollectionScoreFor sums the recorded bests for exactly the characters in
// ids, ignoring any saved entry whose ID is not listed. It is the roster-aware
// counterpart of CollectionScore: a removed or renamed character can never
// inflate the total or the recorded count, even though its entry stays in the
// save file for forward compatibility.
func (d Data) CollectionScoreFor(collectionID string, ids []string) (sum, recorded int) {
	collection, ok := d.Collections[collectionID]
	if !ok {
		return 0, 0
	}
	for _, id := range ids {
		cp, ok := collection.Characters[id]
		if !ok || cp.BestScore == nil {
			continue
		}
		sum += *cp.BestScore
		recorded++
	}
	return sum, recorded
}

func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("progress: user config directory: %w", err)
	}
	return filepath.Join(dir, "midnight-royale", "save.json"), nil
}

// DevelopmentPath keeps implicit DEV/demo progress away from the established
// production save. Explicit SavePath callers bypass this resolver for QA.
func DevelopmentPath() (string, error) {
	dir, err := appdata.Dir(appdata.Development)
	if err != nil {
		return "", fmt.Errorf("progress: development data directory: %w", err)
	}
	return filepath.Join(dir, "save.json"), nil
}

type Store struct {
	path       string
	loadFailed bool
}

// NamespacedPath resolves the edition-specific local save path inside the
// requested appdata namespace, keeping edition saves separate from the
// canonical 0xNorin production save.
func NamespacedPath(namespace appdata.Namespace) (string, error) {
	dir, err := appdata.Dir(namespace)
	if err != nil {
		return "", fmt.Errorf("progress: namespaced data directory: %w", err)
	}
	return filepath.Join(dir, "save.json"), nil
}

func NewStore(path string) *Store { return &Store{path: path} }

func (s *Store) Path() string { return s.path }

func (s *Store) Load() (Data, error) {
	b, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return NewData(), nil
	}
	if err != nil {
		s.loadFailed = true
		return Data{}, fmt.Errorf("progress: read: %w", err)
	}
	var header struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(b, &header); err != nil {
		s.loadFailed = true
		return Data{}, fmt.Errorf("progress: decode: %w", err)
	}
	switch header.Version {
	case CurrentVersion:
		var data Data
		if err := json.Unmarshal(b, &data); err != nil {
			s.loadFailed = true
			return Data{}, fmt.Errorf("progress: decode: %w", err)
		}
		if err := validateScoreCompatibility(&data); err != nil {
			s.loadFailed = true
			return Data{}, err
		}
		normalize(&data)
		return data, nil
	case PreScoreVersion:
		var data Data
		if err := json.Unmarshal(b, &data); err != nil {
			s.loadFailed = true
			return Data{}, fmt.Errorf("progress: decode v4: %w", err)
		}
		return s.migrateV4(b, data)
	case 1, 2, 3:
		var legacy legacyData
		if err := json.Unmarshal(b, &legacy); err != nil {
			s.loadFailed = true
			return Data{}, fmt.Errorf("progress: decode legacy: %w", err)
		}
		return s.migrateLegacy(b, legacy, header.Version)
	default:
		s.loadFailed = true
		return Data{}, fmt.Errorf("%w: got %d, support %d", ErrUnsupportedVersion, header.Version, CurrentVersion)
	}
}

type legacyCharacterProgress struct {
	Completed bool `json:"completed"`
}

type legacyData struct {
	Characters  map[string]legacyCharacterProgress `json:"characters"`
	Preferences Preferences                        `json:"preferences"`
}

// migrateLegacy trusts only the durable v1/v2/v3 Completed flag. Historical
// stage, won_hands, owned, installed, and fixture fields are deliberately not
// decoded and therefore cannot infer an intermediate Gallery unlock.
func (s *Store) migrateLegacy(original []byte, legacy legacyData, version int) (Data, error) {
	migrated := NewData()
	migrated.Preferences = legacy.Preferences
	if migrated.Preferences.ScrollStep < 1 {
		migrated.Preferences.ScrollStep = 3
	}
	for id, cp := range legacy.Characters {
		if !cp.Completed {
			continue
		}
		for image := FirstEarnedArtworkImage; image <= FinalArtworkImage; image++ {
			migrated.Earn(GrandRivieraHotel, id, image)
		}
	}
	if err := s.backupOnce(original, version); err != nil {
		s.loadFailed = true
		return Data{}, fmt.Errorf("progress: backup during migration: %w", err)
	}
	payload, err := json.MarshalIndent(migrated, "", "  ")
	if err != nil {
		s.loadFailed = true
		return Data{}, fmt.Errorf("progress: encode migration: %w", err)
	}
	if err := s.writePayload(append(payload, '\n')); err != nil {
		s.loadFailed = true
		return Data{}, fmt.Errorf("progress: write migration: %w", err)
	}
	return migrated, nil
}

// migrateV4 upgrades a pre-score v4 save to the current schema in place,
// preserving earned artwork, completion and preferences. It never invents
// Royal Scores: a plain v4 save has no best_score/score_version, so migrated
// data keeps ScoreVersion 0 and no bests. A v4 file that already carries
// score_version == RoyalScoreVersion alongside best_score (development data
// written before 1.2.1) is migrated with its scores preserved, never erased.
// A v4 file carrying best_score without a score_version (or with an
// unsupported score_version) fails the load so the original bytes are never
// overwritten.
func (s *Store) migrateV4(original []byte, data Data) (Data, error) {
	migrated := data
	migrated.Version = CurrentVersion
	if err := validateScoreCompatibility(&migrated); err != nil {
		s.loadFailed = true
		return Data{}, err
	}
	normalize(&migrated)
	if err := s.backupOnce(original, PreScoreVersion); err != nil {
		s.loadFailed = true
		return Data{}, fmt.Errorf("progress: backup during migration: %w", err)
	}
	payload, err := json.MarshalIndent(migrated, "", "  ")
	if err != nil {
		s.loadFailed = true
		return Data{}, fmt.Errorf("progress: encode migration: %w", err)
	}
	if err := s.writePayload(append(payload, '\n')); err != nil {
		s.loadFailed = true
		return Data{}, fmt.Errorf("progress: write migration: %w", err)
	}
	return migrated, nil
}

// backupOnce retains the original legacy bytes without replacing an unrelated
// backup left by an earlier installation or restored save.
func (s *Store) backupOnce(original []byte, version int) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	base := fmt.Sprintf("%s.v%d.bak", s.path, version)
	for collision := 0; ; collision++ {
		candidate := base
		if collision > 0 {
			candidate = fmt.Sprintf("%s.%d", base, collision)
		}
		if info, err := os.Lstat(candidate); err == nil {
			if info.Mode().IsRegular() {
				existing, readErr := os.ReadFile(candidate)
				if readErr != nil {
					return readErr
				}
				if bytes.Equal(existing, original) {
					return nil
				}
			}
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		file, err := os.OpenFile(candidate, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return err
		}
		_, writeErr := file.Write(original)
		if writeErr == nil {
			writeErr = file.Sync()
		}
		closeErr := file.Close()
		if writeErr != nil || closeErr != nil {
			_ = os.Remove(candidate)
			return errors.Join(writeErr, closeErr)
		}
		return nil
	}
}

func (s *Store) Save(data Data) error {
	if s.loadFailed {
		return ErrUnsafeOverwrite
	}
	if data.Version == 0 {
		data.Version = CurrentVersion
	}
	if data.Version != CurrentVersion {
		return fmt.Errorf("%w: got %d, support %d", ErrUnsupportedVersion, data.Version, CurrentVersion)
	}
	if err := validateScoreCompatibility(&data); err != nil {
		return err
	}
	normalize(&data)
	payload, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("progress: encode: %w", err)
	}
	return s.writePayload(append(payload, '\n'))
}

func normalize(data *Data) {
	if data.Collections == nil {
		data.Collections = make(map[string]CollectionProgress)
	}
	for collectionID, collection := range data.Collections {
		if collection.Characters == nil {
			collection.Characters = make(map[string]CharacterProgress)
		}
		for characterID, cp := range collection.Characters {
			valid := cp.EarnedImages[:0]
			for _, image := range cp.EarnedImages {
				if image >= FirstEarnedArtworkImage && image <= FinalArtworkImage && !slices.Contains(valid, image) {
					valid = append(valid, image)
				}
			}
			slices.Sort(valid)
			cp.EarnedImages = valid
			if cp.Completed || slices.Contains(cp.EarnedImages, FinalArtworkImage) {
				cp.Completed = true
				cp.EarnedImages = []int{2, 3, 4, 5}
			}
			collection.Characters[characterID] = cp
		}
		data.Collections[collectionID] = collection
	}
}

// hasAnyBestScore reports whether any character carries a recorded best score.
func hasAnyBestScore(data *Data) bool {
	for _, collection := range data.Collections {
		for _, cp := range collection.Characters {
			if cp.BestScore != nil {
				return true
			}
		}
	}
	return false
}

// validateScoreCompatibility rejects score data this build cannot safely
// interpret, so a load fails closed instead of mixing results computed under
// different scoring rules or silently erasing them on a later save. It accepts
// exactly two consistent states: no scoring at all (score_version 0 and no
// best_score anywhere) and the current Royal Score rules
// (score_version == RoyalScoreVersion). best_score values without a
// score_version are ambiguous (which rule produced them?) and are rejected;
// any other score_version is a rule version this build does not implement and
// is rejected. On rejection the Store marks the load failed, so a subsequent
// Save refuses to overwrite the original file bytes.
func validateScoreCompatibility(data *Data) error {
	switch data.ScoreVersion {
	case RoyalScoreVersion:
		return nil
	case 0:
		if hasAnyBestScore(data) {
			return ErrUnversionedScore
		}
		return nil
	default:
		return fmt.Errorf("%w: got %d, support %d", ErrUnsupportedScoreVersion, data.ScoreVersion, RoyalScoreVersion)
	}
}

func (s *Store) writePayload(payload []byte) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("progress: create directory: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".save-*.tmp")
	if err != nil {
		return fmt.Errorf("progress: create temporary save: %w", err)
	}
	tmpName := tmp.Name()
	committed := false
	defer func() {
		_ = tmp.Close()
		if !committed {
			_ = os.Remove(tmpName)
		}
	}()
	if err := tmp.Chmod(0o600); err != nil {
		return fmt.Errorf("progress: secure temporary save: %w", err)
	}
	if _, err := tmp.Write(payload); err != nil {
		return fmt.Errorf("progress: write temporary save: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("progress: sync temporary save: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("progress: close temporary save: %w", err)
	}
	if err := os.Rename(tmpName, s.path); err != nil {
		return fmt.Errorf("progress: replace save: %w", err)
	}
	committed = true
	return nil
}
