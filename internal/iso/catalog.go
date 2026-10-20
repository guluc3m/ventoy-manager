package iso

import (
	"path/filepath"
	"regexp"
	"strings"
)

// Distribution describes one entry in the built-in catalog: how its
// installation images are named and where its newest release is published.
// The same entry recognizes installed files and resolves upstream releases,
// so there is no separate download list to keep in sync.
type Distribution struct {
	Family  string
	Edition string
	Flavor  string

	// Arches lists the architectures upstream publishes for this
	// distribution, in catalog order. The first one is the default used when
	// a filename carries no architecture token. An empty list means amd64.
	Arches []string

	// Filename matches the distribution's ISO files. Named capture groups
	// "version" and "arch" (or version2/arch2 for regexp alternatives) carry
	// the release version and architecture; the first plain capture group is
	// still accepted as a version fallback.
	Filename *regexp.Regexp

	// Index is the official directory or page that lists releases.
	Index string

	// Indexes overrides Index for architectures whose images live in a
	// different directory (Debian publishes each architecture separately).
	Indexes map[string]string

	// Release, when set, selects the newest matching release subdirectory
	// under Index. Path is appended to that directory before looking for the
	// ISO; it is ignored for listings that live directly at Index. Paths
	// overrides Path when several layouts exist; each candidate release is
	// tried in order until one contains a matching ISO. PathsByArch overrides
	// both for architectures whose image sits under a different directory.
	Release     *regexp.Regexp
	Path        string
	Paths       []string
	PathsByArch map[string][]string
}

