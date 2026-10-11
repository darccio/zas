<!--
title: Publishing with Zas
summary: Generate sitemaps, RSS and rendered Markdown twins for a static host.
language: en
feed: false
template: false
-->
# Publishing

Build all files with `zas -full`, then upload `.zas/deploy` to a static host. Choose `-no-plugins` when content is not fully controlled by the site author.

Without `-full`, Zas updates changed inputs incrementally. Layout, configuration, translation files, directory configuration and resolvable built-in embed dependencies can also trigger rendering. Use a full build after changing a dynamic embed source or a plugin's additional inputs.

## Sitemap

```yaml
site:
  baseurl: https://example.com
  sitemap: true
```

Zas writes a sitemap containing generated pages and updates the deploy copy of `robots.txt` with its `Sitemap:` directive. Large sites are split into numbered sitemaps and an index automatically.

Sitemap modification dates use source git history, falling back to file modification time when needed. Unlike sitemap dates, publication metadata never falls back to file modification time. In GitHub Actions, use `fetch-depth: 0` for accurate history.

Matching relative paths under language directories receive reciprocal sitemap `hreflang` links. Pages without a translated sibling remain valid.

## RSS 2.0

```yaml
site:
  baseurl: https://example.com
  language: en
  feed: true
  feed_title: My writing
  feed_description: Articles about Go and static sites.
  feed_sections: [posts]
  feed_limit: 50
  feed_per_language: true
  feed_strict: true
```

The root feed is `/feed.xml`. With `feed_per_language`, non-default languages that have articles also get `/<language>/feed.xml`. Each feed is sorted newest first and capped independently by `feed_limit`.

Feeds are disabled by default. When enabled, `feed_title` falls back to `site.title` and must be non-empty. `feed_description` falls back to `site.description`, then the title. The default limit is 50, and strict validation is enabled by default.

Every selected article needs an explicit RFC3339 `date` with a timezone. A configured `feed_sections` includes descendant pages but treats the section-root index as an archive. A nested `posts/article/index.md` remains an article. Without sections, dated pages or pages marked `article: true` are selected, apart from the unclassified root homepage.

`publish: false`, `draft: true` and `feed: false` exclude pages before metadata validation. `article: false` excludes utility pages. Non-strict mode warns and skips invalid selected items; it does not invent their dates.

RSS items contain the full pre-layout HTML body, a plain-text summary, stable canonical permalink GUIDs, RFC1123Z publication dates and tags. Relative content links are made absolute. The channel has a self link and a build date derived from item updates, so rebuilding unchanged content does not insert a new build timestamp.

Add feed discovery to your own layout:

```html
<link rel="alternate" type="application/rss+xml" title="My writing" href="https://example.com/feed.xml">
```

Configure your host to serve feeds as `application/rss+xml; charset=utf-8`. Zas writes files; it does not configure HTTP response headers.

## Rendered Markdown twins

```yaml
site:
  publish_markdown: true
```

`section/index.md` produces both `section/index.html` and `section/index.md` in deploy. `contact.md` produces `contact.html` and `contact.md`. HTML source pages do not receive automatic Markdown conversions.

The twin contains the post-template Markdown body, with leading configuration comments removed and without the shared HTML layout. Template values are expanded once before Markdown parsing. This is an opt-in change in rendering order from the legacy path; template values containing Markdown participate in Markdown rendering.

`publish: false`, `draft: true` and page-level `publish_markdown: false` suppress twins. With `template: false`, literal template actions are intentionally preserved in both formats. Unsupported external content plugins fail with opt-out guidance before execution; the ordinary HTML layout pass can still contain permitted plugins.

Zas records paths and hashes in the private `.zas/markdown-outputs.json` ownership manifest. Incremental builds clean previous owned twins when sources disappear or are opted out, while preserving manually changed or unowned files. Disabling the site option cleans previous owned twins after a successful build. Full builds retain their normal deploy-directory replacement behavior after preflight checks.

Choose the conventional deploy directory or a new, empty destination. Output collisions and unsafe source/deploy overlap fail preflight rather than overwriting unrelated source files.

Zas does not add discovery links or HTTP content negotiation. Add links and a host adapter separately when those are useful to your site.

## Reproducible deployment

Pin the Zas version and external documentation revisions. Keep credentials in your CI or host secret store. Build in a temporary assembly directory, verify the output, and promote the completed deploy directory only after validation.

This repository's docs workflow requests a pinned update in `darccio/darccio`; it does not publish a site directly. The site update is reviewed as a pull request, and Cloudflare Pages deploys it after merge.
