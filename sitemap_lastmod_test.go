package zas

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// initGitRepoWithCommit runs git init + a single commit of relPath inside
// the current directory (expected to already be a copied temp site via
// newTestSite), with explicit author/committer dates so the resulting
// commit date is deterministic regardless of when the test runs. It skips
// the test if git isn't available on PATH, rather than failing: the
// lastmod git path is a best-effort enhancement (see sitemapLastmod's
// documented fallback to mtime), and CI environments without git
// installed should still be able to run the rest of the suite.
func initGitRepoWithCommit(t *testing.T, relPath string, when time.Time) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	whenStr := when.Format(time.RFC3339)
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_DATE="+whenStr,
			"GIT_COMMITTER_DATE="+whenStr,
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	run("-c", "user.email=test@example.com", "-c", "user.name=Test", "add", relPath)
	run("-c", "user.email=test@example.com", "-c", "user.name=Test", "commit", "-q", "-m", "add "+relPath)
}

func TestSitemapLastmodUsesGitCommitDate(t *testing.T) {
	newTestSite(t, "sitemap-site")
	when := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	initGitRepoWithCommit(t, "index.html", when)

	if err := generate(t); err != nil {
		t.Fatalf("generate() error = %v, want nil", err)
	}
	out := readDeploy(t, "sitemap.xml")
	want := when.Format(time.RFC3339)
	if !strings.Contains(out, "<lastmod>"+want+"</lastmod>") {
		t.Fatalf("sitemap.xml = %q, want it to contain <lastmod>%s</lastmod> from the git commit date", out, want)
	}
}

func TestSitemapLastmodFallsBackToMtimeWhenUntracked(t *testing.T) {
	newTestSite(t, "sitemap-site")
	// A real git repo exists, but about.md is never committed to it - only
	// index.html is - so gitCommitDate must fall through to about.md's own
	// mtime.
	initGitRepoWithCommit(t, "index.html", time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC))

	if err := generate(t); err != nil {
		t.Fatalf("generate() error = %v, want nil", err)
	}
	info, err := os.Stat("about.md")
	if err != nil {
		t.Fatal(err)
	}
	want := info.ModTime().Format(time.RFC3339)
	out := readDeploy(t, "sitemap.xml")
	if !strings.Contains(out, "<lastmod>"+want+"</lastmod>") {
		t.Fatalf("sitemap.xml = %q, want it to contain about.html's mtime-based <lastmod>%s</lastmod>", out, want)
	}
}

func TestSitemapLastmodFallsBackToMtimeWhenNotAGitRepo(t *testing.T) {
	newTestSite(t, "sitemap-site")
	// No git init at all.
	if err := generate(t); err != nil {
		t.Fatalf("generate() error = %v, want nil", err)
	}
	info, err := os.Stat("index.html")
	if err != nil {
		t.Fatal(err)
	}
	want := info.ModTime().Format(time.RFC3339)
	out := readDeploy(t, "sitemap.xml")
	if !strings.Contains(out, "<lastmod>"+want+"</lastmod>") {
		t.Fatalf("sitemap.xml = %q, want it to contain index.html's mtime-based <lastmod>%s</lastmod>", out, want)
	}
}

func TestGitCommitDateUnknownSourceReturnsFalse(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	if _, ok := gitCommitDate("does-not-exist.md"); ok {
		t.Fatal("gitCommitDate() ok = true for a file with no history, want false")
	}
}

func TestSitemapLastmodOmittedWhenSourceUnavailable(t *testing.T) {
	if got := sitemapLastmod("/nonexistent/path/does-not-exist.md", false); got != "" {
		t.Fatalf("sitemapLastmod() = %q, want \"\" when neither git nor mtime are available", got)
	}
}