// catalog is the single source of truth for the distributions this manager
// supports. Order is the order shown by the new-ISO screen.
var catalog = []Distribution{
	{
		Family: "Ubuntu", Edition: "Desktop LTS",
		Arches:   []string{"amd64", "arm64"},
		Filename: regexp.MustCompile(`(?i)^ubuntu-(?P<version>[0-9]+\.[0-9]+(?:\.[0-9]+)?)-desktop-(?P<arch>amd64|arm64)\.iso$`),
		Index:    "https://releases.ubuntu.com/",
		Release:  regexp.MustCompile(`^[0-9]{2}\.04(\.[0-9]+)?/$`),
	},
	{
		Family: "Fedora", Edition: "Workstation", Flavor: "GNOME",
		Arches:      []string{"amd64", "arm64"},
		Filename:    regexp.MustCompile(`(?i)^Fedora-Workstation-Live-(?P<version>[0-9]+)(?:-[0-9.]+)?\.(?P<arch>x86_64|aarch64)\.iso$`),
		Index:       "https://dl.fedoraproject.org/pub/fedora/linux/releases/",
		Release:     regexp.MustCompile(`^[0-9]+/$`),
		Path:        "Workstation/x86_64/iso/",
		PathsByArch: map[string][]string{"arm64": {"Workstation/aarch64/iso/"}},
	},
	{
		Family: "Fedora", Edition: "KDE Desktop",
		Arches:      []string{"amd64", "arm64"},
		Filename:    regexp.MustCompile(`(?i)^Fedora-KDE-Desktop-Live-(?P<version>[0-9]+)(?:-[0-9.]+)?\.(?P<arch>x86_64|aarch64)\.iso$`),
		Index:       "https://dl.fedoraproject.org/pub/fedora/linux/releases/",
		Release:     regexp.MustCompile(`^[0-9]+/$`),
		Path:        "KDE/x86_64/iso/",
		PathsByArch: map[string][]string{"arm64": {"KDE/aarch64/iso/"}},
	},
	{
		// Silverblue is an Atomic Desktop image: its filename puts the
		// architecture before the release number.
		Family: "Fedora", Edition: "Silverblue",
		Arches:      []string{"amd64", "arm64"},
		Filename:    regexp.MustCompile(`(?i)^Fedora-Silverblue-ostree-(?P<arch>x86_64|aarch64)-(?P<version>[0-9]+)(?:-[0-9.]+)?\.iso$`),
		Index:       "https://dl.fedoraproject.org/pub/fedora/linux/releases/",
		Release:     regexp.MustCompile(`^[0-9]+/$`),
		Path:        "Silverblue/x86_64/iso/",
		PathsByArch: map[string][]string{"arm64": {"Silverblue/aarch64/iso/"}},
	},
	{
		// Rocky publishes each point release under its own directory. Bare
		// major-version directories (10/) hold repositories, not images.
		Family:      "Rocky Linux",
		Arches:      []string{"amd64", "arm64"},
		Filename:    regexp.MustCompile(`(?i)^Rocky-(?P<version>[0-9]+\.[0-9]+)-(?P<arch>x86_64|aarch64)-dvd1?\.iso$`),
		Index:       "https://dl.rockylinux.org/pub/rocky/",
		Release:     regexp.MustCompile(`^[0-9]+\.[0-9]+/$`),
		Path:        "isos/x86_64/",
		PathsByArch: map[string][]string{"arm64": {"isos/aarch64/"}},
	},
	{
		Family: "Debian", Flavor: "GNOME",
		Arches:   []string{"amd64", "arm64"},
		Filename: regexp.MustCompile(`(?i)^debian-(?P<version>[0-9]+(?:\.[0-9]+){1,2})-(?P<arch>amd64|arm64)-DVD-1\.iso$`),
		Index:    "https://cdimage.debian.org/debian-cd/current/amd64/iso-dvd/",
		Indexes: map[string]string{
			"arm64": "https://cdimage.debian.org/debian-cd/current/arm64/iso-dvd/",
		},
	},
	{
		Family: "Linux Mint", Flavor: "Cinnamon",
		Arches:   []string{"amd64"},
		Filename: regexp.MustCompile(`(?i)^linuxmint-(?P<version>[0-9]+\.[0-9]+)-cinnamon-64bit\.iso$`),
		Index:    "https://mirrors.edge.kernel.org/linuxmint/stable/",
		Release:  regexp.MustCompile(`^[0-9]+\.[0-9]+/$`),
	},
	{
		Family: "MX Linux", Flavor: "XFCE",
		Arches: []string{"amd64"},
		// Each edition has its own SourceForge folder; the regular Xfce image
		// is the flagship, and the "ahs" hardware-support build is excluded.
		Filename: regexp.MustCompile(`(?i)^MX-(?P<version>[0-9]+(?:\.[0-9]+)?)_Xfce_x64\.iso$`),
		Index:    "https://sourceforge.net/projects/mx-linux/files/Final/Xfce/",
	},
	{
		Family:   "Tails OS",
		Arches:   []string{"amd64"},
		Filename: regexp.MustCompile(`(?i)^tails-(?P<arch>amd64)-(?P<version>[0-9]+(?:\.[0-9]+)+)\.iso$`),
		Index:    "https://download.tails.net/tails/stable/",
		Release:  regexp.MustCompile(`^tails-amd64-[0-9.]+/$`),
	},
	{
		Family:   "Arch Linux",
		Arches:   []string{"amd64"},
		Filename: regexp.MustCompile(`(?i)^archlinux-(?P<version>[0-9]{4}\.[0-9]{2}\.[0-9]{2})-x86_64\.iso$`),
		Index:    "https://geo.mirror.pkgbuild.com/iso/latest/",
	},
	{
		// Artix builds are dated, and the weekly/testing mirrors reuse the same
		// naming pattern, so the stable mirror (which lists released ISOs only)
		// is used as the index.
		Family: "Artix Linux", Edition: "Base", Flavor: "OpenRC",
		Arches:   []string{"amd64"},
		Filename: regexp.MustCompile(`(?i)^artix-base-openrc-(?P<version>[0-9]{8})-x86_64\.iso$`),
		Index:    "https://mirror1.artixlinux.org/iso/",
	},
	{
		Family: "openSUSE", Edition: "Tumbleweed",
		Arches:   []string{"amd64", "arm64"},
		Filename: regexp.MustCompile(`(?i)^openSUSE-Tumbleweed-DVD-(?P<arch>x86_64|aarch64)-Snapshot(?P<version>[0-9]+)-Media\.iso$`),
		Index:    "https://download.opensuse.org/tumbleweed/iso/",
	},
	{
		Family: "openSUSE", Edition: "Leap",
		Arches: []string{"amd64", "arm64"},
		// Leap 16.x ships a unified offline installer, while 15.x publishes a
		// DVD image. Mirrors keep build-qualified aliases beside both spellings.
		Filename: regexp.MustCompile(`(?i)^(?:Leap-(?P<version>[0-9]+\.[0-9]+)-offline-installer-(?P<arch>x86_64|aarch64)(?:-Build[0-9.]+)?\.install\.iso|openSUSE-Leap-(?P<version2>[0-9]+\.[0-9]+)-DVD-(?P<arch2>x86_64|aarch64)(?:-Build[0-9.]+)?(?:-Current|-Media)?\.iso)$`),
		Index:    "https://download.opensuse.org/distribution/leap/",
		Release:  regexp.MustCompile(`^[12][0-9]\.[0-9]+/$`),
		Paths:    []string{"offline/", "iso/"},
	},
	{
		Family: "Manjaro", Flavor: "KDE",
		Arches:   []string{"amd64"},
		Filename: regexp.MustCompile(`(?i)^manjaro-kde-(?P<version>[0-9]+(?:\.[0-9]+){1,2})-[^.]+\.iso$`),
		Index:    "https://manjaro.org/products/download/x86/",
	},
	{
		Family: "Manjaro", Flavor: "XFCE",
		Arches:   []string{"amd64"},
		Filename: regexp.MustCompile(`(?i)^manjaro-xfce-(?P<version>[0-9]+(?:\.[0-9]+){1,2})-[^.]+\.iso$`),
		Index:    "https://manjaro.org/products/download/x86/",
	},
	{
		Family: "Manjaro", Flavor: "GNOME",
		Arches:   []string{"amd64"},
		Filename: regexp.MustCompile(`(?i)^manjaro-gnome-(?P<version>[0-9]+(?:\.[0-9]+){1,2})-[^.]+\.iso$`),
		Index:    "https://manjaro.org/products/download/x86/",
	},
	{
		// The codename changes with every release (Titan-Nova today), so only
		// the trailing date is stable enough to version on.
		Family:   "EndeavourOS",
		Arches:   []string{"amd64"},
		Filename: regexp.MustCompile(`(?i)^EndeavourOS_.+-(?P<version>[0-9]{4}\.[0-9]{2}\.[0-9]{2})\.iso$`),
		// The official page lists the current ISO once per mirror as absolute
		// links, so the first mirror named there is used.
		Index: "https://endeavouros.com/download/",
	},
	{
		Family: "Nobara", Edition: "Official",
		Arches:   []string{"amd64"},
		Filename: regexp.MustCompile(`(?i)^Nobara-(?P<version>[0-9]+)-Official-[0-9]{4}-[0-9]{2}-[0-9]{2}\.iso$`),
		// The download page carries each image in a data-url attribute on its
		// button rather than an href.
		Index: "https://nobaraproject.org/download.html",
	},
	{
		Family: "CachyOS", Flavor: "Desktop",
		Arches:   []string{"amd64"},
		Filename: regexp.MustCompile(`(?i)^cachyos-desktop-linux-(?P<version>[0-9]{6})\.iso$`),
		Index:    "https://mirror.cachyos.org/ISO/desktop/",
		Release:  regexp.MustCompile(`^[0-9]{6}/$`),
	},
	{
		Family:   "Omarchy",
		Arches:   []string{"amd64"},
		Filename: regexp.MustCompile(`(?i)^omarchy-(?P<version>[0-9]+(?:\.[0-9]+)*)\.iso$`),
		// The project's page links the current ISO on its own download host.
		Index: "https://omarchy.org/",
	},
	{
		Family: "ShredOS",
		Arches: []string{"amd64"},
		// Releases repeat the upstream tag (2025.11_32_x86-64_v0.43.1) plus a
		// build date; the date is what advances with every rebuild.
		Filename: regexp.MustCompile(`(?i)^shredos-[0-9]{4}\.[0-9]{2}_[0-9]+_x86-64_v[0-9.]+_(?P<version>[0-9]{8})\.iso$`),
		// Upstream publishes GitHub release assets, whose page hides the asset
		// links behind JavaScript; the exact SourceForge mirror linked from the
		// project README serves a plain, crawlable listing instead.
		Index:   "https://sourceforge.net/projects/shredos.mirror/files/",
		Release: regexp.MustCompile(`(?i)/v?[0-9]{4}\.[0-9]{2}_[0-9]+_x86-64_[0-9.]+/$`),
	},
	{
		Family:   "GParted Live",
		Arches:   []string{"amd64"},
		Filename: regexp.MustCompile(`(?i)^gparted-live-(?P<version>[0-9]+(?:\.[0-9]+)+-[0-9]+)-amd64\.iso$`),
		// The project page links downloads.sourceforge.net directly, which
		// answers Go's metadata probes with an HTML interstitial instead of
		// the file. Listing the SourceForge release directory yields the
		// /download URL, whose Last-Modified header supplies the age.
		Index:   "https://sourceforge.net/projects/gparted/files/gparted-live-stable/",
		Release: regexp.MustCompile(`(?i)/gparted-live-stable/[0-9]+(?:\.[0-9]+)+-[0-9]+/$`),
	},
}

