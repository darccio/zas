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
