package bronze

import (
	"context"
	"encoding/binary"
	"sort"
	"unicode/utf16"
	"unicode/utf8"

	"twdll/cli/internal/extractdb"
)

// StringsExtractor deterministically extracts ASCII, UTF-32LE (ELF64), and UTF-16LE (PE32 & ELF64)
// printable strings from data sections staged on Source.
type StringsExtractor struct{}

const (
	minWideStringUnits = 4
	maxWideStringUnits = 4096
)

func (StringsExtractor) ID() string { return "data.strings" }

func (StringsExtractor) AppliesTo(src *Source) bool {
	return src != nil && len(src.DataSlices) > 0
}

func (StringsExtractor) Extract(ctx context.Context, src *Source, store *extractdb.BronzeStore) error {
	return scanAndPersistStrings(ctx, store, src.DataSlices)
}

type rawStringRecord struct {
	ea       uint64
	encoding string
	value    string
}

func scanAndPersistStrings(ctx context.Context, store *extractdb.BronzeStore, sections []DataSlice) error {
	w := store.NewStringWriter(ctx)
	defer func() { _ = w.Close() }()

	for _, sec := range sections {
		if len(sec.Data) == 0 {
			continue
		}
		claimed := make([]bool, len(sec.Data))
		var secRecs []rawStringRecord

		// Extract NUL-terminated ASCII runs first so claimed[] prevents trailing "<char>\x00"
		// bytes of an ASCII string from being misdecoded as wide text, without breaking
		// wide strings preceded by random non-string bytes (e.g. floats ending in 0x3F).
		// On 64-bit targets, scan UTF-32LE before UTF-16LE so claimed[] prevents UTF-32LE
		// wchar_t literals from being partially re-decoded as UTF-16LE while still capturing
		// UTF-16LE CA::UniString / char16_t literals in .rodata.
		scanASCIISection(sec, claimed, &secRecs)
		if sec.Is64Bit {
			scanUTF32LESection(sec, claimed, &secRecs)
		}
		scanUTF16LESection(sec, claimed, &secRecs)

		sort.Slice(secRecs, func(i, j int) bool {
			return secRecs[i].ea < secRecs[j].ea
		})
		for _, rec := range secRecs {
			if err := w.Add(rec.ea, rec.encoding, rec.value); err != nil {
				return err
			}
		}
	}

	return w.Flush()
}

func isASCIIAlnumOrUnderscore(r rune) bool {
	return (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_'
}

func isWideASCIIPrintable(u uint32) bool {
	return (u >= 0x0020 && u <= 0x007E) || u == '\t' || u == '\r' || u == '\n'
}

func isWidePrintable(u uint32) bool {
	return isWideASCIIPrintable(u) || (u >= 0x00A0 && u <= 0x00FF)
}

func hasClaimedBytes(claimed []bool, offset, width int) bool {
	for k := 0; k < width; k++ {
		if claimed[offset+k] {
			return true
		}
	}
	return false
}

func hasRelocSpan(hasReloc func(int) bool, offset, width int) bool {
	if hasReloc == nil {
		return false
	}
	for k := 0; k < width; k++ {
		if hasReloc(offset + k) {
			return true
		}
	}
	return false
}

func scanUTF16LESection(sec DataSlice, claimed []bool, out *[]rawStringRecord) {
	data := sec.Data
	n := len(data)
	// Enforce natural 2-byte alignment relative to section VA.
	i := int(sec.VA & 1)
	for i+1 < n {
		if hasClaimedBytes(claimed, i, 2) || hasRelocSpan(sec.HasRelocAt, i, 2) {
			i += 2
			continue
		}
		u0 := binary.LittleEndian.Uint16(data[i : i+2])
		if !isWideASCIIPrintable(uint32(u0)) {
			i += 2
			continue
		}
		start := i
		units := 0
		alnum := 0
		asciiCount := 0
		relocHit := false
		tooLong := false
		for i+1 < n {
			if hasClaimedBytes(claimed, i, 2) {
				break
			}
			u := binary.LittleEndian.Uint16(data[i : i+2])
			if u == 0 {
				break
			}
			if !isWidePrintable(uint32(u)) {
				break
			}
			if hasRelocSpan(sec.HasRelocAt, i, 2) {
				relocHit = true
			}
			if units >= maxWideStringUnits {
				tooLong = true
				i += 2
				continue
			}
			units++
			if isWideASCIIPrintable(uint32(u)) {
				asciiCount++
			}
			if isASCIIAlnumOrUnderscore(rune(u)) {
				alnum++
			}
			i += 2
		}
		hasNull := false
		if i+1 < n && !hasClaimedBytes(claimed, i, 2) {
			if binary.LittleEndian.Uint16(data[i:i+2]) == 0 {
				if hasRelocSpan(sec.HasRelocAt, i, 2) {
					relocHit = true
				}
				hasNull = true
			}
			i += 2
		}

		if hasNull && !tooLong && !relocHit && units >= minWideStringUnits && alnum >= 2 && asciiCount*100 >= units*75 {
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
					ea:       sec.VA + uint64(start),
					encoding: extractdb.StringEncUTF16LE,
					value:    val,
				})
			}
		}
	}
}

