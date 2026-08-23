package zas

import (
	"os"
	"strings"
	"testing"
)

func TestGenerateRobotsSynthesizedWhenMissing(t *testing.T) {
	newTestSite(t, "sitemap-site")
	if err := generate(t); err != nil {
		t.Fatalf("generate() error = %v, want nil", err)
	}
	out := readDeploy(t, "robots.txt")
	if !strings.Contains(out, "Sitemap: https://example.com/sitemap.xml") {
		t.Fatalf("robots.txt = %q, want a Sitemap directive", out)
	}
	if !strings.Contains(out, "User-agent: *") {
		t.Fatalf("robots.txt = %q, want a synthesized minimal crawl policy", out)
	}
}

func TestGenerateRobotsAppendedToExisting(t *testing.T) {
	newTestSite(t, "sitemap-site")
	if err := os.WriteFile("robots.txt", []byte("User-agent: *\nDisallow: /private/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := generate(t); err != nil {
		t.Fatalf("generate() error = %v, want nil", err)
	}
	out := readDeploy(t, "robots.txt")
	if !strings.Contains(out, "Disallow: /private/") {
		t.Fatalf("robots.txt = %q, want the site-authored content preserved", out)
	}
	if !strings.Contains(out, "Sitemap: https://example.com/sitemap.xml") {
		t.Fatalf("robots.txt = %q, want an appended Sitemap directive", out)
	}
}

func TestGenerateRobotsSitemapLineNotDuplicatedOnSecondRun(t *testing.T) {
	newTestSite(t, "sitemap-site")
	if err := generate(t); err != nil {
		t.Fatalf("generate() error = %v, want nil", err)
	}
	if err := generate(t); err != nil {
		t.Fatalf("second generate() error = %v, want nil", err)
	}
	out := readDeploy(t, "robots.txt")
	if n := strings.Count(out, "Sitemap:"); n != 1 {
		t.Fatalf("robots.txt has %d Sitemap directives after two runs, want exactly 1: %q", n, out)
	}
}

func TestGenerateRobotsNotCreatedWhenSitemapOff(t *testing.T) {
	newTestSite(t, "site")
	if err := generate(t); err != nil {
		t.Fatalf("generate() error = %v, want nil", err)
	}
	assertDeployMissing(t, "robots.txt")
}

func TestRobotsHasSitemapDirective(t *testing.T) {
	tests := []struct {
		name    string
		content string
		url     string
		want    bool
	}{
		{"present exact match", "User-agent: *\nSitemap: https://example.com/sitemap.xml\n", "https://example.com/sitemap.xml", true},
		{"present case-insensitive key", "SITEMAP: https://example.com/sitemap.xml\n", "https://example.com/sitemap.xml", true},
		{"different url", "Sitemap: https://example.com/other.xml\n", "https://example.com/sitemap.xml", false},
		{"absent", "User-agent: *\nDisallow: /\n", "https://example.com/sitemap.xml", false},
		{"empty", "", "https://example.com/sitemap.xml", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := robotsHasSitemapDirective(tt.content, tt.url); got != tt.want {
				t.Errorf("robotsHasSitemapDirective(%q, %q) = %v, want %v", tt.content, tt.url, got, tt.want)
			}
		})
	}
}
