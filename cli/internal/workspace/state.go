package workspace

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

const CurrentStateSchemaVersion = 1

// NewState creates an initialized State with empty maps and schema version 1.
func NewState() State {
	return State{
		SchemaVersion: CurrentStateSchemaVersion,
		Tools:         make(map[ToolID]ToolState),
		Binaries:      make(map[TargetID]BinaryState),
	}
}

// LoadState reads .twdll/state.json from path.
// If the file does not exist, it returns (NewState(), false, nil).
func LoadState(path string) (State, bool, error) {
	st := NewState()
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return st, false, nil
		}
		return st, false, fmt.Errorf("read state %s: %w", path, err)
	}

	if err := json.Unmarshal(data, &st); err != nil {
		return st, true, fmt.Errorf("parse state %s: %w", path, err)
	}

	// Guard against null maps in JSON
	if st.Tools == nil {
		st.Tools = make(map[ToolID]ToolState)
	}
	if st.Binaries == nil {
		st.Binaries = make(map[TargetID]BinaryState)
	}

	return st, true, nil
}

// SaveState writes st to path atomically as formatted JSON.
func SaveState(path string, st State) error {
	if st.SchemaVersion == 0 {
		st.SchemaVersion = CurrentStateSchemaVersion
	}
	if st.Tools == nil {
		st.Tools = make(map[ToolID]ToolState)
	}
	if st.Binaries == nil {
		st.Binaries = make(map[TargetID]BinaryState)
	}

	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("encode state: %w", err)
	}
	data = append(data, '\n')

	return writeFileAtomic(path, data, 0o644)
}

// SetBinary records or updates the state of an isolated binary target.
func (s *State) SetBinary(t TargetID, b BinaryState) {
	if s.Binaries == nil {
		s.Binaries = make(map[TargetID]BinaryState)
	}
	s.Binaries[t] = b
}

// SetAnalyzed records the completed analysis SHA-256 for a given target and decompiler.
func (s *State) SetAnalyzed(t TargetID, d DecompilerID, sha256 string) {
	if s.Binaries == nil {
		s.Binaries = make(map[TargetID]BinaryState)
	}
	b := s.Binaries[t]
	b.Analyzed.Set(d, sha256)
	s.Binaries[t] = b
}

// SetTool records or updates an installed portable tool's version and archive digest.
func (s *State) SetTool(id ToolID, ts ToolState) {
	if s.Tools == nil {
		s.Tools = make(map[ToolID]ToolState)
	}
	s.Tools[id] = ts
}
