// Package source resolves the newest image from official distro release metadata.
package source

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/guluc3m/ventoy-updater/internal/iso"
)

// Candidate is a parsed ISO, its official download URL, the mirror-reported
// upload timestamp (zero when the mirror does not provide one), and the
// advertised file size (zero when unknown).
type Candidate struct {
	Image     iso.Image
	URL       string
	Published time.Time
	Size      int64
}

// Resolver fetches official distribution release pages.
type Resolver struct {
	Client *http.Client
	// ubuntuRoot points at the Ubuntu release index; tests may override it.
	ubuntuRoot string
}

// New creates a resolver with a bounded HTTP timeout.
func New() *Resolver {
	return &Resolver{Client: &http.Client{Timeout: 30 * time.Second}, ubuntuRoot: "https://releases.ubuntu.com/"}
}

// withTestRoot retargets the Ubuntu index; used by tests against fake mirrors.
func (r *Resolver) withTestRoot(root string) *Resolver {
	r.ubuntuRoot = root
	return r
}

// Latest starts at each distro's release index, rather than an installed-version
// URL, so it can discover new major releases as well as point releases.
func (r *Resolver) Latest(ctx context.Context, current iso.Image) (Candidate, error) {
	switch current.Family {
	case "Ubuntu":
		return r.latestReleaseDirectory(ctx, r.ubuntuRoot, regexp.MustCompile(`^[0-9]{2}\.04(\.[0-9]+)?/$`), regexp.MustCompile(`^ubuntu-[0-9.]+-desktop-amd64\.iso$`))
	case "Fedora":
		return r.latestReleaseDirectory(ctx, "https://dl.fedoraproject.org/pub/fedora/linux/releases/", regexp.MustCompile(`^[0-9]+/$`), regexp.MustCompile(`^Fedora-Workstation-Live-[0-9.-]+\.x86_64\.iso$`), "Workstation/x86_64/iso/")
	case "Linux Mint":
		return r.latestReleaseDirectory(ctx, "https://mirrors.edge.kernel.org/linuxmint/stable/", regexp.MustCompile(`^[0-9]+\.[0-9]+/$`), regexp.MustCompile(`^linuxmint-[0-9.]+-cinnamon-64bit\.iso$`))
	case "Tails":
		return r.latestTails(ctx)
	case "Manjaro":
		return r.latestManjaro(ctx, current.Flavor)
	case "openSUSE":
		if current.Edition == "Tumbleweed" {
			return r.latestListing(ctx, "https://download.opensuse.org/tumbleweed/iso/", regexp.MustCompile(`^openSUSE-Tumbleweed-DVD-x86_64-Snapshot[0-9]+-Media\.iso$`))
		}
		return r.latestReleaseDirectory(ctx, "https://download.opensuse.org/distribution/leap/", regexp.MustCompile(`^[12][0-9]\.[0-9]+/$`), regexp.MustCompile(`^Leap-[0-9.]+-offline-installer-x86_64\.install\.iso$`), "offline/")
	case "Debian":
		return r.latestListing(ctx, "https://cdimage.debian.org/debian-cd/current/amd64/iso-dvd/", regexp.MustCompile(`^debian-[0-9.]+-amd64-DVD-1\.iso$`))
	case "Arch Linux":
		return r.latestListing(ctx, "https://geo.mirror.pkgbuild.com/iso/latest/", regexp.MustCompile(`^archlinux-[0-9.]+-x86_64\.iso$`))
	}
	return Candidate{}, fmt.Errorf("no official source configured for %s (%s)", current.Family, current.Filename)
}

// latestReleaseDirectory finds the newest stable release directory then its ISO.
func (r *Resolver) latestReleaseDirectory(ctx context.Context, root string, directory, filename *regexp.Regexp, suffix ...string) (Candidate, error) {
	links, err := r.links(ctx, root)
	if err != nil {
		return Candidate{}, err
	}
	var dirs []string
	for _, link := range links {
		if directory.MatchString(link) {
			dirs = append(dirs, link)
		}
	}
	if len(dirs) == 0 {
		return Candidate{}, fmt.Errorf("no release directories found at %s", root)
	}
	sort.Slice(dirs, func(i, j int) bool { return compareVersions(dirs[i], dirs[j]) < 0 })
	return r.latestListing(ctx, root+dirs[len(dirs)-1]+strings.Join(suffix, ""), filename)
}

// latestTails follows the versioned directories used by the Tails stable mirror.
func (r *Resolver) latestTails(ctx context.Context) (Candidate, error) {
	const root = "https://download.tails.net/tails/stable/"
	links, err := r.links(ctx, root)
	if err != nil {
		return Candidate{}, err
	}
	dirRE := regexp.MustCompile(`^tails-amd64-[0-9.]+/$`)
	var dirs []string
	for _, link := range links {
		if dirRE.MatchString(link) {
			dirs = append(dirs, link)
		}
	}
	if len(dirs) == 0 {
		return Candidate{}, fmt.Errorf("no Tails stable releases found")
	}
	sort.Slice(dirs, func(i, j int) bool { return compareVersions(dirs[i], dirs[j]) < 0 })
	return r.latestListing(ctx, root+dirs[len(dirs)-1], regexp.MustCompile(`^tails-amd64-[0-9.]+\.iso$`))
}

