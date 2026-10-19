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
	if last := entries[len(entries)-1].Image(); last.Flavor != "KDE" {
		t.Errorf("last entry flavor = %q, want KDE", last.Flavor)
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
		"Ubuntu":   "ubuntu-26.04.1-desktop-amd64.iso",
		"Fedora":   "Fedora-Workstation-Live-44-1.2.x86_64.iso",
		"Debian":   "debian-13.1.0-amd64-DVD-1.iso",
		"Tails":    "tails-amd64-7.2.iso",
		"Manjaro":  "manjaro-kde-25.1.0-250101-linux612.iso",
		"Arch":     "archlinux-2026.01.01-x86_64.iso",
		"openSUSE": "Leap-16.0-offline-installer-x86_64.install.iso",
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
