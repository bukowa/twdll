package bronze

import (
	"bytes"
	"context"
	"encoding/binary"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"

	"twdll/cli/internal/extractdb"
)

func encodeUTF16LE(s string) []byte {
	var buf bytes.Buffer
	for _, u := range utf16.Encode([]rune(s)) {
		var b [2]byte
		binary.LittleEndian.PutUint16(b[:], u)
		buf.Write(b[:])
	}
	buf.Write([]byte{0, 0})
	return buf.Bytes()
}

func encodeUTF32LE(s string) []byte {
	var buf bytes.Buffer
	for _, r := range s {
		var b [4]byte
		binary.LittleEndian.PutUint32(b[:], uint32(r))
		buf.Write(b[:])
	}
	buf.Write([]byte{0, 0, 0, 0})
	return buf.Bytes()
}

func TestScanUTF32LESection_RespectsClaimedAndHasRelocAt(t *testing.T) {
	t.Run("skips UTF-32LE spans overlapping claimed bytes", func(t *testing.T) {
		first := encodeUTF32LE("ClaimedWide")
		raw := append(first, encodeUTF32LE("CleanWide")...)
		claimed := make([]bool, len(raw))
		// Mark only the NUL terminator of the first string as claimed so the entire span must be rejected.
		for k := len(first) - 4; k < len(first); k++ {
			claimed[k] = true
		}

		var out []rawStringRecord
		scanUTF32LESection(DataSlice{
			VA:      0x400000,
			Data:    raw,
			Is64Bit: true,
		}, claimed, &out)

		if len(out) != 1 || out[0].value != "CleanWide" {
			t.Fatalf("got %+v, want single CleanWide string", out)
		}
	})

	t.Run("skips UTF-32LE spans overlapping relocations (including NUL terminator)", func(t *testing.T) {
		s1 := encodeUTF32LE("RelocBody")
		s2 := encodeUTF32LE("RelocTerm")
		s3 := encodeUTF32LE("PureWide")

		var raw []byte
		raw = append(raw, s1...)
		s2Start := len(raw)
		raw = append(raw, s2...)
		raw = append(raw, s3...)

		// Relocation inside s1's body (offset 6) and inside s2's NUL terminator (last 4 bytes of s2).
		relocOffsets := map[int]bool{
			6:                   true,
			s2Start + len(s2) - 2: true,
		}

		var out []rawStringRecord
		scanUTF32LESection(DataSlice{
			VA:      0x500000,
			Data:    raw,
			Is64Bit: true,
			HasRelocAt: func(off int) bool {
				return relocOffsets[off]
			},
		}, make([]bool, len(raw)), &out)

		if len(out) != 1 || out[0].value != "PureWide" {
			t.Fatalf("got %+v, want single PureWide string", out)
		}
	})
}

func TestScanWideStrings_NaturalAlignment(t *testing.T) {
	t.Run("UTF-16LE skips odd VA offsets and honors odd section base VA", func(t *testing.T) {
		var raw []byte
		raw = append(raw, 0x00) // shift payload to offset 1
		raw = append(raw, encodeUTF16LE("Unaligned16")...)
		if len(raw)%2 != 0 {
			raw = append(raw, 0x00)
		}
		alignedOff := len(raw)
		raw = append(raw, encodeUTF16LE("Aligned16")...)

		var outEven []rawStringRecord
		scanUTF16LESection(DataSlice{VA: 0x1000, Data: raw}, make([]bool, len(raw)), &outEven)
		if len(outEven) != 1 || outEven[0].value != "Aligned16" || outEven[0].ea != 0x1000+uint64(alignedOff) {
			t.Fatalf("even VA got %+v, want single Aligned16 at 0x%x", outEven, 0x1000+uint64(alignedOff))
		}

		// With an odd section VA (0x1001), offset 1 has even VA 0x1002 and alignedOff has odd VA.
		var outOdd []rawStringRecord
		scanUTF16LESection(DataSlice{VA: 0x1001, Data: raw}, make([]bool, len(raw)), &outOdd)
		if len(outOdd) != 1 || outOdd[0].value != "Unaligned16" || outOdd[0].ea != 0x1002 {
			t.Fatalf("odd VA got %+v, want single Unaligned16 at 0x1002", outOdd)
		}
	})

	t.Run("UTF-32LE skips non-4-byte-aligned VA offsets and honors unaligned section base VA", func(t *testing.T) {
		var raw []byte
		raw = append(raw, 0x00, 0x00) // shift payload to offset 2
		raw = append(raw, encodeUTF32LE("OffsetTwo32")...)
		for len(raw)%4 != 0 {
			raw = append(raw, 0x00)
		}
		alignedOff := len(raw)
		raw = append(raw, encodeUTF32LE("Aligned32")...)

		var outAligned []rawStringRecord
		scanUTF32LESection(DataSlice{VA: 0x2000, Data: raw, Is64Bit: true}, make([]bool, len(raw)), &outAligned)
		if len(outAligned) != 1 || outAligned[0].value != "Aligned32" || outAligned[0].ea != 0x2000+uint64(alignedOff) {
			t.Fatalf("VA 0x2000 got %+v, want single Aligned32 at 0x%x", outAligned, 0x2000+uint64(alignedOff))
		}

		// With VA 0x2002, offset 2 has 4-byte aligned VA 0x2004.
		var outShifted []rawStringRecord
		scanUTF32LESection(DataSlice{VA: 0x2002, Data: raw, Is64Bit: true}, make([]bool, len(raw)), &outShifted)
		if len(outShifted) != 1 || outShifted[0].value != "OffsetTwo32" || outShifted[0].ea != 0x2004 {
			t.Fatalf("VA 0x2002 got %+v, want single OffsetTwo32 at 0x2004", outShifted)
		}
	})
}

