package workspace

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// DefaultConfig returns a Config initialized with sensible defaults.
func DefaultConfig() Config {
	return Config{
		DefaultGame: GameAttila,
		Decompiler:  DecompilerGhidra,
	}
}

// LoadConfig reads twdll.toml from path.
// If the file does not exist, it returns (DefaultConfig(), false, nil).
func LoadConfig(path string) (Config, bool, error) {
	cfg := DefaultConfig()
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, false, nil
		}
		return cfg, false, fmt.Errorf("read config %s: %w", path, err)
	}

	meta, err := toml.Decode(string(data), &cfg)
	if err != nil {
		return cfg, true, fmt.Errorf("parse config %s: %w", path, err)
	}

	// Validate unknown keys if needed (can be surfaced via meta.Undecoded())
	_ = meta

	return cfg, true, nil
}

// SaveConfig normalizes path slashes and writes cfg to path atomically.
func SaveConfig(path string, cfg Config) error {
	cfg.IDADir = filepath.ToSlash(cfg.IDADir)
	cfg.Games.Attila = filepath.ToSlash(cfg.Games.Attila)
	cfg.Games.Rome2 = filepath.ToSlash(cfg.Games.Rome2)
	cfg.Games.ToB = filepath.ToSlash(cfg.Games.ToB)

	var buf bytes.Buffer
	enc := toml.NewEncoder(&buf)
	if err := enc.Encode(cfg); err != nil {
		return fmt.Errorf("encode config: %w", err)
	}

	return writeFileAtomic(path, buf.Bytes(), 0o644)
}
