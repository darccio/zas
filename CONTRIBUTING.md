# Contributing to Zas

Thanks for looking into contributing. This document covers building and
testing locally, what a pull request is expected to look like, and how to
write tests that fit the existing suite.

## Building and testing

Zas requires the Go version declared in `go.mod`. The core loop:

```sh
go build ./...
go vet ./...
go test ./... -race
```

Lint with golangci-lint, using the version CI pins in
`.github/workflows/ci.yml` (currently v2.11.4; config lives in
`.golangci.yml`):

```sh
golangci-lint run ./...
```

CI (`.github/workflows/ci.yml`) runs the same build and vet steps, then
`go test -race -shuffle=on -covermode=atomic -coverprofile=coverage.out ./...`
and a separate lint job. Running the four commands above locally before
opening a PR catches the same problems earlier: `go vet` and
`golangci-lint run ./...` should report nothing, and the tests should pass.

## Pull requests

- Keep a PR focused on one change; split unrelated fixes across separate
  PRs/commits rather than bundling them.
- Write commit messages (and PR titles) as [Conventional
  Commits](https://www.conventionalcommits.org/), matching this repo's
  history (`git log`): `fix(generate): ...`, `test: ...`, `docs: ...`,
  `build(deps): ...`, `ci: ...`, `chore: ...`. The type, and usually a scope
  in parentheses naming the package or area the change centers on, precede a
  lower-case, imperative summary.
- For anything non-trivial, use the commit body to explain *why*: what was
  broken, why the fix takes the shape it does, not just a restatement of the
  diff. Recent commits under `fix(generate): ...` are good examples of the
  level of detail expected.
- Add or update tests for behavior changes. A regression fix should land with
  a test that would have failed before it.

## Test fixtures

Some tests need a real `.zas/config.yml`, a layout, or other on-disk content
to drive `Generator.Run` end-to-end. Prefer a `testdata/<name>/` fixture
directory over inlining YAML/HTML/template content as Go string literals:

```go
func TestSomething(t *testing.T) {
	newTestSite(t, "<name>")
	// ... generate(t), assertions ...
}
```

`newTestSite` (in `harness_test.go`) copies `testdata/<name>/` into a fresh
`t.TempDir()`, `t.Chdir`s into it, and snapshots/restores the package's
global default config so a test can't leak mutations into the next one.
`testdata/site/` is the main fixture - `.zas/config.yml`, `.zas/layout.html`,
`.zas/i18n.yml`, a couple of pages, a subdirectory with its own `.zas.yml`, a
partial, a JSON asset - and is reused by most of the end-to-end tests in
`generate_e2e_test.go` and `dependency_staleness_e2e_test.go`. Look there
first: a scenario that's `testdata/site/` plus one or two extra source files
written after `newTestSite` returns usually doesn't need a fixture of its
own.

When a test needs the site built into an arbitrary directory rather than the
current one (see `buildWalkErrorFixture` in `generate_walk_error_e2e_test.go`
for an example), use `copyFixture(t, "<name>", dir)` instead - it does the
same copy without `t.Chdir`.

Fixtures are plain files: no templating layer, nothing generated at test
time. Add a directory under `testdata/`, write real
`config.yml`/`layout.html`/etc. content into it, and check it in like any
other file in the repo.

The judgment call on whether something belongs in a fixture:

- **Leave it inline** when it's small, single-purpose, and tightly coupled to
  one assertion: a one-line `.zas.yml` snippet, a deliberately malformed YAML
  string for a negative parse test, a one-line embed or content string.
  Jumping to a separate file to read three bytes of YAML costs more than it
  saves.
- **Use a fixture** when the content is reused or near-duplicated across
  multiple tests, is non-trivial in size, or is the kind of thing a
  maintainer would plausibly want to open and edit as a real config/template
  file - a full `config.yml` + `layout.html` pair, or a small multi-file
  site. If you're about to copy-paste the same `zas:\n  layout: ...` block
  into a third test, that's the signal to extract it instead.

When extracting, pull out only the static parts. Code that generates content
dynamically (a loop writing hundreds of files, `os.Chmod` to force a
permission error, etc.) stays in the test or helper function - a fixture is
for content a maintainer would want to read as-is, not a place to encode
logic.

## Releasing

Pushing a tag matching `v*` (e.g. `v0.1.0`) triggers
`.github/workflows/release.yml`, which runs
[GoReleaser](https://goreleaser.com) (config in `.goreleaser.yaml`) to build
`cmd/zas` for linux/darwin/windows on amd64/arm64, archive and checksum the
binaries, generate a changelog grouped by commit type from the Conventional
Commits history since the previous tag, and publish all of it as a GitHub
Release. `zas version`'s own output (see `cmd/zas/main.go`) needs no extra
wiring for this - it already reads the module version and VCS revision Go's
toolchain stamps into the binary at build time.

To try the whole pipeline locally without publishing anything, install
[goreleaser](https://goreleaser.com/install/) and run:

```sh
goreleaser release --snapshot --skip=publish --clean
```

This is a plain binary+checksum+changelog release; packaging for a package
manager (Homebrew, apt, etc.) isn't set up yet.
