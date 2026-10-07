package pdf

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"
)

var writeSamples = flag.Bool("write-samples", false, "write the sample PDFs to docs/templates/ (synthetic photos only)")

// sampleBook is a small, fully synthetic book: invented names, generated photos with distinct widths.
func sampleBook(t testing.TB) (Book, mapSource) {
	t.Helper()
	src := mapSource{"cover": synthJPEG(t, 1050, 1400, 1), "me": synthJPEG(t, 900, 1100, 2)}
	long := ""
	for range 60 {
		long += "Cảm ơn bạn vì bốn năm đại học tuyệt vời, những buổi học nhóm và những chuyến đi. "
	}
	notes := []Note{
		{ID: "n1", Author: "Lê Văn Ưu", Relationship: "Bạn cùng phòng", Message: "Chúc mừng tốt nghiệp! Hẹn gặp lại ở Sài Gòn nhé 🎓🎉❤", PhotoIDs: []string{"p1"}},
		{ID: "n2", Author: "Trần Thị Quỳnh Phương", Relationship: "Lớp trưởng", Message: "Cảm ơn Hồng đã luôn giúp đỡ cả lớp. Chúc bạn thật nhiều thành công!", PhotoIDs: []string{"p1", "p2", "p3"}},
		{ID: "n3", Author: "Nguyễn Minh Khôi", Relationship: "Bạn thân", Message: long},
		{ID: "n4", Author: "Phạm Ngọc Ánh", Message: "👍🏽😀🎉"},
		{ID: "n5", Author: "Đỗ Hữu Nghĩa", Relationship: "Thầy chủ nhiệm", Message: "Chúc em vững bước trên con đường phía trước."},
		{ID: "n6", Author: "Võ Thùy Dương", Relationship: "Bạn cùng nhóm", Message: "Đặng Thị Hồng, cậu là nhất!", PhotoIDs: []string{"p2"}},
		{ID: "n7", Author: "Hoàng Gia Bảo", Relationship: "Câu lạc bộ", Message: "Nhớ về thăm CLB nhé."},
	}
	for i, id := range []string{"p1", "p2", "p3"} {
		src[id] = synthJPEG(t, 700+13*i, 500, 10+i)
	}
	return Book{
		Title: "Kỷ yếu K66", School: "Đại học Bách khoa", Class: "Công nghệ thông tin K66", Year: "2026",
		Motto: "Hẹn gặp lại, bốn năm không quên", CoverPhoto: "cover",
		Profile: Profile{FullName: "Đặng Thị Hồng", Nickname: "Hồng", PhotoID: "me",
			Quote:   "Mỗi kết thúc là một khởi đầu mới.",
			Hobbies: "Đọc sách, chạy bộ, nhiếp ảnh và nấu ăn cùng bạn bè",
			Plans:   "Làm kỹ sư phần mềm, học thêm tiếng Nhật và du lịch khắp Việt Nam"},
		Notes: notes,
	}, src
}

// The sample PDFs in docs/templates/ are for human review; regenerate them with
//
//	go test ./internal/pdf -run TestSamples -write-samples
func TestSamples(t *testing.T) {
	b, src := sampleBook(t)
	for _, id := range []string{"classic", "modern"} {
		out, rep := render(t, id, b, src, Options{Lang: "vi"})
		if rep.Pages != 6 || !rep.Has(WarnTextTruncated) {
			t.Errorf("%s: pages=%d warnings=%s", id, rep.Pages, codes(rep))
		}
		if *writeSamples {
			p := filepath.Join("..", "..", "docs", "templates", id+".pdf")
			if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, out, 0o644); err != nil {
				t.Fatal(err)
			}
			t.Logf("wrote %s (%d KB)", p, len(out)/1024)
		}
	}
}

// AC9: a 24-page A5 book with 30 photos renders in at most 20 s and stays under 512 MB of heap.
func TestBenchmarkBook(t *testing.T) {
	const photos, notes = 30, 63 // 1 cover + 1 profile + 21 notes pages (3 per page) + 1 back = 24 pages
	base := make([][]byte, 6)
	var wg sync.WaitGroup
	for i := range base {
		wg.Add(1)
		go func() { defer wg.Done(); base[i] = synthJPEG(t, 2480+7*i, 1748, i) }() // A5 at 300 DPI, about 4 MP
	}
	wg.Wait()
	src := mapSource{}
	photo := func(i int) string { // distinct bytes per id (a trailing byte after the JPEG end marker)
		id := fmt.Sprintf("photo%02d", i)
		src[id] = append(bytes.Clone(base[i%len(base)]), byte(i))
		return id
	}
	b := Book{Title: "Benchmark", School: "S", Class: "C", Year: "2026", Motto: "M", CoverPhoto: photo(0),
		Profile: Profile{FullName: "Đặng Thị Hồng", PhotoID: photo(1)}}
	next := 2
	for i := range notes {
		n := Note{ID: fmt.Sprint(i), Author: fmt.Sprintf("Bạn số %d", i), Message: "Chúc mừng tốt nghiệp! 🎓 Đặng Thị Hồng"}
		if next < photos {
			n.PhotoIDs = []string{photo(next)}
			next++
		}
		b.Notes = append(b.Notes, n)
	}

	runtime.GC()
	var peak uint64
	stop, done := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		var m runtime.MemStats
		for {
			runtime.ReadMemStats(&m)
			peak = max(peak, m.HeapAlloc)
			select {
			case <-stop:
				return
			case <-time.After(5 * time.Millisecond):
			}
		}
	}()
	start := time.Now()
	var out bytes.Buffer
	rep, err := Render(context.Background(), tmpl(t, "classic"), b, src, &out, Options{Now: fixedNow})
	elapsed := time.Since(start)
	close(stop)
	<-done
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("BENCH pages=%d photos=%d wall=%.2fs peak_heap=%.0fMB pdf=%.1fMB warnings=%d",
		rep.Pages, photos, elapsed.Seconds(), float64(peak)/1e6, float64(out.Len())/1e6, len(rep.Warnings))
	if rep.Pages != 24 {
		t.Errorf("pages = %d, want 24", rep.Pages)
	}
	if elapsed > 20*time.Second {
		t.Errorf("render took %s, limit 20s", elapsed)
	}
	if peak > 512<<20 {
		t.Errorf("peak heap %d MB, limit 512 MB", peak>>20)
	}
}
