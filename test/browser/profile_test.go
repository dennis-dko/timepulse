//go:build browser

package browser

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	cdpbrowser "github.com/chromedp/cdproto/browser"
	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/chromedp"

	"github.com/dennis-dko/timepulse/test/tempdir"
)

// TestABrowserLeavesNoProfileBehind closes a browser while something still
// holds a file in its profile, lets go once the browser is gone, and asks
// whether the profile went with it.
//
// A full run produces that order by itself on Windows: a helper process of
// Chrome's still writing to the profile when chromedp removed it, which chromedp
// does once, ten milliseconds after the browser's own process has exited. One
// green run of the suite left seventeen profiles behind, mostly empty
// directories and the odd cache journal. The order is forced here rather than
// waited for, because a race proves nothing on a day it is not lost.
//
// On Linux an open file does not stop a removal, so this passes there either
// way; as with everything test/tempdir is for, only a Windows run shows it.
func TestABrowserLeavesNoProfileBehind(t *testing.T) {
	t.Parallel()

	var profile string

	t.Run("a browser", func(t *testing.T) {
		alloc, cancelAlloc := chromedp.NewExecAllocator(context.Background(), launch(t, "en-US")...)
		defer cancelAlloc()

		ctx, cancel := chromedp.NewContext(alloc)
		defer cancel()

		ctx, cancelTimeout := context.WithTimeout(ctx, interactionTimeout)
		defer cancelTimeout()

		dir, err := profileDirectoryOf(ctx)
		if err != nil {
			t.Fatal(err)
		}

		profile = dir

		// Held the way Chrome's helper held it: open past the moment the browser
		// is closed.
		held, err := os.Create(filepath.Join(dir, "held"))
		if err != nil {
			t.Fatal(err)
		}

		// Closed the way a case's cleanups close it, the allocator last: it
		// returns once the browser has exited and chromedp has had its one try
		// at whatever profile it made.
		cancelTimeout()
		cancel()
		cancelAlloc()

		if err := held.Close(); err != nil {
			t.Fatal(err)
		}
	})

	if profile == "" {
		return
	}

	if _, err := os.Stat(profile); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the profile outlived its browser: %s still holds %q", profile, leftIn(profile))
	}
}

// profileDirectoryOf asks the browser behind ctx where it keeps its profile.
//
// Asked of the browser rather than worked out from the options, so the answer is
// the same whoever chose the directory - chromedp or this suite.
func profileDirectoryOf(ctx context.Context) (string, error) {
	var arguments []string

	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		var err error
		arguments, err = cdpbrowser.GetBrowserCommandLine().Do(cdp.WithExecutor(ctx, chromedp.FromContext(ctx).Browser))

		return err
	}))
	if err != nil {
		return "", fmt.Errorf("ask the browser for its command line: %w", err)
	}

	for _, argument := range arguments {
		if dir, ok := strings.CutPrefix(argument, "--user-data-dir="); ok {
			return dir, nil
		}
	}

	return "", fmt.Errorf("the browser names no profile directory among %q", arguments)
}

// leftIn lists the files still under dir, for a failure to name.
// TestAProfileIsRemovedOnlyOnceItsLastProcessHasGone stands a process in for
// Chrome's network service - one naming the profile on its command line, as
// every helper of Chrome's does - which writes into the profile a moment after
// the case has ended, and asks whether the profile stays gone.
//
// That is what failed two cases that had passed, in one run on the CI runner:
// the browser's own process had been killed, its network service went on
// writing its cache index and its network state, re-creating the profile
// tempdir had just removed, and testing.T's own removal met a directory still
// being written to. Measured with Google Chrome on two CPUs, the profile came
// back in six cases of 120. The order is forced here, as above.
func TestAProfileIsRemovedOnlyOnceItsLastProcessHasGone(t *testing.T) {
	if _, err := os.Stat("/proc/self/cmdline"); err != nil {
		t.Skip("only Linux says which processes name a profile")
	}

	t.Parallel()

	var profile string

	t.Run("a browser's helper", func(t *testing.T) {
		profile = profileFor(t)

		helper := exec.Command("sh", "-c",
			`sleep 0.3; dir="${1#--user-data-dir=}"; mkdir -p "$dir/Default"; echo late > "$dir/Default/late"`,
			"network-service", "--user-data-dir="+profile)
		if err := helper.Start(); err != nil {
			t.Fatal(err)
		}

		// Reaped when it ends, and waited for by nothing here: whatever waits
		// for it has to be what removes the profile.
		go func() { _ = helper.Wait() }()

		// Seen running before the case may end, as a browser's helpers long
		// have been by the time one does. Start returns once the child's exec
		// has closed the pipe it reports through, which the kernel does before
		// it lays out the new program's arguments, so for a moment the helper's
		// /proc/<pid>/cmdline reads empty - measured in a container, in 1,372 of
		// 2,000 starts read at once. A removal scanning in that moment found
		// nobody naming the profile, and this case failed on main once.
		named := []byte("--user-data-dir=" + profile + "\x00")

		for appeared := time.Now().Add(5 * time.Second); !processNames(named); {
			if time.Now().After(appeared) {
				t.Fatal("the stand-in helper never appeared under /proc")
			}

			time.Sleep(5 * time.Millisecond)
		}
	})

	time.Sleep(time.Second)

	if _, err := os.Stat(profile); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the profile came back after it was removed: %s holds %q", profile, leftIn(profile))
	}
}

