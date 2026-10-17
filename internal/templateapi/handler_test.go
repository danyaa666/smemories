package templateapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/danyaa666/smemories/internal/httpx"
	"github.com/danyaa666/smemories/internal/templates"
)

type listed struct {
	Status string
	Data   struct {
		Items  []item `json:"items"`
		NextID string `json:"next_id"`
	}
}

func get(t *testing.T, query string) (int, listed) {
	t.Helper()
	pass := func(h http.Handler) http.Handler { return h } // the session check has its own tests
	h := httpx.NewRouter(slog.New(slog.DiscardHandler), NewHandler(pass).Routes)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/template/get-list"+query, nil))
	var out listed
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("%s: %v", rec.Body.String(), err)
	}
	return rec.Code, out
}

func ids(items []item) []string {
	out := []string{}
	for _, i := range items {
		out = append(out, i.ID)
	}
	return out
}

func TestListAll(t *testing.T) {
	code, out := get(t, "")
	if code != 200 || out.Status != "OK" || out.Data.NextID != "" {
		t.Fatalf("%d %+v", code, out)
	}
	if want := len(templates.List()); len(out.Data.Items) != want {
		t.Fatalf("%d items, want %d", len(out.Data.Items), want)
	}
	for _, i := range out.Data.Items {
		if i.Name["en"] == "" || i.Name["vi"] == "" || len(i.PageSizes) == 0 || (i.Renderer != "html" && i.Renderer != "go") || i.Preview != PreviewDir+i.ID+".jpg" {
			t.Errorf("incomplete item %+v", i)
		}
	}
}

// The page size filter is the registry's own: a template that does not declare the size is not offered for it.
func TestListByPageSize(t *testing.T) {
	for _, size := range PageSizes {
		code, out := get(t, "?page_size="+size)
		if code != 200 {
			t.Fatalf("%s: %d", size, code)
		}
		for _, i := range out.Data.Items {
			if !slices.Contains(i.PageSizes, size) {
				t.Errorf("%s offered for %s: %v", i.ID, size, i.PageSizes)
			}
		}
		if !slices.Contains(ids(out.Data.Items), "memory-book") {
			t.Errorf("memory-book missing for %s: %v", size, ids(out.Data.Items))
		}
	}
	_, letter := get(t, "?page_size=Letter")
	if slices.Contains(ids(letter.Data.Items), "classic") {
		t.Errorf("classic has no Letter: %v", ids(letter.Data.Items))
	}
}

func TestListBadPageSize(t *testing.T) {
	for _, q := range []string{"?page_size=A3", "?page_size=a5", "?page_size=%00"} {
		code, out := get(t, q)
		if code != 400 || out.Status != "ERROR_PARAM" {
			t.Errorf("%s: %d %s", q, code, out.Status)
		}
	}
}

// Every template the browser draws has a preview image in the web app's public folder.
func TestHTMLTemplatesHavePreviews(t *testing.T) {
	for _, i := range templates.List() {
		if i.Renderer != templates.RendererHTML {
			continue
		}
		p := filepath.Join("..", "..", "web", "public", strings.TrimPrefix(PreviewDir, "/"), i.ID+".jpg")
		if st, err := os.Stat(p); err != nil || st.Size() == 0 {
			t.Errorf("template %s has no preview at %s: %v", i.ID, p, err)
		}
	}
}
