package bronze

import (
	"context"
	"encoding/binary"
	"sort"
	"unicode/utf16"
	"unicode/utf8"

	"twdll/cli/internal/extractdb"
)

// StringsExtractor deterministically extracts ASCII, UTF-16LE (PE32), and UTF-32LE (ELF64)
// printable strings from data sections staged on Source.
type StringsExtractor struct{}

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
		// bytes of an ASCII string from being misdecoded as UTF-16LE, without breaking
		// UTF-16LE strings preceded by random non-string bytes (e.g. floats ending in 0x3F).
		scanASCIISection(sec, claimed, &secRecs)
		if sec.Is64Bit {
			scanUTF32LESection(sec, claimed, &secRecs)
		} else {
			scanUTF16LESection(sec, claimed, &secRecs)
		}

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

func isWidePrintable(u uint32) bool {
	return (u >= 0x0020 && u <= 0x007E) || (u >= 0x00A0 && u <= 0x00FF) || u == '\t' || u == '\r' || u == '\n'
}

func scanUTF16LESection(sec DataSlice, claimed []bool, out *[]rawStringRecord) {
	data := sec.Data
	n := len(data)
	i := 0
	for i+1 < n {
		if claimed[i] || claimed[i+1] {
			i++
			continue
		}
		u0 := binary.LittleEndian.Uint16(data[i : i+2])
		if !isWidePrintable(uint32(u0)) {
			i++
			continue
		}
		start := i
		units := 0
		alnum := 0
		printable := 0
		relocHit := false
		for i+1 < n && units < 4096 {
			if claimed[i] || claimed[i+1] {
				break
			}
			if sec.HasRelocAt != nil && (sec.HasRelocAt(i) || sec.HasRelocAt(i+1)) {
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
		if i+1 < n && !claimed[i] && !claimed[i+1] && binary.LittleEndian.Uint16(data[i:i+2]) == 0 {
			if sec.HasRelocAt != nil && (sec.HasRelocAt(i) || sec.HasRelocAt(i+1)) {
				relocHit = true
			}
			hasNull = true
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
					ea:       sec.VA + uint64(start),
					encoding: extractdb.StringEncUTF16LE,
					value:    val,
				})
				continue
			}
		}
		i = start + 1
	}
}

func scanUTF32LESection(sec DataSlice, claimed []bool, out *[]rawStringRecord) {
	data := sec.Data
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
