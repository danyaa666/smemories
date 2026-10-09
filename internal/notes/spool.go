package notes

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	spoolPrefix = "smem-upload-"
	// spoolMaxAge is the longest a live upload can take (route timeout) plus a minute: older files are orphans.
	spoolMaxAge = submitRouteTimeout + time.Minute
)

// SweepSpool deletes orphaned upload spool files (a crash, kill or cut-off drain skips the handler's cleanup) and
// returns how many it removed. Call it once at start-up, before the listener accepts traffic. Only regular files
// directly in the spool directory whose name starts with smem-upload- and whose age exceeds spoolMaxAge are
// touched; failures are logged at WARN and never stop the caller.
func (h *Handler) SweepSpool() int {
	dir := h.tmpDir
	if dir == "" {
		dir = os.TempDir()
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		h.logger.Warn("upload spool sweep: cannot list directory", "error", err)
		return 0
	}
	cutoff := time.Now().Add(-spoolMaxAge)
	removed := 0
	for _, e := range ents {
		if !strings.HasPrefix(e.Name(), spoolPrefix) || !e.Type().IsRegular() { // symlinks and directories have another type
			continue
		}
		if fi, err := e.Info(); err != nil || !fi.ModTime().Before(cutoff) {
			continue // vanished, or maybe still being written by a live upload
		}
		if err := os.Remove(filepath.Join(dir, e.Name())); err != nil {
			h.logger.Warn("upload spool sweep: cannot remove a file", "error", err)
			continue
		}
		removed++
	}
	h.logger.Info("upload spool sweep", "removed", removed)
	return removed
}
