# Contributing

Issues and pull requests are welcome.

## Before you start

This library reads, validates and renders skills; it does not decide
where skills live, fetch anything, run scripts or enforce
`allowed-tools`. A change that needs a transport, a permission model
or the agent loop belongs in the product or in a sibling module, and
the AGENTS.md convention is the sibling `agentsmd`.
`docs/plans/skill-layer.md` is the design; read it first.

For anything larger than a bug fix, open an issue first so the shape of
the change can be discussed before you spend time on it.

## Development

Go 1.25 or later is required. The full local check is:

```sh
make check        # gofmt, tidy, vet, deps, staticcheck, govulncheck, race tests
```

The module depends on `openresponses`, `agenttool`, `go.yaml.in/yaml/v3`
and the standard library; `make deps` fails if anything else creeps
in. `agentturn` is a test dependency of the tool's integration test
and nothing more; a test enforces both rules.

Interoperability with the reference implementation is checked by
running the `skills-ref` CLI over the fixtures:

```sh
make interop      # needs uvx and the network
```

Fixtures live under `testdata/skills` with golden outputs under
`testdata/golden`; regenerate them with `go test . -update` and review
the diff. A fixture used by the
interop diff must produce the same output from both CLIs; a case where
this module differs by design is listed in `TestInterop`.

## Releases

This repository is a single module, tagged `vX.Y.Z`. With the
changelog's *Unreleased* section written:

```sh
make release VERSION=v0.1.0
```

dates the changelog, runs `make tidy` and `make check`, commits, guards
and tags the version with the changelog section as the message, and
pushes the branch and the tag in one atomic push. The release workflow
publishes a GitHub release from the tag message, and the Go module
proxy picks the version up. Before v1.0.0 the API may change between
minor versions; the changelog records every break.

`make release-guard TAG=<tag>` is what stands between a mistake and a
permanent one, and `make release` runs it before the tag it writes. It
refuses a dirty tree, a tag that already exists locally or on origin,
and a version that does not sort above the current release — the one
mistake nothing can undo, since the proxy and the checksum database
keep every published version forever. It runs after the release commit
and before the tag, while everything is still local, so a refusal costs
a `git reset --hard HEAD~1`.

The sibling repositories with nested modules carry a longer
`release-guard.sh` that also holds a `<dir>/vX.Y.Z` tag to its module's
`go.mod`. If a nested module is ever added here, that is the part to
bring over.

## Pull requests

- Keep the change focused; unrelated cleanups belong in their own PR.
- Add or update tests. Tests are table-driven and run offline.
- Run `make check` before pushing. CI runs the same steps on the minimum
  and current Go versions.
- Note user-visible changes under *Unreleased* in `CHANGELOG.md`.
