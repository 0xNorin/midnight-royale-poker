// Package adultcontent persists the independent first-launch adult-content
// self-declaration. It does not contain buyer, purchase, recovery, network, or
// verified-age state.
package adultcontent

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"midnight-royale/internal/appdata"
	"os"
	"path/filepath"
)

const (
	CurrentVersion = 1
	FileName       = "adult-content.json"
	maxRecordBytes = 4096
)

type record struct {
	Version  int  `json:"version"`
	Accepted bool `json:"adult_content_accepted"`
}

// Store is the file adapter used by the UI's adult-content access seam.
type Store struct {
	path       string
	pathErr    error
	renameFile func(string, string) error
}

func NewStore(path string) *Store { return &Store{path: path} }

// NewDefaultStore resolves a conventional application user-data path. Path
// resolution failures stay attached to the store so the terminal can render
// its safe Retry/Exit state instead of exiting or silently bypassing the gate.
func NewDefaultStore(namespace appdata.Namespace) *Store {
	dir, err := appdata.Dir(namespace)
	return &Store{path: filepath.Join(dir, FileName), pathErr: err}
}

func (s *Store) Path() string { return s.path }

// Status reports only a validated, explicit true record as accepted.
func (s *Store) Status() (bool, error) {
	if s.pathErr != nil {
		return false, fmt.Errorf("adult content: data path: %w", s.pathErr)
	}
	if s.path == "" {
		return false, fmt.Errorf("adult content: data path is empty")
	}
	f, err := os.Open(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("adult content: read acceptance: %w", err)
	}
	defer f.Close()
	payload, err := io.ReadAll(io.LimitReader(f, maxRecordBytes+1))
	if err != nil {
		return false, fmt.Errorf("adult content: read acceptance: %w", err)
	}
	if len(payload) > maxRecordBytes {
		return false, fmt.Errorf("adult content: acceptance record is too large")
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var saved record
	if err := decoder.Decode(&saved); err != nil {
		return false, fmt.Errorf("adult content: decode acceptance: %w", err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return false, fmt.Errorf("adult content: decode acceptance: %w", err)
	}
	if saved.Version != CurrentVersion {
		return false, fmt.Errorf("adult content: unsupported acceptance version %d", saved.Version)
	}
	if !saved.Accepted {
		return false, fmt.Errorf("adult content: acceptance record is not explicitly true")
	}
	return true, nil
}

func requireJSONEOF(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err == nil {
		return fmt.Errorf("multiple JSON values")
	}
	return err
}

// Accept atomically records explicit acceptance. Existing malformed or
// unsupported data is never overwritten; the UI remains fail-closed.
func (s *Store) Accept() error {
	accepted, err := s.Status()
	if err != nil {
		return err
	}
	if accepted {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("adult content: create data directory: %w", err)
	}
	payload, err := json.MarshalIndent(record{Version: CurrentVersion, Accepted: true}, "", "  ")
	if err != nil {
		return fmt.Errorf("adult content: encode acceptance: %w", err)
	}
	payload = append(payload, '\n')

	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".adult-content-*.tmp")
	if err != nil {
		return fmt.Errorf("adult content: create temporary acceptance: %w", err)
	}
	tmpPath := tmp.Name()
	committed := false
	defer func() {
		_ = tmp.Close()
		if !committed {
			_ = os.Remove(tmpPath)
		}
	}()
	if err := tmp.Chmod(0o600); err != nil {
		return fmt.Errorf("adult content: secure temporary acceptance: %w", err)
	}
	if _, err := tmp.Write(payload); err != nil {
		return fmt.Errorf("adult content: write temporary acceptance: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("adult content: sync temporary acceptance: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("adult content: close temporary acceptance: %w", err)
	}
	renameFile := s.renameFile
	if renameFile == nil {
		renameFile = os.Rename
	}
	if err := renameFile(tmpPath, s.path); err != nil {
		// A concurrent process may have completed the same one-way write.
		if nowAccepted, statusErr := s.Status(); statusErr == nil && nowAccepted {
			return nil
		}
		return fmt.Errorf("adult content: replace acceptance: %w", err)
	}
	committed = true
	return nil
}
