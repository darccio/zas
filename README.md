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
4. Your imagination is your limit. Zas has a simple extension mechanism based on subcommands. Do you need to handle a blog with Zas? Install/create a new extension and do it!
5. Unobtrusive structure, no `_` files. More in the [Usage section](#usage).

## Usage

Install:

```sh
go install github.com/darccio/zas/cmd/zas@latest
```

Go to your site's directory and do:

```sh
zas init
```

Zas will create a `.zas` directory with sane defaults. Put your layout in `.zas/layout.html`, and you are good to go.

```sh
zas
```

Yes. Enough. Your delightful site is on .zas/deploy. Enjoy.

What is happening here? Well, Zas calls the `generate` subcommand by default. This subcommand accepts the following flags:

* `-verbose`: print ALL the things!
* `-full`: generate all the input files. By default, it has an incremental mode that keeps source and deploys directories in sync - it also picks up changes to `layout.html`, `config.yml`, `i18n.yml`, and any `.zas.yml` in a page's own directory tree, not just the page's own source. One gap: a page pulling in another file via `<embed>` is not regenerated when only the embedded file changes - use `-full` after editing an embedded file.

## Configuration and extension

Zas is like water. It can flow, or it can cr... Nah, Zas doesn't crash (please file an issue if it does).

Everything is configurable at .zas/config.yml. It is initialized with default values every time you create a repository. Beware, it happens every time you execute init.

You can override the `site` config section in two ways:

1. HTML comment in files (most precedence).
2. `.zas.yml` file at the directory level. Its scope is its directory and subdirectories (until another `.zas.yml` is found).

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

Maybe you are asking yourself: "Where is mzsmarkdown?". Nowhere! It is a particular case where Zas calls an exported method Markdown. I wanted to allow anyone to override internal Markdown processing if they wish.

If you develop a new plugin, please contact me, and I will list it here :) Please, keep in mind: make it [idempotent](http://en.wikipedia.org/wiki/Idempotence).

#### Plugin trust model

All plugin mechanisms resolve a name to a binary on `PATH` and execute it - Zas does no sandboxing, signing, or verification of what it finds there. That's a deliberate design, in the same spirit as how `git <subcommand>` resolves to `git-<subcommand>` on `PATH`, but it's worth being explicit about the three different ways a plugin name gets chosen, since they carry different levels of trust:

- **`zas <name>` subcommands** are only ever invoked from a name you (or a script you wrote) typed directly as a command-line argument - the same trust level as running any other program by name in your shell.
- **`mzs*` MIME type plugins** are chosen by `mimetypes:` config and triggered by `<embed type="...">` tags found in site *content*. If you ever run `zas generate` over content you don't fully control - a preview build from an external contribution, for example - that content effectively gets to pick which already-installed plugin binary runs, with the embed's `src` as an argument.
- **`zs*` script-tag plugins** go further still: a `<script type="application/zas+name">` tag in page content picks the exact same `zs<name>` binary the command line would, *and* supplies both its arguments (`data-args`) and its stdin (the tag's own content). Note this means the `zs<name>` binaries themselves are no longer reachable only from something you typed yourself - content can name one directly.

Every plugin name - from `mimetypes:` config or from a script tag's `type` - is validated as a plain `[a-zA-Z0-9_-]+` string before anything is executed, so content can't smuggle in a path (`../../something`) to make `exec.Command` skip `PATH` lookup entirely.

If you run `zas generate` over content you don't fully control, pass `-no-plugins`: any embed needing an external MIME type plugin, or any script tag naming one, fails with a clear error instead of executing anything. This does not cover the `zas <name>` command line itself, which is never content-triggered. Zas's own built-in embed handlers (like `Markdown`) aren't affected either - they never spawn a process.

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
* `{{.Extra "/path/"}}`: direct access to map holding `.zas/config.yml` as it is. You can access to any value with its full path. E.g. BaseURL is also available as `/site/baseurl`.
* `{{.Resolve id}}`: indirect access to site, directory and page config. It works with simple keys (no paths), checking for them in page, directory and site config (as `/site/<id>`), in this order.
* `{{.Language}}`: file current language, if defined in the first comment (as YAML property `language`). By default, `/site/language` value.
* `{{.E "Some key"}}`: translates a string for the page's resolved language (see I18N below), falling back to `**Some key**` when no translation is found. Takes optional `fmt.Sprintf`-style arguments: `{{.E "Hello, %s" .Name}}`.
* `{{.H "Some key"}}`: like `{{.E}}`, but the translation is marked as trusted HTML rather than plain text - see the escaping note right below for what that means and where it matters.

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

### What about layout.html?

It is plain HTML. No frills. Just add a placeholder `{{.Body}}` in your template.

First header level 1 from Markdown files will be made available as `{{.Title}}`, unless it is overridden.

`layout.html` is parsed with Go's `html/template`, which auto-escapes values by default - unlike a page's own content, which has none at all (see "A page's own content has no escaping at all" above). A `noescape` helper is available if you need to output a string as trusted, unescaped HTML - e.g. `{{noescape .SomeTrustedHTML}}`. Only use it on content you trust: passing it anything that could contain attacker-controlled input (a value from user-submitted content, an untrusted third-party feed, etc.) reintroduces the XSS risk `html/template` exists to prevent. If you don't need it, don't use it.

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

`<embed>` also works directly inside `layout.html` itself, not just inside a page's own body - useful for something every page shares, like a site-wide footer.

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

If I can help you, you have an idea or you are using Zas in your projects, don't hesitate to drop me a line (or a pull request): [@darccio](https://twitter.com/darccio)

## About

Written by [Dario Castañé](https://dario.im).

## License

Zas is under [AGPL v3](http://www.gnu.org/licenses/agpl-3.0.html) license.

## Other cool projects

Recently I found this cool generator inspired by zas: [zs](https://github.com/zserge/zs). I'm happy to be a humble reference for somebody :)