func TestScanWideStrings_OversizedRunsSkippedWhole(t *testing.T) {
	longStr := strings.Repeat("A", maxWideStringUnits+200)

	t.Run("UTF-16LE skips entire >4096 run without emitting suffix", func(t *testing.T) {
		var raw []byte
		raw = append(raw, encodeUTF16LE(longStr)...)
		raw = append(raw, encodeUTF16LE("AfterLong16")...)
		// Oversized run terminated by non-printable uint16 (not NUL) followed by another valid string.
		for k := 0; k < maxWideStringUnits+50; k++ {
			raw = append(raw, 'B', 0x00)
		}
		raw = append(raw, 0xFF, 0xFF)
		raw = append(raw, encodeUTF16LE("AfterNonPrint16")...)

		var out []rawStringRecord
		scanUTF16LESection(DataSlice{VA: 0x3000, Data: raw}, make([]bool, len(raw)), &out)
		if len(out) != 2 || out[0].value != "AfterLong16" || out[1].value != "AfterNonPrint16" {
			t.Fatalf("got %+v, want [AfterLong16, AfterNonPrint16]", out)
		}
	})

	t.Run("UTF-32LE skips entire >4096 run without emitting suffix", func(t *testing.T) {
		var raw []byte
		raw = append(raw, encodeUTF32LE(longStr)...)
		raw = append(raw, encodeUTF32LE("AfterLong32")...)
		for k := 0; k < maxWideStringUnits+50; k++ {
			raw = append(raw, 'B', 0x00, 0x00, 0x00)
		}
		raw = append(raw, 0xFF, 0xFF, 0x00, 0x00)
		raw = append(raw, encodeUTF32LE("AfterNonPrint32")...)

		var out []rawStringRecord
		scanUTF32LESection(DataSlice{VA: 0x4000, Data: raw, Is64Bit: true}, make([]bool, len(raw)), &out)
		if len(out) != 2 || out[0].value != "AfterLong32" || out[1].value != "AfterNonPrint32" {
			t.Fatalf("got %+v, want [AfterLong32, AfterNonPrint32]", out)
		}
	})
}

