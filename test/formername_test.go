package test

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// formerName matches every name that is not this project's, however it is
// written: what the project was called before 2026-11-17, and the two names of
// the ticket system cloned from it on 2026-11-12 - its first and the one it
// carries now - which this tree has no reason to name and since 2026-11-22 is
// held to not naming, because a change carried over between the two is how one
// would arrive. A name is its words joined by a hyphen, an underscore, a space
// or nothing, in any case, and its abbreviations go with it: the three letters
// the former name and the ticket system's first name carried in their cookies,
// token prefixes, variables and sealed secrets, and the two the ticket system's
// backups carry.
//
// The three letters count as a word of their own after anything but a letter
// or a digit; glued to a word where no + or / before them could make them the
// inside of a hash; and anywhere before a separator their prefixes used, which
// is how a value glued to the option in front of it carries them, as a MySQL
// password follows -p. All of it is spelled so that this file does not match
// it.
var formerName = regexp.MustCompile(`(?i)go[-_ ]?time[-_ ]?recording|go[-_ ]?tick[e]t[-_ ]?system|` +
	`qubit[-_ ]?de[s]k|q[d](bak|backup)|(^|[^a-z0-9])g[t][rs]([^a-z0-9]|$)|(^|[^a-z0-9+/])g[t][rs]|` +
	`g[t][rs][_.:-]`)

// oldNameKeptOnPurpose are the files that may still use a name formerName
// matches, each with its reason. A row whose file has stopped using it fails as well, so the
// list cannot go on excusing what is no longer there. It is empty: the seal
// conversion stopped naming the former marker on 2026-11-22, once every
// installation had run it, and the audit ledger's quotes of a database name
// were brought to the present name the same day - examples in findings about
// how a connection is written, which the name does not change.
var oldNameKeptOnPurpose = map[string]string{}

// The project was renamed to TimePulse on 2026-11-17, and the sweep that did it
// looked for the spellings it thought of. One got past it: the Android app's
// download keeper marked every page it prepared with the former name in camel
// case, which nothing reported, because nothing reads that mark but the script
// that sets it. The ticket system's own sweep later let its first name's
// abbreviation past where it was glued to an option, in two health checks that
// went on passing because mysqladmin ping answers success to a refused
// password. So every spelling of every name is looked for here at once, in
// every text file git keeps and in the name of every file.
func TestNothingStillCarriesTheFormerName(t *testing.T) {
	// safe.directory, because in the Linux container the suite runs in here the
	// mounted repository belongs to another user and git refuses it otherwise.
	out, err := exec.Command("git", "-c", "safe.directory=*", "-C", "..", "ls-files", "--eol").Output()
	if err != nil {
		t.Fatalf("asking git which files it keeps: %v", err)
	}

	listed := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(listed) < 500 {
		t.Fatalf("git listed %d files; the reading has stopped finding them", len(listed))
	}

	usesIt := map[string]bool{}

	for _, line := range listed {
		meta, path, _ := strings.Cut(line, "\t")
		if formerName.MatchString(path) {
			t.Errorf("%s is named with a name that is not this project's", path)
		}

		if strings.HasPrefix(meta, "i/-text") {
			continue
		}

		content, err := os.ReadFile(filepath.Join("..", filepath.FromSlash(path)))
		if err != nil {
			t.Errorf("reading %s: %v", path, err)

			continue
		}

		for n, text := range strings.Split(string(content), "\n") {
			if !formerName.MatchString(text) {
				continue
			}

			usesIt[path] = true

			if _, kept := oldNameKeptOnPurpose[path]; !kept {
				t.Errorf("%s:%d names what this project is not called: %s", path, n+1, strings.TrimSpace(text))
			}
		}
	}

	for path, reason := range oldNameKeptOnPurpose {
		if !usesIt[path] {
			t.Errorf("%s no longer uses a name formerName matches, so its row (%q) excuses nothing; remove it",
				path, reason)
		}
	}
}
