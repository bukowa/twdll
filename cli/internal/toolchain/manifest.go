package toolchain

import (
	"runtime"
	"twdll/cli/internal/workspace"
)

// StandardToolOrder defines the deterministic build toolchain presentation order.
var StandardToolOrder = []workspace.ToolID{
	workspace.ToolCMake,
	workspace.ToolNinja,
	workspace.ToolXwin,
	workspace.ToolRPFM,
}

// ToolDef specifies download source and installation layout for a tool.
type ToolDef struct {
	ID          workspace.ToolID
	Name        string
	Version     string
	URL         string
	SHA256      string // optional pinned sha256
	ArchiveType string // "zip", "tar.gz", "exe"
	StripDirs   int    // strip N leading path segments when extracting
	MarkerRel   string // relative path inside .twdll/tools/<id>/ that marks tool as installed
}

// GetWindowsManifest returns the latest pinned portable tools for 64-bit Windows.
func GetWindowsManifest() map[workspace.ToolID]ToolDef {
	return map[workspace.ToolID]ToolDef{
		workspace.ToolCMake: {
			ID:          workspace.ToolCMake,
			Name:        "CMake",
			Version:     "4.4.5",
			URL:         "https://github.com/Kitware/CMake/releases/download/v4.4.5/cmake-4.4.5-windows-x86_64.zip",
			ArchiveType: "zip",
			StripDirs:   1, // cmake-4.4.5-windows-x86_64/
			MarkerRel:   "bin/cmake.exe",
		},
		workspace.ToolNinja: {
			ID:          workspace.ToolNinja,
			Name:        "Ninja",
			Version:     "1.13.2",
			URL:         "https://github.com/ninja-build/ninja/releases/download/v1.13.2/ninja-win.zip",
			ArchiveType: "zip",
			StripDirs:   0,
			MarkerRel:   "ninja.exe",
		},
		workspace.ToolXwin: {
			ID:          workspace.ToolXwin,
			Name:        "xwin",
			Version:     "0.10.0",
			URL:         "https://github.com/Jake-Shadle/xwin/releases/download/0.10.0/xwin-0.10.0-x86_64-pc-windows-msvc.tar.gz",
			ArchiveType: "tar.gz",
			StripDirs:   1, // xwin-0.10.0-x86_64-pc-windows-msvc/
			MarkerRel:   "xwin.exe",
		},
		workspace.ToolRPFM: {
			ID:          workspace.ToolRPFM,
			Name:        "RPFM CLI",
			Version:     "4.7.4",
			URL:         "https://github.com/Frodo45127/rpfm/releases/download/v4.7.4/rpfm-v4.7.4-x86_64-pc-windows-msvc.zip",
			ArchiveType: "zip",
			StripDirs:   0,
			MarkerRel:   "rpfm_cli.exe",
		},
		workspace.ToolJDK: {
			ID:          workspace.ToolJDK,
			Name:        "Adoptium OpenJDK",
			Version:     "21.0.13+9",
			URL:         "https://github.com/adoptium/temurin21-binaries/releases/download/jdk-21.0.13%2B9/OpenJDK21U-jdk_x64_windows_hotspot_21.0.13_9.zip",
			ArchiveType: "zip",
			StripDirs:   1, // jdk-21.0.13+9/
			MarkerRel:   "bin/java.exe",
		},
		workspace.ToolGhidra: {
			ID:          workspace.ToolGhidra,
			Name:        "Ghidra",
			Version:     "12.2",
			URL:         "https://github.com/NationalSecurityAgency/ghidra/releases/download/Ghidra_12.2_build/ghidra_12.2_PUBLIC_20261016.zip",
			ArchiveType: "zip",
			StripDirs:   1, // ghidra_12.2_PUBLIC/
			MarkerRel:   "support/analyzeHeadless.bat",
		},
	}
}

// CurrentPlatformManifest returns the tool definitions for the host operating system.
func CurrentPlatformManifest() map[workspace.ToolID]ToolDef {
	if runtime.GOOS == "windows" {
		return GetWindowsManifest()
	}
	// Linux definitions placeholder
	return map[workspace.ToolID]ToolDef{}
}
