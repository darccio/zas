# ![Zas](https://i.imgur.com/e9abWRX.png)

Most simple static site generator ever.

## Why another one? C'mon, you must be kidding

I just wanted to set up a simple website, just some pages, using Jekyll, and it didn't feel right. I didn't want a blog.

I checked other projects, but they were incomplete, cumbersome, or solved the wrong problem (blogs, blogs everywhere). I wanted a zen-like experience: a layout and some Markdown files as pages with unobtrusive structure and configuration.

Yes, it is another NIH, but... I think Zas is a different kind of beast. I admit that I probably overlooked some projects at the moment.

### What is the difference?

1. Gophers. Yes, there is [Hugo](http://gohugo.io/) (kudos!) but... Who wants to learn another directory layout? There is also [Hastie](https://github.com/mkaz/hastie) and [lots of other static site generators](https://jamstack.org/generators/).
2. Pure Markdown. And HTML, if you want.
3. Just a loop. Zas loops over the current directory (and subdirectories), converting .md and .html files and copying everything else as-is - except dot-files and dot-directories, which are ignored entirely.
4. Your imagination is your limit. Zas has a simple extension mechanism based on subcommands, plus optional native sitemaps and RSS feeds for article sites.
5. Unobtrusive structure, no `_` files. More in the [Usage section](#usage).

## Documentation

The [documentation source](docs/index.md) is published at [dario.cat/zas](https://dario.cat/zas/), with focused guides to configuration, templates, plugins and publishing. Hosting the guides there does not change the module path `github.com/darccio/zas`.

Docs changes on `master` and published releases request a pinned source update in the site repository. A reviewed site pull request must be merged before Cloudflare Pages deploys the update. Configure `SITE_DISPATCH_TOKEN` as described in [the dispatch workflow](.github/workflows/docs-dispatch.yml); without it, the workflow reports a notice and skips dispatch.

## Usage

Install:

```sh
go install github.com/darccio/zas/cmd/zas@latest
```

Go to your site's directory and do:

```sh
zas init
```

Zas will create a `.zas` directory with sane defaults, including a starter `.zas/layout.html` - replace it with your own whenever you like.

```sh
zas
```

Yes. Enough. Your delightful site is on .zas/deploy. Enjoy.

What is happening here? Well, Zas calls the `generate` subcommand by default. This subcommand accepts the following flags:

* `-verbose`: print ALL the things!
* `-full`: generate all the input files. By default, it has an incremental mode that keeps source and deploys directories in sync - it also picks up changes to `layout.html`, `config.yml`, `i18n.yml`, and any `.zas.yml` in a page's own directory tree, not just the page's own source, and it follows a page's (or `layout.html`'s own) `<embed src="...">` targets too, recursively through further `Markdown`/`Html` embeds. Two narrower gaps remain: an `<embed src="{{...}}">` whose `src` is itself a template action can't be resolved without running the page's own template, and an `mzs*` MIME type plugin's `src` file is tracked but whatever else the plugin reads isn't - use `-full` after editing either of those.

## Configuration and extension

Zas is like water. It can flow, or it can cr... Nah, Zas doesn't crash (please file an issue if it does).

Everything is configurable at .zas/config.yml. It is initialized with default values the first time you run `zas init`; running it again leaves an existing config.yml (and layout.html) alone unless you pass `-force`, which overwrites both with their defaults.

You can override the `site` config section in two ways:

1. HTML comment in files (most precedence).
2. `.zas.yml` file at the directory level. Its scope is its directory and subdirectories (until another `.zas.yml` is found).

By default, dot-files and dot-directories (anything whose name starts with `.`) are skipped entirely, at any depth - `.git`, editor swap files, and so on. A site that genuinely needs a specific top-level dot-directory published - `.well-known/`, for example, which browsers and ACME clients expect to find at a site's root - can opt it back in with `allowed_dotdirs` under the `zas` section:

```yaml
zas:
  allowed_dotdirs: [".well-known"]
```

Only an exact, top-level match is honored: no prefix or glob matching, and a dot-directory nested anywhere - including inside an allowed one - still gets skipped. `.zas` and `.git` can never be allowlisted this way, no matter what's listed in config.

Here's every key Zas itself understands in `config.yml`, together in one place (a real site's own file will typically be much shorter, since every one of these has a default and nothing here is required):

```yaml
zas:
  layout: .zas/layout.html      # default: .zas/layout.html
  deploy: .zas/deploy           # default: .zas/deploy
  allowed_dotdirs: [".well-known"] # default: none - see above
site:
  baseurl: https://example.com  # default: http://example.com - see {{.Site.BaseURL}} below
  language: en                  # default: en - see {{.Language}} and I18N below
  image: https://example.com/og-image.png # default: unset - see {{.Site.Image}} below
  sitemap: true                 # default: false - see "Sitemap generation" below
  page_listings: true           # default: false - expose static page lists to templates
  feed: true                    # default: false - see "RSS feeds" below
  feed_title: My articles       # default: site.title; required when feeds are enabled
  feed_description: New writing # default: site.description, then feed title
  feed_sections: [posts]        # default: all directories, with conservative article selection
  feed_limit: 50                # default: 50; positive integer, applied independently per feed
  feed_per_language: true       # default: false; discover non-default article languages
  feed_strict: true             # default: true; invalid selected articles fail generation
mimetypes:
  text/markdown: markdown       # default
  text/plain: plain             # default
  text/html: html               # default
  text/yaml+myplugin: myplugin  # example custom MIME type plugin - see below
```

This is illustrative, not exhaustive of every key a *plugin* might read from its own section: plugins are free to define and read their own config (see "Beware" under Plugins below), and `config.yml` will happily carry whatever additional sections they need.

To extend Zas functionality, you can use and create plugins. You can develop them in any language (not only in Golang) thanks to Unix magic. And more gophers.

### Plugins

Any prefixed by `zs` or `mzs` is a potential Zas plugin. All plugins are Zas subcommands.

For example, we invoke an imaginary plugin called `zshello` as a subcommand:

```sh
$ zas hello
Hello!

$ zas hello World
Hello World!
```

That's all. Zas passes any command-line argument after subcommand name to `zshello`. (The same `zshello` binary is also reachable from page content - see the script tag mechanism below.)

**Beware:** Zas won't pass any configuration information. Plugins are responsible for reading configuration, even from directory and page levels. Helper libraries in different languages are welcome!

Also, plugins are free to use `.zas` directory for their own needs. I recommend creating this directory's structure to avoid colliding issues:

```text
.zas
+-- plugins
|   +-- github.com
|       +-- darccio
|           +-- myplugin
+-- ...    
```

Any `zs` plugin can also be invoked from page content through a script tag with type `application/zas+myplugin`:

```html
<script type="application/zas+myplugin" data-args="arg1 'arg two' arg3">
  whatever this tag's own content is
</script>
```

The tag is deleted and replaced by whatever the plugin writes to stdout, as HTML. `data-args` supplies argv, split with shell-like quoting - `'single'` and `"double"` quotes group an argument containing spaces, and a backslash escapes the next character - but nothing here is ever handed to an actual shell, so none of `$`, `` ` ``, `*`, `~`, `#`, `|`, `;`, `&`, `<`, `>` are special; they reach the plugin literally.

Unlike an `<embed>`, this tag's own inner content isn't parsed as HTML at all: `<script>` is one of the few HTML elements whose content is raw text, so whatever you write between the tags - a JSON object, a CSV table, a block of YAML, anything - survives byte for byte and is piped to the plugin's stdin exactly as written, entities and all. That's the point of reaching for a script tag instead of `embed`/`mzs*`: it lets you write inline data a plugin turns into HTML, rather than only ever pointing at a separate file.

A few things to know:

- **No `async` yet.** Every zas script tag runs synchronously, in document order, one at a time. The tag accepts an `async` attribute, but it's currently ignored.
- **Placement matters.** In a page's own content, the tag must resolve somewhere in the eventual `<body>` - a leading config comment does *not* stop the parser from placing a script written as the very first thing in a file into `<head>` instead, and Zas will refuse to guess what you meant, failing the build with a clear error instead. Inside `layout.html` specifically, a tag in `<head>` is allowed, but only for output that's actually valid there (`<meta>`, `<link>`, `<base>`, `<style>`, `<title>`) - handy for a plugin that injects per-build metadata into every page's head. Anything else placed in `layout.html`'s `<head>` also fails the build rather than silently vanishing.
- **Not re-scanned.** A plugin's own output isn't searched for further zas script tags in the same pass - except that a page-body tag's output does get one more look, since the whole assembled page is parsed again to merge it with the layout (see below). A tag written directly into `layout.html` gets no such second pass.
- Only tags whose `type` starts with `application/zas+` are ever touched. Ordinary JavaScript, `application/ld+json`, or any other `<script>` - anywhere, including inside `layout.html` - is left completely alone.

#### What's the deal with "mzs" prefix? (a.k.a. MIME types plugins)

These are MIME type plugins. Zas uses embed tags to allow easy integration beyond command line. Any MIME type can be configured in `.zas/config.yml` under mimetypes section.

```yaml
mimetypes:
  text/markdown: markdown
  text/yaml+myplugin: myplugin
```

If Zas finds an embed tag with a type attribute set to `text/yaml+myplugin`, it will invoke `mzsmyplugin`. Zas expects to process the plugin's stdout as HTML. It also pipes stderr to the user's shell. Any plugin will be called with the embed's `src` attribute as its only argument, resolved relative to the site's root rather than to the file containing the `<embed>` tag.

```html
<embed src="navigation.md" type="text/markdown" />
```

Maybe you are asking yourself: "Where is mzsmarkdown?". Nowhere! It is a particular case where Zas has a built-in handler for it. I wanted to allow anyone to override internal Markdown processing if they wish.

If you develop a new plugin, please contact me, and I will list it here :) Please, keep in mind: make it [idempotent](http://en.wikipedia.org/wiki/Idempotence).

#### Plugin trust model

All plugin mechanisms resolve a name to a binary on `PATH` and execute it - Zas does no sandboxing, signing, or verification of what it finds there. That's a deliberate design, in the same spirit as how `git <subcommand>` resolves to `git-<subcommand>` on `PATH`, but it's worth being explicit about the three different ways a plugin name gets chosen, since they carry different levels of trust:

- **`zas <name>` subcommands** are only ever invoked from a name you (or a script you wrote) typed directly as a command-line argument - the same trust level as running any other program by name in your shell.
- **`mzs*` MIME type plugins** are chosen by `mimetypes:` config and triggered by `<embed type="...">` tags found in site *content*. If you ever run `zas generate` over content you don't fully control - a preview build from an external contribution, for example - that content effectively gets to pick which already-installed plugin binary runs, with the embed's `src` as an argument.
- **`zs*` script-tag plugins** go further still: a `<script type="application/zas+name">` tag in page content picks the exact same `zs<name>` binary the command line would, *and* supplies both its arguments (`data-args`) and its stdin (the tag's own content). Note this means the `zs<name>` binaries themselves are no longer reachable only from something you typed yourself - content can name one directly.

Every plugin name - from `mimetypes:` config or from a script tag's `type` - is validated as a plain `[a-zA-Z0-9_-]+` string before anything is executed, so content can't smuggle in a path (`../../something`) to make `exec.Command` skip `PATH` lookup entirely.

If you run `zas generate` over content you don't fully control, pass `-no-plugins`: any embed needing an external MIME type plugin, or any script tag naming one, fails with a clear error instead of executing anything. This does not cover the `zas <name>` command line itself, which is never content-triggered. Zas's own built-in embed handlers (like `Markdown`) aren't affected either - they never spawn a process.

### Sitemap generation

Zas can generate a standards-compliant XML sitemap (and keep your `robots.txt` pointing at it) as a native part of `zas generate` - no plugin involved, since building one needs the complete deploy set, the site's base URL, and every page's resolved language, none of which a plugin has access to (see "Beware" above).

Turn it on with `site.sitemap: true` in `config.yml` (default `false`); it also needs a real `site.baseurl` (not the `http://example.com` placeholder `zas init` scaffolds), since every `<loc>` is built from it:

```yaml
site:
  baseurl: https://example.com
  sitemap: true
```

With that set, every `zas generate` run writes `sitemap.xml` at the deploy root, containing one `<loc>` per deployed page plus an accurate `<lastmod>` - deliberately nothing else. `<changefreq>` and `<priority>` are never emitted: Google and Bing both ignore them as of this writing, so they'd only add file size for no benefit. Past roughly 45,000 pages (well under the sitemaps.org 50,000-URL/50MB hard limits), Zas automatically switches to a `sitemap-index.xml` plus numbered `sitemap-1.xml`, `sitemap-2.xml`, ... shards instead of one file - nothing to configure for that either.

**`<lastmod>` accuracy.** Zas derives each page's `<lastmod>` from its source file's most recent git commit date when the site lives inside a git working tree, falling back to the source file's own mtime when git isn't available (or the file isn't tracked), and omitting `<lastmod>` entirely for that page only if neither is available - a wrong or fabricated date is worse than none, since both Google and Bing stop trusting a sitemap's `<lastmod>` values once they look unreliable. If you build in CI, make sure your checkout has full history (e.g. `fetch-depth: 0` on `actions/checkout` - the shallow default only sees the checkout's own commit, which would otherwise look like every page's "true" last-modified date).

**Multilingual sites.** A site using the language-subdirectory i18n convention described below (see "你会说普通话?") gets reciprocal `hreflang` annotations for free: pages that share the same relative path across language directories (`es/faq.md`, `ca/faq.md`, root `faq.md`, ...) are grouped and cross-linked automatically, with no extra configuration beyond what I18N already requires. A page not grouped that way - one whose language comes only from its own leading-comment override, with no directory-level grouping - doesn't get hreflang treatment; partial translation coverage (a page with no sibling translation yet) is never an error, it simply ships without hreflang links of its own.

**`robots.txt`.** When sitemap generation is on, Zas makes sure deploy's `robots.txt` declares a `Sitemap:` directive pointing at whatever was generated (the plain sitemap, or the index once sharded): if your own `robots.txt` reaches deploy (it's just another file, copied like any other unrecognized extension), Zas appends the directive to it if it's missing; if your site has no `robots.txt` at all, Zas writes a minimal permissive one (`User-agent: *` / `Allow: /`) so the sitemap stays discoverable with zero extra setup.

## Page listings

Enable `site.page_listings: true` to use `{{.Pages "posts"}}` in a page's
HTML or Markdown template, or in the layout. Without this option, Zas does
no listing scan and existing output is unchanged; calling `.Pages` reports
the required option.

For example, an archive page can use a literal leading configuration
comment and heading, followed by:

```html
<!-- title: Posts -->
<h1>Posts</h1>
<ul>
{{range .Pages "posts"}}
  <li><a href="{{html .URL}}">{{html .Title}}</a>{{if not .Date.IsZero}} — {{.Date.Format "2006-01-02"}}{{end}}</li>
{{end}}
</ul>
```

Page content uses `text/template`, so the `html` builtin explicitly escapes
plain strings placed into HTML. In the layout, `html/template` escapes
`.URL`, `.Title` and `.Summary` automatically; use those fields directly.

`.Pages(section)` returns `([]PageSummary, error)`. Each summary has exported
`Title`, `URL`, `Canonical`, `Date`, `Language` and `Summary` fields. `Date`
is a `time.Time`; the other fields are plain strings. `URL` is an escaped,
root-relative navigation path, including any `site.baseurl` hosting subpath,
with `/index.html` normalized to `/`, so links stay on the current preview
host. `Canonical` uses the same path and
the existing `.Canonical` helper's `site.baseurl` prefix, or remains
root-relative when that prefix is empty.

A section selects all descendants at directory boundaries: `posts` includes
`posts/older.md` and `posts/slug/index.md`, excludes `posts/index.md`, and
does not match `posts-old`. Only the requested section's own index is
omitted. `""` and `"."` select the site root; a missing section returns an
empty list. Use relative directory paths, optionally ending in `/`.
Absolute paths, traversal, backslashes, URL queries/fragments and encoded
path syntax are rejected when the template calls `.Pages`. As with other
template errors, a failing full build may already have cleared deploy;
use a staging directory when published output must survive a failed build.

The list is a complete, immutable snapshot of static Markdown and HTML
sources, made once before rendering. Hidden sources, symlinks, the layout,
deploy files, `publish: false` and `draft: true` are excluded. Allowed
top-level dot-directories follow the normal source rules. Draft exclusion
affects the list; ordinary HTML publication keeps its existing behavior.
Metadata uses the literal leading YAML comment: `title` falls back to the
first static H1, `date` and `summary` use their existing helpers, and
language resolves from page, nearest directory config, then site. Missing
titles/summaries/languages are empty. Missing dates stay zero and sort last;
known dates sort newest first, with URL breaking equal-date ties.

The scan never executes templates, embeds or plugins. Listed metadata must
be valid static values; template actions in metadata are rejected unless
the page has `template: false`. A template-derived H1 produces an empty
listing title: provide a literal `title` instead. A leading template action
that could generate the configuration comment is rejected; put a literal
configuration comment or heading before body actions. Invalid static
metadata fails before full-build cleanup. Generated publication flags that
disagree with the snapshot fail the build when rendering reveals them.
Every HTML/Markdown page renders on each enabled run so additions, removals
and metadata edits update every archive.
Newly unpublished pages also lose their stale standalone HTML in incremental
builds. This opt-in favors consistent listings over incremental render speed.

## RSS feeds

Set `site.feed: true` to generate a native [RSS 2.0](https://www.rssboard.org/rss-specification) feed at `/feed.xml`. This uses existing dependencies. Feeds are disabled by default; an untouched disabled site produces the same page bytes and no feed files or ownership manifest. Unused feed options are ignored while disabled.

```yaml
site:
  baseurl: https://example.com
  language: mul
  feed: true
  feed_title: My articles
  feed_description: New writing in English and Spanish.
  feed_sections: [posts]
  feed_limit: 50
  feed_per_language: true
  feed_strict: true
```

`feed_sections` is a list of relative article directories, including descendants; `.` selects the whole site. Within configured sections, non-archive Markdown and HTML pages are articles and must have an explicit `date`. An index directly at a section root, such as `posts/index.md`, is an archive; a nested article such as `posts/slug/index.md` is included. Utility pages in an article section can set `article: false`. With sections omitted or empty, Zas selects pages with a `date` property or `article: true`, excluding the unclassified root homepage index. An explicit boolean `article` always overrides this selection; `article: true` also selects an otherwise excluded archive and requires a date. `publish: false`, `draft: true`, and `feed: false` exclude a page before metadata validation.

Each article can use the shared metadata helpers' properties:

```yaml
<!--
date: 2024-11-27T22:22:00+01:00
updated: 2025-01-02T03:04:05Z
summary: A short plain-text summary.
tags: [writing, software]
author: Dario
language: es
-->
```

Dates must be RFC3339 timestamps with a timezone. No publication date is inferred from git, file timestamps or generation time. Feed titles come from the page title or first H1. Summaries and tags use `Summary()` and `Tags()` without legacy-property fallbacks. Creator resolves `author` from page, directory and site config, then falls back to the feed title. Language follows the same page/directory/site resolution as `Language()` and must be a path-safe language tag such as `en`, `es`, or `pt-BR`.

Items contain the full rendered article body in `content:encoded` CDATA, after the article's templates, embeds and plugins, before the layout. Layout navigation/footer and the leading config comment are absent. Relative `href`, `src` and `srcset` URLs resolve against the article's canonical URL; non-HTTP schemes and srcset descriptors are preserved. Plain resolved summaries are escaped for RSS readers' HTML interpretation. Items include canonical permalink GUIDs, RFC1123Z publication dates, categories, `dc:creator`, `dc:language`, and `xml:lang`. RSS core elements have no namespace; Dublin Core terms use the standard [elements/1.1 namespace](https://www.dublincore.org/specifications/dublin-core/dcmi-terms/).

Articles are sorted by publication instant, newest first, with canonical URL as a tie-breaker. `feed_limit` applies to each feed after language filtering. `feed_per_language: true` discovers every non-default resolved article language, creating `/en/feed.xml`, `/es/feed.xml`, etc. English and Spanish feed titles receive `(English)` and `(Español)` suffixes; other languages use their tag. The combined feed includes every language. Each channel has an Atom self link. `lastBuildDate` is the maximum `Updated()` among its emitted items: explicit `updated`, otherwise the last commit of a currently tracked source. It is omitted when unavailable, without mtime or generation-time fallback.

Incremental runs collect unchanged articles through the same render pass, so feeds retain their full content and templates/plugins execute once per page per run. A plugin that emits another Zas script or embed directive requires the layout's second processing pass; RSS rejects that residual directive rather than rerunning it. Use `feed: false` for such a page, or produce its complete body in the first pass. `feed_strict: true` makes invalid selected metadata or residual directives stop generation before feed finalization; an incremental failure keeps the prior feeds and ownership intact. `false` logs and skips invalid articles. Duplicate canonical GUIDs, invalid XML, unsafe output paths, and collisions still fail generation. Full generation retains its existing clear-on-start behavior; use a staging directory when the prior published output must survive any render failure.

Zas records feed paths, content hashes and the absolute deploy directory in private `.zas/feed-outputs.json`, outside deploy. Existing unmanaged feed outputs and source `feed.xml` collisions are rejected before full cleanup. Per-language feeds reserve language-tag `/feed.xml` source paths; known article-language ancestor collisions are also checked before cleanup. A language supplied dynamically by a template receives its final collision check after rendering; `-full` still follows its existing clear-on-start behavior in that case, so build into staging before publication. Each feed file is replaced atomically and completed writes are recorded individually, allowing retries after a later write fails. A failed ownership write restores the previous feed bytes or absence.

Turning feeds off, disabling language feeds or removing a language's articles removes only recorded, hash-matching outputs. A real source file copied over a former feed is retained and ownership is relinquished. Edited managed feeds cause an error; move them aside before rebuilding. Keep the manifest with the build state, and remove old managed feeds plus their manifest before changing `zas.deploy`. Losing the manifest intentionally relinquishes ownership: existing feeds must then be moved aside before re-enabling generation.

## Building sites

Your site layout will look like this:

```sh
$ ls
$
```

Just kidding. A site would be:

```sh
$ ls -laR
total 8
drwxr-xr-x   5 Dario  staff   170 30 mar 16:18 .
drwxr-xr-x   6 Dario  staff   204 30 mar 13:17 ..
drwxr-xr-x  13 Dario  staff   442 27 mar 20:05 .git
drwxr-xr-x   3 Dario  staff   102 30 mar 13:18 .zas
-rw-r--r--   1 Dario  staff   941 30 mar 16:19 about.md
-rw-r--r--@  1 Dario  staff  1645 30 mar 15:31 index.md
drwxr-xr-x   4 Dario  staff   136 30 mar 16:20 section
    
# [...]
    
./.zas:
total 0
drwxr-xr-x  4 Dario  staff   136 30 mar 16:22 .
drwxr-xr-x  7 Dario  staff   238 30 mar 16:19 ..
-rw-r--r--  1 Dario  staff    29 30 mar 13:18 config.yml
-rw-r--r--  1 Dario  staff  2438 30 mar 16:22 layout.html
    
./section:
total 0
drwxr-xr-x  4 Dario  staff  136 30 mar 16:20 .
drwxr-xr-x  7 Dario  staff  238 30 mar 16:19 ..
-rw-r--r--  1 Dario  staff  718 30 mar 16:19 index.md
-rw-r--r--  1 Dario  staff  991 30 mar 16:20 more.md
```

All .md files will be converted to HTML and copied in `.zas/deploy` using `.zas/layout.html` as layout and copying any other files and their structure. The former is also true for HTML files.

Markdown is parsed as [GitHub Flavored Markdown](https://github.github.com/gfm/) (tables, strikethrough, task lists, autolinks) plus footnotes, on top of CommonMark. No configuration needed; it's always on.

Fenced and indented code blocks are rendered as `<pre><code>`, with a fence's info string (e.g. ` ```go `) becoming a `class="language-go"` on the `<code>` element. There is no syntax highlighting built in; style or highlight that class yourself if you want one.

Keep in mind that any file will be treated as a Go text template before any further processing, **including the contents of code blocks**: `{{...}}` inside a fenced or indented block is executed as a template, not shown literally. To display literal double braces, write `{{"{{"}}`. You have access to these fields and methods from anywhere - a page's own content and `layout.html` alike - though `{{.Body}}`, `{{.Title}}`, `{{.Page}}`, and `{{.FirstTitle}}` behave slightly differently depending on which one you use them from; see each below.

* `{{.Body}}`: the file's own rendered content in HTML. This is only ever set once the page's own template has finished executing, so it's `layout.html` that receives it - used from inside a page's own content, `{{.Body}}` always evaluates empty, since a page can't contain a preview of its own not-yet-finished render.
* `{{.Title}}`: autodetected title (first H1 header in file, see `{{.FirstTitle}}` below), overridden by `title` property in page's config (see `{{.Page}}` below). Used from inside a page's own content, this reads a best-effort preview of the same value, extracted from the page's own raw source ahead of its own templating; that preview is occasionally unavailable (falling back to empty, never to unexecuted `{{...}}` syntax) in edge cases `layout.html` doesn't have to worry about, since `layout.html` always sees the final, authoritative value instead.
* `{{.Path}}`: file's path (also valid as URL).
* `{{.Site.BaseURL}}`: URL where this site will be deployed, e.g. http://example.com (without final slash).
* `{{.Site.Image}}`: URL to main image. Useful for Open Graph and Twitter meta tags.
* `{{.Page}}`: YAML map from first HTML comment (in Markdown and HTML files). It is optional. Used from inside a page's own content, this is likewise a best-effort preview parsed from that same leading comment ahead of the page's own templating; `layout.html` always sees the authoritative value, parsed later from the fully rendered page.
* `{{.FirstTitle}}`: the file's first H1 header text, before any `title` override is applied - what `{{.Title}}` falls back to. Same best-effort preview behavior as `{{.Page}}` when used from inside a page's own content, with one more guard: if the H1 itself is written as `<h1>{{.Title}}</h1>`, the preview is deliberately left unavailable there instead of echoing the literal, unexecuted `{{.Title}}` text back into itself.
* `{{.Directory}}`: YAML map from above (up to project's directory) or current directory's `.zas.yml` file. It is optional.
* `{{.URL}}`: full URL for this file.
* `{{.Canonical}}`: full URL with a final `/index.html` replaced by `/`, so `section/index.md` and `section/index.html` both yield `/section/`. A standalone `article.md` still yields `/article.html`. This helper leaves `{{.URL}}`, `{{.Path}}` and sitemap URLs unchanged.
* `{{.Date}}`: publication date as Go's `time.Time`, from the explicit `date` page property only. Missing dates return zero time; Zas never infers a publication date.
* `{{.Updated}}`: Go's `time.Time` from the explicit `updated` page property, otherwise the last git commit's committer date for the original, currently tracked source file. An untracked file, unavailable git or missing history returns zero time; source mtimes and build time are never substituted. Use a full-history checkout in CI for reliable git dates.
* `{{.Summary}}`: the explicit `summary` page string, empty when absent.
* `{{.Tags}}`: the `tags` page list of strings in its original order, empty when absent.
* `{{.Extra "/path/"}}`: direct access to map holding `.zas/config.yml` as it is. You can access to any value with its full path. E.g. BaseURL is also available as `/site/baseurl`.
* `{{.Resolve id}}`: indirect access to site, directory and page config. It works with simple keys (no paths), checking for them in page, directory and site config (as `/site/<id>`), in this order.
* `{{.Language}}`: file current language, if defined in the first comment (as YAML property `language`). By default, `/site/language` value.
* `{{.E "Some key"}}`: translates a string for the page's resolved language (see I18N below), falling back to `**Some key**` when no translation is found. Takes optional `fmt.Sprintf`-style arguments: `{{.E "Hello, %s" .Name}}`.
* `{{.H "Some key"}}`: like `{{.E}}`, but the translation is marked as trusted HTML rather than plain text - see the escaping note right below for what that means and where it matters.

Metadata helpers read the same `{{.Page}}` view as other page methods: a best-effort preview in the page's own content, then the final rendered config in the layout. They validate values only when called, so adding them to Zas changes no existing output. Invalid explicit `date`, `updated`, `summary` or `tags` values stop template execution with a diagnostic; raw `{{.Page}}` access keeps its existing behavior. Dates must be RFC3339 timestamps with a timezone, quoted or unquoted in YAML. Day-only and timezone-free timestamps are rejected even when YAML would otherwise convert them to `time.Time`. The helpers do not read legacy `date_created`, `date_updated` or `description` properties.

For example, a page may start with:

```yaml
<!--
date: 2024-11-27T22:22:00+01:00
updated: "2025-01-02T03:04:05Z"
summary: A short summary.
tags: [writing, software]
-->
```

In `layout.html`, format a date after checking that it is present:

```html
{{if not .Date.IsZero}}<time datetime="{{.Date.Format "2006-01-02T15:04:05Z07:00"}}">{{.Date.Format "2006-01-02"}}</time>{{end}}
{{with .Summary}}<p>{{.}}</p>{{end}}
{{range .Tags}}<span>{{.}}</span>{{end}}
```

#### A page's own content has no escaping at all

A page's own content runs through Go's `text/template`, not `html/template` - this matters, and it's not an accident. Escaping-by-context (the thing `html/template` does) needs to understand the surrounding HTML structure at parse time, but a page's raw source usually isn't HTML yet when its template executes: it might be Markdown, and even an `.html` page is normally just a body fragment, not a full document. `text/template` sidesteps that by doing plain text substitution with no escaping whatsoever, which is also what lets a page inject real markup through a field or method - a translation containing a link, a config value that's meant to become an `<img>` tag - without a `noescape`-style helper. `{{.E}}` and `{{.H}}` above look distinct, but from inside a page they're identical: neither escapes anything, ever.

`layout.html`, by contrast, is a single, fixed template that only ever runs once, over the already-finished page - `html/template` fits there, so that's what it uses (see "What about layout.html?" below), with `noescape`/`{{.H}}` as the deliberate opt-out. The result is that the exact same expression behaves completely differently depending on where you write it: `{{.E "greeting"}}` auto-escapes in `layout.html` but not inside a page's own Markdown or HTML.

None of this matters if you're the only one writing your site's content and config - a static site generator has no runtime attacker separate from its own author. It matters if you ever generate from content you don't fully control - an external contribution, a value pulled from somewhere you don't trust - since anything reaching a page through `{{.Extra}}`, `{{.Resolve}}`, `{{.E}}`/`{{.H}}`, or any other field lands in deployed output completely unescaped, with no equivalent of `layout.html`'s protection available inside a page.

If a file's own content needs to contain literal `{{...}}` - a Vue/Angular/Handlebars snippet, Go template documentation, or a code sample showing off Zas's own template syntax - set `template: false` in its config comment instead of escaping every brace:

```markdown
<!--
title: Templating in Zas
template: false
-->
Zas actions look like `{{.Title}}`, and this whole page is written to
show them off, so it opts out of being templated itself.
```

The file is still fully parsed and processed otherwise (HTML5 parsing, `<embed>`, and its own config comment - `title` above still works), only its content is never run through `text/template`. Defaults to `true` (templated), so existing files are unaffected.

There is also a page config property that isn't exposed as a template field, since it steers generation itself rather than the page's content:

* `publish`: set to `false` in a file's config comment to keep that file out of `.zas/deploy` as a standalone page, while it stays fully available to be pulled into another page via `<embed>`. Defaults to `true` (published), so existing files are unaffected.

### Optional Markdown output

To publish a rendered Markdown body beside each Markdown page's HTML, opt in
in `.zas/config.yml`:

```yaml
site:
  publish_markdown: true
```

`section/index.md` produces `section/index.html` and `section/index.md` in
deploy; `contact.md` produces `contact.html` and `contact.md`. Source extension
matching is case insensitive: `section/INDEX.MD` produces `section/INDEX.html`
and `section/INDEX.md`. Sources are never changed, and existing HTML URLs,
`.Path`, `.URL` and `.Canonical` retain their meanings. HTML sources do not
get Markdown conversions. This setting does not add HTTP content negotiation.
The default is off: sites that have never enabled it retain the existing
rendering path and gain no ownership state or extra output files.

For eligible Markdown pages, Zas executes the page's Go template once on the
Markdown source, before converting it to HTML. The resulting Markdown body
feeds both outputs. Markdown syntax and code fences survive in the twin;
template values that contain Markdown now participate in Markdown parsing.
This is an opt-in ordering change from the legacy conversion-before-template
path. Static Markdown H1 headings and explicit `title` metadata remain
available to `.Title`/`.FirstTitle` during execution; the HTML layout receives
the final rendered metadata and body as usual. The leading YAML configuration
comment is removed from twins, including embedded files' leading comments.
Twins contain the body only: no automatic serialization of `.Page`,
`.Directory`, `.Extra`, or other metadata maps, and no layout chrome.

`publish: false`, `draft: true`, and page-level `publish_markdown: false`
suppress a twin. A draft still produces its existing HTML page; this feature
does not unpublish live drafts. An explicit page opt-out keeps the legacy
HTML rendering path, including its normal plugin behavior:

```markdown
<!--
publish_markdown: false
-->
# HTML-only page
```

Literal Go template documentation requires `template: false`; in that case
literal `{{...}}` is intentionally preserved as public content in both
formats. On templated pages, rendered output containing unexecuted Go action
delimiters is rejected rather than being published or executed a second time.
As with ordinary page templates, authors choose which values to insert into
the body and must avoid inserting private configuration themselves.

Built-in Markdown embeds expand into Markdown, and must occupy a standalone
line. Reference definitions or footnotes combined with Markdown embeds are
rejected because merging documents would change their separate reference
scopes; use an HTML embed or page opt-out for those pages. HTML embeds execute
their own template once and supply their body as raw
HTML. Plain embeds execute once and escape Markdown punctuation as entities
so their text retains its literal meaning. Nested built-in embeds resolve
relative to the containing file, enforce site-root containment (including
symlinks), and retain the existing nesting bound. Embeds and script examples
inside Markdown code are literal. External MIME embeds and content script
plugins are unsupported in twins: Zas fails with page opt-out guidance before
invoking them. Layout plugins still follow the ordinary HTML-only layout
pass; twins do not include the layout. `-no-plugins` continues to govern
every content-triggered external plugin in the HTML path.

Output claims and source/deploy overlap are checked before deploy mutation,
even for unchanged sources. Use the conventional `.zas/deploy`, or a new or
empty destination. An existing source directory, `.zas/partials`, the site
root, a parent of it, or `.git` cannot be repurposed as deploy. A previous
matching ownership manifest identifies an existing opt-in destination.
An existing twin without a matching ownership hash is a collision; move it
or opt its page out before rebuilding. Deploy parent symlinks cannot redirect
outputs outside the selected deploy directory.

Zas stores output paths and hashes in the private
`.zas/markdown-outputs.json`, with the resolved deploy identity. Incremental
builds recreate missing twins and clean previous owned outputs when a source
is deleted, renamed, unpublished, marked draft, or opted out. Disabling
`site.publish_markdown` removes owned twins on the next successful build and
removes the manifest; HTML pages remain. Incremental cleanup deletes only
files whose hashes still match ownership, preserving manual edits and
unowned Markdown files. It may leave empty deploy directories. Ownership
from another deploy directory is never used for cleanup. Ordinary incremental
asset cleanup remains unchanged. `-full` retains its existing behavior of
clearing the entire selected deploy directory after preflight and rebuilding
it, including ordinary assets/HTML and removal of stale directories.
If a render fails after other twins were written, Zas records those successful
outputs' ownership privately without deleting stale outputs, so a corrected
build can safely retry.

### What about layout.html?

It is plain HTML. No frills. Just add a placeholder `{{.Body}}` in your template.

First header level 1 from Markdown files will be made available as `{{.Title}}`, unless it is overridden.

`layout.html` is parsed with Go's `html/template`, which auto-escapes values by default - unlike a page's own content, which has none at all (see "A page's own content has no escaping at all" above). A `noescape` helper is available if you need to output a string as trusted, unescaped HTML - e.g. `{{noescape .SomeTrustedHTML}}`. Only use it on content you trust: passing it anything that could contain attacker-controlled input (a value from user-submitted content, an untrusted third-party feed, etc.) reintroduces the XSS risk `html/template` exists to prevent. If you don't need it, don't use it.

#### JSON in layouts

Three layout-only helpers build and serialize structured data:

- `dict "key" value ...` creates an object. Keys must be strings and appear once; an odd number of arguments is an error. `dict` with no arguments produces `{}`.
- `list value ...` creates an array, including `[]` when called without arguments.
- `json value` serializes a Go value with `encoding/json.Marshal`. Unsupported values, invalid custom marshalers and cyclic values stop template execution with an error.

Use the helpers together to emit a JSON-LD data block:

```html
{{$person := dict "@type" "Person" "name" "Dario Castañé" "url" .Site.BaseURL}}
<script type="application/ld+json">{{json (dict "@context" "https://schema.org" "@graph" (list $person))}}</script>
```

`json` marks only its freshly serialized output as a JSON expression for `html/template`, so the block contains an object rather than a quoted JSON document. Ordinary and pre-encoded strings remain JSON strings; construct objects with `dict` instead of passing raw JSON text. The serializer retains its default escaping of `<`, `>`, `&`, U+2028 and U+2029, including custom `json.Marshaler` and `json.RawMessage` values, so data cannot close the script element. Go types with custom JSON behavior remain responsible for their data's meaning.

Use `json` for a complete JSON expression, outside any JavaScript quotes. HTML text and attribute contexts still receive `html/template`'s contextual escaping; the helper is not an HTML bypass. It is unavailable in a page's own `text/template` content and emits nothing unless a layout calls it. No JSON-LD schema or metadata is added automatically.

### But... I want to do pages beyond post-like format

No problem! Just use our old friend `<embed>`. Imagine `<layout>` is a valid tag.

```html
<layout>
  <nav>
    <embed src="navigation.md" type="text/markdown" />
  </nav>
  <article>
    <embed src="section/index.md" type="text/markdown" />
  </article>
</layout>
```

What does it mean? It means you can have .html files with embedded markdown files. Or anything else supported by Zas.

`navigation.md` here probably isn't meant to be its own page, only content embedded into others - so mark it with `publish: false` in its own config comment:

```markdown
<!-- publish: false -->
* [Home](/)
* [About](/about.html)
```

Zas still reads and processes `navigation.md` normally wherever it's embedded, but it won't also show up on its own at `/navigation.html` in the deploy output.

An `<embed>`'s `src` written inside a page's own body resolves relative to that page's own directory - the same way a relative `<img src>`/`<a href>` would once the page is deployed and viewed in a browser. So a page at `section/index.html` embedding `<embed src="navigation.md" ...>` looks for `section/navigation.md`, not a `navigation.md` at the site root, even if one happens to exist there too. A leading `../` is allowed as long as it still lands inside the site: `<embed src="../shared.md" ...>` from `section/index.html` reaches a `shared.md` at the site root, but nothing can be made to resolve outside the site root itself, however many `../` it uses.

`<embed>` also works directly inside `layout.html` itself, not just inside a page's own body - useful for something every page shares, like a site-wide footer. This case is different: `layout.html` is a single, fixed file shared by every page rather than page content that lives in a particular directory, so an embed written there always resolves relative to the site root, regardless of which page is currently being rendered.

Note that this is specific to Zas's own built-in embed handlers (`Markdown`, `Plain`, `Html` - the ones `mimetypes:` maps a `text/*` type to by default). An `mzs*` MIME type plugin (see "MIME type plugins" above) still receives its `src` argument exactly as written in `<embed src="...">`, with no resolution applied at all - the plugin decides for itself how to interpret it.

#### A note on output vs. source

Every page goes through Zas's HTML5 parser twice: once on its own, to extract its body and settings, and once more after the layout wraps it, so any `<embed>` in the layout itself gets its turn too. Both passes re-serialize what they parse, and HTML5's parser is lenient by design - it repairs markup as it goes rather than rejecting it - so deployed output can differ mechanically from what you wrote: attributes get quoted, tag names get lowercased, void elements like `<img>`/`<br>` get self-closed, stray `&` characters get entity-escaped. Nothing is lost, and this is also why the embed mechanism above can splice arbitrary snippets together reliably - but don't expect deployed HTML to be a byte-for-byte copy of your source.

One consequence of that first, page-only parse: HTML5 places a `<script>`, `<meta>`, `<link>`, `<base>`, `<style>`, or `<title>` written before any other real content into `<head>` rather than `<body>` - and a leading `<!-- key: value -->` config comment doesn't change that. Since only a page's `<body>` carries over into deployed output, such a tag would otherwise vanish silently; Zas instead fails the build for that page and names the tag. Put it after the page's first real content (even just an `<h1>`) and it renders exactly as written.

## 你会说普通话?

對不起。我不会说普通话。That's all my Chinese! If you are here, I guess you will enjoy I18N support in Zas.

### I18N?

Yeah, internationalization: you can build multilingual sites with Zas!

You only need to do three simple steps. First, create an i18n.yml file inside your .zas directory, like this:

```yaml
Main page:
  zh: 首页
  ru: Заглавная страница 
  es: Portada
  ca: Portada
Create account:
  zh: 创建账户
  ru: Создать учётную запись
  es: Crear una cuenta
  ca: Crea un compte
Log in:
  zh: 登录
  ru: Войти
  es: Acceder
  ca: Inicia la sessió
```

Set your site's main language in `.zas/config.yml`:

```yaml
site:
  language: en
```

Also, set each file's language in first comment or, if you have lots of files, as a `.zas.yml` in a subdirectory where to group them.

```sh
.zas/
index.md
faq.md
+-- zh
    +-- .zas.yml
    +-- index.md
    +-- faq.md
+-- ru
    +-- .zas.yml
    +-- index.md
    +-- faq.md
+-- es
    +-- .zas.yml
    +-- index.md
    +-- faq.md
+-- ca
    +-- .zas.yml
    +-- index.md
    +-- faq.md
```

Your `.zas.yml` will look like this, i.e. for Russian (ru):

```yaml
language: ru
```

Then use `{{.E "Main page"}}` anywhere - a page's own content or `layout.html` - to get the translated string for that page's resolved language.

## Roadmap

There is no roadmap. I wrote some possible enhancements [here](https://github.com/darccio/zas/issues?q=is%3Aissue+is%3Aopen+label%3Aenhancement).

Feel free to open an issue if you think Zas should do something specific in its core.

## Contact me

If I can help you, you have an idea or you are using Zas in your projects, don't hesitate to drop me a line (or a pull request): [my website](https://dario.cat/), [Bluesky](https://bsky.app/profile/dario.cat), or [Mastodon](https://mastodont.cat/@dario).

## About

Written by [Dario Castañé](https://dario.cat).

## License

Zas is under [AGPL v3](http://www.gnu.org/licenses/agpl-3.0.html) license.

## Other cool projects

Recently I found this cool generator inspired by zas: [zs](https://github.com/zserge/zs). I'm happy to be a humble reference for somebody :)
