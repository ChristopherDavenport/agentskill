# Changelog

All user-visible changes to this library. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project
uses [Semantic Versioning](https://semver.org/); before v1.0.0 minor
versions may break the API.

## Unreleased

- Changed: an `ErrSkillChanged` error ends `the host must discover the
  skills again`, where it ended `discover the skills again`. The skill
  tool returns it to the model, which cannot discover anything, and the
  host reads the same text.
- Dependencies: agentturn v0.0.15 to v0.0.16, which is used by the
  tests alone, agenttool v0.0.14 to v0.0.15, which agentturn v0.0.16
  requires, and openresponses v0.0.12 to v0.0.14. No API of this
  module changes with them.

## v0.0.10 - 2026-10-01

- Added: `Skill.Instructions` returns the text the skill tool serves
  for a read of a skill's instructions, whose SHA-256 is the
  `Read.SHA256` the read records. `Skill.FrontmatterSHA256` is the
  digest of the frontmatter the skill was loaded from, which every
  `Read` now records as `FrontmatterSHA256`. A product can bind an
  approval to both without serving the skill through the tool, so a
  rewrite of `allowed-tools` that leaves the body alone no longer
  matches it. (#32)
- Changed: the skill tool reads the skill file at each call rather
  than serving the body from discovery. A reply used to put that body
  beside the file list as it is now, a text that had never been on
  disk. An edited body is now served as it is. When the frontmatter
  changed too, the body is served under the frontmatter as loaded,
  so the listing and any grant stay the ones approved until the
  product discovers again, and the `Read` sets `FrontmatterChanged`. A
  skill file that is gone, renamed or no longer parses is refused with
  the new `ErrSkillChanged`. `Skill.Body` stays as loaded. (#31)
- Changed: the skill tool serves a path naming the skill file,
  `SKILL.md` in any case, as the skill's instructions: the body and
  file list, recorded as a `Read` with no `Path`. It used to serve the
  whole file, frontmatter included, recorded as a file read, so a host
  that grants `allowed-tools` on a read of the instructions, as
  agentkit does, granted nothing to a model that asked for the
  instructions by that name. (#30)
- Changed: `Skill.Rules` separates `allowed-tools` tokens only at the
  six ASCII whitespace characters, as agentpolicy's RFC 0001 grammar
  does. A no-break space or any other Unicode space is now part of a
  token, so `Read<NBSP>Bash`, pasted from a rendered page, is one
  unknown tool name rather than a grant of bare `Bash` that
  agentpolicy's parser does not read. The tests run agentpolicy
  v0.0.9's `testdata/policy/grammar.json`. (#29)
- Dependencies: agenttool v0.0.12 to v0.0.14, which agentturn v0.0.15
  requires, and agentturn v0.0.13 to v0.0.15, which is used by the
  tests alone. Neither agenttool release changes the root package this
  module imports, and no API of this module changes with them.

## v0.0.9 - 2026-10-01

- Documentation: `docs/source.md` restates the parts of the Agent
  Skills specification and client guide this module relies on. It also
  states what they leave out: a source as three operations, the path
  rules, and the symlink guard as a MUST for any reader. (#9)
- Changed: `Discover` finds a skill file by listing the directory and
  comparing names exactly, as `Load` has since v0.0.3, rather than
  probing with `fs.Stat`. A directory whose skill file is `Skill.md`,
  `SKILL.MD` or any other case of `SKILL.md` but the two the format
  accepts is now skipped on every host. On a case-insensitive file
  system, macOS's or Windows', it used to come up as a load error
  keyed under a `SKILL.md` that did not exist, while Linux skipped it
  silently. Either way it now has a `Warning` in `Catalog.Problems`
  under the file's real name, saying the file must be named
  `SKILL.md`. (#11)
- Fixed: `Skill.Files` lists a symlink only when it stats as a file,
  so the skill tool offers only paths it can serve. Under `Dir`, a link
  leading outside the skill was listed and then refused when read, and
  a link to a directory was listed as a file. Such links, dangling ones
  and links to a directory inside the skill are now left out. (#26)
- Dependencies: agenttool v0.0.11 to v0.0.12, and agentturn v0.0.12 to
  v0.0.13, which is used by the tests alone. No API of this module
  changes with them.

## v0.0.8 - 2026-09-29

- Added: `Skill.ShadowedBy`, the `Location` of the skill that holds the
  name a shadowed skill wanted. `Discover` sets it on each skill in
  `Catalog.Shadowed`. A skill shadowed under a taken qualified name
  has its `Qualifier` cleared, so nothing on it said which name it
  lost, and a product reading the winner from `Listed` by its bare
  name blamed a skill that never competed for it. (#23)
- Dependencies: agenttool v0.0.10 to v0.0.11, and agentturn v0.0.11 to
  v0.0.12, which is used by the tests alone.

## v0.0.7 - 2026-09-29

- Dependencies: agenttool v0.0.9 to v0.0.10, and agentturn v0.0.10 to
  v0.0.11, which is used by the tests alone. No API of this module
  changes with them.

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
