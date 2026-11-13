package toolchain

import (
	"runtime"

	"twdll/cli/internal/workspace"
)

var BuildToolOrder = []workspace.ToolID{
	workspace.ToolCMake,
	workspace.ToolNinja,
	workspace.ToolLLVM,
	workspace.ToolXwin,
	workspace.ToolRPFM,
}

type ToolDef struct {
	ID          workspace.ToolID
	Name        string
	Version     string
	URL         string
	SHA256      string
	ArchiveType string
	StripDirs   int
	MarkerRel   string
}

func WindowsManifest() map[workspace.ToolID]ToolDef {
	return map[workspace.ToolID]ToolDef{
		workspace.ToolCMake: {
			ID:          workspace.ToolCMake,
			Name:        "CMake",
			Version:     "4.4.5",
			URL:         "https://github.com/Kitware/CMake/releases/download/v4.4.5/cmake-4.4.5-windows-x86_64.zip",
			SHA256:      "326c826486d46a23cf9d5a1c9ee3a567d427bdd02c4a34071937a2a675cff004",
			ArchiveType: "zip",
			StripDirs:   1,
			MarkerRel:   "bin/cmake.exe",
		},
		workspace.ToolNinja: {
			ID:          workspace.ToolNinja,
			Name:        "Ninja",
			Version:     "1.13.2",
			URL:         "https://github.com/ninja-build/ninja/releases/download/v1.13.2/ninja-win.zip",
			SHA256:      "07fc8261b42b20e71d1720b39068c2e14ffcee6396b76fb7a795fb460b78dc65",
			ArchiveType: "zip",
			StripDirs:   0,
			MarkerRel:   "ninja.exe",
		},
		workspace.ToolLLVM: {
			ID:          workspace.ToolLLVM,
			Name:        "LLVM / Clang",
			Version:     "23.1.5",
			URL:         "https://github.com/llvm/llvm-project/releases/download/llvmorg-23.1.5/clang%2Bllvm-23.1.5-x86_64-pc-windows-msvc.tar.xz",
			SHA256:      "62fdada7f6fa2b73856486dfbbdba3d2c1882991621615bcb55c60207611c6d4",
			ArchiveType: "tar.xz",
			StripDirs:   1,
			MarkerRel:   "bin/clang-cl.exe",
		},
		workspace.ToolXwin: {
			ID:          workspace.ToolXwin,
			Name:        "xwin",
			Version:     "0.10.0",
			URL:         "https://github.com/Jake-Shadle/xwin/releases/download/0.10.0/xwin-0.10.0-x86_64-pc-windows-msvc.tar.gz",
			SHA256:      "96e83665ef5c1406aabd0be603a8d08e67e604f36fda630b6807ee948a614e26",
			ArchiveType: "tar.gz",
			StripDirs:   1,
			MarkerRel:   "xwin.exe",
		},
		workspace.ToolRPFM: {
			ID:          workspace.ToolRPFM,
			Name:        "RPFM CLI",
			Version:     "4.7.4",
			URL:         "https://github.com/Frodo45127/rpfm/releases/download/v4.7.4/rpfm-v4.7.4-x86_64-pc-windows-msvc.zip",
			SHA256:      "6cebf042d5416ab6c8e871d9cb6ae4cd0f1a4d76f69e35932ace06591eec3b2f",
			ArchiveType: "zip",
			StripDirs:   0,
			MarkerRel:   "rpfm_cli.exe",
		},
		workspace.ToolJDK: {
			ID:          workspace.ToolJDK,
			Name:        "Adoptium OpenJDK",
			Version:     "25+36",
			URL:         "https://github.com/adoptium/temurin25-binaries/releases/download/jdk-25%2B36/OpenJDK25U-jdk_x64_windows_hotspot_25_36.zip",
			SHA256:      "ff76428a600a0925abbfb1dc55da30828b500f8038a13679a8c75b37892a96c3",
			ArchiveType: "zip",
			StripDirs:   1,
			MarkerRel:   "bin/java.exe",
		},
		workspace.ToolGhidra: {
			ID:          workspace.ToolGhidra,
			Name:        "Ghidra",
			Version:     "12.2",
			URL:         "https://github.com/NationalSecurityAgency/ghidra/releases/download/Ghidra_12.2_build/ghidra_12.2_PUBLIC_20261016.zip",
			SHA256:      "2c2cd776db3c1f6a097ba418922378ac3649cadd5e844a6f5f088ae628a7be67",
			ArchiveType: "zip",
			StripDirs:   1,
			MarkerRel:   "support/analyzeHeadless.bat",
		},
	}
}

func HostManifest() map[workspace.ToolID]ToolDef {
	if runtime.GOOS == "windows" {
		return WindowsManifest()
	}
	return map[workspace.ToolID]ToolDef{}
}
