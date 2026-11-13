package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"twdll/cli/internal/term"
	"twdll/cli/internal/workspace"
	"twdll/cli/internal/worktree"
)

type WTListRequest struct {
	Layout workspace.Layout
	JSON   bool
	UI     *term.UI
}

type WTListEntry struct {
	Path      string               `json:"path"`
	Branch    string               `json:"branch"`
	Head      string               `json:"head"`
	IsPrimary bool                 `json:"is_primary"`
	Detached  bool                 `json:"detached"`
	Locked    bool                 `json:"locked"`
	Prunable  bool                 `json:"prunable"`
	Status    *worktree.LinkStatus `json:"status,omitempty"`
}

type WTListReport struct {
	PrimaryRoot    string        `json:"primary_root"`
	PrimaryDataDir string        `json:"primary_data_dir"`
	Worktrees      []WTListEntry `json:"worktrees"`
}

func RunWTList(ctx context.Context, req WTListRequest) (*WTListReport, error) {
	list, err := worktree.List(ctx, req.Layout.Paths.Root)
	if err != nil {
		return nil, err
	}
	primary := list[0]

	primaryLayout, err := worktree.ResolvePrimaryLayout(primary.Path)
	if err != nil {
		return nil, err
	}

	primaryRoot := filepath.Clean(primary.Path)
	primaryDataDir := filepath.Clean(primaryLayout.Paths.DataDir)

	entries := make([]WTListEntry, 0, len(list))
	for _, wt := range list {
		entry := WTListEntry{
			Path:      filepath.Clean(wt.Path),
			Branch:    wt.Branch,
			Head:      wt.Head,
			IsPrimary: wt.IsPrimary,
			Detached:  wt.Detached,
			Locked:    wt.Locked,
			Prunable:  wt.Prunable,
		}
		if !wt.IsPrimary {
			st := worktree.Inspect(wt.Path, primaryDataDir)
			entry.Status = &st
		}
		entries = append(entries, entry)
	}

	report := &WTListReport{
		PrimaryRoot:    primaryRoot,
		PrimaryDataDir: primaryDataDir,
		Worktrees:      entries,
	}

	if req.UI != nil {
		if req.JSON {
			data, err := json.MarshalIndent(report, "", "  ")
			if err != nil {
				return nil, fmt.Errorf("marshal wt list json: %w", err)
			}
			req.UI.WriteData("%s\n", string(data))
		} else {
			renderHumanWTList(report, req.UI)
		}
	}

	return report, nil
}

func renderHumanWTList(r *WTListReport, ui *term.UI) {
	st := ui.OutStyle()
	var b strings.Builder

	const (
		colBranch = 24
		colHead   = 10
		colStatus = 18
	)

	fmt.Fprintf(&b, "%s%s%s%s\n",
		st.Bold(padRight("BRANCH", colBranch)),
		st.Bold(padRight("HEAD", colHead)),
		st.Bold(padRight("STATUS", colStatus)),
		st.Bold("PATH"),
	)

	for _, wt := range r.Worktrees {
		branchLabel := wt.Branch
		if branchLabel == "" {
			if wt.Detached {
				branchLabel = "(detached)"
			} else {
				branchLabel = "-"
			}
		}

		headShort := wt.Head
		if len(headShort) > 7 {
			headShort = headShort[:7]
		}

		rawStatus := "unlinked"
		styledStatus := st.Yellow(rawStatus)
		switch {
		case wt.IsPrimary:
			rawStatus = "primary"
			styledStatus = st.Cyan(rawStatus)
		case wt.Prunable:
			rawStatus = "prunable"
			styledStatus = st.Red(rawStatus)
		case wt.Status != nil && wt.Status.Ready:
			rawStatus = "ready"
			styledStatus = st.Green(rawStatus)
		}
		if wt.Locked {
			rawStatus += " [locked]"
			styledStatus += " " + st.Yellow("[locked]")
		}

		statusPad := ""
		if colStatus > len(rawStatus) {
			statusPad = strings.Repeat(" ", colStatus-len(rawStatus))
		}

		fmt.Fprintf(&b, "%s%s%s%s%s\n",
			padRight(branchLabel, colBranch),
			st.Dim(padRight(headShort, colHead)),
			styledStatus,
			statusPad,
			wt.Path,
		)
	}

	ui.WriteData("%s", b.String())
}

type WTAddOptions struct {
	Name   string
	Base   string
	Path   string
	Detach bool
	Force  bool
	JSON   bool
}

type WTAddRequest struct {
	Layout  workspace.Layout
	Options WTAddOptions
	UI      *term.UI
}

