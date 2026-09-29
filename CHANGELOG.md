# Changelog

All user-visible changes to this library. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project
uses [Semantic Versioning](https://semver.org/); before v1.0.0 minor
versions may break the API.

## v0.0.6 - 2026-09-28

- Fixed: a skill `Catalog.Listed` will not offer, for want of a name or
  a description, no longer claims a name in `Discover`. It was keyed by
  its directory name, or by a name it had without a description, so a
  later source's working skill of that name went to `Shadowed` and the
  catalogue offered nothing under it. It stays in `Skills` and its
  problems in `Problems`, as before. (#19)
- A skill whose name holds a colon is no longer offered: `Listed`,
  `Prompt`, `Names`, `Lookup` and the tool leave it out, and it claims
  no name. The specification never allows one, and listed it would
  take the qualified name below from the skill it belongs to. The
  reference renders it, so this is a divergence, as for a skill with
  no name.
- **Breaking**: `Discover` refuses two sources with one `Location`,
  a trailing slash aside, with an error naming both indices, and so
  does `DiscoverDirs` given one directory twice. Their skills of one
  directory name shared a location, so the second's load error was filed in `Problems` under
  the first's healthy skill, and the prompt showed the model the same
  `<location>` for both. Migration: give each source its own
  `Location`, such as `builtin` and `builtin/pack` for two embedded
  bundles. (#20)
- `Source.Qualifier` lists a skill whose name an earlier source already
  claimed as `<qualifier>:<name>`, such as `apps/web:deploy` or
  `my-plugin:deploy`, rather than shadowing it, as Claude Code lists a
  nested directory's or a plugin's skill; a free name is listed under
  its name, and a taken qualified name, or a second skill of one name
  in the same source, is shadowed. A qualifier holding whitespace or a
  control character is an error. `Discover` sets
  `Skill.Qualifier` on such a skill, and `Skill.ListedName()` is the
  name `Prompt`, `Names`, `Lookup`, the tool and its `Read` record use.
  `Validate` still checks `Name` against the directory, since the
  qualifier is the product's. A source without one shadows as before,
  and the prompt then matches skills-ref as before. Which sources are
  nested or plugins, and what to call them, is the product's to say.
  (#18)
- Dependencies: agenttool v0.0.8 to v0.0.9, and agentturn v0.0.9 to
  v0.0.10, which is used by the tests alone. No API of this module
  changes with them.

## v0.0.5 - 2026-09-28

- Dependencies: agenttool v0.0.7 to v0.0.8, and agentturn v0.0.8 to
  v0.0.9, which is used by the tests alone. No API of this module
  changes with them.

## v0.0.4 - 2026-09-24

- Dependencies: openresponses v0.0.9 to v0.0.12, agenttool v0.0.5 to
  v0.0.7, and agentturn v0.0.5 to v0.0.8, which is used by the tests
  alone. No API of this module changes with them.

## v0.0.3 - 2026-09-23

- Fixed: `Load` reports the skill file under the spelling it has on
  disk. The name was taken from the probe that succeeded, and on a
  case-insensitive file system — APFS and HFS+ on macOS, NTFS on
  Windows — reading `SKILL.md` succeeds against a file really named
  `skill.md`, so `Skill.Location` named a path that does not exist on a
  case-sensitive host. The spelling now comes from the directory
  listing, which is exact; a source that does not implement `ReadDir`
  still falls back to probing.

- Security, **Breaking**: `Skill.Rules` refuses a token that opens a
  specifier and supplies none, `Bash()`, and the bare carve-out
  `Bash(!)`, as `agentpolicy`'s parser of the same grammar refuses
  both. An empty specifier used to parse as `{Tool: "Bash", Spec: ""}`,
  which is indistinguishable from the bare token `Bash` and, crossing
  to a policy, matches every call of the tool: the narrowest-looking
  thing a skill can write arrived as the widest grant there is. Both
  tokens are now an error from `Rules` and an error-severity `Problem`
  on the `allowed-tools` field, naming the token. The reference
  validator still accepts them, so this is a deliberate divergence.

- **Breaking**: `Catalog.Prompt` renders, and the `skill` tool serves,
  only the skills with both a name and a description. A skill missing
  either loads and is reported in `Catalog.Problems` as before, but an
  entry the model cannot call or choose is no longer put in front of
  it; the empty name used to appear in the block and in the tool's own
  list of available skills, between two commas. `Catalog.Listed`
  returns that subset, and `Catalog.Names` and `Catalog.Lookup` now
  agree with it.

- Added: the `skill` tool sets a `Read` as its result's `Details`,
  carrying the skill's name, the `SKILL.md` behind that name, the path
  served and a sha256 of the bytes served. It implements
  `agenttool.Recordable` under the exported namespace
  `agentskill.RecordNS`, so a recorder that knows nothing about skills
  writes it beside the call: a session can then say which `SKILL.md`
  was served, where a name alone is whatever discovery resolved to at
  the time, and a replay that serves different bytes for the same name
  is detectable. Requires agenttool v0.0.5, where `Recordable` arrived.

- **Breaking**: a repeated frontmatter key is a load error, as
  `strictyaml` makes it for the reference reader, rather than a silent
  last-wins. A SKILL.md whose `description` or `allowed-tools` is
  written twice used to load carrying the second value, so a reviewer
  reading the diff and the model reading the skill were told different
  things; there is no correct value to load, so the file does not load
  at all.

- Documentation: the README bounds the conformance claim and names the
  three classes of input outside it, the YAML dialect, Unicode
  normalisation and an empty `allowed-tools` specifier, saying which
  way each parts; `Validate`'s doc comment says the same in one
  sentence. The fixture tree and the reference CLI's output over it are
  under `testdata/ref`, and the tests hold both claims to it.

## v0.0.2 - 2026-09-20

- Removed: the `instructions` package. The AGENTS.md convention is now
  the separate module
  [`github.com/ChristopherDavenport/agentsmd`](https://github.com/ChristopherDavenport/agentsmd),
  whose v0.0.1 carries the package as it stood here, renamed
  `agentsmd`. Skills load on use and AGENTS.md loads up front; they are
  two concepts, and this module is named for the first.

## v0.0.1 - 2026-09-20

- Initial release: `Parse`, `Encode`, `Load`, `LoadDir` and `Validate`
  for the Agent Skills format, with the reference validator's verdicts;
  `Source`, `Dir`, `Discover`, `DiscoverDirs` and `Catalog` with
  PATH-style precedence, `Prompt` in the shape of `skills-ref
  to-prompt` and `Usage`; `Files`, `Open` and `Rules` on a skill; the
  `skill` tool from `Catalog.Tool`, serving a skill's body, its file
  list, its text files and its images through one `agenttool.Tool`;
  the `instructions` package with `Chain`, one file per directory
  within a byte budget, reporting every file found and left out, and
  `Render` for AGENTS.md files; and the `agentskill` CLI with
  `validate`, `read-properties` and `to-prompt`.
