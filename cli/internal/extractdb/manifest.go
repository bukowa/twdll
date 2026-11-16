package extractdb

import (
	"encoding/json"
	"fmt"
	"os"

	"twdll/cli/internal/workspace"
)

const ManifestSchemaVersion = 1

type TargetManifest struct {
	SchemaVersion      int               `json:"schema_version"`
	Target             string            `json:"target"`
	BinarySHA256       string            `json:"binary_sha256"`
	ExtractedAt        string            `json:"extracted_at"`
	FactsSchemaVersion int               `json:"facts_schema_version"`
	Sources            []string          `json:"sources,omitempty"`
	Counts             map[string]int    `json:"counts,omitempty"`
	Decomp             map[string]string `json:"decomp,omitempty"`
}

func WriteManifestAtomic(path string, m TargetManifest) error {
	if m.SchemaVersion == 0 {
		m.SchemaVersion = ManifestSchemaVersion
	}
	if m.FactsSchemaVersion == 0 {
		m.FactsSchemaVersion = FactsSchemaVersion
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("encode target manifest: %w", err)
	}
	data = append(data, '\n')
	return workspace.WriteFileAtomic(path, data, 0o644)
}

func ReadManifest(path string) (TargetManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return TargetManifest{}, fmt.Errorf("read target manifest %s: %w", path, err)
	}
	var m TargetManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return TargetManifest{}, fmt.Errorf("parse target manifest %s: %w", path, err)
	}
	return m, nil
}
