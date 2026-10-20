package iso

import "testing"

// TestParseCanonicalName verifies ISO metadata produces a stable alias.
func TestParseCanonicalName(t *testing.T) {
	i, ok := Parse("/ISO/ubuntu-26.04.1-desktop-amd64.iso")
	if !ok || i.CanonicalName() != "Ubuntu Desktop LTS 26.04.1" {
		t.Fatalf("got %#v, %v", i, ok)
	}
}

// TestDisplayName omits version data while retaining useful distro metadata.
func TestDisplayName(t *testing.T) {
	i, _ := Parse("/ISO/linuxmint-22.3-cinnamon-64bit.iso")
	if got := i.DisplayName(); got != "Linux Mint Cinnamon" {
		t.Fatalf("got %q", got)
	}
}

// TestParseArchitectures verifies both upstream spellings map to the amd64 and
// arm64 names used by the manager without disturbing version extraction.
func TestParseArchitectures(t *testing.T) {
	cases := map[string]struct{ arch, version string }{
		"ubuntu-26.04.1-desktop-amd64.iso":                           {"amd64", "26.04.1"},
		"ubuntu-26.04.1-desktop-arm64.iso":                           {"arm64", "26.04.1"},
		"ubuntu-26.10-desktop-amd64.iso":                             {"amd64", "26.10"},
		"ubuntu-26.10-desktop-arm64.iso":                             {"arm64", "26.10"},
		"Fedora-Workstation-Live-44-1.2.x86_64.iso":                  {"amd64", "44"},
		"Fedora-Workstation-Live-44-1.2.aarch64.iso":                 {"arm64", "44"},
		"Fedora-KDE-Desktop-Live-44-1.7.x86_64.iso":                  {"amd64", "44"},
		"Fedora-KDE-Desktop-Live-44-1.7.aarch64.iso":                 {"arm64", "44"},
		"Fedora-Silverblue-ostree-x86_64-44-1.7.iso":                 {"amd64", "44"},
		"Fedora-Silverblue-ostree-aarch64-44-1.7.iso":                {"arm64", "44"},
		"Rocky-10.2-x86_64-dvd1.iso":                                 {"amd64", "10.2"},
		"Rocky-10.2-aarch64-dvd1.iso":                                {"arm64", "10.2"},
		"Rocky-9.8-x86_64-dvd.iso":                                   {"amd64", "9.8"},
		"MX-25.3_Xfce_x64.iso":                                       {"amd64", "25.3"},
		"artix-base-openrc-20260813-x86_64.iso":                      {"amd64", "20260813"},
		"manjaro-xfce-26.1.0-260812-linux71.iso":                     {"amd64", "26.1.0"},
		"manjaro-gnome-26.1.0-260812-linux71.iso":                    {"amd64", "26.1.0"},
		"debian-13.1.0-arm64-DVD-1.iso":                              {"arm64", "13.1.0"},
		"openSUSE-Tumbleweed-DVD-aarch64-Snapshot20260101-Media.iso": {"arm64", "20260101"},
		"Leap-16.0-offline-installer-aarch64.install.iso":            {"arm64", "16.0"},
		"openSUSE-Leap-15.6-DVD-aarch64-Media.iso":                   {"arm64", "15.6"},
		"linuxmint-22.3-cinnamon-64bit.iso":                          {"amd64", "22.3"},
		"linuxmint-22.3-xfce-64bit.iso":                              {"amd64", "22.3"},
		"linuxmint-22.3-mate-64bit.iso":                              {"amd64", "22.3"},
		"EndeavourOS_Titan-Nova-2026.08.15.iso":                      {"amd64", "2026.08.15"},
		"cachyos-desktop-linux-260809.iso":                           {"amd64", "260809"},
		"shredos-2025.11_31_x86-64_v0.42_20260716.iso":               {"amd64", "20260716"},
	}
	for name, want := range cases {
		image, ok := Parse(name)
		if !ok {
			t.Errorf("Parse(%q) did not recognize the image", name)
			continue
		}
		if image.Arch != want.arch || image.Version != want.version {
			t.Errorf("Parse(%q) = arch %q version %q, want %q %q",
				name, image.Arch, image.Version, want.arch, want.version)
		}
	}
}

