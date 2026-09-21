# agentskill

The [Agent Skills](https://agentskills.io/specification) format for Go
agents over Open Responses: skills loaded, validated and rendered into
the text a product puts in its instructions, plus the `skill` tool
through which the model reads a skill's body and files. The
[AGENTS.md](https://agents.md/) convention, loaded up front rather
than on use, is the separate
[agentsmd](https://github.com/ChristopherDavenport/agentsmd) module.

A skill is read through `fs.FS`, so it can come from a directory, an
embedded bundle, an archive or an adapter over a remote store, and the
model reaches every level of it through one tool rather than through a
local read tool that may not exist.

```go
catalog, err := agentskill.DiscoverDirs(".dex/skills", filepath.Join(home, ".dex", "skills"))
if err != nil {
	return err
}
for location, problems := range catalog.Problems {
	log.Printf("%s: %v", location, problems) // broken skills are listed, not hidden
}

cfg := agentturn.Config{
	Instructions: catalog.Prompt() + "\n\n" + catalog.Usage(),
	Tools:        []agenttool.Tool{catalog.Tool()},
}
```

`Prompt` renders the `<available_skills>` block byte for byte as the
reference `skills-ref to-prompt` does for the same directories. It
renders the skills the model can use: a skill with no name to call it
by, or no description to choose it from, is loaded and reported in
`Problems` but is neither listed nor served, since an entry the model
cannot follow costs tokens on every turn. The tool serves the other
two levels of progressive disclosure: called with a name it returns
the body and the list of files; called with a name and a path it
returns the file, text as text and an image as an image part.

## Skills

- `Load(fsys, location)` reads the `SKILL.md` at the root of any
  `fs.FS`; `LoadDir` does it for a directory; `Parse` reads bytes.
- `Validate` reports every rule of the specification as a `Problem`,
  with the reference validator's verdicts and wording, and warns on a
  body over 500 lines. A malformed file fails to load; a valid file
  with a bad name loads and says what is wrong.
- `Discover(sources...)` walks each source's direct children; the
  first skill with a given name wins, as PATH resolves a command.
  `Catalog.Listed` is the subset the prompt renders and the tool
  serves, and `Lookup` and `Names` agree with it.
- `Dir(path)` is the source for a local directory, with one guard
  `os.DirFS` lacks: a symlink resolving outside the directory is
  refused.
- `Files`, `Open` and `Rules` reach a skill's resources and its parsed
  `allowed-tools`; enforcement is the host's. A token that opens a
  specifier and supplies none, `Bash()`, is refused rather than parsed
  as the bare `Bash`, which grants every call of the tool.
- `Encode` writes a skill back; `Parse(Encode(s))` yields `s`.

## CLI

`go run ./cmd/agentskill` mirrors the reference CLI: `validate`,
`read-properties` and `to-prompt`. `make interop` diffs its output
against `skills-ref` over the fixtures.

## Development

```sh
make check    # gofmt, tidy, vet, deps, staticcheck, govulncheck, race tests
make interop  # needs uvx and the network
```

Golden outputs live under `testdata/golden`; regenerate with `go test
. -update` and review the diff.
