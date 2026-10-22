package signature

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/danyaa666/smemories/internal/apperr"
)

// tiny builds a stroke of n points with one-digit coordinates (so 600 of them still fit the byte limit), the first one wide.
func tiny(n int, wide bool) []int32 {
	out := make([]int32, 0, n*2)
	for i := range n {
		out = append(out, int32(i%7), int32(i%5))
	}
	if wide {
		out[len(out)-2] = 60
	}
	return out
}

func doc(strokes ...[]int32) string {
	b, _ := json.Marshal(&Signature{V: Version, W: Width, H: Height, S: strokes})
	return string(b)
}

func TestParse(t *testing.T) {
	long := strings.Repeat("[1,2],", 10) // not a flat stroke: wrong shape
	var thirteen [][]int32
	for range MaxStrokes + 1 {
		thirteen = append(thirteen, []int32{0, 0, 500, 300})
	}
	max600 := [][]int32{tiny(MaxPoints/MaxStrokes, true)}
	for range MaxStrokes - 1 {
		max600 = append(max600, tiny(MaxPoints/MaxStrokes, false))
	}
	over := append(max600[:MaxStrokes-1:MaxStrokes-1], tiny(MaxPoints/MaxStrokes+1, false))

	// 600 points of three-digit coordinates are within the point limit and far over the byte limit.
	var fat [][]int32
	for range MaxStrokes {
		s := make([]int32, 0, MaxPoints/MaxStrokes*2)
		for range MaxPoints / MaxStrokes {
			s = append(s, 999, 399)
		}
		s[0], s[1] = 0, 0
		fat = append(fat, s)
	}

	cases := []struct {
		name string
		in   string
		ok   bool
	}{
		{"minimal", `{"v":1,"w":1000,"h":400,"s":[[0,0,50,0]]}`, true},
		{"wide enough, flat", `{"v":1,"w":1000,"h":400,"s":[[10,200,60,200]]}`, true},
		{"tall enough, narrow", `{"v":1,"w":1000,"h":400,"s":[[10,10,10,30]]}`, true},
		{"odd spacing and newlines", " {\n\"v\" : 1 ,\"w\":1000,\"h\":400,\"s\":[ [ 0 , 0 , 50 , 0 ] ]}\n", true},
		{"edge coordinates", `{"v":1,"w":1000,"h":400,"s":[[0,0,1000,400]]}`, true},
		{"600 points", doc(max600...), true},

		{"speck", `{"v":1,"w":1000,"h":400,"s":[[10,10,59,19]]}`, false},
		{"single point", `{"v":1,"w":1000,"h":400,"s":[[10,10]]}`, false},
		{"odd count", `{"v":1,"w":1000,"h":400,"s":[[0,0,500,200,7]]}`, false},
		{"empty stroke", `{"v":1,"w":1000,"h":400,"s":[[]]}`, false},
		{"empty strokes", `{"v":1,"w":1000,"h":400,"s":[]}`, false},
		{"null strokes", `{"v":1,"w":1000,"h":400,"s":null}`, false},
		{"13 strokes", doc(thirteen...), false},
		{"601 points", doc(over...), false},
		{"over 3072 bytes", doc(fat...), false},
		{"x too large", `{"v":1,"w":1000,"h":400,"s":[[0,0,1001,0]]}`, false},
		{"y too large", `{"v":1,"w":1000,"h":400,"s":[[0,0,500,401]]}`, false},
		{"negative", `{"v":1,"w":1000,"h":400,"s":[[-1,0,500,0]]}`, false},
		{"float", `{"v":1,"w":1000,"h":400,"s":[[0,0,500.5,0]]}`, false},
		{"float zero", `{"v":1,"w":1000,"h":400,"s":[[0,0,500.0,0]]}`, false},
		{"exponent", `{"v":1,"w":1000,"h":400,"s":[[0,0,5e2,0]]}`, false},
		{"string number", `{"v":1,"w":1000,"h":400,"s":[[0,0,"500",0]]}`, false},
		{"huge number", `{"v":1,"w":1000,"h":400,"s":[[0,0,99999999999999999999,0]]}`, false},
		{"extra key", `{"v":1,"w":1000,"h":400,"s":[[0,0,500,0]],"t":[1]}`, false},
		{"duplicate key", `{"v":1,"v":1,"w":1000,"h":400,"s":[[0,0,500,0]]}`, false},
		{"upper-case key", `{"V":1,"w":1000,"h":400,"s":[[0,0,500,0]]}`, false},
		{"missing key", `{"v":1,"w":1000,"s":[[0,0,500,0]]}`, false},
		{"wrong v", `{"v":2,"w":1000,"h":400,"s":[[0,0,500,0]]}`, false},
		{"wrong w", `{"v":1,"w":500,"h":400,"s":[[0,0,400,0]]}`, false},
		{"wrong h", `{"v":1,"w":1000,"h":200,"s":[[0,0,500,0]]}`, false},
		{"nested stroke", `{"v":1,"w":1000,"h":400,"s":[[[0,0],[500,0]]]}`, false},
		{"shape", `{"v":1,"w":1000,"h":400,"s":[` + long + `]}`, false},
		{"trailing data", `{"v":1,"w":1000,"h":400,"s":[[0,0,500,0]]} {}`, false},
		{"trailing garbage", `{"v":1,"w":1000,"h":400,"s":[[0,0,500,0]]}x`, false},
		{"array", `[1,2]`, false},
		{"empty", ``, false},
		{"null", `null`, false},
		{"markup", `<svg onload=alert(1)>`, false},
		{"deep nesting", strings.Repeat("[", 3000), false},
		{"control chars", "{\"v\":1,\u0000}", false},
		{"too large input", `{"v":1,"w":1000,"h":400,"s":[[0,0,500,0]]}` + strings.Repeat(" ", MaxInput), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sig, canon, err := Parse([]byte(c.in))
			if c.ok {
				if err != nil || sig == nil || len(canon) > MaxBytes {
					t.Fatalf("want valid, got %v", err)
				}
				return
			}
			if err == nil || !apperr.Is(err, CodeInvalid) {
				t.Fatalf("want ERROR_INVALID_SIGNATURE, got %v", err)
			}
			if sig != nil || canon != nil {
				t.Fatal("a rejected value must return nothing")
			}
		})
	}
}

