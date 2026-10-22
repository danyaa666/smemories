// Package signature validates a friend's handwritten signature (E11, decision L-14): a short list of integer points, never an image.
// The client's string is parsed strictly and re-serialised; only the canonical form is stored or returned, so the API can never hand
// out anything but integers. Only positions are kept: no timing, pressure, pointer or device data.
package signature

import (
	"bytes"
	"encoding/json"
	"io"
	"strconv"

	"github.com/danyaa666/smemories/internal/apperr"
)

// The v1 format. Every number here is part of the contract in docs/note-fields.md ("Signature") and api/openapi.yaml.
const (
	Version    = 1
	Width      = 1000 // logical drawing space
	Height     = 400
	MaxStrokes = 12
	MaxPoints  = 600  // in total
	MaxBytes   = 3072 // canonical serialised size
	MaxInput   = 4096 // what the transport reads from the client; larger is rejected without parsing
	minPoints  = 2    // per stroke
	keyCount   = 4    // v, w, h, s
	minBoxPct  = 5    // the bounding box must cover this share of the width or of the height
	percent    = 100
)

// CodeInvalid is ERROR_INVALID_SIGNATURE. The message names the rule, never the content.
var CodeInvalid = apperr.NewCode("ERROR_INVALID_SIGNATURE", 400)

// Signature is the canonical v1 value. Field order is the canonical key order.
type Signature struct {
	V int       `json:"v"`
	W int       `json:"w"`
	H int       `json:"h"`
	S [][]int32 `json:"s"` // strokes: x0, y0, x1, y1, ...
}

// Info is what the public form is told (data.signature of public-collect/get).
type Info struct {
	Enabled    bool `json:"enabled"`
	W          int  `json:"w"`
	H          int  `json:"h"`
	MaxStrokes int  `json:"max_strokes"`
	MaxPoints  int  `json:"max_points"`
}

// NewInfo describes the limits; enabled is the template's switch.
func NewInfo(enabled bool) Info {
	return Info{Enabled: enabled, W: Width, H: Height, MaxStrokes: MaxStrokes, MaxPoints: MaxPoints}
}

func bad(rule string) error { return apperr.New(CodeInvalid, "signature: "+rule) }

// Parse checks raw against every rule of format v1 and returns the value and its canonical JSON. Duplicate keys, extra keys,
// floats, exponents, out-of-range and negative numbers, trailing data and a wrong size are all CodeInvalid.
func Parse(raw []byte) (*Signature, []byte, error) {
	if len(raw) > MaxInput {
		return nil, nil, bad("too large")
	}
	sig, err := decode(raw)
	if err != nil {
		return nil, nil, err
	}
	if err := sig.check(); err != nil {
		return nil, nil, err
	}
	canon, err := json.Marshal(sig)
	if err != nil {
		return nil, nil, apperr.Wrap(apperr.Internal, err, "encode signature")
	}
	if len(canon) > MaxBytes {
		return nil, nil, bad("more than " + strconv.Itoa(MaxBytes) + " bytes")
	}
	return sig, canon, nil
}

// Decode reads a stored value. It returns nil for anything that is not a valid signature, so a damaged row can only ever
// produce "no signature", never unchecked output.
func Decode(stored string) *Signature {
	sig, _, err := Parse([]byte(stored))
	if err != nil {
		return nil
	}
	return sig
}

// decode reads the four keys v, w, h and s, each exactly once, and nothing else.
func decode(raw []byte) (*Signature, error) {
	malformed := bad("not a JSON object of the form {\"v\",\"w\",\"h\",\"s\"}")
	dec := json.NewDecoder(bytes.NewReader(raw))
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return nil, malformed
	}
	sig := &Signature{}
	seen := map[string]bool{}
	for dec.More() {
		t, err := dec.Token()
		key, ok := t.(string)
		if err != nil || !ok || seen[key] {
			return nil, malformed
		}
		seen[key] = true
		switch key {
		case "v":
			err = dec.Decode(&sig.V)
		case "w":
			err = dec.Decode(&sig.W)
		case "h":
			err = dec.Decode(&sig.H)
		case "s":
			err = dec.Decode(&sig.S)
		default:
			return nil, bad("unknown key")
		}
		if err != nil {
			return nil, malformed
		}
	}
	if t, err := dec.Token(); err != nil || t != json.Delim('}') {
		return nil, malformed
	}
	if _, err := dec.Token(); err != io.EOF || len(seen) != keyCount {
		return nil, malformed
	}
	return sig, nil
}

func (s *Signature) check() error {
	if s.V != Version || s.W != Width || s.H != Height {
		return bad("v, w and h must be " + strconv.Itoa(Version) + ", " + strconv.Itoa(Width) + " and " + strconv.Itoa(Height))
	}
	if len(s.S) == 0 || len(s.S) > MaxStrokes {
		return bad("1 to " + strconv.Itoa(MaxStrokes) + " strokes")
	}
	points := 0
	minX, minY, maxX, maxY := int32(Width), int32(Height), int32(0), int32(0)
	for _, st := range s.S {
		if len(st)%2 != 0 || len(st) < minPoints*2 {
			return bad("every stroke needs at least 2 points (x and y pairs)")
		}
		points += len(st) / 2
		if points > MaxPoints {
			return bad("at most " + strconv.Itoa(MaxPoints) + " points")
		}
		for i := 0; i < len(st); i += 2 {
			x, y := st[i], st[i+1]
			if x < 0 || x > Width || y < 0 || y > Height {
				return bad("a coordinate is outside the drawing space")
			}
			minX, maxX = min(minX, x), max(maxX, x)
			minY, maxY = min(minY, y), max(maxY, y)
		}
	}
	if int(maxX-minX)*percent < minBoxPct*Width && int(maxY-minY)*percent < minBoxPct*Height {
		return bad("the signature is too small")
	}
	return nil
}
