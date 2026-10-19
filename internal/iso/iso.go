// Package iso describes the supported Linux distributions, recognizes their
// installation images, and gives those images stable names.
package iso

import "strings"

// DefaultArch is the architecture assumed when a filename does not name one.
const DefaultArch = "amd64"

type Image struct {
	Path, Filename, Family, Edition, Flavor, Version, Arch string
}

// Parse identifies a supported ISO filename and extracts its release metadata.
func Parse(path string) (Image, bool) {
	for _, distribution := range catalog {
		if image, ok := distribution.Parse(path); ok {
			return image, true
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
	// Keep the familiar amd64 alias unchanged, but distinguish other
	// architectures so both variants can coexist on one drive.
	if i.Arch != "" && i.Arch != DefaultArch {
		parts = append(parts, i.Arch)
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
