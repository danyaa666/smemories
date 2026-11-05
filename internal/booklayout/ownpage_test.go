package booklayout

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestStripRemovesImagesAndPictureBackground(t *testing.T) {
	page := `{"bg":{"media":"01K7QB1B2C3D4E5F6G7H8J9K0M"},"overrides":{"cover.photo":{"x":1}},"items":[` +
		`{"id":"aaaaaa","t":"text","text":"Hi"},{"id":"bbbbbb","t":"image","media":"01K7QB1B2C3D4E5F6G7H8J9K0M"},{"id":"cccccc","t":"asset","asset":"star"}]}`
	out, n, err := strip(json.RawMessage(page))
	if err != nil || n != 2 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if strings.Contains(string(out), "01K7QB1B2C3D4E5F6G7H8J9K0M") || strings.Contains(string(out), `"bg"`) || strings.Contains(string(out), "image") {
		t.Errorf("not stripped: %s", out)
	}
	if !strings.Contains(string(out), "aaaaaa") || !strings.Contains(string(out), "cccccc") || !strings.Contains(string(out), "cover.photo") {
		t.Errorf("kept too little: %s", out)
	}
	// a colour or asset background stays, a page without items is fine
	out, n, err = strip(json.RawMessage(`{"bg":{"color":"soft"}}`))
	if err != nil || n != 0 || !strings.Contains(string(out), "soft") {
		t.Errorf("colour bg: %s %d %v", out, n, err)
	}
	if _, _, err := strip(json.RawMessage(`[]`)); err == nil {
		t.Error("a page that is not an object was accepted")
	}
}

func TestWithPageReplacesOnlyThatPage(t *testing.T) {
	cur := `{"v":1,"pages":{"cover":{"items":[{"id":"aaaaaa"}]},"back":{"items":[{"id":"bbbbbb"}]}}}`
	out, err := withPage([]byte(cur), "cover", []byte(`{"items":[{"id":"cccccc"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	var d struct {
		V     int                        `json:"v"`
		Pages map[string]json.RawMessage `json:"pages"`
	}
	if err := json.Unmarshal(out, &d); err != nil || d.V != 1 || len(d.Pages) != 2 {
		t.Fatalf("%s %v", out, err)
	}
	if !strings.Contains(string(d.Pages["cover"]), "cccccc") || strings.Contains(string(d.Pages["cover"]), "aaaaaa") || !strings.Contains(string(d.Pages["back"]), "bbbbbb") {
		t.Errorf("pages: %s", out)
	}
	// no document yet: a fresh one with the single page
	out, err = withPage(nil, "profile", []byte(`{}`))
	if err != nil || string(out) != `{"pages":{"profile":{}},"v":1}` {
		t.Errorf("fresh: %s %v", out, err)
	}
	if _, err := withPage([]byte(`{"v":1,"pages":[]}`), "cover", []byte(`{}`)); err == nil {
		t.Error("a stored document with pages that is not an object was accepted")
	}
}