// TestParseUbuntuReleaseChannel verifies LTS and interim Ubuntu images keep
// distinct identities, since they share one filename pattern.
func TestParseUbuntuReleaseChannel(t *testing.T) {
	cases := map[string]string{
		"ubuntu-26.04-desktop-amd64.iso":   "Desktop LTS",
		"ubuntu-26.04.1-desktop-amd64.iso": "Desktop LTS",
		"ubuntu-26.10-desktop-amd64.iso":   "Desktop",
		"ubuntu-25.10-desktop-amd64.iso":   "Desktop",
		"ubuntu-25.04-desktop-amd64.iso":   "Desktop",
	}
	for name, want := range cases {
		image, ok := Parse(name)
		if !ok {
			t.Errorf("Parse(%q) did not recognize the image", name)
			continue
		}
		if image.Edition != want {
			t.Errorf("Parse(%q) edition = %q, want %q", name, image.Edition, want)
		}
	}
}

// TestParseFedoraKDEIsWorkstationFlavor verifies Fedora's KDE image is a
// Workstation spin rather than an edition of its own.
func TestParseFedoraKDEIsWorkstationFlavor(t *testing.T) {
	image, ok := Parse("Fedora-KDE-Desktop-Live-44-1.7.x86_64.iso")
	if !ok {
		t.Fatal("Fedora KDE image not recognized")
	}
	if image.Edition != "Workstation" || image.Flavor != "KDE" {
		t.Errorf("edition = %q flavor = %q, want Workstation KDE", image.Edition, image.Flavor)
	}
}

// TestParseDebianHasNoDesktopFlavor verifies Debian's DVD is not branded with
// a desktop environment; it ships several.
func TestParseDebianHasNoDesktopFlavor(t *testing.T) {
	image, ok := Parse("debian-13.1.0-amd64-DVD-1.iso")
	if !ok {
		t.Fatal("Debian image not recognized")
	}
	if image.Flavor != "" {
		t.Errorf("flavor = %q, want none", image.Flavor)
	}
}

// TestParseArtixVariants verifies every Artix image upstream publishes maps to
// its desktop (or base/community variant) and init system.
func TestParseArtixVariants(t *testing.T) {
	cases := map[string]struct{ edition, flavor string }{
		"artix-plasma-openrc-20260813-x86_64.iso":        {"Graphical", "Plasma OpenRC"},
		"artix-plasma-dinit-20260813-x86_64.iso":         {"Graphical", "Plasma Dinit"},
		"artix-xfce-openrc-20260813-x86_64.iso":          {"Graphical", "XFCE OpenRC"},
		"artix-xfce-dinit-20260813-x86_64.iso":           {"Graphical", "XFCE Dinit"},
		"artix-lxqt-runit-20260816-x86_64.iso":           {"Graphical", "LXQt Runit"},
		"artix-mate-openrc-20260813-x86_64.iso":          {"Graphical", "MATE OpenRC"},
		"artix-mate-dinit-20260813-x86_64.iso":           {"Graphical", "MATE Dinit"},
		"artix-cinnamon-openrc-20260814-x86_64.iso":      {"Graphical", "Cinnamon OpenRC"},
		"artix-cinnamon-dinit-20260814-x86_64.iso":       {"Graphical", "Cinnamon Dinit"},
		"artix-community-gtk-openrc-20260821-x86_64.iso": {"Community", "GTK OpenRC"},
		"artix-community-qt-openrc-20260821-x86_64.iso":  {"Community", "Qt OpenRC"},
		"artix-base-openrc-20260813-x86_64.iso":          {"Base", "OpenRC"},
		"artix-base-runit-20260813-x86_64.iso":           {"Base", "Runit"},
		"artix-base-dinit-20260813-x86_64.iso":           {"Base", "Dinit"},
		"artix-base-s6-20260813-x86_64.iso":              {"Base", "s6"},
	}
	for name, want := range cases {
		image, ok := Parse(name)
		if !ok {
			t.Errorf("Parse(%q) did not recognize the image", name)
			continue
		}
		if image.Edition != want.edition || image.Flavor != want.flavor {
			t.Errorf("Parse(%q) = edition %q flavor %q, want %q %q",
				name, image.Edition, image.Flavor, want.edition, want.flavor)
		}
	}
}

