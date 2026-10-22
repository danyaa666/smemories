package signature

import (
	"encoding/json"
	"os"
	"testing"
)

// testdata/canonical.json is shared with the web pad (web/src/signature.test.ts reads the same file): every "valid" document is in
// canonical form and must come back from Parse byte for byte; every "invalid" one must be refused. One file passes both validators.
func TestSharedFixture(t *testing.T) {
	raw, err := os.ReadFile("testdata/canonical.json")
	if err != nil {
		t.Fatal(err)
	}
	var fx struct {
		Valid []struct {
			Name      string `json:"name"`
			Canonical string `json:"canonical"`
		} `json:"valid"`
		Invalid []struct {
			Name string `json:"name"`
			JSON string `json:"json"`
		} `json:"invalid"`
	}
	if err := json.Unmarshal(raw, &fx); err != nil {
		t.Fatal(err)
	}
	if len(fx.Valid) == 0 || len(fx.Invalid) == 0 {
		t.Fatal("fixture is empty")
	}
	for _, c := range fx.Valid {
		_, canon, err := Parse([]byte(c.Canonical))
		if err != nil {
			t.Errorf("valid %q refused: %v", c.Name, err)
		} else if string(canon) != c.Canonical {
			t.Errorf("valid %q is not canonical: re-serialised to a different string", c.Name)
		}
	}
	for _, c := range fx.Invalid {
		if _, _, err := Parse([]byte(c.JSON)); err == nil {
			t.Errorf("invalid %q accepted", c.Name)
		}
	}
}