// Catalog returns every supported distribution in display order.
func Catalog() []Distribution {
	return append([]Distribution(nil), catalog...)
}

// Lookup returns the catalog entry matching an image's distribution identity.
func Lookup(image Image) (Distribution, bool) {
	for _, distribution := range catalog {
		if distribution.Family == image.Family &&
			distribution.Edition == image.Edition &&
			distribution.Flavor == image.Flavor {
			return distribution, true
		}
	}
	return Distribution{}, false
}

// Image returns the versionless identity of the distribution on its default
// architecture.
func (d Distribution) Image() Image {
	return d.Images()[0]
}

// Images returns the versionless identity of the distribution once per
// supported architecture. Distributions without an explicit architecture list
// are treated as amd64-only.
func (d Distribution) Images() []Image {
	arches := d.Arches
	if len(arches) == 0 {
		arches = []string{DefaultArch}
	}
	images := make([]Image, 0, len(arches))
	for _, arch := range arches {
		images = append(images, Image{
			Family: d.Family, Edition: d.Edition, Flavor: d.Flavor, Arch: arch,
		})
	}
	return images
}

// DefaultArch is the architecture used when an image does not name one.
func (d Distribution) DefaultArch() string {
	if len(d.Arches) > 0 {
		return d.Arches[0]
	}
	return DefaultArch
}

