package silver

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"twdll/cli/internal/bronze"
	"twdll/cli/internal/extractdb"
	"twdll/cli/internal/workspace"
)

const DefaultFastTimeoutSecs = 15

type SilverOptions struct {
	Paths        workspace.Paths
	Target       workspace.TargetID
	Engine       workspace.DecompilerID
	IDADir       string
	BinaryPath   string
	BinaryDBPath string
	ManifestPath string
	MaxMem       string
	TmpBaseDir   string
}

type DecompOptions struct {
	Paths         workspace.Paths
	Target        workspace.TargetID
	Engine        workspace.DecompilerID
	IDADir        string
	BinaryDBPath  string
	DecompDBPath  string
	TimeoutSecs   int
	NoTimeout     bool
	Workers       int
	BatchSize     int
	Force         bool
	RetryFailed   bool
	RetryTimeouts bool
	MaxMem        string
	TmpBaseDir    string
	FilterSpecs   []extractdb.FuncSpec
	OnProgress    func(done, total int)
}

type DecompPassResult struct {
	Target          workspace.TargetID
	Engine          workspace.DecompilerID
	Resume          extractdb.ResumeStats
	Written         int
	StatusCounts    map[string]int
	Elapsed         time.Duration
	PeakJobMemBytes uint64
}

type silverJSONLRecord struct {
	EA            flexUint64   `json:"ea"`
	Size          flexUint64   `json:"size"`
	Ranges        [][2]flexUint64 `json:"ranges"`
	Name          string       `json:"name"`
	CallingConv   string       `json:"calling_conv"`
	StackPurge    int          `json:"stack_purge"`
	IsThunk       bool         `json:"is_thunk"`
	IsNoRet       bool         `json:"is_noret"`
	ThunkTargetEA flexUint64   `json:"thunk_target_ea"`
}

func (r silverJSONLRecord) toBronze() bronze.SilverFunc {
	ranges := make([]bronze.SilverRange, 0, len(r.Ranges))
	for _, pair := range r.Ranges {
		ranges = append(ranges, bronze.SilverRange{
			StartEA: uint64(pair[0]),
			EndEA:   uint64(pair[1]),
		})
	}
	return bronze.SilverFunc{
		EA:            uint64(r.EA),
		Size:          uint64(r.Size),
		Ranges:        ranges,
		Name:          r.Name,
		CallingConv:   r.CallingConv,
		StackPurge:    r.StackPurge,
		IsThunk:       r.IsThunk,
		IsNoRet:       r.IsNoRet,
		ThunkTargetEA: uint64(r.ThunkTargetEA),
	}
}

type decompFrameJSON struct {
	Event     string              `json:"event"`
	EA        flexUint64          `json:"ea"`
	Size      flexUint64          `json:"size"`
	Name      string              `json:"name"`
	Status    string              `json:"status"`
	ElapsedMs int64               `json:"elapsed_ms"`
	RawC      string              `json:"raw_c"`
	LineEA    []json.RawMessage   `json:"line_ea"`
	Vars      []extractdb.DecompVar `json:"vars"`
	Error     string              `json:"error"`
}

func (f decompFrameJSON) toDecompRecord(engine workspace.DecompilerID) extractdb.DecompRecord {
	status := f.Status
	switch status {
	case extractdb.DecompStatusOK, extractdb.DecompStatusTimeout, extractdb.DecompStatusFailed:
	default:
		status = extractdb.DecompStatusFailed
	}

	rawC := NormalizeLineEndings(f.RawC)
	lineEAJSON := "[]"
	if len(f.LineEA) > 0 {
		if b, err := json.Marshal(f.LineEA); err == nil {
			lineEAJSON = string(b)
		}
	}

	vars := make([]extractdb.DecompVar, len(f.Vars))
	for i, v := range f.Vars {
		v.Storage = NormalizeStorage(v.Storage)
		vars[i] = v
	}
	varsJSON := "[]"
	if len(vars) > 0 {
		if b, err := json.Marshal(vars); err == nil {
			varsJSON = string(b)
		}
	}

	return extractdb.DecompRecord{
		FuncEA:             uint64(f.EA),
		Engine:             string(engine),
		FuncSize:           uint64(f.Size),
		FuncName:           f.Name,
		RawC:               rawC,
		LineEAJSON:         lineEAJSON,
		DecompilerVarsJSON: varsJSON,
		Status:             status,
		ErrorText:          f.Error,
		ElapsedMs:          f.ElapsedMs,
	}
}

