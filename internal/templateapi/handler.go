// Package templateapi serves GET /api/template/get-list: the built-in templates a signed-in owner can pick for a book
// (T-061). The registry is in memory (internal/templates), so there is no service or store behind it.
package templateapi

import (
	"net/http"
	"slices"

	"github.com/danyaa666/smemories/internal/apperr"
	"github.com/danyaa666/smemories/internal/httpx"
	"github.com/danyaa666/smemories/internal/templates"
)

// PreviewDir is where the web app serves the preview images (web/public/template-previews/<id>.jpg).
const PreviewDir = "/template-previews/"

// PageSizes are the sizes a book can have, the valid values of the page_size filter.
var PageSizes = []string{"A5", "A4", "Letter"}

// item is one template as the picker shows it.
type item struct {
	ID        string            `json:"id"`
	Name      map[string]string `json:"name"`
	PageSizes []string          `json:"page_sizes"`
	Renderer  string            `json:"renderer"`
	Preview   string            `json:"preview"`
}

// Handler serves the template endpoints.
type Handler struct {
	requireUser func(http.Handler) http.Handler // auth.Handler.RequireUser
}

// NewHandler builds the handler.
func NewHandler(requireUser func(http.Handler) http.Handler) *Handler {
	return &Handler{requireUser: requireUser}
}

// Routes registers the endpoint on mux (pass to httpx.NewRouter). A GET changes nothing, so there is no origin guard.
func (h *Handler) Routes(mux *http.ServeMux) {
	mux.Handle("GET /api/template/get-list", h.requireUser(http.HandlerFunc(h.list)))
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	infos := templates.List()
	if size := r.URL.Query().Get("page_size"); size != "" {
		if !slices.Contains(PageSizes, size) {
			httpx.Fail(w, r, apperr.New(apperr.Param, "page_size must be A5, A4 or Letter"))
			return
		}
		infos = templates.ForPageSize(size)
	}
	items := make([]item, 0, len(infos))
	for _, i := range infos {
		items = append(items, item{ID: i.ID, Name: i.Name, PageSizes: i.PageSizes, Renderer: i.Renderer, Preview: PreviewDir + i.ID + ".jpg"})
	}
	httpx.OK(w, r, httpx.Page[item]{Items: items}) // not paginated: the registry is a handful of templates
}
