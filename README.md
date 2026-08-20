# ![Zas](http://i.imgur.com/e9abWRX.png)

Most simple static site generator ever.

## Why another one? C'mon, you must be kidding

I just wanted to set up a simple website, just some pages, using Jekyll, and it didn't feel right. I didn't want a blog.

I checked other projects, but they were incomplete, cumbersome, or solved the wrong problem (blogs, blogs everywhere). I wanted a zen-like experience: a layout and some Markdown files as pages with unobtrusive structure and configuration.

Yes, it is another NIH, but... I think Zas is a different kind of beast. I admit that I probably overlooked some projects at the moment.

### What is the difference?

1. Gophers. Yes, there is [Hugo](http://gohugo.io/) (kudos!) but... Who wants to learn another directory layout? There is also [Hastie](https://github.com/mkaz/hastie) and [lots of other static site generators](https://jamstack.org/generators/).
2. Pure Markdown. And HTML, if you want.
3. Just a loop. Zas loops over all .md and .html files in the current directory (and subdirectories), ignoring other files (including dot-files).
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
* `-full`: generate all the input files. By default, it has an incremental mode that keeps source and deploys directories in sync.

## Configuration and extension

Zas is like water. It can flow, or it can cr... Nah, Zas doesn't crash (please fill an issue if it does).

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

That's all. Zas passes any command-line argument after subcommand name to `zshello`.

**Beware:** Zas won't pass any configuration information. Plugins are responsible for reading configuration, even from directory and page levels. Helper libraries in different languages are welcome!

Also, plugins are free to use `.zas` directory for their own needs. I recommend creating this directory's structure to avoid colliding issues:

```text
.zas
+-- plugins
|   +-- github.com
|       +-- imdario
|           +-- myplugin
+-- ...    
```

Any `zs` plugin can be invoked through a script tag with type `application/zas+myplugin`. Arguments can be passed with data-args attribute, which content will be used as command-line invoke.

```html
<script type="application/zas+myplugin" data-args="arg1 arg2 ... argN"></script>
```

The tag is deleted after execution. Any output in stdout will replace the tag.

All plugins will be called in order. Oh, you can even ask for async execution with same-name attribute (hint: async).

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
* `{{.Extra /path/}}`: direct access to map holding `.zas/config.yml` as it is. You can access to any value with its full path. E.g. BaseURL is also available as `/site/baseurl`.
* `{{.Resolve id}}`: indirect access to site, directory and page config. It works with simple keys (no paths), checking for them in page, directory and site config (as `/site/<id>`), in this order.
* `{{.Language}}`: file current language, if defined in the first comment (as YAML property `language`). By default, `/site/language` value.

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

## 你会说普通话?

對不起。T我不会说普通话。That's all my Chinese! If you are here, I guess you will enjoy I18N support in Zas.

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

## Roadmap

There is no roadmap. I wrote some possible enhancements [here](https://github.com/darccio/zas/issues?q=is%3Aissue+is%3Aopen+label%3Aenhancement).

Feel free to open an issue if you think Zas should do something specific in its core.

## Contact me

If I can help you, you have an idea or you are using Zas in your projects, don't hesitate to drop me a line (or a pull request): [@darccio](https://twitter.com/darccio)

## About

Written by [Dario Castañé](http://dario.im).

## License

Zas is under [AGPL v3](http://www.gnu.org/licenses/agpl-3.0.html) license.

## Other cool projects

Recently I found this cool generator inspired by zas: [zs](https://github.com/zserge/zs). I'm happy to be a humble reference for somebody :)
