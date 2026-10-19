package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// makeVentoy creates a fake mounted drive with a ventoy config and ISO files.
func makeVentoy(t *testing.T, base, name string, isos ...string) string {
	t.Helper()
	mount := makeVentoyWithoutConfig(t, base, name, isos...)
	if err := os.WriteFile(filepath.Join(mount, "ventoy", "ventoy.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return mount
}

// makeVentoyWithoutConfig creates a Ventoy drive whose ventoy directory has no
// ventoy.json, as with a fresh or manually prepared drive.
func makeVentoyWithoutConfig(t *testing.T, base, name string, isos ...string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(base, name, "ventoy"), 0o755); err != nil {
		t.Fatal(err)
	}
	isoDir := filepath.Join(base, name, "ISO")
	if err := os.MkdirAll(isoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, iso := range isos {
		if err := os.WriteFile(filepath.Join(isoDir, iso), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return filepath.Join(base, name)
}

// stubVtoyEFI replaces the VTOYEFI partition probe for the test's duration.
func stubVtoyEFI(t *testing.T, f func(mount string) bool) {
	t.Helper()
	orig := hasVtoyEFI
	hasVtoyEFI = f
	t.Cleanup(func() { hasVtoyEFI = orig })
}

func TestDiscoverFromFindsVentoyDrivesWithOrWithoutConfig(t *testing.T) {
	// Tests run on plain directories, never on a real Ventoy disk, so the
	// partition probe must stay out of the way.
	stubVtoyEFI(t, func(string) bool { return false })
	base := t.TempDir()
	makeVentoy(t, base, "Ventoy", "ubuntu-26.04.1-desktop-amd64.iso", "tails-amd64-7.13.iso")
	// A second drive with a different config and one ISO plus one non-ISO.
	makeVentoy(t, base, "Ventoy1", "archlinux-2026.09.01-x86_64.iso")
	if err := os.WriteFile(filepath.Join(base, "Ventoy1", "ISO", "notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A third drive carries ISOs but no ventoy.json; it must still be listed.
	makeVentoyWithoutConfig(t, base, "VentoyNoConfig", "debian-13.1.0-amd64-DVD-1.iso")
	// A mount without a ventoy directory must be ignored, as must plain files.
	if err := os.MkdirAll(filepath.Join(base, "PlainUSB"), 0o755); err != nil {
		t.Fatal(err)
	}

	drives := DiscoverFrom([]string{base})
	if len(drives) != 3 {
		t.Fatalf("got %d drives, want 3: %+v", len(drives), drives)
	}
	if drives[0].Mount != filepath.Join(base, "Ventoy") || drives[1].Mount != filepath.Join(base, "Ventoy1") ||
		drives[2].Mount != filepath.Join(base, "VentoyNoConfig") {
		t.Errorf("drives not sorted by mount point: %+v", drives)
	}
	if drives[0].ISOs != 2 || drives[1].ISOs != 1 || drives[2].ISOs != 1 {
		t.Errorf("ISO counts: %d, %d and %d, want 2, 1 and 1",
			drives[0].ISOs, drives[1].ISOs, drives[2].ISOs)
	}
	if drives[0].Dir != filepath.Join(base, "Ventoy", "ventoy") {
		t.Errorf("Dir = %q", drives[0].Dir)
	}
	if drives[2].Dir != filepath.Join(base, "VentoyNoConfig", "ventoy") {
		t.Errorf("config-less Dir = %q", drives[2].Dir)
	}
	if drives[0].Size == 0 {
		t.Error("Size should come from the filesystem the drive lives on")
	}
}

// TestDiscoverFromFindsFreshVentoyByVtoyEFI verifies a drive with neither ISOs
// nor a ventoy directory is recognized through its VTOYEFI bootstrap
// partition, like a freshly installed stick.
func TestDiscoverFromFindsFreshVentoyByVtoyEFI(t *testing.T) {
	base := t.TempDir()
	fresh := filepath.Join(base, "Ventoy1")
	if err := os.MkdirAll(filepath.Join(fresh, "System Volume Information"), 0o755); err != nil {
		t.Fatal(err)
	}
	// A plain stick without any Ventoy marker stays hidden.
	if err := os.MkdirAll(filepath.Join(base, "PlainUSB"), 0o755); err != nil {
		t.Fatal(err)
	}
	stubVtoyEFI(t, func(mount string) bool { return mount == fresh })

	drives := DiscoverFrom([]string{base})
	if len(drives) != 1 || drives[0].Mount != fresh {
		t.Fatalf("got %+v, want only the fresh Ventoy drive", drives)
	}
	if drives[0].ISOs != 0 {
		t.Errorf("fresh drive ISOs = %d, want 0", drives[0].ISOs)
	}
	if drives[0].Dir != filepath.Join(fresh, "ventoy") {
		t.Errorf("Dir = %q, want the future ventoy directory", drives[0].Dir)
	}
}

func TestChooseDriveAcceptsNumbersAndDefault(t *testing.T) {
	drives := []Drive{
		{Mount: "/run/media/u/Ventoy", Name: "sda", Dir: "/run/media/u/Ventoy/ventoy", Size: 128 << 30, Free: 29 << 30, ISOs: 24},
		{Mount: "/run/media/u/Ventoy1", Name: "sdb", Dir: "/run/media/u/Ventoy1/ventoy", Size: 7 << 30, Free: 7 << 30, ISOs: 1},
	}
	for _, tc := range []struct {
		input, want string
	}{
		{"1\r", drives[0].Dir},
		{"2\r", drives[1].Dir},
		{" 2 \r", drives[1].Dir},
		{"\r", drives[0].Dir},
	} {
		var out strings.Builder
		got, err := chooseDrive(drives, strings.NewReader(tc.input), &out)
		if err != nil || got != tc.want {
			t.Errorf("chooseDrive(%q) = %q, %v; want %q", tc.input, got, err, tc.want)
		}
	}

	// Out-of-range and non-number keys are ignored until a valid one arrives;
	// Out-of-range and non-number keys are ignored; the arrows move the cursor
	// before enter confirms.
	var out strings.Builder
	got, err := chooseDrive(drives, strings.NewReader("9x\x1b[B\r"), &out)
	if err != nil {
		t.Errorf("invalid input then arrows: %v", err)
	}
	if got != drives[len(drives)-1].Dir {
		t.Errorf("arrow selection confirmed %q, want the last drive %q", got, drives[len(drives)-1].Dir)
	}
	for _, want := range []string{"1-2", "sda", "sdb", "total, 29 GB free, 24 ISOs", "7.0 GB"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("prompt output missing %q:\n%s", want, out.String())
		}
	}
	for _, unwanted := range []string{"/run/media"} {
		if strings.Contains(out.String(), unwanted) {
			t.Errorf("prompt output should use drive names, found %q:\n%s", unwanted, out.String())
		}
	}
}

// TestLoadConfigLeavesFreshDrivesUntouched verifies a discovered drive without
// a ventoy directory loads as an empty in-memory configuration instead of
// creating one, while other paths keep failing on missing directories.
func TestLoadConfigLeavesFreshDrivesUntouched(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ventoy")

	cfg, err := loadConfig(dir, true)
	if err != nil {
		t.Fatalf("loadConfig(discovered): %v", err)
	}
	if cfg.Exists() {
		t.Error("fresh drive should load an empty configuration")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Error("loadConfig created the ventoy directory")
	}

	if _, err := loadConfig(dir, false); err == nil {
		t.Error("undiscovered missing directory should stay an error")
	}
}

func TestChooseDriveReportsCancelOnQ(t *testing.T) {
	drives := []Drive{{Mount: "/m", Name: "sda", Dir: "/m/ventoy", Size: 1 << 30, Free: 1, ISOs: 0}}
	if _, err := chooseDrive(drives, strings.NewReader("q"), &strings.Builder{}); err == nil {
		t.Error("q should cancel the selection with an error")
	}
}
