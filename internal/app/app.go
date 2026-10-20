package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/guluc3m/ventoy-updater/internal/drive"
	"github.com/guluc3m/ventoy-updater/internal/iso"
	"github.com/guluc3m/ventoy-updater/internal/source"
	"github.com/guluc3m/ventoy-updater/internal/space"
	"github.com/guluc3m/ventoy-updater/internal/ventoy"
)

// Progress reports download advancement in bytes for a single file; total is
// zero when the server does not advertise a Content-Length.
type Progress func(filename string, done, total int64)

// chunkProgress throttles progress callbacks to at most one per step.
const chunkProgress = 1 << 20 // one report per MiB

// downloadIdleTimeout aborts a transfer that stops producing data. It guards
// against a half-open connection without capping the total download time.
// It is a variable only so tests can shrink the window; production never
// changes it.
var downloadIdleTimeout = 60 * time.Second

// errCancelled reports that the user stopped a download batch partway through;
// the interactive manager treats it specially instead of as a failure.
var errCancelled = errors.New("download cancelled")

// downloadClient fetches ISO bodies. Unlike the resolver's client it has no
// overall timeout: a multi-gigabyte transfer legitimately outlives any fixed
// deadline, and a whole-request timeout is what made large updates fail with
// "context deadline exceeded ... while reading body". Dial, TLS, and
// response-header timeouts still bound connection setup, and download wraps the
// body in an idle watchdog so a stalled server is reported instead of hanging.
var downloadClient = &http.Client{
	Transport: &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   30 * time.Second,
		ExpectContinueTimeout: 10 * time.Second,
		ResponseHeaderTimeout: 60 * time.Second,
	},
}

type Update struct {
	Current     iso.Image // installed file, or an empty skeleton for catalog rows
	Latest      source.Candidate
	CurrentSize int64 // bytes of the ISO currently on the drive
	LatestSize  int64 // bytes the replacement will occupy, zero when unknown
	KeepOld     bool  // keep the installed ISO after a successful update
	DeleteISO   bool  // free space by removing the ISO without replacing it
	New         bool  // row comes from the download catalog, not the drive
	Checked     bool  // Latest resolution already attempted
	Selected    bool
	Err         error
}

// Run starts the ventoy-manager workflow: batch when --yes was passed, the
// interactive manager otherwise. When ventoyDir was not passed explicitly it
// is resolved from the mounted drives, prompting the user if several qualify.
// The manager lists every ISO found on the drive but only checks upstream for
// updates once the user asks for it.
func Run(ventoyDir string, explicit, yes, keepOld bool) error {
	discovered := false
	if !explicit {
		resolved, err := selectVentoyDir(yes)
		if err != nil {
			return err
		}
		ventoyDir, discovered = resolved, true
	}
	ventoyDir, _ = filepath.Abs(ventoyDir)
	cfg, err := loadConfig(ventoyDir, discovered)
	if err != nil {
		return fmt.Errorf("load Ventoy configuration: %w", err)
	}
	root, err := searchRoot(ventoyDir, cfg.SearchRoot())
	if err != nil {
		return err
	}
	images, err := scanImages(root)
	if err != nil {
		return err
	}
	rows := diskRows(images, keepOld)
	if len(rows) == 0 && yes {
		fmt.Println("No ISOs found under the Ventoy search root.")
		return nil
	}
	capacity, err := space.Free(root)
	if err != nil {
		return fmt.Errorf("measure free space: %w", err)
	}
	if yes {
		checked := resolveLatest(source.New(), rows)
		if len(checked) == 0 {
			fmt.Println("Everything recognised is already up to date.")
			return nil
		}
		for i := range checked {
			checked[i].Selected = checked[i].Err == nil
		}
		add, freed := spaceEffect(checked)
		driveName := "Drive"
		if label := drive.Label(root); label != "" {
			driveName = "Drive (" + label + ")"
		}
		fmt.Printf("%s: %s free of %s; downloading %s, deleting %s, leaving %s.\n",
			driveName, space.HumanBytes(capacity.Free), space.HumanBytes(capacity.Total),
			space.HumanBytes(add), space.HumanBytes(freed), space.HumanBytes(capacity.Free+freed-add))
		return apply(context.Background(), ventoyDir, root, checked, printDownload)
	}
	m := newModel(ventoyDir, root, rows, capacity)
	final, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	if err != nil {
		return err
	}
	// The TUI exits once applying finishes (see appliedMsg), restoring the
	// normal screen; report the outcome here where the user can still read it.
	if finished, ok := final.(model); ok {
		switch {
		case finished.err != "":
			return errors.New(finished.err)
		case finished.done != "":
			fmt.Println(finished.done)
		}
	}
	return nil
}

