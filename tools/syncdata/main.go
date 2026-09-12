// syncdata refreshes the knowledge base files the app embeds from a
// checkout of the community repo, usb-device-kb, and records where they
// came from.
//
//	go run ./tools/syncdata [-from usb-device-kb]
//
// go:embed can only read files inside core/kb/data, and a build must not
// depend on the sibling checkout existing, so the app keeps a copy. This
// is the one way that copy changes: pull the data repo, run this, commit
// the app repo. PROVENANCE names the data repo commit the copy matches.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// files are the knowledge base files the app embeds, relative to both
// the data repo root and core/kb/data.
var files = []string{"docks.json", "devices.json"}

const target = "core/kb/data"

func main() {
	// The data repo is cloned inside the project, next to plan/, and
	// ignored by git; see .gitignore.
	from := flag.String("from", "usb-device-kb", "checkout of the usb-device-kb repo")
	flag.Parse()

	commit, dirty, err := gitState(*from)
	if err != nil {
		fmt.Fprintf(os.Stderr, "syncdata: %v\n", err)
		os.Exit(1)
	}
	if dirty {
		fmt.Fprintln(os.Stderr, "syncdata: the data repo has uncommitted changes; commit them first so PROVENANCE can name a commit")
		os.Exit(1)
	}

	changed := 0
	for _, name := range files {
		src, err := os.ReadFile(filepath.Join(*from, name))
		if err != nil {
			fmt.Fprintf(os.Stderr, "syncdata: %v\n", err)
			os.Exit(1)
		}
		dst := filepath.Join(target, name)
		old, _ := os.ReadFile(dst)
		if bytes.Equal(old, src) {
			fmt.Printf("%s: unchanged\n", dst)
			continue
		}
		if err := os.WriteFile(dst, src, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "syncdata: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("%s: updated\n", dst)
		changed++
	}

	provenance := fmt.Sprintf("source: https://github.com/harvey-withington/usb-device-kb\ncommit: %s\nsynced: %s\nfiles: %s\n",
		commit, time.Now().UTC().Format("2006-01-02"), strings.Join(files, ", "))
	if err := os.WriteFile(filepath.Join(target, "PROVENANCE"), []byte(provenance), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "syncdata: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("%d file(s) updated from %s at %s\n", changed, *from, commit[:12])
}

// gitState returns the checkout's HEAD commit and whether it has changes
// to the files this tool copies.
func gitState(dir string) (commit string, dirty bool, err error) {
	if _, err := git(dir, "rev-parse", "--git-dir"); err != nil {
		return "", false, fmt.Errorf("%s is not a git checkout: %w", dir, err)
	}
	head, err := git(dir, "rev-parse", "HEAD")
	if err != nil {
		return "", false, fmt.Errorf("%s has no commits yet; commit it first so PROVENANCE can name one", dir)
	}
	status, err := git(dir, append([]string{"status", "--porcelain", "--"}, files...)...)
	if err != nil {
		return "", false, err
	}
	return strings.TrimSpace(head), strings.TrimSpace(status) != "", nil
}

func git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}
