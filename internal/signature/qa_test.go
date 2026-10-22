package signature

import (
	"regexp"
	"testing"
)

// Only the alphabet of the canonical form may ever leave Parse: digits, brackets, braces, comma, colon and the four keys.
var canonAlphabet = regexp.MustCompile(`^\{"v":1,"w":1000,"h":400,"s":\[\[[0-9,\[\]]*\]\]\}$`)

func FuzzCanonicalAlphabet(f *testing.F) {
	f.Add(`{"v":1,"w":1000,"h":400,"s":[[0,0,500,0]]}`)
	f.Add(`{"v":1,"w":1000,"h":400,"s":[[-0,0,500,0]]}`)
	f.Add("{\"v\":1,\"w\":1000,\"h\":400,\"s\":[[0,0,500,0]]}\u2028")
	f.Fuzz(func(t *testing.T, in string) {
		_, canon, err := Parse([]byte(in))
		if err != nil {
			return
		}
		if !canonAlphabet.Match(canon) {
			t.Fatalf("canonical form outside the alphabet: %q", canon)
		}
	})
}

// Unicode and escape tricks: accepted only when they decode to exactly the four keys and integers, and then stored canonical.
func TestUnicodeTricks(t *testing.T) {
	bad := []string{
		"\xef\xbb\xbf{\"v\":1,\"w\":1000,\"h\":400,\"s\":[[0,0,500,0]]}",    // BOM
		"{\"v\":1,\"w\":1000,\"h\":400,\"s\":[[0,0,500,0]]}\u2028",          // trailing line separator
		"{\"v\":\uff11,\"w\":1000,\"h\":400,\"s\":[[0,0,500,0]]}",           // full-width digit
		"{\"v\":1,\"w\":1000,\"h\":400,\"s\":[[0,0,\u0665\u0660\u0660,0]]}", // arabic-indic digits
		`{"v":1,"v":1,"w":1000,"h":400,"s":[[0,0,500,0]]}`,                  // duplicate key through an escape
		`{"v":1,"w":1000,"h":400,"s":[[0,0,500,0]],"\u0000":1}`,             // extra key, NUL
		"{\"v\":1,\"w\":1000,\"h\":400,\"s\":[[0,0,500,0]],\"x\xff\":1}",    // invalid UTF-8 key
		`{"v":1,"w":1000,"h":400,"s":[[0,0,500,0]],"__proto__":{}}`,
		`{"v":1,"w":1000,"h":400,"s":[[0,0,500,0]],"s":[[0,0,500,0]]}`,
		`{"v":true,"w":1000,"h":400,"s":[[0,0,500,0]]}`,
		`{"v":1,"w":1000,"h":400,"s":[[0,0,500,0],null]}`,
		`{"v":1,"w":1000,"h":400,"s":[[0,0,500,0]],"x":null}`,
		`{"v":1,"w":1e3,"h":400,"s":[[0,0,500,0]]}`,
		`{"v":1,"w":1000,"h":400,"s":[[0,0,500,0]]}` + "\x00",
	}
	for i, in := range bad {
		if _, _, err := Parse([]byte(in)); err == nil {
			t.Errorf("bad[%d] accepted: %q", i, in)
		}
	}
	// escaped key names and -0 normalise to the canonical form
	_, canon, err := Parse([]byte(`{"v":1,"w":1000,"h":400,"s":[[-0,0,500,0]]}`))
	if err != nil || string(canon) != `{"v":1,"w":1000,"h":400,"s":[[0,0,500,0]]}` {
		t.Errorf("escaped key: %s %v", canon, err)
	}
}
