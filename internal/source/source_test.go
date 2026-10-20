package source

import (
	"context"
	"fmt"
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

// TestLatestUbuntuArm64UsesCDImage verifies arm64 desktop images resolve from
// the cdimage tree, which nests each release's images under release/.
func TestLatestUbuntuArm64UsesCDImage(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			_, _ = w.Write([]byte(`<a href="24.04/">24.04/</a><a href="26.04/">26.04/</a>`))
		case "/26.04/":
			_, _ = w.Write([]byte(`<a href="release/">release/</a>`))
		case "/26.04/release/":
			_, _ = w.Write([]byte(`<a href="ubuntu-26.04.1-desktop-arm64.iso">iso</a>`))
		default:
			http.NotFound(w, r)
		}
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	current := iso.Image{Family: "Ubuntu", Edition: "Desktop LTS", Version: "24.04", Arch: "arm64"}
	got, err := New().withTestRoot(server.URL+"/").Latest(context.Background(), current)
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if want := "26.04.1"; got.Image.Version != want {
		t.Errorf("available version = %q, want %q", got.Image.Version, want)
	}
	if want := "ubuntu-26.04.1-desktop-arm64.iso"; !strings.HasSuffix(got.URL, "/release/"+want) {
		t.Errorf("URL = %q, want the arm64 image below release/", got.URL)
	}
}