// TestCanonicalNameIncludesNonDefaultArch verifies arm64 aliases stay distinct
// from the unchanged amd64 alias.
func TestCanonicalNameIncludesNonDefaultArch(t *testing.T) {
	image, ok := Parse("ubuntu-26.04.1-desktop-arm64.iso")
	if !ok {
		t.Fatal("arm64 image not recognized")
	}
	if got, want := image.CanonicalName(), "Ubuntu Desktop LTS 26.04.1 arm64"; got != want {
		t.Errorf("CanonicalName() = %q, want %q", got, want)
	}
}

// TestParseOpenSUSELeap verifies both Leap image layouts, including the
// build-qualified aliases used by mirrors.
func TestParseOpenSUSELeap(t *testing.T) {
	cases := []string{
		"Leap-16.0-offline-installer-x86_64.install.iso",
		"Leap-16.1-offline-installer-x86_64-Build55.2.install.iso",
		"openSUSE-Leap-15.6-DVD-x86_64-Media.iso",
		"openSUSE-Leap-15.6-DVD-x86_64-Current.iso",
		"openSUSE-Leap-15.6-DVD-x86_64-Build710.3-Media.iso",
	}
	for _, name := range cases {
		i, ok := Parse(name)
		if !ok {
			t.Errorf("Parse(%q) did not recognize the image", name)
			continue
		}
		if i.Family != "openSUSE" || i.Edition != "Leap" {
			t.Errorf("Parse(%q) = %#v, want openSUSE Leap", name, i)
		}
	}
	if i, _ := Parse("Leap-16.0-offline-installer-x86_64.install.iso"); i.Version != "16.0" {
		t.Errorf("version = %q, want 16.0", i.Version)
	}
	if i, _ := Parse("openSUSE-Leap-15.6-DVD-x86_64-Media.iso"); i.Version != "15.6" {
		t.Errorf("version = %q, want 15.6", i.Version)
	}
}

// TestCatalog verifies the single catalog is complete and unambiguous: every
// entry can be looked up by its identity and carries both the filename rule
// and the upstream index needed to recognize and update its images.
func TestCatalog(t *testing.T) {
	entries := Catalog()
	if len(entries) == 0 {
		t.Fatal("catalog is empty")
	}
	seen := map[string]bool{}
	for _, entry := range entries {
		image := entry.Image()
		key := image.DisplayName()
		if image.Family == "" {
			t.Errorf("entry %q has no family", key)
		}
		if entry.Filename == nil {
			t.Errorf("%s has no filename rule", key)
		}
		if entry.Index == "" {
			t.Errorf("%s has no release index", key)
		}
		if seen[key] {
			t.Errorf("duplicate catalog identity %q", key)
		}
		seen[key] = true
		if found, ok := Lookup(image); !ok || found.Index != entry.Index {
			t.Errorf("Lookup(%q) = %+v, %v; want its catalog entry", key, found, ok)
		}
	}
	if first := entries[0].Image(); first.Family != "Ubuntu" || first.Edition != "Desktop LTS" {
		t.Errorf("first entry = %+v, want Ubuntu Desktop LTS", first)
	}
	if last := entries[len(entries)-1].Image(); last.Family != "GParted Live" {
		t.Errorf("last entry = %+v, want GParted Live", last)
	}
	opensuse := 0
	for _, entry := range entries {
		if entry.Image().Family == "openSUSE" {
			opensuse++
		}
	}
	if opensuse != 2 {
		t.Errorf("openSUSE editions = %d, want Tumbleweed and Leap", opensuse)
	}
}

