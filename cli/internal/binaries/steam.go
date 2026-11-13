package binaries

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"twdll/cli/internal/workspace"
)

const (
	AttilaAppID      = "325610"
	AttilaDepotID    = "325624"
	Attila64Manifest = "3804137149572368482"
	Rome2AppID       = "214950"
	ToBAppID         = "712100"
)

var ErrSteamNotFound = errors.New("steam installation not found")

type SteamDir struct {
	Path string
}

func LocateSteam() (*SteamDir, error) {
	for _, candidate := range steamCandidatePaths() {
		if candidate == "" {
			continue
		}
		clean := filepath.Clean(candidate)
		vdfPath := filepath.Join(clean, "steamapps", "libraryfolders.vdf")
		if info, err := os.Stat(vdfPath); err == nil && !info.IsDir() {
			return &SteamDir{Path: filepath.ToSlash(clean)}, nil
		}
	}
	return nil, ErrSteamNotFound
}

func steamCandidatePaths() []string {
	candidates := []string{
		strings.TrimSpace(os.Getenv("STEAM_DIR")),
		strings.TrimSpace(os.Getenv("STEAM_PATH")),
	}
	if pf86 := os.Getenv("ProgramFiles(x86)"); pf86 != "" {
		candidates = append(candidates, filepath.Join(pf86, "Steam"))
	}
	if pf := os.Getenv("ProgramFiles"); pf != "" {
		candidates = append(candidates, filepath.Join(pf, "Steam"))
	}
	candidates = append(candidates,
		`C:\Program Files (x86)\Steam`,
		`C:\Program Files\Steam`,
		`C:\Steam`,
		`D:\Steam`,
		`E:\Steam`,
		`G:\Steam`,
	)
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		candidates = append(candidates,
			filepath.Join(home, ".steam", "steam"),
			filepath.Join(home, ".local", "share", "Steam"),
			filepath.Join(home, "Library", "Application Support", "Steam"),
		)
	}
	return candidates
}

func (s *SteamDir) FindGameDir(game workspace.GameID) (string, bool) {
	if s == nil || s.Path == "" {
		return "", false
	}
	appID := gameAppID(game)
	if appID == "" {
		return "", false
	}
	for _, libDir := range s.libraryDirs() {
		dir, ok := resolveInstalledGameInLibrary(libDir, appID)
		if ok {
			return dir, true
		}
	}
	return "", false
}

func (s *SteamDir) FindAttila64Depot() (string, bool) {
	if s == nil || s.Path == "" {
		return "", false
	}
	depotBin := filepath.Join(
		filepath.FromSlash(s.Path),
		"steamapps",
		"content",
		"app_"+AttilaAppID,
		"depot_"+AttilaDepotID,
		"Attila",
	)
	if info, err := os.Stat(depotBin); err == nil && !info.IsDir() {
		return filepath.ToSlash(depotBin), true
	}
	return "", false
}

func DepotCommand() string {
	return fmt.Sprintf("download_depot %s %s %s", AttilaAppID, AttilaDepotID, Attila64Manifest)
}

func OpenSteamDepotConsole() error {
	if runtime.GOOS != "windows" {
		return errors.New("steam console automation is only supported on Windows")
	}
	clipCmd := exec.Command("cmd.exe", "/c", "clip")
	clipCmd.Stdin = strings.NewReader(DepotCommand())
	if err := clipCmd.Run(); err != nil {
		return fmt.Errorf("copy depot command to clipboard: %w", err)
	}
	openCmd := exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", "steam://nav/console")
	if err := openCmd.Run(); err != nil {
		return fmt.Errorf("open steam console: %w", err)
	}
	return nil
}

func (s *SteamDir) libraryDirs() []string {
	root := filepath.FromSlash(s.Path)
	seen := map[string]bool{strings.ToLower(filepath.Clean(root)): true}
	dirs := []string{root}

	vdfPath := filepath.Join(root, "steamapps", "libraryfolders.vdf")
	data, err := os.ReadFile(vdfPath)
	if err != nil {
		return dirs
	}
	parsed, err := ParseVDF(string(data))
	if err != nil {
		return dirs
	}
	lf := parsed.Get("libraryfolders")
	if lf == nil {
		return dirs
	}
	for _, entry := range lf.ChildrenOrdered() {
		p := entry.GetString("path", entry.Value)
		if p == "" {
			continue
		}
		clean := filepath.Clean(filepath.FromSlash(p))
		key := strings.ToLower(clean)
		if !seen[key] {
			seen[key] = true
			dirs = append(dirs, clean)
		}
	}
	return dirs
}

func resolveInstalledGameInLibrary(libDir, appID string) (string, bool) {
	manifestPath := filepath.Join(libDir, "steamapps", "appmanifest_"+appID+".acf")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return "", false
	}
	parsed, err := ParseVDF(string(data))
	if err != nil {
		return "", false
	}
	appState := parsed.Get("AppState")
	if appState == nil {
		return "", false
	}
	flags := appState.GetInt("StateFlags", 0)
	// StateFlags: 4 = FullyInstalled, 2048 = Uninstalling, 32|128 = UpdatePaused/Corrupt.
	if flags&4 == 0 || flags&2048 != 0 || flags&(32|128) != 0 {
		return "", false
	}
	installDirName := appState.GetString("installdir", "")
	if installDirName == "" {
		return "", false
	}
	gameDir := filepath.Join(libDir, "steamapps", "common", installDirName)
	dllPath := filepath.Join(gameDir, "empire.retail.dll")
	if info, err := os.Stat(dllPath); err == nil && !info.IsDir() {
		return filepath.ToSlash(filepath.Clean(gameDir)), true
	}
	return "", false
}

func gameAppID(game workspace.GameID) string {
	switch game {
	case workspace.GameAttila:
		return AttilaAppID
	case workspace.GameRome2:
		return Rome2AppID
	case workspace.GameToB:
		return ToBAppID
	default:
		return ""
	}
}