// loadConfig reads the drive's configuration. A discovered Ventoy whose ventoy
// directory does not exist yet loads as an empty in-memory configuration;
// nothing is created on the drive. Anywhere else a missing directory stays an
// error, so explicit --ventoy-dir typos keep failing loudly.
func loadConfig(dir string, discovered bool) (*ventoy.Config, error) {
	cfg, err := ventoy.Load(dir)
	if err == nil || !discovered {
		return cfg, err
	}
	if _, statErr := os.Stat(dir); os.IsNotExist(statErr) {
		return ventoy.Empty(), nil
	}
	return cfg, err
}

// searchRoot resolves and validates Ventoy's configured search root.
func searchRoot(ventoyDir, searchRoot string) (string, error) {
	partitionRoot := filepath.Dir(ventoyDir)
	root := filepath.Join(partitionRoot, filepath.FromSlash(strings.TrimPrefix(searchRoot, "/")))
	if relative, err := filepath.Rel(partitionRoot, root); err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("invalid VTOY_DEFAULT_SEARCH_ROOT %q", searchRoot)
	}
	if info, err := os.Stat(root); err != nil {
		return "", fmt.Errorf("Ventoy search root %q: %w", searchRoot, err)
	} else if !info.IsDir() {
		return "", fmt.Errorf("Ventoy search root %q is not a directory", searchRoot)
	}
	return root, nil
}

// scanImages walks the search root and parses every ISO; names too unusual to
// match are still returned so the manager can list them, with an empty Family.
func scanImages(root string) ([]iso.Image, error) {
	var found []iso.Image
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.EqualFold(filepath.Ext(path), ".iso") {
			if i, ok := iso.Parse(path); ok {
				found = append(found, i)
			} else {
				found = append(found, iso.Image{Path: path, Filename: filepath.Base(path)})
			}
		}
		return nil
	})
	return found, err
}

// diskRows builds the manager rows from the ISOs found on the drive, sorted so
// recognised distributions come first with their newest versions on top and
// unrecognized files list last.
func diskRows(images []iso.Image, keepOld bool) []Update {
	rows := make([]Update, 0, len(images))
	for _, cur := range images {
		rows = append(rows, Update{
			Current:     cur,
			CurrentSize: fileSize(cur.Path),
			KeepOld:     keepOld,
		})
	}
	sortRows(rows)
	return rows
}

// recognizedSorting orders recognised distributions by family and, within a
// family, by newest version first.
func recognizedSorting(a, b Update) bool {
	if a.Current.Family != b.Current.Family {
		return a.Current.Family < b.Current.Family
	}
	if v := source.CompareVersions(a.Current.Version, b.Current.Version); v != 0 {
		return v > 0
	}
	if a.Current.Arch != b.Current.Arch {
		return a.Current.Arch < b.Current.Arch
	}
	return a.Current.Filename < b.Current.Filename
}

// sortRows puts recognized ISOs ahead of unrecognized files, deduplicates
// catalog rows whose candidate is already installed, and is stable otherwise.
func sortRows(rows []Update) {
	sort.SliceStable(rows, func(i, j int) bool {
		aU, bU := rows[i].Current.Family != "", rows[j].Current.Family != ""
		if aU != bU {
			return aU
		}
		if !aU {
			return false
		}
		return recognizedSorting(rows[i], rows[j])
	})
}

// resolveLatest checks each recognized row against upstream, redrawing a
// one-line progress bar as it goes, and keeps only rows needing attention.
// It is the check phase of the --yes batch path; the interactive manager
// checks on demand instead.
func resolveLatest(r *source.Resolver, rows []Update) []Update {
	recognized := 0
	for i := range rows {
		if rows[i].Current.Family != "" {
			recognized++
		}
	}
	if recognized > 0 {
		fmt.Println(lipgloss.NewStyle().Faint(true).Render(fmt.Sprintf(
			"Checking %d installed ISO%s for updates…", recognized, plural(recognized))))
	}
	var updates []Update
	for i, cur := range rows {
		if cur.Current.Family == "" {
			continue
		}
		printProgress(i, len(rows), cur.Current.DisplayName())
		candidate, err := r.Latest(context.Background(), cur.Current)
		rows[i].Latest, rows[i].CurrentSize = candidate, fileSize(cur.Current.Path)
		rows[i].LatestSize, rows[i].Checked, rows[i].Err = candidate.Size, true, err
		if err == nil && candidate.Image.Filename == cur.Current.Filename {
			continue
		}
		updates = append(updates, rows[i])
	}
	printProgress(len(rows), len(rows), "")
	fmt.Println()
	return updates
}

// printProgress redraws a single-line check-phase progress bar.
func printProgress(done, total int, name string) {
	const width = 28
	filled := 0
	if total > 0 {
		filled = done * width / total
	}
	bar := lipgloss.NewStyle().Foreground(lipgloss.Color("86")).Render(
		strings.Repeat("█", filled) + strings.Repeat("░", width-filled))
	fmt.Printf("\r  %s %d/%d %s\x1b[K", bar, done, total, truncate(name, 44))
}

