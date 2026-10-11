<!--
title: Zas
summary: Build a static site from Markdown and a shared HTML layout.
language: en
feed: false
template: false
-->
# Zas

Zas is a static site generator written in Go. It walks a site's files, renders Markdown and HTML through a shared layout, and copies ordinary assets into the deploy directory. The source directory structure becomes the site's URL structure.

## Install and start

The current source requires Go 1.26 or newer.

```sh
go install github.com/darccio/zas/cmd/zas@latest
mkdir my-site
cd my-site
zas init
```

Write `index.md`, edit `.zas/layout.html`, and build:

```sh
zas -full
```

The generated site is in `.zas/deploy` by default. Upload that directory to your static host.

`zas init` preserves an existing configuration and layout. `zas init -force` replaces both with scaffolded defaults.

These guides describe the current source. For reproducible builds, install a selected release tag or commit instead of `@latest`, and choose a revision containing the features you use.

## Guides

- [Configuration](configuration.md): site settings, page overrides and languages.
- [Templates and plugins](templates.md): layouts, metadata, embeds and the trust model.
- [Publishing](publishing.md): sitemaps, RSS, Markdown twins and deployment.

## Project links

- [Source repository](https://github.com/darccio/zas)
- [API reference](https://pkg.go.dev/github.com/darccio/zas)
- [Releases](https://github.com/darccio/zas/releases)
- [Report a bug](https://github.com/darccio/zas/issues)
- [Sponsor maintenance](https://github.com/sponsors/darccio)

Zas retains its module path `github.com/darccio/zas`. Hosting these guides on dario.cat does not change how Go users install or import it.

## License

Zas and these docs are distributed under the [GNU Affero General Public License, version 3](LICENSE.txt), or any later version.
