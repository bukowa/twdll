package bronze

import (
	"bytes"
	"encoding/binary"
	"testing"
)

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
