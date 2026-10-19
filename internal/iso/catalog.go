package iso

import (
	"path/filepath"
	"regexp"
)

// Distribution describes one entry in the built-in catalog: how its
// installation images are named and where its newest release is published.
// The same entry recognizes installed files and resolves upstream releases,
// so there is no separate download list to keep in sync.
type Distribution struct {
	Family  string
	Edition string
	Flavor  string

	// Filename matches the distribution's ISO files; its first capture group
	// is the release version.
	Filename *regexp.Regexp

	// Index is the official directory or page that lists releases.
	Index string

	// Release, when set, selects the newest matching release subdirectory
	// under Index. Path is appended to that directory before looking for the
	// ISO; it is ignored for listings that live directly at Index.
	Release *regexp.Regexp
	Path    string
}

// catalog is the single source of truth for the distributions this manager
// supports. Order is the order shown by the new-ISO screen.
var catalog = []Distribution{
	{
		Family: "Ubuntu", Edition: "Desktop LTS",
		Filename: regexp.MustCompile(`(?i)^ubuntu-([0-9]+\.[0-9]+(?:\.[0-9]+)?)-desktop-amd64\.iso$`),
		Index:    "https://releases.ubuntu.com/",
		Release:  regexp.MustCompile(`^[0-9]{2}\.04(\.[0-9]+)?/$`),
	},
	{
		Family: "Fedora", Edition: "Workstation", Flavor: "GNOME",
		Filename: regexp.MustCompile(`(?i)^Fedora-Workstation-Live-([0-9]+)(?:-[0-9.]+)?\.x86_64\.iso$`),
		Index:    "https://dl.fedoraproject.org/pub/fedora/linux/releases/",
		Release:  regexp.MustCompile(`^[0-9]+/$`),
		Path:     "Workstation/x86_64/iso/",
	},
	{
		Family: "Debian", Flavor: "GNOME",
		Filename: regexp.MustCompile(`(?i)^debian-([0-9]+(?:\.[0-9]+){1,2})-amd64-DVD-1\.iso$`),
		Index:    "https://cdimage.debian.org/debian-cd/current/amd64/iso-dvd/",
	},
	{
		Family: "Linux Mint", Flavor: "Cinnamon",
		Filename: regexp.MustCompile(`(?i)^linuxmint-([0-9]+\.[0-9]+)-cinnamon-64bit\.iso$`),
		Index:    "https://mirrors.edge.kernel.org/linuxmint/stable/",
		Release:  regexp.MustCompile(`^[0-9]+\.[0-9]+/$`),
	},
	{
		Family:   "Tails OS",
		Filename: regexp.MustCompile(`(?i)^tails-amd64-([0-9]+(?:\.[0-9]+)+)\.iso$`),
		Index:    "https://download.tails.net/tails/stable/",
		Release:  regexp.MustCompile(`^tails-amd64-[0-9.]+/$`),
	},
	{
		Family:   "Arch Linux",
		Filename: regexp.MustCompile(`(?i)^archlinux-([0-9]{4}\.[0-9]{2}\.[0-9]{2})-x86_64\.iso$`),
		Index:    "https://geo.mirror.pkgbuild.com/iso/latest/",
	},
	{
		Family: "openSUSE", Edition: "Tumbleweed",
		Filename: regexp.MustCompile(`(?i)^openSUSE-Tumbleweed-DVD-x86_64-Snapshot([0-9]+)-Media\.iso$`),
		Index:    "https://download.opensuse.org/tumbleweed/iso/",
	},
	{
		Family: "openSUSE", Edition: "Leap",
		Filename: regexp.MustCompile(`(?i)^Leap-([0-9]+\.[0-9]+)-offline-installer-x86_64\.install\.iso$`),
		Index:    "https://download.opensuse.org/distribution/leap/",
		Release:  regexp.MustCompile(`^[12][0-9]\.[0-9]+/$`),
		Path:     "offline/",
	},
	{
		Family: "Manjaro", Flavor: "KDE",
		Filename: regexp.MustCompile(`(?i)^manjaro-kde-([0-9]+(?:\.[0-9]+){1,2})-[^.]+\.iso$`),
		Index:    "https://manjaro.org/products/download/x86/",
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

// Image returns the versionless identity of the distribution.
func (d Distribution) Image() Image {
	return Image{Family: d.Family, Edition: d.Edition, Flavor: d.Flavor}
}

// Parse identifies path as an image of this distribution and extracts its
// release version.
func (d Distribution) Parse(path string) (Image, bool) {
	name := filepath.Base(path)
	match := d.Filename.FindStringSubmatch(name)
	if match == nil {
		return Image{}, false
	}
	return Image{
		Path:     path,
		Filename: name,
		Family:   d.Family,
		Edition:  d.Edition,
		Flavor:   d.Flavor,
		Version:  match[1],
	}, true
}
