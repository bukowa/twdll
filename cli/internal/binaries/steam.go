package binaries

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"twdll/cli/internal/workspace"
)

const (
	AttilaAppID      = "325610"
	AttilaDepotID    = "325624"
	Attila64Manifest = "3804137149572368482"

	Rome2AppID = "214950"
	ToBAppID   = "712100"
)

// AppIDForGame returns the numeric Steam AppID for the given game.
func AppIDForGame(game workspace.GameID) int {
	switch game {
	case workspace.GameAttila:
		return 325610
	case workspace.GameRome2:
		return 214950
	case workspace.GameToB:
		return 712100
	default:
		return 0
	}
}

// App represents an installed Steam application parsed from its appmanifest_<id>.acf.
type App struct {
	AppID      int
	Name       string
	InstallDir string
	StateFlags int
}

// IsInstalled returns whether the app is fully installed and valid according to EAppState.
func (a *App) IsInstalled() bool {
	// StateFullyInstalled = 4, StateUninstalling = 2048, StateFilesMissing = 32, StateFilesCorrupt = 128
	return (a.StateFlags&4 != 0) && (a.StateFlags&2048 == 0) && (a.StateFlags&(32|128) == 0)
}

// Library represents a Steam library containing installed games.
type Library struct {
	Path   string
	AppIDs []int
}

// ResolveAppDir returns the directory where the game's files are installed.
func (l *Library) ResolveAppDir(app *App) string {
	return filepath.Join(l.Path, "steamapps", "common", app.InstallDir)
}

// App attempts to return the App metadata for appID from this library's appmanifest_<id>.acf.
func (l *Library) App(appID int) (*App, error) {
	manifestPath := filepath.Join(l.Path, "steamapps", fmt.Sprintf("appmanifest_%d.acf", appID))
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, err
	}

	root, err := ParseVDF(string(data))
	if err != nil {
		return nil, fmt.Errorf("parse manifest %s: %w", manifestPath, err)
	}

	appState := root.Get("appstate")
	if appState == nil {
		return nil, fmt.Errorf("missing appstate in %s", manifestPath)
	}

	return &App{
		AppID:      appState.GetInt("appid", appID),
		Name:       appState.GetString("name", ""),
		InstallDir: appState.GetString("installdir", ""),
		StateFlags: appState.GetInt("stateflags", 0),
	}, nil
}

// SteamDir represents the root Steam client installation.
type SteamDir struct {
	Path string
}

// Locate discovers the Steam client installation directory.
func Locate() (*SteamDir, error) {
	// 1. Check environment variables
	if env := os.Getenv("STEAM_DIR"); env != "" && isValidSteamDir(env) {
		return &SteamDir{Path: filepath.Clean(env)}, nil
	}
	if env := os.Getenv("STEAM_PATH"); env != "" && isValidSteamDir(env) {
		return &SteamDir{Path: filepath.Clean(env)}, nil
	}

	// 2. Platform well-known filesystem paths (mirrors steamlocate-rs)
	for _, c := range candidateSteamDirs() {
		if isValidSteamDir(c) {
			return &SteamDir{Path: filepath.Clean(c)}, nil
		}
	}

	return nil, fmt.Errorf("steam installation directory not found")
}

// Libraries returns all steam library folders registered in steamapps/libraryfolders.vdf.
func (s *SteamDir) Libraries() ([]Library, error) {
	vdfPath := filepath.Join(s.Path, "steamapps", "libraryfolders.vdf")
	data, err := os.ReadFile(vdfPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", vdfPath, err)
	}

	root, err := ParseVDF(string(data))
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", vdfPath, err)
	}

	lfNode := root.Get("libraryfolders")
	if lfNode == nil {
		lfNode = root.Get("LibraryFolders")
	}
	if lfNode == nil {
		return nil, fmt.Errorf("missing libraryfolders node in %s", vdfPath)
	}

	var libraries []Library
	seen := make(map[string]bool)

	// Iterate numeric library nodes in parsed order
	for _, child := range lfNode.ChildrenOrdered() {
		if _, err := strconv.Atoi(child.Name); err != nil {
			continue
		}

		path := child.GetString("path", "")
		if path == "" && child.Value != "" {
			path = child.Value // Legacy scalar path
		}
		if path == "" {
			continue
		}

		cleanPath := filepath.Clean(filepath.FromSlash(path))
		if seen[strings.ToLower(cleanPath)] {
			continue
		}

		steamapps := filepath.Join(cleanPath, "steamapps")
		entries, err := os.ReadDir(steamapps)
		if err != nil {
			continue
		}

		var appIDs []int
		for _, entry := range entries {
			name := entry.Name()
			if strings.HasPrefix(name, "appmanifest_") && strings.HasSuffix(name, ".acf") {
				idStr := strings.TrimSuffix(strings.TrimPrefix(name, "appmanifest_"), ".acf")
				if id, err := strconv.Atoi(idStr); err == nil {
					appIDs = append(appIDs, id)
				}
			}
		}

		libraries = append(libraries, Library{
			Path:   cleanPath,
			AppIDs: appIDs,
		})
		seen[strings.ToLower(cleanPath)] = true
	}

	return libraries, nil
}

