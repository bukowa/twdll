package workspace

import (
	"fmt"
	"os"
	"path"
	"strings"
)

// ConfiguredTargets returns the workspace's active target list in deterministic order:
// attila_64 plus each 32-bit target configured in twdll.toml or present on disk.
func ConfiguredTargets(layout Layout) []TargetID {
	out := []TargetID{TargetAttila64}
	for _, game := range AllGames() {
		tid := game.Target32()
		configured := strings.TrimSpace(layout.Config.Games.Get(game)) != ""
		info, err := os.Stat(layout.Paths.BinaryPath(tid))
		present := err == nil && !info.IsDir()
		if configured || present {
			out = append(out, tid)
		}
	}
	return out
}

// ExpandTargets resolves positional target arguments against candidates, expanding
// path.Match glob patterns (such as "*_32" or "attila_*") and preserving order
// without duplicates. When rawTargets is empty, a copy of candidates is returned.
func ExpandTargets(rawTargets []string, candidates []TargetID) ([]TargetID, error) {
	if len(rawTargets) == 0 {
		out := make([]TargetID, len(candidates))
		copy(out, candidates)
		return out, nil
	}

	seen := make(map[TargetID]bool, len(candidates))
	out := make([]TargetID, 0, len(rawTargets))
	for _, raw := range rawTargets {
		pat := strings.ToLower(strings.TrimSpace(raw))
		if !strings.ContainsAny(pat, "*?[") {
			tid, ok := ParseTargetID(pat)
			if !ok {
				return nil, fmt.Errorf("unknown target %q", raw)
			}
			if !seen[tid] {
				seen[tid] = true
				out = append(out, tid)
			}
			continue
		}

		if _, err := path.Match(pat, ""); err != nil {
			return nil, fmt.Errorf("invalid target pattern %q: %w", raw, err)
		}

		matchedAny := false
		for _, tid := range candidates {
			ok, _ := path.Match(pat, string(tid))
			if !ok {
				continue
			}
			matchedAny = true
			if !seen[tid] {
				seen[tid] = true
				out = append(out, tid)
			}
		}
		if !matchedAny {
			for _, tid := range AllTargets() {
				if ok, _ := path.Match(pat, string(tid)); ok {
					return nil, fmt.Errorf("target pattern %q matched no configured targets", raw)
				}
			}
			return nil, fmt.Errorf("target pattern %q matched no valid targets", raw)
		}
	}
	return out, nil
}