// TestCatalogParsesEveryImage verifies the catalog's own filename rule is what
// Parse uses, so recognizing an installed ISO and updating it share one entry.
func TestCatalogParsesEveryImage(t *testing.T) {
	samples := map[string]string{
		"Ubuntu":          "ubuntu-26.04.1-desktop-amd64.iso",
		"Ubuntu arm64":    "ubuntu-26.04.1-desktop-arm64.iso",
		"Ubuntu interim":  "ubuntu-26.10-desktop-amd64.iso",
		"Fedora":          "Fedora-Workstation-Live-44-1.2.x86_64.iso",
		"Fedora arm64":    "Fedora-Workstation-Live-44-1.2.aarch64.iso",
		"Fedora KDE":      "Fedora-KDE-Desktop-Live-44-1.7.x86_64.iso",
		"Silverblue":      "Fedora-Silverblue-ostree-x86_64-44-1.7.iso",
		"Rocky":           "Rocky-10.2-x86_64-dvd1.iso",
		"Debian":          "debian-13.1.0-amd64-DVD-1.iso",
		"Debian arm64":    "debian-13.1.0-arm64-DVD-1.iso",
		"MX":              "MX-25.3_Xfce_x64.iso",
		"Tails":           "tails-amd64-7.2.iso",
		"Manjaro":         "manjaro-kde-25.1.0-250101-linux612.iso",
		"Manjaro XFCE":    "manjaro-xfce-25.1.0-250101-linux612.iso",
		"Manjaro GNOME":   "manjaro-gnome-25.1.0-250101-linux612.iso",
		"Arch":            "archlinux-2026.01.01-x86_64.iso",
		"Mint Cinnamon":   "linuxmint-22.3-cinnamon-64bit.iso",
		"Mint XFCE":       "linuxmint-22.3-xfce-64bit.iso",
		"Mint MATE":       "linuxmint-22.3-mate-64bit.iso",
		"Artix Plasma":    "artix-plasma-dinit-20260813-x86_64.iso",
		"Artix Community": "artix-community-gtk-openrc-20260821-x86_64.iso",
		"Artix base":      "artix-base-s6-20260813-x86_64.iso",
		"openSUSE":        "Leap-16.0-offline-installer-x86_64.install.iso",
		"openSUSE arm64":  "Leap-16.0-offline-installer-aarch64.install.iso",
		"EndeavourOS":     "EndeavourOS_Titan-Nova-2026.08.15.iso",
		"Nobara":          "Nobara-44-Official-2026-09-02.iso",
		"CachyOS":         "cachyos-desktop-linux-260809.iso",
		"Omarchy":         "omarchy-4.0.4.iso",
		"ShredOS":         "shredos-2025.11_31_x86-64_v0.42_20260716.iso",
		"GParted Live":    "gparted-live-1.7.0-8-amd64.iso",
	}
	for name, filename := range samples {
		image, ok := Parse(filename)
		if !ok {
			t.Errorf("%s: Parse(%q) failed", name, filename)
			continue
		}
		if _, ok := Lookup(image); !ok {
			t.Errorf("%s: parsed identity %+v has no catalog entry", name, image)
		}
	}
}

// TestCatalogArches verifies every claimed architecture is reachable through
// the catalog's own filename rule, so a download row can be resolved.
func TestCatalogArches(t *testing.T) {
	for _, entry := range Catalog() {
		for _, image := range entry.Images() {
			if image.Arch == "" {
				t.Errorf("%s %s has no architecture", image.Family, image.Edition)
			}
		}
		if len(entry.Images()) != len(entry.Arches) && len(entry.Arches) > 0 {
			t.Errorf("%s: Images() = %d, Arches = %d", entry.Family, len(entry.Images()), len(entry.Arches))
		}
		if arch := entry.DefaultArch(); arch == "" {
			t.Errorf("%s has no default architecture", entry.Family)
		}
	}
}