// FindApp locates an installed app by ID across all Steam libraries.
func (s *SteamDir) FindApp(appID int) (*App, *Library, error) {
	libs, err := s.Libraries()
	if err != nil {
		return nil, nil, err
	}

	for i := range libs {
		lib := &libs[i]
		for _, id := range lib.AppIDs {
			if id == appID {
				app, err := lib.App(appID)
				if err == nil && app != nil {
					return app, lib, nil
				}
			}
		}
	}

	return nil, nil, fmt.Errorf("app %d not found in any steam library", appID)
}

// FindGameDir locates an installed Total War game and returns its directory.
func (s *SteamDir) FindGameDir(game workspace.GameID) (string, bool) {
	appID := AppIDForGame(game)
	if appID == 0 {
		return "", false
	}

	app, lib, err := s.FindApp(appID)
	if err == nil && app != nil && app.IsInstalled() {
		dir := lib.ResolveAppDir(app)
		if _, err := os.Stat(filepath.Join(dir, "empire.retail.dll")); err == nil {
			return filepath.ToSlash(dir), true
		}
	}
	return "", false
}

// FindAttila64Depot checks the canonical console depot location at <SteamRoot>/steamapps/content/app_325610/depot_325624/Attila.
func (s *SteamDir) FindAttila64Depot() (string, bool) {
	canonical := filepath.Join(s.Path, "steamapps", "content", "app_"+AttilaAppID, "depot_"+AttilaDepotID, "Attila")
	if _, err := os.Stat(canonical); err == nil {
		return filepath.ToSlash(canonical), true
	}
	return "", false
}

// DepotCommand returns the Steam console download command.
func DepotCommand() string {
	return fmt.Sprintf("download_depot %s %s %s", AttilaAppID, AttilaDepotID, Attila64Manifest)
}

// TriggerDepotDownload copies the console command to the clipboard and opens steam://nav/console.
func TriggerDepotDownload() error {
	cmdText := DepotCommand()
	if runtime.GOOS == "windows" {
		_ = exec.Command("cmd.exe", "/c", "echo|set /p="+cmdText+"|clip").Run()
		_ = exec.Command("cmd.exe", "/c", "start", "steam://nav/console").Run()
	}
	return nil
}

func isValidSteamDir(path string) bool {
	vdf := filepath.Join(path, "steamapps", "libraryfolders.vdf")
	info, err := os.Stat(vdf)
	return err == nil && !info.IsDir()
}

func candidateSteamDirs() []string {
	var candidates []string
	if runtime.GOOS == "windows" {
		if p := os.Getenv("ProgramFiles(x86)"); p != "" {
			candidates = append(candidates, filepath.Join(p, "Steam"))
		}
		if p := os.Getenv("ProgramFiles"); p != "" {
			candidates = append(candidates, filepath.Join(p, "Steam"))
		}
		if p := os.Getenv("LocalAppData"); p != "" {
			candidates = append(candidates, filepath.Join(p, "Programs", "Steam"))
		}
		candidates = append(candidates, `C:\Steam`, `D:\Steam`, `C:\SteamLibrary`, `D:\SteamLibrary`)
		for d := 'C'; d <= 'Z'; d++ {
			candidates = append(candidates, string(d)+`:\Steam`, string(d)+`:\SteamLibrary`)
		}
	} else {
		home, _ := os.UserHomeDir()
		candidates = []string{
			filepath.Join(home, ".local", "share", "Steam"),
			filepath.Join(home, ".steam", "steam"),
			filepath.Join(home, ".steam", "root"),
			filepath.Join(home, ".steam", "debian-installation"),
			filepath.Join(home, ".var", "app", "com.valvesoftware.Steam", "data", "Steam"),
		}
	}
	return candidates
}
