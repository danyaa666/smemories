package pdf

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/danyaa666/smemories/internal/templates"
)

// An html template is drawn by the web app: the Go renderer must refuse it, not panic or emit an empty PDF.
func TestRenderRefusesHTMLTemplate(t *testing.T) {
	tt, err := templates.Parse([]byte(`{"id":"web","renderer":"html","name":{"en":"W","vi":"W"},"page_sizes":["A5"]}`))
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	_, err = Render(context.Background(), tt, Book{}, nil, &buf, Options{})
	if err == nil || !strings.Contains(err.Error(), "html template") || buf.Len() != 0 {
		t.Errorf("err = %v, %d bytes written", err, buf.Len())
	}
}
