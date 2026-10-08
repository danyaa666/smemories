package media

import (
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestMeasurePeak is the measurement behind docs/media.md, skipped unless SMEM_MEASURE_FILES is set:
//
//	SMEM_MEASURE_FILES=a.png,b.jpg SMEM_MEASURE_PAR=2 SMEM_MEASURE_CONC=2 /usr/bin/time -l go test ./internal/media -run MeasurePeak -v
//
// It starts SMEM_MEASURE_PAR copies of each file at once, of which SMEM_MEASURE_CONC (like SMEM_MEDIA_MAX_CONCURRENT) run process at a time, and prints the peak Go heap and OS memory
// (runtime Sys). The peak resident set size is what `time -l` reports for the whole run.
func TestMeasurePeak(t *testing.T) {
	files := os.Getenv("SMEM_MEASURE_FILES")
	if files == "" {
		t.Skip("set SMEM_MEASURE_FILES to measure")
	}
	par, _ := strconv.Atoi(os.Getenv("SMEM_MEASURE_PAR"))
	par = max(par, 1)
	conc, _ := strconv.Atoi(os.Getenv("SMEM_MEASURE_CONC"))
	sem := make(chan struct{}, max(conc, 1))
	var peakHeap, peakSys uint64
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		var m runtime.MemStats
		for {
			runtime.ReadMemStats(&m)
			peakHeap, peakSys = max(peakHeap, m.HeapInuse), max(peakSys, m.Sys-m.HeapReleased)
			select {
			case <-stop:
				return
			case <-time.After(2 * time.Millisecond):
			}
		}
	}()
	var wg sync.WaitGroup
	for range par {
		for _, f := range strings.Split(files, ",") {
			data, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				if _, err := process(data); err != nil {
					t.Errorf("%s: %v", f, err)
				}
			}()
		}
	}
	wg.Wait()
	close(stop)
	<-done
	t.Logf("%s x%d, %d at a time: peak heap in use %d MiB, peak Go-held OS memory %d MiB", files, par, cap(sem), peakHeap>>20, peakSys>>20)
}