// latestManjaro parses the official download page, which provides current ISO URLs.
func (r *Resolver) latestManjaro(ctx context.Context, flavor string) (Candidate, error) {
	if flavor != "KDE" {
		return Candidate{}, fmt.Errorf("unsupported Manjaro flavour %q", flavor)
	}
	return r.latestListing(ctx, "https://manjaro.org/products/download/x86/", regexp.MustCompile(`^manjaro-kde-[0-9.]+-[0-9]+-linux[0-9]+\.iso$`))
}

// latestListing selects the greatest matching ISO version from an official page.
func (r *Resolver) latestListing(ctx context.Context, index string, filename *regexp.Regexp) (Candidate, error) {
	links, err := r.links(ctx, index)
	if err != nil {
		return Candidate{}, err
	}
	var images []iso.Image
	for _, link := range links {
		if filename.MatchString(path.Base(link)) {
			if image, ok := iso.Parse(path.Base(link)); ok {
				images = append(images, image)
			}
		}
	}
	if len(images) == 0 {
		return Candidate{}, fmt.Errorf("no matching ISO found at %s", index)
	}
	// Compare parsed versions, not filenames: trailing tokens like "amd64"
	// would otherwise poison the numeric runs (26.04.1 vs 26.04 amd64).
	sort.Slice(images, func(i, j int) bool { return compareVersions(images[i].Version, images[j].Version) < 0 })
	image := images[len(images)-1]
	base, _ := url.Parse(index)
	ref, _ := url.Parse(image.Filename)
	resolved := base.ResolveReference(ref).String()
	published, size := r.probe(ctx, resolved)
	return Candidate{Image: image, URL: resolved, Published: published, Size: size}, nil
}

// probe reads the mirror's Last-Modified timestamp and advertised size for the
// chosen ISO. Some mirrors reject HEAD, so a one-byte ranged GET is used as
// fallback; both values are best-effort and missing ones are not errors.
func (r *Resolver) probe(ctx context.Context, address string) (time.Time, int64) {
	if t, n := r.metadata(ctx, http.MethodHead, address); !t.IsZero() || n > 0 {
		return t, n
	}
	return r.metadata(ctx, http.MethodGet, address)
}

// metadata performs a request and parses Last-Modified and the file length.
// A HEAD answers with Content-Length; a ranged GET hides the total length
// behind its Content-Range header instead.
func (r *Resolver) metadata(ctx context.Context, method, address string) (time.Time, int64) {
	req, err := http.NewRequestWithContext(ctx, method, address, nil)
	if err != nil {
		return time.Time{}, 0
	}
	if method == http.MethodGet {
		req.Header.Set("Range", "bytes=0-0")
	}
	resp, err := r.Client.Do(req)
	if err != nil {
		return time.Time{}, 0
	}
	defer resp.Body.Close()
	t, _ := http.ParseTime(resp.Header.Get("Last-Modified"))
	if resp.ContentLength <= 0 {
		return t, 0
	}
	if method == http.MethodHead {
		return t, resp.ContentLength
	}
	if _, total, ok := strings.Cut(resp.Header.Get("Content-Range"), "/"); ok {
		if n, err := strconv.ParseInt(total, 10, 64); err == nil && n > 0 {
			return t, n
		}
	}
	return t, 0
}

// AgeSuffix formats elapsed time like yay's age column, e.g. "1d8h", or an
// empty string when the release date is unknown or in the future.
func AgeSuffix(published, now time.Time) string {
	if published.IsZero() || now.Before(published) {
		return ""
	}
	d := now.Sub(published)
	const day, month = 24 * time.Hour, 30 * 24 * time.Hour
	switch {
	case d >= 365*day:
		return fmt.Sprintf("%dy%dm", int(d/(365*day)), int(d%(365*day)/month))
	case d >= day:
		return fmt.Sprintf("%dd%dh", int(d/day), int(d%day/time.Hour))
	case d >= time.Hour:
		return fmt.Sprintf("%dh%dm", int(d/time.Hour), int(d%time.Hour/time.Minute))
	default:
		return fmt.Sprintf("%dm", int(d/time.Minute))
	}
}

// links extracts href values from an official directory or release page.
func (r *Resolver) links(ctx context.Context, address string) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	resp, err := r.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s returned %s", address, resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	re := regexp.MustCompile(`(?i)href=["']([^"']+)["']`)
	var links []string
	for _, m := range re.FindAllStringSubmatch(string(body), -1) {
		links = append(links, strings.TrimPrefix(m[1], "./"))
	}
	return links, nil
}

// compareVersions compares numeric runs in filenames and release directory names.
func compareVersions(a, b string) int {
	numbers := regexp.MustCompile(`[0-9]+`)
	ax, bx := numbers.FindAllString(a, -1), numbers.FindAllString(b, -1)
	for i := 0; i < len(ax) && i < len(bx); i++ {
		av, bv := strings.TrimLeft(ax[i], "0"), strings.TrimLeft(bx[i], "0")
		if av == "" {
			av = "0"
		}
		if bv == "" {
			bv = "0"
		}
		if len(av) != len(bv) {
			if len(av) < len(bv) {
				return -1
			}
			return 1
		}
		if av != bv {
			if av < bv {
				return -1
			}
			return 1
		}
	}
	if len(ax) != len(bx) {
		if len(ax) < len(bx) {
			return -1
		}
		return 1
	}
	return strings.Compare(a, b)
}
