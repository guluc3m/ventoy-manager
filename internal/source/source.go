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
	// testRoot, when set, replaces a catalog entry's release index; tests use
	// it to point resolution at a fake mirror.
	testRoot string
}

// New creates a resolver with a bounded HTTP timeout.
func New() *Resolver {
	return &Resolver{Client: &http.Client{Timeout: 30 * time.Second}}
}

// withTestRoot retargets catalog resolution at a fake mirror for tests.
func (r *Resolver) withTestRoot(root string) *Resolver {
	r.testRoot = root
	return r
}

// Latest looks the installed image up in the built-in catalog and resolves its
// newest release from the same entry that recognizes its filename. Resolution
// starts at the distro's release index, rather than an installed-version URL,
// so it can discover new major releases as well as point releases.
func (r *Resolver) Latest(ctx context.Context, current iso.Image) (Candidate, error) {
	distribution, ok := iso.Lookup(current)
	if !ok {
		return Candidate{}, fmt.Errorf("no official source configured for %s (%s)", current.Family, current.Filename)
	}
	arch := current.Arch
	if arch == "" {
		arch = distribution.DefaultArch()
	}
	if !distribution.SupportsArch(arch) {
		return Candidate{}, fmt.Errorf("no %s image configured for %s (%s)", arch, current.Family, current.Filename)
	}
	index := distribution.IndexFor(arch)
	if r.testRoot != "" {
		index = r.testRoot
	}
	if distribution.Release == nil {
		return r.latestListing(ctx, index, distribution, arch)
	}
	return r.latestReleaseDirectory(ctx, index, distribution, arch)
}

// latestReleaseDirectory finds the newest stable release directory then its
// ISO. Release directories are considered newest first because mirrors publish
// a directory before it carries images; an empty or partially synced release
// falls back to the previous one instead of failing the update check.
func (r *Resolver) latestReleaseDirectory(ctx context.Context, root string, distribution iso.Distribution, arch string) (Candidate, error) {
	links, err := r.links(ctx, root)
	if err != nil {
		return Candidate{}, err
	}
	var dirs []string
	for _, link := range links {
		if distribution.Release.MatchString(link) {
			dirs = append(dirs, link)
		}
	}
	if len(dirs) == 0 {
		return Candidate{}, fmt.Errorf("no release directories found at %s", root)
	}
	sort.Slice(dirs, func(i, j int) bool { return CompareVersions(dirs[i], dirs[j]) < 0 })
	paths := distribution.PathsFor(arch)
	var lastErr error
	for i := len(dirs) - 1; i >= 0; i-- {
		for _, releasePath := range paths {
			candidate, err := r.latestListing(ctx, root+dirs[i]+releasePath, distribution, arch)
			if err == nil {
				return candidate, nil
			}
			lastErr = err
		}
	}
	return Candidate{}, fmt.Errorf("no matching ISO found under %s: %w", root, lastErr)
}

// latestListing selects the greatest matching ISO version from an official page.
func (r *Resolver) latestListing(ctx context.Context, index string, distribution iso.Distribution, arch string) (Candidate, error) {
	links, err := r.links(ctx, index)
	if err != nil {
		return Candidate{}, err
	}
	var images []iso.Image
	for _, link := range links {
		if image, ok := distribution.Parse(path.Base(link)); ok {
			if arch != "" && image.Arch != arch {
				continue
			}
			images = append(images, image)
		}
	}
	if len(images) == 0 {
		if arch != "" {
			return Candidate{}, fmt.Errorf("no %s ISO found at %s", arch, index)
		}
		return Candidate{}, fmt.Errorf("no matching ISO found at %s", index)
	}
	// Compare parsed versions, not filenames: trailing tokens like "amd64"
	// would otherwise poison the numeric runs (26.04.1 vs 26.04 amd64).
	// For equal versions, prefer the unqualified alias (e.g. the Leap image
	// without a Build suffix) so the reported filename stays stable.
	sort.Slice(images, func(i, j int) bool {
		if c := CompareVersions(images[i].Version, images[j].Version); c != 0 {
			return c < 0
		}
		ri, rj := filenameRank(images[i].Filename), filenameRank(images[j].Filename)
		if ri != rj {
			return ri > rj
		}
		return images[i].Filename < images[j].Filename
	})
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

// filenameRank orders equally versioned mirror aliases. Unqualified names win
// over build-qualified ones so the chosen download URL is the stable alias.
func filenameRank(name string) int {
	if strings.Contains(name, "-Build") {
		return 1
	}
	return 0
}

// CompareVersions compares numeric runs in filenames, release directory names,
// and version strings so version-based sorting is consistent everywhere.
func CompareVersions(a, b string) int {
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
