package bronze

import (
	"context"
	"database/sql"
	"encoding/binary"
	"fmt"
	"sort"
	"unicode/utf16"
	"unicode/utf8"

	"twdll/cli/internal/extractdb"
)

type stringEntry struct {
	strID int64
	ea    uint64
}

type stringIndex struct {
	entries []stringEntry
}

func (idx *stringIndex) LookupEA(ea uint64) (int64, bool) {
	if idx == nil || len(idx.entries) == 0 {
		return 0, false
	}
	i := sort.Search(len(idx.entries), func(i int) bool {
		return idx.entries[i].ea >= ea
	})
	if i < len(idx.entries) && idx.entries[i].ea == ea {
		return idx.entries[i].strID, true
	}
	return 0, false
}

type dataSectionSlice struct {
	name string
	va   uint64
	data []byte
	// hasRelocAt reports whether a given byte offset within data overlaps a .reloc entry (PE32).
	hasRelocAt func(off int) bool
}

type rawStringRecord struct {
	strID    int64
	ea       uint64
	encoding string
	value    string
}

func scanAndPersistStrings(ctx context.Context, db *sql.DB, sections []dataSectionSlice, is64Bit bool) (*stringIndex, error) {
	b := newTxStmtBatcher(ctx, db, `
		INSERT INTO strings(str_id, ea, encoding, value, df_count)
		VALUES (?, ?, ?, ?, 0)
	`)
	defer func() { _ = b.Close() }()

	var (
		nextID  int64 = 1
		entries       = make([]stringEntry, 0, 65536)
	)

	emit := func(ea uint64, enc, val string) error {
		id := nextID
		nextID++
		if err := b.Exec(id, int64(ea), enc, val); err != nil {
			return fmt.Errorf("insert string 0x%x: %w", ea, err)
		}
		entries = append(entries, stringEntry{strID: id, ea: ea})
		return nil
	}

	for _, sec := range sections {
		if len(sec.data) == 0 {
			continue
		}
		claimed := make([]bool, len(sec.data))
		var secRecs []rawStringRecord

		if is64Bit {
			scanUTF32LESection(sec, claimed, &secRecs)
		} else {
			scanUTF16LESection(sec, claimed, &secRecs)
		}
		scanASCIISection(sec, claimed, &secRecs)

		sort.Slice(secRecs, func(i, j int) bool {
			return secRecs[i].ea < secRecs[j].ea
		})
		for _, rec := range secRecs {
			if err := emit(rec.ea, rec.encoding, rec.value); err != nil {
				return nil, err
			}
		}
	}

	if err := b.Flush(); err != nil {
		return nil, err
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].ea < entries[j].ea
	})
	return &stringIndex{entries: entries}, nil
}