// The canonical form is the server's own serialisation, whatever the client's spacing was; parsing it again changes nothing.
func TestCanonical(t *testing.T) {
	_, canon, err := Parse([]byte(" {\"s\" : [[ 1 ,2,\n300,4]], \"h\":400, \"w\":1000,\"v\":1}"))
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"v":1,"w":1000,"h":400,"s":[[1,2,300,4]]}`; string(canon) != want {
		t.Fatalf("canonical = %s, want %s", canon, want)
	}
	_, again, err := Parse(canon)
	if err != nil || string(again) != string(canon) {
		t.Fatalf("not stable: %s %v", again, err)
	}
}

// An error message names the rule and never echoes the content.
func TestErrorsDoNotEchoContent(t *testing.T) {
	_, _, err := Parse([]byte(`{"v":1,"w":1000,"h":400,"s":[[0,0,31337,0]],"SECRET-KEY":1}`))
	if err == nil || strings.Contains(err.Error(), "31337") || strings.Contains(err.Error(), "SECRET") {
		t.Fatalf("error leaks content: %v", err)
	}
}

// Whatever is stored, Decode returns a valid value or nil, never unchecked data.
func TestDecode(t *testing.T) {
	if Decode(`{"v":1,"w":1000,"h":400,"s":[[0,0,500,0]]}`) == nil {
		t.Fatal("valid value dropped")
	}
	for _, s := range []string{"", "{}", `{"v":1,"w":1000,"h":400,"s":[["<script>",0,500,0]]}`, "garbage"} {
		if Decode(s) != nil {
			t.Fatalf("Decode(%q) accepted", s)
		}
	}
}

// A maximal valid signature (the largest the format allows) fits the byte limit: the limits are consistent with each other.
func FuzzParse(f *testing.F) {
	f.Add(`{"v":1,"w":1000,"h":400,"s":[[0,0,500,0]]}`)
	f.Add(`{"v":1,"w":1000,"h":400,"s":[[`)
	f.Fuzz(func(t *testing.T, in string) {
		sig, canon, err := Parse([]byte(in))
		if err != nil {
			return
		}
		if len(canon) > MaxBytes || sig.check() != nil {
			t.Fatalf("accepted an invalid value: %s", canon)
		}
		if _, again, err := Parse(canon); err != nil || string(again) != string(canon) {
			t.Fatal("canonical form is not stable")
		}
	})
}
