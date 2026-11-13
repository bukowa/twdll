package workspace

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

const CurrentStateSchemaVersion = 1

func DefaultConfig() Config {
	return Config{
		DefaultGame: GameAttila,
		Decompiler:  DecompilerGhidra,
	}
}

func LoadConfig(path string) (Config, bool, error) {
	cfg := DefaultConfig()
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, false, nil
		}
		return cfg, false, fmt.Errorf("read config %s: %w", path, err)
	}
	if _, err := toml.Decode(string(data), &cfg); err != nil {
		return cfg, true, fmt.Errorf("parse config %s: %w", path, err)
	}
	if cfg.DefaultGame == "" {
		cfg.DefaultGame = GameAttila
	}
	if cfg.Decompiler == "" {
		cfg.Decompiler = DecompilerGhidra
	}
	return cfg, true, nil
}

func SaveConfig(path string, cfg Config) error {
	cfg.DataDir = filepath.ToSlash(strings.TrimSpace(cfg.DataDir))
	cfg.IDADir = filepath.ToSlash(strings.TrimSpace(cfg.IDADir))
	cfg.Games.Attila = filepath.ToSlash(strings.TrimSpace(cfg.Games.Attila))
	cfg.Games.Rome2 = filepath.ToSlash(strings.TrimSpace(cfg.Games.Rome2))
	cfg.Games.ToB = filepath.ToSlash(strings.TrimSpace(cfg.Games.ToB))

	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(cfg); err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	return WriteFileAtomic(path, buf.Bytes(), 0o644)
}

func NewState() State {
	return State{
		SchemaVersion: CurrentStateSchemaVersion,
		Tools:         make(map[ToolID]ToolState),
		Binaries:      make(map[TargetID]BinaryState),
	}
}

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
	if st.Tools == nil {
		st.Tools = make(map[ToolID]ToolState)
	}
	if st.Binaries == nil {
		st.Binaries = make(map[TargetID]BinaryState)
	}
	return st, true, nil
}

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
	return WriteFileAtomic(path, data, 0o644)
}

func (s *State) SetTool(id ToolID, ts ToolState) {
	if s.Tools == nil {
		s.Tools = make(map[ToolID]ToolState)
	}
	s.Tools[id] = ts
}

// UpsertBinary preserves Analyzed hashes when the staged binary digest is unchanged,
// preventing re-runs of `twdll init` from invalidating multi-hour decompiler databases.
func (s *State) UpsertBinary(t TargetID, sourcePath, sha256 string) {
	if s.Binaries == nil {
		s.Binaries = make(map[TargetID]BinaryState)
	}
	prev := s.Binaries[t]
	next := BinaryState{
		SourcePath:   sourcePath,
		BinarySHA256: sha256,
	}
	if prev.BinarySHA256 != "" && strings.EqualFold(prev.BinarySHA256, sha256) {
		next.Analyzed = prev.Analyzed
	}
	s.Binaries[t] = next
}

func (s *State) SetAnalyzed(t TargetID, d DecompilerID, sha256 string) {
	if s.Binaries == nil {
		s.Binaries = make(map[TargetID]BinaryState)
	}
	b := s.Binaries[t]
	b.Analyzed.Set(d, sha256)
	s.Binaries[t] = b
}

func ComputeFileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("hash %s: %w", path, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("ensure dir %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp.*")
	if err != nil {
		return fmt.Errorf("create temp file in %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = tmp.Close()
			_ = os.Remove(tmpName)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("write temp file %s: %w", tmpName, err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync temp file %s: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file %s: %w", tmpName, err)
	}
	if err := commitTempFile(tmpName, path, perm); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func CopyFileAtomic(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open %s: %w", src, err)
	}
	defer in.Close()

	perm := os.FileMode(0o644)
	if info, err := in.Stat(); err == nil && info.Mode().Perm() != 0 {
		perm = info.Mode().Perm()
	}

	dir := filepath.Dir(dst)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("ensure dir %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(dst)+".tmp.*")
	if err != nil {
		return fmt.Errorf("create temp file in %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = tmp.Close()
			_ = os.Remove(tmpName)
		}
	}()

	if _, err := io.Copy(tmp, in); err != nil {
		return fmt.Errorf("copy %s to %s: %w", src, tmpName, err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync temp file %s: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file %s: %w", tmpName, err)
	}
	if err := commitTempFile(tmpName, dst, perm); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func commitTempFile(tmpName, dst string, perm os.FileMode) error {
	if err := os.Chmod(tmpName, perm); err != nil {
		return fmt.Errorf("chmod temp file %s: %w", tmpName, err)
	}
	if err := os.Rename(tmpName, dst); err != nil {
		_ = os.Remove(dst)
		if rErr := os.Rename(tmpName, dst); rErr != nil {
			return fmt.Errorf("rename %s to %s: %w", tmpName, dst, rErr)
		}
	}
	return nil
}