// apply downloads selected ISOs, removes delete-marked and superseded images,
// and saves the refreshed menu aliases. root is the search-root directory that
// catalog downloads land in.
func apply(ctx context.Context, ventoyDir, root string, rows []Update, progress Progress) error {
	if ctx.Err() != nil {
		return errCancelled
	}
	cfg, err := ventoy.Load(ventoyDir)
	if err != nil {
		return err
	}
	hasConfig := cfg.Exists()
	for _, u := range rows {
		if ctx.Err() != nil {
			return errCancelled
		}
		if u.DeleteISO {
			if u.Current.Path != "" && fileSize(u.Current.Path) > 0 {
				if err := os.Remove(u.Current.Path); err != nil {
					return fmt.Errorf("remove %s: %w", u.Current.Filename, err)
				}
			}
			continue
		}
		if !downloadable(u) {
			continue
		}
		var target string
		if u.New {
			target = filepath.Join(root, u.Latest.Image.Filename)
		} else {
			target = filepath.Join(filepath.Dir(u.Current.Path), u.Latest.Image.Filename)
		}
		if err := download(ctx, u.Latest.URL, target, func(done, total int64) {
			if progress != nil {
				progress(u.Latest.Image.Filename, done, total)
			}
		}); err != nil {
			if errors.Is(ctx.Err(), errCancelled) || ctx.Err() != nil {
				return errCancelled
			}
			return err
		}
		if !u.KeepOld && u.Current.Path != "" {
			if err := os.Remove(u.Current.Path); err != nil {
				return fmt.Errorf("remove %s: %w", u.Current.Filename, err)
			}
		}
		if u.Current.Path != "" && hasConfig {
			cfg.SetAliasForPath(filepath.ToSlash(strings.TrimPrefix(u.Current.Path, filepath.Dir(ventoyDir)+string(os.PathSeparator))), u.Latest.Image.CanonicalName())
		}
	}
	if !hasConfig {
		return nil
	}
	return cfg.Save(ventoyDir)
}

// download streams url into a temporary sibling file before atomically renaming
// it, reporting byte progress at most once per MiB and once at completion. A
// cancelled context aborts with errCancelled; the context also feeds the idle
// watchdog, which aborts transfers that go quiet on their own.
func download(ctx context.Context, url, target string, progress func(done, total int64)) error {
	watch, cancelWatch := context.WithCancel(ctx)
	defer cancelWatch()
	req, err := http.NewRequestWithContext(watch, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	// Cancel the request if the body goes quiet for downloadIdleTimeout; each
	// successful read restarts the timer, so slow-but-progressing links survive.
	stalled := time.AfterFunc(downloadIdleTimeout, cancelWatch)
	defer stalled.Stop()
	resp, err := downloadClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return errCancelled // user stop; a raw wrapped error hides it
		}
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("download %s: %s", url, resp.Status)
	}
	total := resp.ContentLength
	if total < 0 {
		total = 0
	}
	tmp, err := os.CreateTemp(filepath.Dir(target), ".ventoy-download-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	buffer := make([]byte, 128*1024)
	var done, next int64
	for {
		read, readErr := resp.Body.Read(buffer)
		if read > 0 {
			stalled.Reset(downloadIdleTimeout)
			if _, err := tmp.Write(buffer[:read]); err != nil {
				tmp.Close()
				return err
			}
			done += int64(read)
			if progress != nil && done >= next {
				next = done + chunkProgress
				progress(done, total)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			tmp.Close()
			// ctx is cancelled either by the user or by the idle watchdog;
			// a watchdog-only cancellation means the transfer went quiet.
			if ctx.Err() != nil {
				return errCancelled
			}
			if watch.Err() != nil {
				return fmt.Errorf("download stalled: no data received for %s", downloadIdleTimeout)
			}
			return readErr
		}
	}
	if progress != nil {
		progress(done, total)
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, target)
}

// printDownload redraws a single-line byte progress bar for batch downloads.
func printDownload(filename string, done, total int64) {
	const width = 28
	bar := strings.Repeat("█", width)
	percent := ""
	if total > 0 {
		filled := int(done * width / total)
		if filled > width {
			filled = width
		}
		bar = strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
		percent = fmt.Sprintf(" %3d%%", done*100/total)
	}
	styled := lipgloss.NewStyle().Foreground(lipgloss.Color("86")).Render(bar)
	sizes := " " + space.HumanBytes(done)
	if total > 0 {
		sizes += " / " + space.HumanBytes(total)
	}
	fmt.Printf("\r  %s%s %s%s\x1b[K", styled, percent, truncate(filename, 36), sizes)
	if total > 0 && done >= total {
		fmt.Println()
	}
}