// flexUint64 accepts either JSON integer numbers or "0x..." hex strings.
type flexUint64 uint64

func (f *flexUint64) UnmarshalJSON(data []byte) error {
	s := strings.TrimSpace(string(data))
	if s == "" || s == "null" {
		*f = 0
		return nil
	}
	if strings.HasPrefix(s, "\"") && strings.HasSuffix(s, "\"") {
		unq, err := strconv.Unquote(s)
		if err != nil {
			return err
		}
		v, err := strconv.ParseUint(strings.TrimSpace(unq), 0, 64)
		if err != nil {
			return fmt.Errorf("parse flexUint64 %q: %w", unq, err)
		}
		*f = flexUint64(v)
		return nil
	}
	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return fmt.Errorf("parse flexUint64 %s: %w", s, err)
	}
	*f = flexUint64(v)
	return nil
}

// NormalizeLineEndings normalizes CRLF/CR to LF so line_ea_json line numbers match strings.Split(raw_c, "\n").
func NormalizeLineEndings(raw string) string {
	if !strings.ContainsRune(raw, '\r') {
		return raw
	}
	return strings.ReplaceAll(strings.ReplaceAll(raw, "\r\n", "\n"), "\r", "\n")
}

// NormalizeStorage maps Ghidra VariableStorage and IDA lvar strings into canonical
// "reg:<lower>" or "stack:<+d>" forms where applicable.
func NormalizeStorage(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	low := strings.ToLower(s)
	if strings.HasPrefix(low, "reg:") {
		return "reg:" + strings.TrimSpace(low[4:])
	}
	if strings.HasPrefix(low, "stack:") {
		if v, err := strconv.ParseInt(strings.TrimSpace(low[6:]), 0, 64); err == nil {
			return fmt.Sprintf("stack:%+d", v)
		}
		return "stack:" + strings.TrimSpace(low[6:])
	}
	if strings.HasPrefix(s, "Stack[") {
		inner := strings.TrimPrefix(s, "Stack[")
		if end := strings.IndexByte(inner, ']'); end >= 0 {
			inner = inner[:end]
		}
		if v, err := strconv.ParseInt(strings.TrimSpace(inner), 0, 64); err == nil {
			return fmt.Sprintf("stack:%+d", v)
		}
	}
	if !strings.ContainsAny(s, " [()") && strings.Contains(s, ":") {
		regPart, sizePart, ok := strings.Cut(s, ":")
		if ok && regPart != "" {
			if _, err := strconv.Atoi(sizePart); err == nil {
				return "reg:" + strings.ToLower(regPart)
			}
		}
	}
	return s
}

func BuildWorkerShards(pending []extractdb.FuncSpec, workers int) [][]extractdb.FuncSpec {
	if len(pending) == 0 {
		return nil
	}
	if workers < 1 {
		workers = 1
	}
	if workers > len(pending) {
		workers = len(pending)
	}

	ordered := make([]extractdb.FuncSpec, len(pending))
	copy(ordered, pending)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Size != ordered[j].Size {
			return ordered[i].Size > ordered[j].Size
		}
		return ordered[i].EA < ordered[j].EA
	})

	shards := make([][]extractdb.FuncSpec, workers)
	for i, spec := range ordered {
		shards[i%workers] = append(shards[i%workers], spec)
	}
	return shards
}

func resolveTmpBase(paths workspace.Paths, override string) string {
	if strings.TrimSpace(override) != "" {
		return override
	}
	if paths.ExtractedDir != "" {
		return filepath.Join(paths.ExtractedDir, "tmp")
	}
	return ""
}

// resolveDecompTimeoutSecs returns 0 when unlimited timeout is requested
// (NoTimeout == true or TimeoutSecs < 0), DefaultFastTimeoutSecs when
// TimeoutSecs == 0, or the explicit positive TimeoutSecs value.
func resolveDecompTimeoutSecs(opts DecompOptions) int {
	if opts.NoTimeout || opts.TimeoutSecs < 0 {
		return 0
	}
	if opts.TimeoutSecs == 0 {
		return DefaultFastTimeoutSecs
	}
	return opts.TimeoutSecs
}

func sendDecompRecord(ctx context.Context, outCh chan<- extractdb.DecompRecord, rec extractdb.DecompRecord) bool {
	select {
	case outCh <- rec:
		return true
	case <-ctx.Done():
		return false
	}
}