func TestScanWideStrings_StartMinUnitsAndASCIIRatio(t *testing.T) {
	t.Run("UTF-32LE strips leading Latin-1 prefix, rejects noisy Latin-1, keeps Zénobie and 4-char tokens", func(t *testing.T) {
		var raw []byte
		// 0x00BB ('»') immediately before "<not connected>\0"
		notConnStart := len(raw) + 4
		raw = append(raw, encodeUTF32LE("»<not connected>")...)
		// High-Latin-1 table noise (50% non-ASCII) -> must be rejected
		raw = append(raw, encodeUTF32LE("ÿ3ÿUÿ")...)
		raw = append(raw, encodeUTF32LE("2dÈÈ")...)
		// Genuine accented name and leading newline format string -> must be kept
		raw = append(raw, encodeUTF32LE("Zénobie")...)
		raw = append(raw, encodeUTF32LE("\nUnits")...)
		// 4-char real tokens -> kept; 3-char -> rejected
		raw = append(raw, encodeUTF32LE("name")...)
		raw = append(raw, encodeUTF32LE(".tga")...)
		raw = append(raw, encodeUTF32LE("abc")...)

		var out []rawStringRecord
		scanUTF32LESection(DataSlice{VA: 0x5000, Data: raw, Is64Bit: true}, make([]bool, len(raw)), &out)

		wantVals := []string{"<not connected>", "Zénobie", "\nUnits", "name", ".tga"}
		if len(out) != len(wantVals) {
			t.Fatalf("got %+v, want %v", out, wantVals)
		}
		if out[0].ea != 0x5000+uint64(notConnStart) {
			t.Errorf("not connected EA = 0x%x, want 0x%x", out[0].ea, 0x5000+uint64(notConnStart))
		}
		for i, want := range wantVals {
			if out[i].value != want {
				t.Errorf("out[%d] = %q, want %q", i, out[i].value, want)
			}
		}
	})

	t.Run("UTF-16LE keeps Zénobie and 4-char tokens, skips leading reloc pointer without swallowing next string", func(t *testing.T) {
		var raw []byte
		// 4-byte relocated value that looks like printable UTF-16LE ("A@") immediately before L"AfterPtr\0"
		raw = append(raw, 'A', 0x00, '@', 0x00)
		afterPtrOff := len(raw)
		raw = append(raw, encodeUTF16LE("AfterPtr")...)
		// Fake wide string with reloc inside body must emit neither the whole string nor its tail.
		relocBodyOff := len(raw)
		raw = append(raw, encodeUTF16LE("RelocFakeWide")...)
		raw = append(raw, encodeUTF16LE("Zénobie")...)
		raw = append(raw, encodeUTF16LE("true")...)
		raw = append(raw, encodeUTF16LE("2dÈÈ")...)
		raw = append(raw, encodeUTF16LE("abc")...)

		relocs := map[int]bool{
			0: true, 1: true, 2: true, 3: true,
			relocBodyOff + 4: true, relocBodyOff + 5: true,
		}

		var out []rawStringRecord
		scanUTF16LESection(DataSlice{
			VA:   0x6000,
			Data: raw,
			HasRelocAt: func(off int) bool {
				return relocs[off]
			},
		}, make([]bool, len(raw)), &out)

		wantVals := []string{"AfterPtr", "Zénobie", "true"}
		if len(out) != len(wantVals) {
			t.Fatalf("got %+v, want %v", out, wantVals)
		}
		if out[0].ea != 0x6000+uint64(afterPtrOff) {
			t.Errorf("AfterPtr EA = 0x%x, want 0x%x", out[0].ea, 0x6000+uint64(afterPtrOff))
		}
		for i, want := range wantVals {
			if out[i].value != want {
				t.Errorf("out[%d] = %q, want %q", i, out[i].value, want)
			}
		}
	})
}

func TestScanAndPersistStrings_64BitExtractsASCIIUTF32AndUTF16(t *testing.T) {
	ctx := context.Context(t.Context())
	dbPath := filepath.Join(t.TempDir(), "bronze_strings.sqlite")
	store, err := extractdb.NewBronzeStore(ctx, dbPath)
	if err != nil {
		t.Fatalf("NewBronzeStore: %v", err)
	}
	defer func() { _ = store.Abort() }()

	var raw []byte
	asciiOff := len(raw)
	raw = append(raw, "FACTION_KEY\x00"...)
	for len(raw)%4 != 0 {
		raw = append(raw, 0x00)
	}
	utf32Off := len(raw)
	raw = append(raw, encodeUTF32LE("AttilaWide32")...)
	utf16Off := len(raw)
	raw = append(raw, encodeUTF16LE("save_games/")...)
	utf16ShortOff := len(raw)
	raw = append(raw, encodeUTF16LE("true")...)

	const baseVA = uint64(0x3785000)
	if err := scanAndPersistStrings(ctx, store, []DataSlice{{
		Name:    ".rodata",
		VA:      baseVA,
		Data:    raw,
		Is64Bit: true,
	}}); err != nil {
		t.Fatalf("scanAndPersistStrings: %v", err)
	}
	if _, err := store.FinalizeAndCommit(ctx, nil); err != nil {
		t.Fatalf("FinalizeAndCommit: %v", err)
	}

	db, err := extractdb.OpenReadOnly(dbPath)
	if err != nil {
		t.Fatalf("OpenReadOnly: %v", err)
	}
	defer db.Close()

	rows, err := db.QueryContext(ctx, `SELECT ea, encoding, value FROM strings ORDER BY ea`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	var got []rawStringRecord
	for rows.Next() {
		var ea int64
		var enc, val string
		if err := rows.Scan(&ea, &enc, &val); err != nil {
			t.Fatal(err)
		}
		got = append(got, rawStringRecord{ea: uint64(ea), encoding: enc, value: val})
	}

	want := []rawStringRecord{
		{ea: baseVA + uint64(asciiOff), encoding: extractdb.StringEncASCII, value: "FACTION_KEY"},
		{ea: baseVA + uint64(utf32Off), encoding: extractdb.StringEncUTF32LE, value: "AttilaWide32"},
		{ea: baseVA + uint64(utf16Off), encoding: extractdb.StringEncUTF16LE, value: "save_games/"},
		{ea: baseVA + uint64(utf16ShortOff), encoding: extractdb.StringEncUTF16LE, value: "true"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("row[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}