func RunWTAdd(ctx context.Context, req WTAddRequest) (worktree.AddResult, error) {
	res, err := worktree.Add(ctx, worktree.AddOptions{
		RepoDir: req.Layout.Paths.Root,
		Name:    req.Options.Name,
		Base:    req.Options.Base,
		Path:    req.Options.Path,
		Detach:  req.Options.Detach,
		Force:   req.Options.Force,
	})
	if err != nil {
		return worktree.AddResult{}, err
	}

	cleanPath := filepath.Clean(res.Worktree.Path)
	res.Worktree.Path = cleanPath
	res.PrimaryRoot = filepath.Clean(res.PrimaryRoot)

	if req.UI != nil {
		if req.Options.JSON {
			data, err := json.MarshalIndent(res, "", "  ")
			if err != nil {
				return res, fmt.Errorf("marshal wt add json: %w", err)
			}
			req.UI.WriteData("%s\n", string(data))
		} else {
			req.UI.Success("Created linked worktree at %s", cleanPath)
			req.UI.WriteData("%s\n", cleanPath)
		}
	}

	return res, nil
}

type WTRemoveOptions struct {
	Target       string
	Force        bool
	DeleteBranch bool
	JSON         bool
	CWD          string
	ExePath      string
}

type WTRemoveRequest struct {
	Layout  workspace.Layout
	Options WTRemoveOptions
	UI      *term.UI
}

func RunWTRemove(ctx context.Context, req WTRemoveRequest) (worktree.RemoveResult, error) {
	res, err := worktree.Remove(ctx, worktree.RemoveOptions{
		RepoDir:      req.Layout.Paths.Root,
		Target:       req.Options.Target,
		CWD:          req.Options.CWD,
		ExePath:      req.Options.ExePath,
		Force:        req.Options.Force,
		DeleteBranch: req.Options.DeleteBranch,
	})
	if err != nil {
		return res, err
	}

	cleanPath := filepath.Clean(res.Removed.Path)
	res.Removed.Path = cleanPath

	if req.UI != nil {
		if req.Options.JSON {
			data, err := json.MarshalIndent(res, "", "  ")
			if err != nil {
				return res, fmt.Errorf("marshal wt remove json: %w", err)
			}
			req.UI.WriteData("%s\n", string(data))
		} else {
			if res.DeletedBranch != "" {
				req.UI.Success("Removed worktree %s (deleted branch %s)", cleanPath, res.DeletedBranch)
			} else {
				req.UI.Success("Removed worktree %s", cleanPath)
			}
		}
	}

	return res, nil
}

type WTSyncOptions struct {
	Target string
	All    bool
	Force  bool
	JSON   bool
	CWD    string
}

type WTSyncRequest struct {
	Layout  workspace.Layout
	Options WTSyncOptions
	UI      *term.UI
}

func RunWTSync(ctx context.Context, req WTSyncRequest) (worktree.SyncResult, error) {
	res, err := worktree.Sync(ctx, worktree.SyncOptions{
		RepoDir: req.Layout.Paths.Root,
		Target:  req.Options.Target,
		CWD:     req.Options.CWD,
		All:     req.Options.All,
		Force:   req.Options.Force,
	})
	if err != nil {
		return res, err
	}

	res.PrimaryRoot = filepath.Clean(res.PrimaryRoot)
	for i := range res.Synced {
		res.Synced[i].Worktree.Path = filepath.Clean(res.Synced[i].Worktree.Path)
	}

	if req.UI != nil {
		if req.Options.JSON {
			data, err := json.MarshalIndent(res, "", "  ")
			if err != nil {
				return res, fmt.Errorf("marshal wt sync json: %w", err)
			}
			req.UI.WriteData("%s\n", string(data))
		} else {
			if len(res.Synced) == 1 {
				req.UI.Success("Synced worktree %s", res.Synced[0].Worktree.Path)
			} else {
				req.UI.Success("Synced %d worktree(s).", len(res.Synced))
			}
		}
	}

	return res, nil
}

type WTPathOptions struct {
	Target string
	CWD    string
}

type WTPathRequest struct {
	Layout  workspace.Layout
	Options WTPathOptions
	UI      *term.UI
}

func RunWTPath(ctx context.Context, req WTPathRequest) (string, error) {
	list, err := worktree.List(ctx, req.Layout.Paths.Root)
	if err != nil {
		return "", err
	}
	wt, err := worktree.ResolveTarget(list, req.Options.Target, req.Options.CWD)
	if err != nil {
		return "", err
	}

	cleanPath := filepath.Clean(wt.Path)
	if req.UI != nil {
		req.UI.WriteData("%s\n", cleanPath)
	}
	return cleanPath, nil
}
