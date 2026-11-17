package silver

import (
	"context"
	"database/sql"
	"fmt"
	"sort"

	"twdll/cli/internal/extractdb"
	"twdll/cli/internal/workspace"
)

const (
	BucketAnchors = "0_anchors"
	BucketTiny    = "1_tiny(<64B)"
	BucketSmall   = "2_small(64B..512B)"
	BucketLarge   = "3_large(512B..4KB)"
	BucketHuge    = "4_huge(4KB..20KB)"
	BucketGiant   = "5_giant(>20KB)"
)

type BenchFuncSpec struct {
	Spec   extractdb.FuncSpec
	Bucket string
}

// SelectBenchmarkCorpus deterministically picks up to `limit` (default 2,000) non-thunk
// functions stratified across 6 buckets:
//   - 100 anchors (50 lua_regs + 50 vtables)
//   - 400 tiny (<64 B)
//   - 800 small/med (64..512 B)
//   - 500 large (512..4096 B)
//   - 160 huge (4096..20480 B)
//   - 40 giant (>20480 B, pinning FACTION::FACTION 0x00f53830 on attila_64 & largest functions)
func SelectBenchmarkCorpus(ctx context.Context, db *sql.DB, target workspace.TargetID, limit int) ([]BenchFuncSpec, error) {
	if limit <= 0 {
		limit = 2000
	}

	luaSet, err := loadDistinctFuncSet(ctx, db, `SELECT DISTINCT func_ea FROM lua_regs`)
	if err != nil {
		return nil, err
	}
	vtSet, err := loadDistinctFuncSet(ctx, db, `SELECT DISTINCT target_ea FROM vtables`)
	if err != nil {
		return nil, err
	}

	rows, err := db.QueryContext(ctx, `
		SELECT ea, size, name
		FROM functions
		WHERE (flags & ?) = 0 AND size > 0
	`, int64(extractdb.FuncFlagThunk))
	if err != nil {
		return nil, fmt.Errorf("query candidate functions: %w", err)
	}
	defer rows.Close()

	type candidate struct {
		spec   extractdb.FuncSpec
		hash   uint64
		isLua  bool
		isVt   bool
		bucket string
	}

	var all []candidate
	for rows.Next() {
		var ea, sz int64
		var name string
		if err := rows.Scan(&ea, &sz, &name); err != nil {
			return nil, fmt.Errorf("scan candidate function: %w", err)
		}
		uEA := uint64(ea)
		uSz := uint64(sz)
		all = append(all, candidate{
			spec:   extractdb.FuncSpec{EA: uEA, Size: uSz, Name: name},
			hash:   (uEA * 11400714819323198485) & 0x7fffffffffffffff,
			isLua:  luaSet[uEA],
			isVt:   vtSet[uEA],
			bucket: sizeBucket(uSz),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate candidate functions: %w", err)
	}

	sort.Slice(all, func(i, j int) bool {
		if all[i].hash != all[j].hash {
			return all[i].hash < all[j].hash
		}
		return all[i].spec.EA < all[j].spec.EA
	})

	selected := make([]BenchFuncSpec, 0, limit)
	used := make(map[uint64]bool, limit)
	addPick := func(c candidate, bucket string) bool {
		if len(selected) >= limit || used[c.spec.EA] {
			return false
		}
		used[c.spec.EA] = true
		selected = append(selected, BenchFuncSpec{Spec: c.spec, Bucket: bucket})
		return true
	}

	scaleQuota := func(base int) int {
		if limit == 2000 {
			return base
		}
		q := (base * limit) / 2000
		if q < 1 && base > 0 {
			q = 1
		}
		return q
	}

	luaTarget := scaleQuota(50)
	vtTarget := scaleQuota(50)
	tinyTarget := scaleQuota(400)
	smallTarget := scaleQuota(800)
	largeTarget := scaleQuota(500)
	hugeTarget := scaleQuota(160)
	giantTarget := scaleQuota(40)

	// 1. Anchors: 50 lua_regs + 50 vtables.
	luaPicked := 0
	for _, c := range all {
		if luaPicked >= luaTarget {
			break
		}
		if c.isLua && addPick(c, BucketAnchors) {
			luaPicked++
		}
	}
	vtPicked := 0
	for _, c := range all {
		if vtPicked >= vtTarget {
			break
		}
		if c.isVt && addPick(c, BucketAnchors) {
			vtPicked++
		}
	}

	// 2. Giant (>20KB): pin 0x00f53830 (FACTION::FACTION on attila_64) + largest functions.
	var giants []candidate
	for _, c := range all {
		if c.bucket == BucketGiant {
			giants = append(giants, c)
		}
	}
	sort.Slice(giants, func(i, j int) bool {
		if target == workspace.TargetAttila64 {
			if giants[i].spec.EA == 0x00f53830 {
				return true
			}
			if giants[j].spec.EA == 0x00f53830 {
				return false
			}
		}
		if giants[i].spec.Size != giants[j].spec.Size {
			return giants[i].spec.Size > giants[j].spec.Size
		}
		return giants[i].hash < giants[j].hash
	})
	giantPicked := 0
	for _, c := range giants {
		if giantPicked >= giantTarget {
			break
		}
		if addPick(c, BucketGiant) {
			giantPicked++
		}
	}

	// 3. Size-stratified buckets.
	pickBucket := func(bucket string, quota int) {
		picked := 0
		for _, c := range all {
			if picked >= quota {
				break
			}
			if c.bucket == bucket && addPick(c, bucket) {
				picked++
			}
		}
	}
	pickBucket(BucketTiny, tinyTarget)
	pickBucket(BucketSmall, smallTarget)
	pickBucket(BucketLarge, largeTarget)
	pickBucket(BucketHuge, hugeTarget)

	// 4. Top up to `limit` if any bucket was short (e.g. >20KB on smaller binaries).
	if len(selected) < limit {
		for _, c := range all {
			if len(selected) >= limit {
				break
			}
			addPick(c, c.bucket)
		}
	}

	return selected, nil
}

func sizeBucket(sz uint64) string {
	switch {
	case sz < 64:
		return BucketTiny
	case sz < 512:
		return BucketSmall
	case sz < 4096:
		return BucketLarge
	case sz <= 20480:
		return BucketHuge
	default:
		return BucketGiant
	}
}

func loadDistinctFuncSet(ctx context.Context, db *sql.DB, query string) (map[uint64]bool, error) {
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query func set: %w", err)
	}
	defer rows.Close()

	set := make(map[uint64]bool)
	for rows.Next() {
		var ea int64
		if err := rows.Scan(&ea); err != nil {
			return nil, fmt.Errorf("scan func set: %w", err)
		}
		set[uint64(ea)] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return set, nil
}
