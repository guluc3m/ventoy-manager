// Package iso finds recognised Linux installation images and gives them stable names.
package iso

import (
	"path/filepath"
	"regexp"
	"strings"
)

type Image struct {
	Path, Filename, Family, Edition, Flavor, Version string
}

type rule struct {
	family, edition, flavor string
	re                      *regexp.Regexp
}

var rules = []rule{
	{"Ubuntu", "Desktop LTS", "", regexp.MustCompile(`(?i)^ubuntu-([0-9]+\.[0-9]+(?:\.[0-9]+)?)-desktop-amd64\.iso$`)},
	{"Fedora", "Workstation", "", regexp.MustCompile(`(?i)^Fedora-Workstation-Live-([0-9]+)(?:-[0-9.]+)?\.x86_64\.iso$`)},
	{"Debian", "DVD", "", regexp.MustCompile(`(?i)^debian-([0-9]+(?:\.[0-9]+){1,2})-amd64-DVD-1\.iso$`)},
	{"Linux Mint", "", "Cinnamon", regexp.MustCompile(`(?i)^linuxmint-([0-9]+\.[0-9]+)-cinnamon-64bit\.iso$`)},
	{"Tails", "", "", regexp.MustCompile(`(?i)^tails-amd64-([0-9]+(?:\.[0-9]+)+)\.iso$`)},
	{"Arch Linux", "", "", regexp.MustCompile(`(?i)^archlinux-([0-9]{4}\.[0-9]{2}\.[0-9]{2})-x86_64\.iso$`)},
	{"openSUSE", "Tumbleweed", "", regexp.MustCompile(`(?i)^openSUSE-Tumbleweed-DVD-x86_64-Snapshot([0-9]+)-Media\.iso$`)},
	{"openSUSE", "Leap", "", regexp.MustCompile(`(?i)^Leap-([0-9]+\.[0-9]+)-offline-installer-x86_64\.install\.iso$`)},
	{"Manjaro", "", "KDE", regexp.MustCompile(`(?i)^manjaro-kde-([0-9]+(?:\.[0-9]+){1,2})-[^.]+\.iso$`)},
}

// Parse identifies a supported ISO filename and extracts its release metadata.
func Parse(path string) (Image, bool) {
	name := filepath.Base(path)
	for _, r := range rules {
		if m := r.re.FindStringSubmatch(name); m != nil {
			return Image{Path: path, Filename: name, Family: r.family, Edition: r.edition, Flavor: r.flavor, Version: m[1]}, true
		}
	}
	return Image{}, false
}

// CanonicalName renders the stable, human-readable name used for Ventoy aliases.
func (i Image) CanonicalName() string {
	parts := []string{i.Family}
	if i.Edition != "" {
		parts = append(parts, i.Edition)
	}
	parts = append(parts, i.Version)
	if i.Flavor != "" {
		parts = append(parts, i.Flavor)
	}
	return strings.Join(parts, " ")
}

// DisplayName renders the distro metadata without its version for tabular UIs.
func (i Image) DisplayName() string {
	parts := []string{i.Family}
	if i.Edition != "" {
		parts = append(parts, i.Edition)
	}
	if i.Flavor != "" {
		parts = append(parts, i.Flavor)
	}
	return strings.Join(parts, " ")
}
