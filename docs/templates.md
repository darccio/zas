<!--
title: Zas templates and plugins
summary: Use layouts and shared metadata while keeping content-triggered plugins controlled.
language: en
feed: false
template: false
-->
# Templates and plugins

Zas uses Go templates in page content and in the shared `.zas/layout.html`, with different escaping behavior.

## Layout

A minimal layout can use:

```html
<!doctype html>
<html lang="{{.Language}}">
<head>
  <meta charset="utf-8">
  <title>{{.Title}}</title>
  <link rel="canonical" href="{{.Canonical}}">
  {{with .Summary}}<meta name="description" content="{{.}}">{{end}}
</head>
<body>
  <main>{{.Body}}</main>
</body>
</html>
```

The layout runs through Go's `html/template`, which escapes values according to their HTML context. `.Body` contains the already-rendered page body.

## Metadata helpers

| Helper | Value |
| --- | --- |
| `.Title` | Explicit page `title`, otherwise the first H1. |
| `.Language` | Resolved page language. |
| `.Canonical` | Absolute page URL with a final `/index.html` replaced by `/`. |
| `.URL` | Absolute generated path; retains `/index.html` when that is the filename. |
| `.Date` | Explicit RFC3339 `date`, or zero time. |
| `.Updated` | Explicit `updated`, otherwise the tracked source's latest git commit date, or zero time. |
| `.Summary` | Explicit `summary`, or an empty string. |
| `.Tags` | Explicit string-list `tags`, in their original order. |
| `.Page` | Page YAML map. |
| `.Directory` | Directory configuration map. |
| `.Resolve "language"` | Page, directory, then site setting. |
| `.Extra "/site/baseurl"` | Direct string lookup in the site configuration. |

A standalone `article.md` has canonical `/article.html`; the helper only normalizes index pages. To format a publication date, first check that it is present:

```html
{{if not .Date.IsZero}}<time datetime="{{.Date.Format "2006-01-02T15:04:05Z07:00"}}">{{.Date.Format "2006-01-02"}}</time>{{end}}
```

Invalid explicit metadata stops template execution when its helper is used. Publication dates are never inferred from git, file timestamps or build time. A full-history checkout gives `.Updated` accurate git history.

Inside a page's own content, `.Page`, `.Title` and `.FirstTitle` use a best-effort preview. The layout sees the final values after page rendering. `.Body` is empty inside the page's own template.

## Page listings

Set `site.page_listings: true` to expose `.Pages "posts"` in page, Markdown
and layout templates. The option defaults to false; disabled sites do no
listing scan, and a `.Pages` call explains how to enable it.

For a page archive, use explicit HTML escaping because page content runs
through `text/template`:

```html
<!-- title: Posts -->
<h1>Posts</h1>
<ul>
{{range .Pages "posts"}}
  <li><a href="{{html .URL}}">{{html .Title}}</a>{{with .Summary}}<p>{{html .}}</p>{{end}}</li>
{{end}}
</ul>
```

In the layout, use `.URL`, `.Title` and `.Summary` directly: `html/template`
auto-escapes their plain strings. `.Pages(section)` returns independent
`[]PageSummary` slices and an error. Summaries contain:

| Field | Value |
| --- | --- |
| `Title` | Literal page `title`, otherwise static first H1 text. |
| `URL` | Escaped root-relative canonical path, including any baseurl hosting subpath, for local preview navigation. |
| `Canonical` | The existing canonical helper's path with `site.baseurl`, or root-relative if unset. |
| `Date` | Explicit RFC3339 publication date as `time.Time`, otherwise zero time. |
| `Language` | Page, nearest directory configuration, then site language. |
| `Summary` | Literal page `summary`, otherwise empty. |

`posts` recursively includes the directory's pages and nested article
indexes, omitting only `posts/index.md` or `posts/index.html`. It does not
match `posts-old`. Empty and `.` sections select the root; missing sections
return empty lists. Paths must be relative directory paths; trailing `/`
is allowed. Traversal, absolute paths, backslashes, queries, fragments and
encoded path syntax are invalid and reported when `.Pages` is called.
As with other template failures, a full build may already have cleared
deploy; use a staging directory to preserve published output on failure.
Known dates sort newest first, then by URL; undated pages sort last by URL.
Dates never come from file timestamps, git or build time.

Listings use a complete static-source snapshot collected before rendering,
without executing templates, embeds or plugins. They skip the layout,
deploy output, symlinks, hidden sources (following the normal dot-directory
allowlist), `publish: false` and `draft: true`. Drafts keep their existing
standalone HTML behavior. Listing metadata must be valid literal values in
the leading YAML comment; template actions there are rejected unless
`template: false`. Template-generated H1 text yields an empty title; provide
a literal `title` to name it. A leading action that could generate the
configuration comment is rejected: put a literal comment or heading first.
Missing static titles are allowed. Invalid static metadata fails before
full-build cleanup; generated publication flags that disagree with the
snapshot fail the build when rendering reveals them. Enabled runs render
every HTML/Markdown source so all archives reflect additions, removals,
renames and metadata edits; newly unpublished
HTML is removed during incremental builds. This trades incremental render
speed for consistent lists.

## JSON in layouts

`dict`, `list` and `json` are layout-only helpers. Build objects rather than passing a pre-encoded JSON string:

```html
{{$person := dict "@type" "Person" "name" "Dario Castañé" "url" .Site.BaseURL}}
<script type="application/ld+json">{{json (dict "@context" "https://schema.org" "@graph" (list $person))}}</script>
```

`json` serializes the value with `encoding/json` and retains its HTML-sensitive character escaping. Serialization errors fail the build. It adds no schema automatically.

## Embeds

Built-in handlers support Markdown, HTML and plain text:

```html
<embed src="navigation.md" type="text/markdown" />
```

Page-body embeds resolve relative to the containing page. Layout embeds resolve relative to the site root. Built-in nested embeds remain inside the site root and have a depth limit.

Mark an embedded-only navigation file `publish: false` to prevent it also becoming a standalone page. With Markdown twins enabled, Markdown embeds must occupy a standalone line; combinations with reference definitions or footnotes are rejected to avoid changing reference scopes.

## Plugin and template trust model

Page content runs through Go's `text/template` with no automatic escaping. This includes template actions inside code fences. To display literal actions, put `template: false` in the page's leading configuration comment. The layout still runs normally.

External subcommands named `zs<name>` and MIME handlers named `mzs<name>` are programs found on `PATH`. Content can trigger MIME handlers through embeds, or `zs` programs through `<script type="application/zas+name">` tags. Zas does not sandbox or verify these binaries.

Build content you do not fully control with:

```sh
zas -full -no-plugins
```

`-no-plugins` rejects content-triggered external MIME and script plugins rather than executing them. Built-in embeds still run, and the flag does not restrict a plugin explicitly invoked as `zas <name>` on the command line.

`template: false` alone does not disable embeds or plugins, and `-no-plugins` alone does not disable Go template execution. For untrusted documentation, render or sanitize it separately, avoid executing its template actions, and keep `-no-plugins` enabled in the assembly build.

Use layout `noescape` or `.H` only for HTML you trust. `.E` is escaped in the layout but not in a page's own template; `.H` marks translations as trusted HTML in the layout.