// SupportsArch reports whether upstream publishes this distribution for arch.
func (d Distribution) SupportsArch(arch string) bool {
	if arch == "" {
		arch = d.DefaultArch()
	}
	if len(d.Arches) == 0 {
		return arch == DefaultArch
	}
	for _, supported := range d.Arches {
		if supported == arch {
			return true
		}
	}
	return false
}

// IndexFor returns the release index for arch, honoring architecture-specific
// layouts such as Debian's per-architecture directories.
func (d Distribution) IndexFor(arch string) string {
	if index, ok := d.Indexes[arch]; ok && index != "" {
		return index
	}
	return d.Index
}

// PathsFor returns the candidate paths inside a release directory for arch.
func (d Distribution) PathsFor(arch string) []string {
	if paths, ok := d.PathsByArch[arch]; ok && len(paths) > 0 {
		return paths
	}
	if len(d.Paths) > 0 {
		return d.Paths
	}
	return []string{d.Path}
}

// Parse identifies path as an image of this distribution and extracts its
// release version and architecture.
func (d Distribution) Parse(path string) (Image, bool) {
	name := filepath.Base(path)
	match := d.Filename.FindStringSubmatch(name)
	if match == nil {
		return Image{}, false
	}
	// Regexes with alternatives may expose the version through any one of
	// several capture groups; the first populated one is the version.
	version := d.capture(match, "version")
	if version == "" {
		for _, capture := range match[1:] {
			if capture != "" {
				version = capture
				break
			}
		}
	}
	arch := NormalizeArch(d.capture(match, "arch"))
	if arch == "" {
		arch = d.DefaultArch()
	}
	return Image{
		Path:     path,
		Filename: name,
		Family:   d.Family,
		Edition:  d.Edition,
		Flavor:   d.Flavor,
		Version:  version,
		Arch:     arch,
	}, true
}

// capture returns the first populated named group with the given prefix.
// Alternatives use suffixed names such as version2/arch2, which this picks up
// without requiring every branch to use the exact same group name.
func (d Distribution) capture(match []string, name string) string {
	for i, group := range d.Filename.SubexpNames() {
		if i == 0 || i >= len(match) || !strings.HasPrefix(group, name) {
			continue
		}
		if match[i] != "" {
			return match[i]
		}
	}
	return ""
}

// NormalizeArch maps upstream architecture spellings to the amd64/arm64 names
// used by the rest of the manager.
func NormalizeArch(arch string) string {
	switch strings.ToLower(arch) {
	case "amd64", "x86_64", "x64", "64bit":
		return "amd64"
	case "arm64", "aarch64":
		return "arm64"
	default:
		return strings.ToLower(arch)
	}
}
