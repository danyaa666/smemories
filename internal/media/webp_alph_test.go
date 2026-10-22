package media

import (
	"encoding/binary"
	"os"
	"testing"
)

// T-113 (QA). webpHasALPH walks the RIFF chunks of untrusted input: it must not panic or read out of range, and it must only see top-level ALPH chunks.
func riffChunk(id string, payload []byte, declared int) []byte {
	b := append([]byte(id), 0, 0, 0, 0)
	binary.LittleEndian.PutUint32(b[4:], uint32(declared))
	b = append(b, payload...)
	if len(payload)%2 == 1 {
		b = append(b, 0)
	}
	return b
}

func riffWebP(chunks ...[]byte) []byte {
	b := []byte("RIFF\x00\x00\x00\x00WEBP")
	for _, c := range chunks {
		b = append(b, c...)
	}
	binary.LittleEndian.PutUint32(b[4:], uint32(len(b)-8))
	return b
}

func TestWebpHasALPHCases(t *testing.T) {
	alph := riffChunk("ALPH", []byte{1, 2, 3}, 3)
	vp8x := riffChunk("VP8X", make([]byte, 10), 10)
	vp8 := riffChunk("VP8 ", make([]byte, 6), 6)
	inner := riffChunk("EXIF", []byte("xxxxALPH\x03\x00\x00\x00abc\x00"), 16)
	huge := append([]byte("ALPH\xff\xff\xff\xff"), 1, 2, 3, 4)
	for _, tc := range []struct {
		name string
		data []byte
		want bool
	}{
		{"nil", nil, false},
		{"short", []byte("RIFF"), false},
		{"header only", riffWebP(), false},
		{"alph", riffWebP(vp8x, alph, vp8), true},
		{"alph first", riffWebP(alph, vp8), true},
		{"after odd chunk", riffWebP(vp8x, riffChunk("ICCP", []byte{1, 2, 3}, 3), alph, vp8), true},
		{"no alph", riffWebP(vp8x, vp8), false},
		{"alph hidden inside another chunk", riffWebP(vp8x, inner, vp8), false},
		{"truncated chunk header", append(riffWebP(vp8x), 'A', 'L', 'P', 'H', 1, 0), false},
		{"alph declared longer than data", riffWebP(vp8x, riffChunk("ALPH", []byte{1, 2}, 200), vp8), false},
		{"preceding chunk oversized hides alph", append(riffWebP(vp8x), append(huge, alph...)...), false},
		{"length 0xffffffff", append(riffWebP(vp8x), huge...), false},
		{"odd length without pad", append(riffWebP(vp8x), []byte("ALPH\x01\x00\x00\x00a")...), true},
		{"zero-length chunks loop", riffWebP(riffChunk("JUNK", nil, 0), riffChunk("JUNK", nil, 0), alph), true},
	} {
		if got := webpHasALPH(tc.data); got != tc.want {
			t.Errorf("%s: webpHasALPH = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func FuzzWebpHasALPH(f *testing.F) {
	for _, p := range []string{"testdata/alpha-lossy-2048.webp", "testdata/rot6.webp", "testdata/alpha.webp", "testdata/vp8x-canvas-mismatch.webp"} {
		if b, err := os.ReadFile(p); err == nil {
			f.Add(b)
			if len(b) > 40 {
				f.Add(b[:40])
			}
		}
	}
	f.Fuzz(func(t *testing.T, d []byte) {
		webpHasALPH(d)
		webpOrientation(d)
	})
}