func scanUTF32LESection(sec DataSlice, claimed []bool, out *[]rawStringRecord) {
	data := sec.Data
	n := len(data)
	// Enforce natural 4-byte alignment relative to section VA.
	i := int((4 - (sec.VA & 3)) & 3)
	for i+3 < n {
		if hasClaimedBytes(claimed, i, 4) || hasRelocSpan(sec.HasRelocAt, i, 4) {
			i += 4
			continue
		}
		u0 := binary.LittleEndian.Uint32(data[i : i+4])
		if !isWideASCIIPrintable(u0) {
			i += 4
			continue
		}
		start := i
		units := 0
		alnum := 0
		asciiCount := 0
		relocHit := false
		tooLong := false
		var runes []rune
		for i+3 < n {
			if hasClaimedBytes(claimed, i, 4) {
				break
			}
			u := binary.LittleEndian.Uint32(data[i : i+4])
			if u == 0 {
				break
			}
			if !isWidePrintable(u) || !utf8.ValidRune(rune(u)) {
				break
			}
			if hasRelocSpan(sec.HasRelocAt, i, 4) {
				relocHit = true
			}
			if units >= maxWideStringUnits {
				tooLong = true
				i += 4
				continue
			}
			units++
			if isWideASCIIPrintable(u) {
				asciiCount++
			}
			if isASCIIAlnumOrUnderscore(rune(u)) {
				alnum++
			}
			runes = append(runes, rune(u))
			i += 4
		}
		hasNull := false
		if i+3 < n && !hasClaimedBytes(claimed, i, 4) {
			if binary.LittleEndian.Uint32(data[i:i+4]) == 0 {
				if hasRelocSpan(sec.HasRelocAt, i, 4) {
					relocHit = true
				}
				hasNull = true
			}
			i += 4
		}

		if hasNull && !tooLong && !relocHit && units >= minWideStringUnits && alnum >= 2 && asciiCount*100 >= units*75 {
			val := string(runes)
			if utf8.ValidString(val) {
				for k := start; k < start+(units+1)*4 && k < len(claimed); k++ {
					claimed[k] = true
				}
				*out = append(*out, rawStringRecord{
					ea:       sec.VA + uint64(start),
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

func scanASCIISection(sec DataSlice, claimed []bool, out *[]rawStringRecord) {
	data := sec.Data
	n := len(data)
	start := -1
	alnum := 0
	for i := 0; i < n; i++ {
		if claimed[i] || (sec.HasRelocAt != nil && sec.HasRelocAt(i)) {
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
						for k := start; k <= i && k < len(claimed); k++ {
							claimed[k] = true
						}
						*out = append(*out, rawStringRecord{
							ea:       sec.VA + uint64(start),
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
