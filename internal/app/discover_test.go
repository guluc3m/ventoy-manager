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

func TestDiscoverFromFindsVentoyDrivesWithOrWithoutConfig(t *testing.T) {
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

func TestChooseDriveAcceptsNumbersAndDefault(t *testing.T) {
	drives := []Drive{
		{Mount: "/run/media/u/Ventoy", Dir: "/run/media/u/Ventoy/ventoy", Size: 128 << 30, Free: 29 << 30, ISOs: 24},
		{Mount: "/run/media/u/Ventoy1", Dir: "/run/media/u/Ventoy1/ventoy", Size: 7 << 30, Free: 7 << 30, ISOs: 1},
	}
	for _, tc := range []struct {
		input, want string
	}{
		{"1\n", drives[0].Dir},
		{"2\n", drives[1].Dir},
		{" 2 \n", drives[1].Dir},
		{"\n", drives[0].Dir},
	} {
		var out strings.Builder
		got, err := chooseDrive(drives, strings.NewReader(tc.input), &out)
		if err != nil || got != tc.want {
			t.Errorf("chooseDrive(%q) = %q, %v; want %q", tc.input, got, err, tc.want)
		}
	}

	var out strings.Builder
	if _, err := chooseDrive(drives, strings.NewReader("9\n3\nx\n1\n"), &out); err != nil {
		t.Errorf("invalid input then valid: %v", err)
	}
	for _, want := range []string{"1-2", "total, 29 GB free, 24 ISOs", "7.0 GB"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("prompt output missing %q:\n%s", want, out.String())
		}
	}
}

func TestChooseDriveReportsEOFAmongInvalidInput(t *testing.T) {
	drives := []Drive{{Mount: "/m", Dir: "/m/ventoy", Size: 1 << 30, Free: 1, ISOs: 0},
		{Mount: "/m2", Dir: "/m2/ventoy", Size: 2 << 30, Free: 1, ISOs: 0}}
	var out strings.Builder
	if _, err := chooseDrive(drives, strings.NewReader("nope\n"), &out); err == nil {
		t.Error("EOF without a valid choice should return an error")
	}
}