// TestAProfileIsRemovedOnlyOnceNothingHoldsAFileInIt stands in for a helper that
// writes into the profile without naming it on its command line: it holds a file
// there open, as a helper keeps its database open, and writes again a moment
// after the case has ended.
//
// Waiting for the processes that name the profile was not enough on the CI
// runner. Cases kept failing with testing.T's own removal meeting "directory not
// empty" in the profile's Default directory, after every process carrying
// --user-data-dir had gone - so a writer was left that names it some other way,
// or not at all.
func TestAProfileIsRemovedOnlyOnceNothingHoldsAFileInIt(t *testing.T) {
	if _, err := os.Stat("/proc/self/fd"); err != nil {
		t.Skip("only Linux says which processes hold a file in a profile")
	}

	t.Parallel()

	var profile string

	t.Run("a writer naming nothing", func(t *testing.T) {
		profile = profileFor(t)

		writer := exec.Command("sh", "-c",
			`exec 3>"$1/held"; sleep 0.3; mkdir -p "$1/Default"; echo late > "$1/Default/late"`,
			"utility", profile)
		if err := writer.Start(); err != nil {
			t.Fatal(err)
		}

		go func() { _ = writer.Wait() }()

		// Seen holding its file before the case ends, for the reason the helper
		// above is seen naming the profile first.
		for appeared := time.Now().Add(5 * time.Second); !processHoldsAFileIn(profile); {
			if time.Now().After(appeared) {
				t.Fatal("the stand-in writer never held its file")
			}

			time.Sleep(5 * time.Millisecond)
		}
	})

	time.Sleep(time.Second)

	if _, err := os.Stat(profile); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the profile came back after it was removed: %s holds %q", profile, leftIn(profile))
	}
}

// profileFor is a profile for one browser, for as long as the case runs, and
// removed only once no process names it, or holds a file in it, any more.
//
// The case closes its browser before the profile goes, but closing it ends the
// browser's own process and no other: Chrome's helpers are processes of their
// own, and its network service went on writing into the profile for a tenth of a
// second after the browser had been killed. Closing it gracefully did not change
// that - measured, it came back three times in 120 rather than six - so what is
// waited for is the last process, not the browser.
func profileFor(t *testing.T) string {
	t.Helper()

	dir := tempdir.New(t)

	// Registered after tempdir's removal, so it runs first; and before anything
	// the case registers to close its browser, so it runs after that.
	t.Cleanup(func() { awaitTheLastProcessOf(dir) })

	return dir
}

// browserGone is how long the processes of a closed browser may take to end.
const browserGone = 10 * time.Second

// awaitTheLastProcessOf waits until no process names dir as its profile or
// holds a file in it, for as long as browserGone. Linux answers both under
// /proc. Elsewhere there is nothing to ask, and tempdir's retries are all there
// is - which on Windows is the question that matters, a file still held rather
// than one still being written.
//
// The command line alone was not enough: a writer the runner left behind named
// the profile some other way, and the profile's Default directory was still
// being written to when testing.T removed it.
func awaitTheLastProcessOf(dir string) {
	named := []byte("--user-data-dir=" + dir + "\x00")
	deadline := time.Now().Add(browserGone)

	for (processNames(named) || processHoldsAFileIn(dir)) && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
}

// processNames says whether any process's command line holds argument, an
// argument with the NUL /proc ends each with.
func processNames(argument []byte) bool {
	processes, err := os.ReadDir("/proc")
	if err != nil {
		return false
	}

	for _, process := range processes {
		if name := process.Name(); name[0] < '0' || name[0] > '9' {
			continue
		}

		// A process that has ended reads as nothing, a zombie included.
		line, err := os.ReadFile(filepath.Join("/proc", process.Name(), "cmdline"))
		if err == nil && bytes.Contains(line, argument) {
			return true
		}
	}

	return false
}

func leftIn(dir string) []string {
	var left []string

	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !entry.IsDir() {
			name, _ := filepath.Rel(dir, path)
			left = append(left, name)
		}

		return nil
	})
	if err != nil {
		left = append(left, err.Error())
	}

	return left
}

// processHoldsAFileIn says whether any process has a file under dir open, or
// stands in it.
func processHoldsAFileIn(dir string) bool {
	processes, err := os.ReadDir("/proc")
	if err != nil {
		return false
	}

	inside := func(target string) bool { return target == dir || strings.HasPrefix(target, dir+"/") }

	for _, process := range processes {
		if name := process.Name(); name[0] < '0' || name[0] > '9' {
			continue
		}

		base := filepath.Join("/proc", process.Name())

		if cwd, err := os.Readlink(filepath.Join(base, "cwd")); err == nil && inside(cwd) {
			return true
		}

		descriptors, err := os.ReadDir(filepath.Join(base, "fd"))
		if err != nil {
			continue
		}

		for _, descriptor := range descriptors {
			if target, err := os.Readlink(filepath.Join(base, "fd", descriptor.Name())); err == nil && inside(target) {
				return true
			}
		}
	}

	return false
}
