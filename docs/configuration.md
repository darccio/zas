<!--
title: Zas configuration
summary: Configure site output, page metadata and language overrides.
language: en
feed: false
template: false
-->
# Configuration

Zas reads `.zas/config.yml`. You can keep the defaults and add only the settings your site needs.

```yaml
zas:
  layout: .zas/layout.html
  deploy: .zas/deploy
  allowed_dotdirs: [".well-known"]
site:
  baseurl: https://example.com
  language: en
  image: https://example.com/static/avatar.webp
  sitemap: true
  publish_markdown: true
```

## Site settings

| Setting | Default | Purpose |
| --- | --- | --- |
| `zas.layout` | `.zas/layout.html` | Shared HTML layout. |
| `zas.deploy` | `.zas/deploy` | Generated output directory. |
| `zas.allowed_dotdirs` | Empty | Specific top-level dot-directories to publish. |
| `site.baseurl` | `http://example.com` | Absolute site URL used by URL helpers, sitemaps and feeds. |
| `site.language` | `en` | Default page language. |
| `site.image` | Unset | Site image available through `.Site.Image`. |
| `site.sitemap` | `false` | Generate a sitemap and its robots directive. |
| `site.publish_markdown` | `false` | Publish rendered Markdown twins. |
| `site.feed` | `false` | Generate RSS feeds. |

Use a real HTTP(S) `baseurl` when generating a sitemap or feed. See [Publishing](publishing.md) for feed options and output behavior.

Dot-files and dot-directories are skipped by default. `allowed_dotdirs` accepts exact top-level names, such as `.well-known`; it does not allow nested dot-directories or patterns. `.git` and `.zas` remain excluded.

## Page metadata

Put YAML in the leading HTML comment of a Markdown or HTML page:

```markdown
<!--
title: My article
language: en
date: 2026-10-11T10:00:00+02:00
summary: A short plain-text description.
tags: [go, writing]
-->
# My article
```

For resolved settings such as `language`, page metadata takes precedence over the nearest directory `.zas.yml`, which takes precedence over site settings. Page metadata remains available through `.Page`.

| Page setting | Effect |
| --- | --- |
| `title` | Override the first H1 as the page title. |
| `language` | Override the resolved language. |
| `date` | Explicit RFC3339 publication timestamp with a timezone. |
| `updated` | Explicit RFC3339 update timestamp with a timezone. |
| `summary` | Plain-text summary string. |
| `tags` | List of strings. |
| `publish: false` | Exclude a standalone page, while retaining it as embed content. |
| `template: false` | Skip the page's own Go template execution. |
| `draft: true` | Exclude the page from feeds and Markdown twins; existing HTML generation remains enabled. |
| `publish_markdown: false` | Opt this page out of Markdown twins. |
| `feed: false` | Opt this page out of feeds. |
| `article: false` | Classify a utility page as outside the article feed selection. |

`template: false` still allows parsing and embed processing. Use `-no-plugins` to prevent content-triggered external programs; see the [trust model](templates.md).

## Languages

Set the site default in `site.language`. For a language subtree, add a `.zas.yml` such as:

```yaml
language: es
```

Place it in `es/`, alongside `es/index.md` and other Spanish pages. Pages can also override their language individually.

Translations live in `.zas/i18n.yml`:

```yaml
Hello:
  en: Hello
  es: Hola
  ca: Hola
```

Use `{{.E "Hello"}}` in a page or layout. Missing translations produce `**Hello**`. Language directories with matching relative page paths are paired automatically for sitemap `hreflang`; a page-only language override does not create that pairing.
