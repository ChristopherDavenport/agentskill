# Contributing

Issues and pull requests are welcome.

## Before you start

This library reads, validates and renders instructions; it does not
decide where skills live, fetch anything, run scripts or enforce
`allowed-tools`. A change that needs a transport, a permission model
or the agent loop belongs in the product or in a sibling module.
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
and nothing more. The `instructions` package imports the standard
library alone, and a test enforces both rules.

Interoperability with the reference implementation is checked by
running the `skills-ref` CLI over the fixtures:

```sh
make interop      # needs uvx and the network
```

Fixtures live under `testdata/skills` and `testdata/instructions` with
golden outputs under `testdata/golden`; regenerate them with `go test
. ./instructions -update` and review the diff. A fixture used by the
interop diff must produce the same output from both CLIs; a case where
this module differs by design is listed in `TestInterop`.

## Pull requests

- Keep the change focused; unrelated cleanups belong in their own PR.
- Add or update tests. Tests are table-driven and run offline.
- Run `make check` before pushing. CI runs the same steps on the minimum
  and current Go versions.
- Note user-visible changes under *Unreleased* in `CHANGELOG.md`.