func isASCIIAlnumOrUnderscore(r rune) bool {
	return (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_'
}

func isWidePrintable(u uint32) bool {
	return (u >= 0x0020 && u <= 0x007E) || (u >= 0x00A0 && u <= 0x00FF) || u == '\t' || u == '\r' || u == '\n'
}

func scanUTF16LESection(sec dataSectionSlice, claimed []bool, out *[]rawStringRecord) {
	data := sec.data
	n := len(data)
	i := 0
	for i+1 < n {
		u0 := binary.LittleEndian.Uint16(data[i : i+2])
		if !isWidePrintable(uint32(u0)) {
			i += 2
			continue
		}
		// Do not treat the final "<char>\x00" of a preceding ASCII string as the first code unit of a UTF-16LE string.
		if i >= 1 && isASCIIPrintable(data[i-1]) && data[i+1] == 0 {
			i += 2
			continue
		}
		start := i
		units := 0
		alnum := 0
		printable := 0
		relocHit := false
		for i+1 < n && units < 4096 {
			if sec.hasRelocAt != nil && (sec.hasRelocAt(i) || sec.hasRelocAt(i+1)) {
				relocHit = true
			}
			u := binary.LittleEndian.Uint16(data[i : i+2])
			if u == 0 {
				break
			}
			if !isWidePrintable(uint32(u)) {
				break
			}
			units++
			printable++
			if isASCIIAlnumOrUnderscore(rune(u)) {
				alnum++
			}
			i += 2
		}
		hasNull := false
		if i+1 < n && binary.LittleEndian.Uint16(data[i:i+2]) == 0 {
			if sec.hasRelocAt != nil && (sec.hasRelocAt(i) || sec.hasRelocAt(i+1)) {
				relocHit = true
			}
			hasNull = true
			i += 2
		} else {
			i += 2
		}

		if hasNull && !relocHit && units >= 5 && alnum >= 2 && printable*100 >= units*75 {
			u16s := make([]uint16, units)
			for k := 0; k < units; k++ {
				u16s[k] = binary.LittleEndian.Uint16(data[start+k*2 : start+k*2+2])
			}
			val := string(utf16.Decode(u16s))
			if utf8.ValidString(val) {
				for k := start; k < start+(units+1)*2 && k < len(claimed); k++ {
					claimed[k] = true
				}
				*out = append(*out, rawStringRecord{
					ea:       sec.va + uint64(start),
					encoding: extractdb.StringEncUTF16LE,
					value:    val,
				})
			}
		}
	}
}

func scanUTF32LESection(sec dataSectionSlice, claimed []bool, out *[]rawStringRecord) {
	data := sec.data
	n := len(data)
	i := 0
	for i+3 < n {
		u0 := binary.LittleEndian.Uint32(data[i : i+4])
		if !isWidePrintable(u0) {
			i += 4
			continue
		}
		start := i
		units := 0
		alnum := 0
		printable := 0
		var runes []rune
		for i+3 < n && units < 4096 {
			u := binary.LittleEndian.Uint32(data[i : i+4])
			if u == 0 {
				break
			}
			if !isWidePrintable(u) || !utf8.ValidRune(rune(u)) {
				break
			}
			units++
			printable++
			if isASCIIAlnumOrUnderscore(rune(u)) {
				alnum++
			}
			runes = append(runes, rune(u))
			i += 4
		}
		hasNull := false
		if i+3 < n && binary.LittleEndian.Uint32(data[i:i+4]) == 0 {
			hasNull = true
			i += 4
		} else {
			i += 4
		}

		if hasNull && units >= 5 && alnum >= 2 && printable*100 >= units*80 {
			val := string(runes)
			if utf8.ValidString(val) {
				for k := start; k < start+(units+1)*4 && k < len(claimed); k++ {
					claimed[k] = true
				}
				*out = append(*out, rawStringRecord{
					ea:       sec.va + uint64(start),
					encoding: extractdb.StringEncUTF32LE,
					value:    val,
				})
			}
		}
	}
}

func isASCIIPrintable(b byte) bool {
	return (b >= 0x20 && b <= 0x7E) || b == '\t' || b == '\r' || b == '\n'
}

func scanASCIISection(sec dataSectionSlice, claimed []bool, out *[]rawStringRecord) {
	data := sec.data
	n := len(data)
	start := -1
	alnum := 0
	for i := 0; i < n; i++ {
		if claimed[i] {
			start = -1
			alnum = 0
			continue
		}
		b := data[i]
		if isASCIIPrintable(b) {
			if start < 0 {
				start = i
				alnum = 0
			}
			if isASCIIAlnumOrUnderscore(rune(b)) {
				alnum++
			}
		} else {
			if b == 0 && start >= 0 {
				length := i - start
				if length >= 4 && length <= 8192 && alnum >= 1 {
					s := string(data[start:i])
					if utf8.ValidString(s) {
						*out = append(*out, rawStringRecord{
							ea:       sec.va + uint64(start),
							encoding: extractdb.StringEncASCII,
							value:    s,
						})
					}
				}
			}
			start = -1
			alnum = 0
		}
	}
}

func applyStringXrefsAndDFCount(ctx context.Context, db *sql.DB, xrefs []stringXrefRow) error {
	if len(xrefs) == 0 {
		return nil
	}
	sort.Slice(xrefs, func(i, j int) bool {
		if xrefs[i].StrID != xrefs[j].StrID {
			return xrefs[i].StrID < xrefs[j].StrID
		}
		return xrefs[i].FuncEA < xrefs[j].FuncEA
	})

	b := newTxStmtBatcher(ctx, db, `
		INSERT OR IGNORE INTO string_xrefs(str_id, func_ea)
		VALUES (?, ?)
	`)
	defer func() { _ = b.Close() }()

	dfCounts := make(map[int64]int64)
	var prev stringXrefRow
	for i, xr := range xrefs {
		if i > 0 && xr.StrID == prev.StrID && xr.FuncEA == prev.FuncEA {
			continue
		}
		prev = xr
		if err := b.Exec(xr.StrID, int64(xr.FuncEA)); err != nil {
			return fmt.Errorf("insert string_xref (%d, 0x%x): %w", xr.StrID, xr.FuncEA, err)
		}
		dfCounts[xr.StrID]++
	}
	if err := b.Flush(); err != nil {
		return err
	}

	upd := newTxStmtBatcher(ctx, db, `UPDATE strings SET df_count = ? WHERE str_id = ?`)
	defer func() { _ = upd.Close() }()
	strIDs := make([]int64, 0, len(dfCounts))
	for id := range dfCounts {
		strIDs = append(strIDs, id)
	}
	sort.Slice(strIDs, func(i, j int) bool { return strIDs[i] < strIDs[j] })
	for _, id := range strIDs {
		if err := upd.Exec(dfCounts[id], id); err != nil {
			return fmt.Errorf("update string df_count %d: %w", id, err)
		}
	}
	return upd.Flush()
}
