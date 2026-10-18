package app

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// shortIdle shrinks the stall window for the duration of a test.
func shortIdle(t *testing.T, d time.Duration) {
	t.Helper()
	previous := downloadIdleTimeout
	downloadIdleTimeout = d
	t.Cleanup(func() { downloadIdleTimeout = previous })
}

// TestDownloadOutlastsIdleWindow verifies a body whose total transfer time
// exceeds the idle window still succeeds: only silence should abort a download,
// never its overall duration. This is the regression guard for large ISOs that
// previously died with "context deadline exceeded ... while reading body".
func TestDownloadOutlastsIdleWindow(t *testing.T) {
	const chunk = 64 << 10
	payload := bytes.Repeat([]byte("ventoy"), 4*chunk/6)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
		flusher, _ := w.(http.Flusher)
		for off := 0; off < len(payload); off += chunk {
			end := min(off+chunk, len(payload))
			if _, err := w.Write(payload[off:end]); err != nil {
				return
			}
			flusher.Flush()
			time.Sleep(15 * time.Millisecond)
		}
	}))
	defer server.Close()

	shortIdle(t, 40*time.Millisecond)

	dir := t.TempDir()
	target := dir + "/image.iso"
	var lastDone, lastTotal int64
	err := download(server.URL, target, func(done, total int64) { lastDone, lastTotal = done, total })
	if err != nil {
		t.Fatalf("download returned %v, want nil", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Errorf("downloaded %d bytes, want %d identical bytes", len(got), len(payload))
	}
	if lastDone != int64(len(payload)) || lastTotal != int64(len(payload)) {
		t.Errorf("final progress = (%d, %d), want (%d, %d)", lastDone, lastTotal, len(payload), len(payload))
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "image.iso" {
		t.Errorf("temporary files left behind: %v", entries)
	}
}

// TestDownloadStopsWhenBodyStalls verifies a connection that goes quiet is
// reported instead of hanging forever.
func TestDownloadStopsWhenBodyStalls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1024")
		w.Write([]byte("partial"))
		w.(http.Flusher).Flush()
		time.Sleep(300 * time.Millisecond)
	}))
	defer server.Close()

	shortIdle(t, 40*time.Millisecond)

	target := t.TempDir() + "/image.iso"
	err := download(server.URL, target, nil)
	if err == nil || !strings.Contains(err.Error(), "stalled") {
		t.Fatalf("download error = %v, want a stalled-download error", err)
	}
	if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
		t.Errorf("stalled download left %s in place (stat err %v)", target, statErr)
	}
}

// TestDownloadReportsHTTPError verifies a non-200 response fails before any
// temporary file is created.
func TestDownloadReportsHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "gone", http.StatusNotFound)
	}))
	defer server.Close()

	dir := t.TempDir()
	err := download(server.URL, dir+"/image.iso", nil)
	if err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("download error = %v, want a 404 report", err)
	}
	if entries, readErr := os.ReadDir(dir); readErr != nil || len(entries) != 0 {
		t.Errorf("failed download left files behind: %v (err %v)", entries, readErr)
	}
}