// TestLatestUbuntuInterimRelease verifies interim releases resolve separately
// from LTS ones: the newest .10 image is offered for Ubuntu Desktop, while the
// LTS entry keeps following even-year April releases only.
func TestLatestUbuntuInterimRelease(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			_, _ = w.Write([]byte(`<a href="24.04/">24.04/</a><a href="25.10/">25.10/</a>` +
				`<a href="26.04/">26.04/</a><a href="26.10/">26.10/</a>`))
		case "/25.10/":
			_, _ = w.Write([]byte(`<a href="ubuntu-25.10-desktop-amd64.iso">iso</a>`))
		case "/26.04/":
			_, _ = w.Write([]byte(`<a href="ubuntu-26.04-desktop-amd64.iso">iso</a>`))
		case "/26.10/":
			_, _ = w.Write([]byte(`<a href="ubuntu-26.10-desktop-amd64.iso">iso</a>`))
		default:
			http.NotFound(w, r)
		}
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	cases := map[iso.Image]string{
		{Family: "Ubuntu", Edition: "Desktop LTS", Version: "24.04"}: "26.04",
		{Family: "Ubuntu", Edition: "Desktop", Version: "25.10"}:     "26.10",
	}
	for current, want := range cases {
		got, err := New().withTestRoot(server.URL+"/").Latest(context.Background(), current)
		if err != nil {
			t.Fatalf("%s: Latest: %v", current.Edition, err)
		}
		if got.Image.Version != want {
			t.Errorf("%s: available version = %q, want %q", current.Edition, got.Image.Version, want)
		}
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

	current := iso.Image{Family: "Debian", Version: "12.5.0"}
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

// TestLatestFollowsArchitecture verifies resolution keeps the installed
// image's architecture when a mirror publishes both variants side by side.
func TestLatestFollowsArchitecture(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<a href="debian-13.1.0-amd64-DVD-1.iso">amd64</a>` +
			`<a href="debian-13.1.0-arm64-DVD-1.iso">arm64</a>`))
	}))
	defer server.Close()

	cases := map[string]string{
		"amd64": "debian-13.1.0-amd64-DVD-1.iso",
		"arm64": "debian-13.1.0-arm64-DVD-1.iso",
	}
	for arch, want := range cases {
		current := iso.Image{Family: "Debian", Version: "12.5.0", Arch: arch}
		got, err := New().withTestRoot(server.URL+"/").Latest(context.Background(), current)
		if err != nil {
			t.Fatalf("%s: Latest: %v", arch, err)
		}
		if !strings.HasSuffix(got.URL, "/"+want) {
			t.Errorf("%s: URL = %q, want %q", arch, got.URL, want)
		}
		if got.Image.Arch != arch {
			t.Errorf("%s: resolved arch = %q", arch, got.Image.Arch)
		}
	}
}

// TestLatestRejectsUnsupportedArchitecture verifies a distro without arm64
// images reports the mismatch instead of silently picking another build.
func TestLatestRejectsUnsupportedArchitecture(t *testing.T) {
	current := iso.Image{Family: "Tails OS", Version: "7.0", Arch: "arm64"}
	_, err := New().Latest(context.Background(), current)
	if err == nil || !strings.Contains(err.Error(), "arm64") {
		t.Fatalf("Latest error = %v, want an arm64 mismatch", err)
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

// TestLatestDownloadPageLinksMirrors verifies a distro download page that
// lists absolute per-mirror URLs (EndeavourOS) resolves to a real mirror URL
// rather than the page itself, and picks the first mirror listed.
func TestLatestDownloadPageLinksMirrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(
			`<a href="https://mirror.example/mirror/iso/EndeavourOS_Titan-Nova-2026.08.15.iso">first</a>` +
				`<a href="https://mirror.example/mirror/iso/EndeavourOS_Titan-Nova-2026.08.15.iso.sha512sum">sum</a>` +
				`<a href="https://other.example/endeavouros/iso/EndeavourOS_Titan-Nova-2026.08.15.iso">second</a>`))
	}))
	defer server.Close()

	current := iso.Image{Family: "EndeavourOS", Version: "2026.05.01"}
	got, err := New().withTestRoot(server.URL+"/").Latest(context.Background(), current)
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if want := "2026.08.15"; got.Image.Version != want {
		t.Errorf("available version = %q, want %q", got.Image.Version, want)
	}
	if want := "https://mirror.example/mirror/iso/EndeavourOS_Titan-Nova-2026.08.15.iso"; got.URL != want {
		t.Errorf("URL = %q, want %q", got.URL, want)
	}
}

// TestLatestDownloadPageDataURL verifies a download page whose image URL only
// appears in a data-url attribute (Nobara) is still resolved.
func TestLatestDownloadPageDataURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(
			`<a href="https://images.example/Nobara-44-Official-2026-09-02.iso.sha256sum">sum</a>` +
				`<button class="download-btn" data-iso="Nobara-44-Official-2026-09-02.iso" ` +
				`data-url="https://images.example/Nobara-44-Official-2026-09-02.iso">Download</button>`))
	}))
	defer server.Close()

	current := iso.Image{Family: "Nobara", Edition: "Official", Version: "43"}
	got, err := New().withTestRoot(server.URL+"/").Latest(context.Background(), current)
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if want := "https://images.example/Nobara-44-Official-2026-09-02.iso"; got.URL != want {
		t.Errorf("URL = %q, want %q", got.URL, want)
	}
}

// TestLatestCachyOSReleaseDirectories verifies six-digit release directories
// (CachyOS) resolve like any other per-release layout.
func TestLatestCachyOSReleaseDirectories(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			_, _ = w.Write([]byte(`<a href="260628/">old</a><a href="260809/">new</a>`))
		case "/260809/":
			_, _ = w.Write([]byte(`<a href="cachyos-desktop-linux-260809.iso">iso</a>`))
		default:
			http.NotFound(w, r)
		}
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	current := iso.Image{Family: "CachyOS", Flavor: "Desktop", Version: "260628"}
	got, err := New().withTestRoot(server.URL+"/").Latest(context.Background(), current)
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if want := "260809"; got.Image.Version != want {
		t.Errorf("available version = %q, want %q", got.Image.Version, want)
	}
	if !strings.HasSuffix(got.URL, "/260809/cachyos-desktop-linux-260809.iso") {
		t.Errorf("URL = %q, want the CachyOS desktop image", got.URL)
	}
}

// TestLatestOmarchyDownloadPage verifies a project page linking its own ISO
// host resolves the image through the absolute link.
func TestLatestOmarchyDownloadPage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(
			`<a href="https://iso.example/omarchy-4.0.4.iso">download</a>` +
				`<a href="https://iso.example/omarchy-4.0.4.iso.sha256">sum</a>`))
	}))
	defer server.Close()

	current := iso.Image{Family: "Omarchy", Version: "4.0.2"}
	got, err := New().withTestRoot(server.URL+"/").Latest(context.Background(), current)
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if want := "4.0.4"; got.Image.Version != want {
		t.Errorf("available version = %q, want %q", got.Image.Version, want)
	}
	if want := "https://iso.example/omarchy-4.0.4.iso"; got.URL != want {
		t.Errorf("URL = %q, want %q", got.URL, want)
	}
}

// TestLatestSourceForgeReleaseDirectories verifies a SourceForge mirror's
// per-release directories (ShredOS) resolve through the page's links and skip
// the lite build that shares the release's version date.
func TestLatestSourceForgeReleaseDirectories(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/files/":
			_, _ = w.Write([]byte(
				`<a href="/files/v2025.11_31_x86-64_0.42/">old</a>` +
					`<a href="/files/v2025.11_32_x86-64_0.43.1/">new</a>`))
		case "/files/v2025.11_32_x86-64_0.43.1/":
			_, _ = w.Write([]byte(
				`<a href="/files/v2025.11_32_x86-64_0.43.1/shredos-2025.11_32_x86-64_v0.43.1_20261010_lite.iso/download">lite</a>` +
					`<a href="/files/v2025.11_32_x86-64_0.43.1/shredos-2025.11_32_x86-64_v0.43.1_20261010.img/download">img</a>` +
					`<a href="/files/v2025.11_32_x86-64_0.43.1/shredos-2025.11_32_x86-64_v0.43.1_20261010.iso/download">full</a>`))
		default:
			http.NotFound(w, r)
		}
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	current := iso.Image{Family: "ShredOS", Version: "20260716"}
	got, err := New().withTestRoot(server.URL+"/files/").Latest(context.Background(), current)
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if want := "20261010"; got.Image.Version != want {
		t.Errorf("available version = %q, want %q", got.Image.Version, want)
	}
	want := server.URL + "/files/v2025.11_32_x86-64_0.43.1/shredos-2025.11_32_x86-64_v0.43.1_20261010.iso/download"
	if got.URL != want {
		t.Errorf("URL = %q, want %q", got.URL, want)
	}
}

// TestLatestGPartedReportsAge verifies GParted resolves through its SourceForge
// release directory and keeps the file's Last-Modified date. The project's own
// download page links downloads.sourceforge.net directly, which serves Go an
// HTML interstitial without file metadata.
func TestLatestGPartedReportsAge(t *testing.T) {
	modified := time.Date(2026, 8, 30, 12, 33, 24, 0, time.UTC)
	const size = 720371712
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/gparted-live-stable/":
			_, _ = w.Write([]byte(
				`<a href="/gparted-live-stable/1.8.1-3/">old</a>` +
					`<a href="/gparted-live-stable/1.8.1-6/">new</a>`))
		case "/gparted-live-stable/1.8.1-6/":
			_, _ = w.Write([]byte(
				`<a href="/gparted-live-stable/1.8.1-6/gparted-live-1.8.1-6-amd64.iso/download">iso</a>`))
		case "/gparted-live-stable/1.8.1-6/gparted-live-1.8.1-6-amd64.iso/download":
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("Content-Length", strconv.Itoa(size))
			w.Header().Set("Last-Modified", modified.UTC().Format(http.TimeFormat))
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	current := iso.Image{Family: "GParted Live", Version: "1.8.1-6"}
	got, err := New().withTestRoot(server.URL+"/gparted-live-stable/").Latest(context.Background(), current)
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if want := "1.8.1-6"; got.Image.Version != want {
		t.Errorf("available version = %q, want %q", got.Image.Version, want)
	}
	if !got.Published.Equal(modified) {
		t.Errorf("published = %v, want %v", got.Published, modified)
	}
	if got.Size != size {
		t.Errorf("size = %d, want %d", got.Size, size)
	}
}

// TestProbeIgnoresHTMLInterstitials verifies a redirect service's HTML page
// length is not mistaken for the ISO size: the ranged GET then supplies the
// real total through Content-Range.
func TestProbeIgnoresHTMLInterstitials(t *testing.T) {
	const size = 700 << 20
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.Header().Set("Content-Type", "text/html; charset=UTF-8")
			w.Header().Set("Content-Length", "147569")
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Range", fmt.Sprintf("bytes 0-0/%d", size))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte("x"))
	}))
	defer server.Close()
	if _, got := New().probe(context.Background(), server.URL); got != size {
		t.Errorf("probe size = %d, want %d", got, size)
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
