package source

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/guluc3m/ventoy-updater/internal/iso"
)

// TestCompareVersions verifies numeric, rather than lexical, comparison.
func TestCompareVersions(t *testing.T) {
	if CompareVersions("ubuntu-26.04.10-desktop.iso", "ubuntu-26.04.2-desktop.iso") <= 0 {
		t.Fatal("did not compare numeric components")
	}
}

// TestAgeSuffix verifies yay-style elapsed-time formatting, without brackets.
func TestAgeSuffix(t *testing.T) {
	now := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	cases := map[string]time.Time{
		"9h41m": now.Add(-9*time.Hour - 41*time.Minute),
		"1d8h":  now.Add(-32 * time.Hour),
		"2y4m":  now.Add(-29 * 30 * 24 * time.Hour),
		"45m":   now.Add(-45 * time.Minute),
		"":      now.Add(2 * time.Hour),
	}
	if got := AgeSuffix(time.Time{}, now); got != "" {
		t.Errorf("AgeSuffix(zero time) = %q, want empty", got)
	}
	for want, published := range cases {
		if got := AgeSuffix(published, now); got != want {
			t.Errorf("AgeSuffix(%v) = %q, want %q", published, got, want)
		}
	}
}

// TestLatestUbuntuPrefersPointRelease verifies that point-release directories
// such as 26.04.1 beat their base release 26.04, and that the base ISO shipped
// inside a point-release directory does not win over the point-release ISO.
func TestLatestUbuntuPrefersPointRelease(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			_, _ = w.Write([]byte(`<a href="24.04/">24.04/</a><a href="26.04/">26.04/</a><a href="26.04.1/">26.04.1/</a>`))
			return
		}
		if r.URL.Path == "/26.04.1/" {
			// Real mirrors keep the base ISO next to the point-release one.
			_, _ = w.Write([]byte(`<a href="ubuntu-26.04-desktop-amd64.iso">old</a><a href="ubuntu-26.04.1-desktop-amd64.iso">new</a>`))
			return
		}
		_, _ = w.Write([]byte(`<a href="ubuntu-` + strings.Trim(r.URL.Path, "/") + `-desktop-amd64.iso">iso</a>`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	current := iso.Image{Family: "Ubuntu", Edition: "Desktop LTS", Version: "24.04"}
	got, err := New().withTestRoot(server.URL+"/").Latest(context.Background(), current)
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if want := "26.04.1"; got.Image.Version != want {
		t.Errorf("available version = %q, want %q", got.Image.Version, want)
	}
	if !strings.Contains(got.URL, "ubuntu-26.04.1-desktop-amd64.iso") {
		t.Errorf("URL = %q, want point-release ISO", got.URL)
	}
}

// TestLatestUsesCatalogForDirectListing verifies distributions whose releases
// live directly at the catalog index (Debian) resolve through the same
// catalog entry as release-directory distributions.
func TestLatestUsesCatalogForDirectListing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<a href="debian-12.5.0-amd64-DVD-1.iso">old</a><a href="debian-13.1.0-amd64-DVD-1.iso">new</a>`))
	}))
	defer server.Close()

	current := iso.Image{Family: "Debian", Edition: "DVD", Version: "12.5.0"}
	got, err := New().withTestRoot(server.URL+"/").Latest(context.Background(), current)
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if want := "13.1.0"; got.Image.Version != want {
		t.Errorf("available version = %q, want %q", got.Image.Version, want)
	}
	if !strings.Contains(got.URL, "debian-13.1.0-amd64-DVD-1.iso") {
		t.Errorf("URL = %q, want newest direct-listing ISO", got.URL)
	}
}

// TestLatestOpenSUSELeap verifies Leap 15.x DVD and 16.x offline layouts are
// both resolvable, that the newest release with an actual image wins, and that
// an empty or not-yet-populated newer release directory is skipped.
func TestLatestOpenSUSELeap(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			_, _ = w.Write([]byte(`<a href="./15.6/">15.6/</a><a href="./16.0/">16.0/</a><a href="./16.1/">16.1/</a>`))
		case "/16.1/":
			_, _ = w.Write([]byte(`<a href="./offline/">offline/</a>`))
		case "/16.1/offline/":
			_, _ = w.Write([]byte(`<a href="README">no images yet</a>`))
		case "/16.0/":
			_, _ = w.Write([]byte(`<a href="./offline/">offline/</a>`))
		case "/16.0/offline/":
			// Mirrors publish a build-qualified copy next to the stable alias.
			_, _ = w.Write([]byte(`<a href="Leap-16.0-offline-installer-x86_64-Build178.27.install.iso">build</a>` +
				`<a href="Leap-16.0-offline-installer-x86_64.install.iso">stable</a>`))
		case "/15.6/":
			_, _ = w.Write([]byte(`<a href="./iso/">iso/</a>`))
		case "/15.6/iso/":
			_, _ = w.Write([]byte(`<a href="openSUSE-Leap-15.6-DVD-x86_64-Build710.3-Media.iso">build</a>` +
				`<a href="openSUSE-Leap-15.6-DVD-x86_64-Media.iso">stable</a>`))
		default:
			http.NotFound(w, r)
		}
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	current := iso.Image{Family: "openSUSE", Edition: "Leap", Version: "15.6"}
	got, err := New().withTestRoot(server.URL+"/").Latest(context.Background(), current)
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if want := "16.0"; got.Image.Version != want {
		t.Errorf("available version = %q, want %q", got.Image.Version, want)
	}
	if !strings.HasSuffix(got.URL, "/16.0/offline/Leap-16.0-offline-installer-x86_64.install.iso") {
		t.Errorf("URL = %q, want the stable (unqualified) Leap 16.0 image", got.URL)
	}
}

// TestLatestOpenSUSELeapDVDOnly verifies a Leap 15.x mirror, which has no
// offline/ directory at all, resolves through the release's iso/ directory.
func TestLatestOpenSUSELeapDVDOnly(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			_, _ = w.Write([]byte(`<a href="./15.6/">15.6/</a>`))
		case "/15.6/":
			_, _ = w.Write([]byte(`<a href="./iso/">iso/</a>`))
		case "/15.6/iso/":
			_, _ = w.Write([]byte(`<a href="openSUSE-Leap-15.6-DVD-x86_64-Build710.3-Media.iso">build</a>` +
				`<a href="openSUSE-Leap-15.6-DVD-x86_64-Media.iso">stable</a>`))
		default:
			http.NotFound(w, r)
		}
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	current := iso.Image{Family: "openSUSE", Edition: "Leap", Version: "15.5"}
	got, err := New().withTestRoot(server.URL+"/").Latest(context.Background(), current)
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if want := "15.6"; got.Image.Version != want {
		t.Errorf("available version = %q, want %q", got.Image.Version, want)
	}
	if !strings.HasSuffix(got.URL, "/15.6/iso/openSUSE-Leap-15.6-DVD-x86_64-Media.iso") {
		t.Errorf("URL = %q, want the Leap 15.6 DVD image", got.URL)
	}
}

// TestLatestOpenSUSELeapBuildOnly verifies that a mirror listing only the
// build-qualified image still yields an update instead of an unavailable row.
func TestLatestOpenSUSELeapBuildOnly(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			_, _ = w.Write([]byte(`<a href="./16.1/">16.1/</a>`))
		case "/16.1/":
			_, _ = w.Write([]byte(`<a href="./offline/">offline/</a>`))
		case "/16.1/offline/":
			_, _ = w.Write([]byte(`<a href="Leap-16.1-offline-installer-x86_64-Build55.2.install.iso">iso</a>`))
		default:
			http.NotFound(w, r)
		}
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	current := iso.Image{Family: "openSUSE", Edition: "Leap", Version: "16.0"}
	got, err := New().withTestRoot(server.URL+"/").Latest(context.Background(), current)
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if want := "16.1"; got.Image.Version != want {
		t.Errorf("available version = %q, want %q", got.Image.Version, want)
	}
	if !strings.Contains(got.URL, "Leap-16.1-offline-installer-x86_64-Build55.2.install.iso") {
		t.Errorf("URL = %q, want the build-qualified image", got.URL)
	}
}

// TestProbe verifies that the Last-Modified header and advertised size fill
// Candidate.Published and Candidate.Size.
func TestProbe(t *testing.T) {
	modified := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	const size = 4 << 30
	sawHead := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			sawHead = true
		}
		w.Header().Set("Last-Modified", modified.UTC().Format(http.TimeFormat))
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	published, got := New().probe(context.Background(), server.URL)
	if !sawHead {
		t.Error("did not try HEAD before ranged GET fallback")
	}
	if !published.Equal(modified) {
		t.Errorf("probe published = %v, want %v", published, modified)
	}
	if got != size {
		t.Errorf("probe size = %d, want %d", got, size)
	}
}
