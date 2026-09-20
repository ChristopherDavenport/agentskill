# Plan: skill layer

Instructions on disk, loaded into the model's context: the
[Agent Skills](https://agentskills.io/specification) format and the
[AGENTS.md](https://agents.md/) convention. A product built on
`agentturn` needs both, the agent-layer plan places them outside the
loop ("skills or prompt context through `Transform`" under *What a
product adds*), and the dex plan buries them in its `prompt/` piece.
This module lifts them out so every product shares one implementation,
the way `agenttool` was lifted out of the loop.

The two formats carry different weight. Agent Skills is a specification
with a reference validator and exact rules for names, descriptions and
directory layout; it is the root package. AGENTS.md is plain Markdown
with one discovery rule and no format; it is a small nested package.
Both produce text that lands in `agentturn.Config.Instructions`, which
`agentsession` records in the config entry, so a replayed session shows
exactly which skills and instruction files the model was given.

A skill is a tree of files, and "on disk" is only the common case. The
module reads skills through `fs.FS`, so a skill can come from a
directory, an embedded bundle, an archive, or an adapter over a remote
store, and the model reaches every level of a skill through one tool
rather than through a local read tool that may not exist.

## Goals

- Load a skill from any `fs.FS` into a value that carries the parsed
  frontmatter, the body and the list of resource files, byte for byte
  from the source.
- Validate against the specification with the same verdicts as the
  reference `skills-ref` validator, so a skill that passes here passes
  there.
- Discover skills across an ordered list of sources with a precedence
  rule, and render the `<available_skills>` block in the reference
  library's exact shape.
- Serve all three levels of progressive disclosure through one
  `agenttool.Tool`: metadata in the instructions, the body and the
  resource files through the tool. A product's own file tools are an
  alternative for local skills, never a requirement.
- Discover AGENTS.md files from a directory to the root, nearest last,
  and render them as pi renders `<project_context>`.
- Standard library plus `openresponses`, `agenttool` and one YAML
  parser. Nothing else. `agentturn` is never imported.

## Non-goals

- Deciding where skills live. The specification does not say, and each
  product has its own directories (`.claude/skills`, `.dex/skills`, a
  home directory) or remote catalogs. The caller passes sources.
- Fetching. An `fs.FS` over HTTP, an MCP server's resources or an
  object store is a product's or a sibling's adapter; this module
  reads whatever `fs.FS` it is given and never opens a socket.
- Running `scripts/`. The tool can return a script's text; running it
  is a shell tool's job, on a product that has one.
- A permission system. `allowed-tools` is parsed into rules the host
  can match a call against; enforcement is the host's `BeforeToolCall`.
- Imports inside instruction files (`@path` in CLAUDE.md). AGENTS.md
  has none. A product that wants them expands them before rendering.
- Prompt templates, slash commands, `SYSTEM.md` and `APPEND_SYSTEM.md`.
  Those are the product's prompt builder, which composes this module's
  output.
- Any transport or persistence.

## Module and packages

Separate module, `github.com/ChristopherDavenport/agentskill`.

```
agentskill/                  Skill, Parse, Load, Validate, Discover, Catalog, prompt rendering, the skill tool
agentskill/instructions      AGENTS.md discovery and rendering (standard library only)
agentskill/cmd/agentskill    validate, read-properties, to-prompt; mirrors the skills-ref CLI
```

The root module depends on `openresponses`, `agenttool` and
`go.yaml.in/yaml/v3`. The YAML dependency is deliberate: frontmatter is
YAML, the standard library has no parser, and real skills use quoted,
folded and multi-line scalars that a hand-rolled subset would reject.
`make deps` allows exactly these three. `instructions` imports the
standard library alone and a test enforces it, so a product that wants
only AGENTS.md pays for nothing else.

The dependency direction is `agentskill -> agenttool -> openresponses`.
`agentturn` never imports this module; a product wires the two
together. dex's `prompt/` piece is the first consumer.

## Core types

```go
// Source is where skills come from: a file system and the name the
// prompt shows for it.
type Source struct {
    // FS holds skill directories as its direct children, each with a
    // SKILL.md. os.DirFS, embed.FS, a zip, or an adapter over a
    // remote store.
    FS fs.FS
    // Location names the source for the model, such as an absolute
    // directory, a URL or "mcp://docs". A skill's location is this
    // joined with the skill's directory name and SKILL.md.
    Location string
}

// Dir is the Source for a local directory: os.DirFS(dir) with the
// absolute path as the location, so the prompt matches skills-ref.
func Dir(dir string) (Source, error)

// Skill is one skill directory, loaded.
type Skill struct {
    // FS is the skill's own tree, rooted at its directory: a fs.Sub
    // of the source. SKILL.md is at its root.
    FS fs.FS
    // Location is what the prompt shows: the source location joined
    // with the directory name and SKILL.md.
    Location string
    // DirName is the directory's base name, which name must match.
    DirName string

    // Frontmatter fields, as the specification names them.
    Name          string
    Description   string
    License       string
    Compatibility string
    Metadata      map[string]string
    AllowedTools  string
    // Extra holds frontmatter keys the specification does not define,
    // decoded but not interpreted, so a newer skill loads in an older
    // reader and re-encodes without loss.
    Extra map[string]any

    // Body is the Markdown after the frontmatter, verbatim.
    Body string
}

func Load(fsys fs.FS, location string) (*Skill, error) // read SKILL.md at the root of fsys, Parse, attach
func LoadDir(dir string) (*Skill, error)                // Load over os.DirFS(dir)
func Parse(src []byte) (*Skill, error)                  // frontmatter and body from bytes; FS nil
func (s *Skill) Validate() []Problem                    // the specification's rules
func (s *Skill) Files() ([]string, error)               // every file except SKILL.md, fs paths, sorted
func (s *Skill) Open(name string) (fs.File, error)      // a resource; name must satisfy fs.ValidPath
func (s *Skill) Rules() ([]ToolRule, error)             // AllowedTools parsed

type Problem struct {
    Severity Severity   // Error or Warning
    Field    string     // "name", "description", "body", ... or "" for the directory
    Message  string
}
```

`Load` is not `Validate`. A malformed file fails to load; a valid file
with a bad name loads and reports the problem, so a product can list a
broken skill and say why rather than make it vanish. `Parse` errors are
about YAML and the frontmatter fence only.

`fs.ValidPath` already refuses `..`, absolute paths and empty
elements, which is the traversal guard for every `fs.FS`. `Dir` adds
the one guard `os.DirFS` lacks: a symlink that resolves outside the
directory is refused, because a local skill tree may be
user-installed and a link to `/etc` inside it must not be readable
through the tool.

### Validation

Every rule of the specification, one `Problem` each, with the reference
validator's verdicts:

- `name`: present, 1 to 64 characters, lowercase letters, digits and
  hyphens only, no leading, trailing or doubled hyphen, equal to
  `DirName` when it is set.
- `description`: present, 1 to 1024 characters after trimming.
- `compatibility`: at most 500 characters when present.
- `metadata`: a map whose values are strings. A non-string value is
  an error, because the specification says string to string.
- `allowed-tools`: a string; each token parses as a rule. Tokens are
  separated by whitespace outside parentheses, so a specifier holds
  spaces, as the documented `Bash(git add *)` does, and literal
  parentheses, as `Edit(./Finance (2024)/**)` does. Unbalanced
  parentheses are an error, because the token cannot then be
  delimited.
- Body: a warning when `SKILL.md` exceeds 500 lines. The specification
  says "keep it under", so it is advice, not a failure.
- Unknown frontmatter keys: an error naming the keys, since the
  reference validator rejects them (resolved below). They are still
  kept in `Extra`, so a skill written for a newer reader loads and
  re-encodes without loss.

`Error` problems make `Validate` fail for the CLI and for `Discover`'s
strict mode; warnings never do.

### Discovery and precedence

```go
// Discover loads every skill under the sources: each direct child
// directory holding a SKILL.md is one skill. Sources are searched in
// order and the first skill with a given name wins, as PATH resolves a
// command, so a caller lists the most specific source first.
func Discover(sources ...Source) (*Catalog, error)

// DiscoverDirs is Discover over Dir for each path.
func DiscoverDirs(dirs ...string) (*Catalog, error)

type Catalog struct {
    Skills   []*Skill            // in source order, then directory-name order, winners only
    Shadowed []*Skill            // later duplicates, kept so a product can report them
    Problems map[string][]Problem // per skill location, from Validate
}

func (c *Catalog) Lookup(name string) (*Skill, bool)
```

A skill that fails to `Load` is not fatal to the catalog: it goes into
`Problems` under its location and discovery continues. A source whose
`FS` cannot be read at the root is an error, because a missing local
directory should be skipped by the caller with `Dir`'s error, not
silently by discovery. A skill with error-severity problems is still
listed; a product that wants only valid skills filters on `Problems`.

### Level one: the prompt

```go
// Prompt renders the available_skills block in the exact shape of the
// reference library's to_prompt, one entry per skill in catalog order,
// with the skill's Location as the location.
func (c *Catalog) Prompt() string
```

For local sources the output is byte for byte what `skills-ref
to-prompt` produces for the same directories, one `<skill>` with
`<name>`, `<description>` and `<location>` each on its own lines, and a
test compares them. For other sources the location is whatever the
source named, and the tool is how the model gets there. The product
concatenates the block into `Config.Instructions`, which is what the
session records, and adds one line saying that the `skill` tool reads
a skill and its files; a ready-made `Catalog.Usage()` string provides
it.

### Levels two and three: the tool

```go
// Tool returns an agenttool.Tool named "skill". Called with a name
// alone it returns the skill's body followed by the list of its files,
// so the model learns both what to do and what there is to read.
// Called with a name and a path it returns that file.
func (c *Catalog) Tool(opts ...ToolOption) agenttool.Tool

type args struct {
    Name string `json:"name" desc:"The skill to read"`
    Path string `json:"path,omitempty" desc:"A file inside the skill, as listed; omit for the instructions"`
}
```

The tool is built with `agenttool.New`, so the schema is generated and
validated like any other, and it returns `openresponses.Contents` so
one tool serves text and images:

- The body call returns the Markdown body, then a `files:` section
  listing every path with its size, so the model can pick without
  guessing.
- A file with a text media type, by extension and by a sniff of the
  first bytes, returns as text.
- An image returns as an image content part.
- Any other binary is refused with its size and detected type, so the
  model is told what it is rather than handed bytes it cannot use.
- A file above the size cap (`WithMaxBytes`, default 1 MiB) is refused
  with its size; there is no partial read, because a half file is a
  worse instruction than none. A product that needs ranges has a read
  tool.
- An unknown skill name is an error listing the names the model could
  have used. An unknown path is an error listing the files.

Because every activation and every read is a function call, the
transcript and the session record which skills a run used and which
files it opened, without a new entry or item type.

The tool is the default, not the only way. A product that prefers to
inject a body through `Transform` reads `Skill.Body` itself, and a
product with a general read tool can let the model read a local
skill's files directly; the location in the prompt is an absolute path
for that reason.

### `allowed-tools`

```go
// ToolRule is one token of allowed-tools: a tool name, or a name with
// a specifier in parentheses, "Bash(git:*)". The specifier's syntax
// belongs to the product; the rule keeps it as a string, spaces and
// nested parentheses included.
type ToolRule struct {
    Tool string
    Spec string // "" when the token has no parentheses
}

func (r ToolRule) Matches(toolName string) bool
```

The field is experimental in the specification and the specifier
grammar is the product's. This module splits tokens at whitespace
outside parentheses, so the specifier is passed through untouched, and
matches on the tool name; a host wires `Rules` into `BeforeToolCall`
however it likes.

### `instructions`

```go
// Chain returns the instruction files that apply at path: in path's
// directory and each of its ancestors up to Root, the first file of
// Names that exists, farthest first and nearest last, followed by
// Extra in order, within Budget. With nearest last, later text
// refines earlier text, which is how the convention's "closest file
// takes precedence" reads when a product includes every file, as pi
// and Claude Code do. A file found and not included is reported.
func Chain(path string, opts Options) (Result, error)

type Options struct {
    Names    []string // tried in order per directory, first found wins; default: AGENTS.md
    Root     string   // stop after this directory; default: the filesystem root
    Extra    []string // explicit paths appended last, such as ~/.dex/AGENTS.md; missing ones are skipped
    MaxBytes int64    // per file that is included; default 1 MiB, a larger one is an error
    Budget   int64    // total; the first file that would exceed it ends the chain, without error; default: none
}

type Result struct {
    Files   []File    // included, in order
    Omitted []Omitted // found and left out, in the order met
}

type File struct {
    Path    string // absolute
    Content string // verbatim
}

type Omitted struct {
    Path   string // absolute
    Size   int64
    Reason Reason // OverBudget or Shadowed
    By     string // the file that stood in for it, for Shadowed
}

// Render wraps the files as pi does: one <project_instructions
// path="..."> per file inside <project_context>, in order.
func Render(files []File) string
```

`Chain` takes a path rather than a cwd so one call serves both the
session's working directory and the file a tool is about to touch in a
monorepo. `Names` is a preference order and yields at most one file
per directory, as Codex reads `AGENTS.override.md` before `AGENTS.md`,
so a developer can shadow a committed file without deleting it, and
dex can let `CLAUDE.md` stand in where `AGENTS.md` is absent; the
package knows no name specially. `Budget` caps the total, the way the
reference's `project_doc_max_bytes` does: the first file that would
exceed it, and everything after it, is left out and `Chain` returns
what fits without error, so a large file deep in a tree degrades the
prompt rather than failing the run. No file is cut short, because a
half instruction file is a worse instruction than none; `MaxBytes`
stays a per-file error for a file that would be included and could
never fit. Neither omission is silent: `Result.Omitted` names every
file `Chain` found and left out, with its size and why, so a product
can tell the user that a rule file was shadowed or did not fit, and
the session can record what the model was not given as well as what
it was. Only a file that is included is read; a shadowed or
over-budget file is stat'd for its size and left alone. This package
stays on the local file system: the convention is about a repository
checkout, and a product with a remote checkout hands the files to
`Render` itself.

## Invariants

- `Parse(Encode(s))` yields `s`. The body is never reflowed; the
  frontmatter re-encodes with the specification's keys in
  specification order and `Extra` after them.
- A `Skill` never holds content the model has not been shown or could
  not be shown: no summaries, no rewrites.
- `Prompt` output for local sources equals `skills-ref to-prompt`
  output for the same directories.
- `Validate` errors for the fixtures equal `skills-ref validate`
  errors for the same fixtures, message text aside.
- `Open` and the tool never return a file outside the skill's `FS`;
  for `Dir`, symlinks resolved.
- Everything the tool returns is bytes from the source, framed, never
  transformed.
- `instructions` imports the standard library only.

## Testing

Table-driven and offline. Fixtures under `testdata/skills/` cover the
valid and invalid cases of the specification, one directory each, with
golden `validate` and `to-prompt` outputs, regenerated with `go test .
-update`. The same fixtures are loaded through `os.DirFS`, `embed.FS`
and `fstest.MapFS` to prove no code path assumes a disk. The tool is
tested against the fixtures for the body call, a text file, an image, a
refused binary, a refused oversize file and the two unknown-name and
unknown-path errors. `testdata/instructions/` is a fixture tree with
files at several depths, a directory holding both `AGENTS.md` and
`AGENTS.override.md`, and a symlink.

`make interop` runs the `skills-ref` CLI over the same fixtures and
diffs its `validate` and `to-prompt` output against ours. It needs
`uvx` and the network, so it is not part of `check`, as `agenttool`'s
MCP interop is not.

## Milestones

1. `Parse`, `Load`, `LoadDir`, `Encode`, `Validate` with the
   specification's rules and the fixtures over three `fs.FS`
   implementations. The CLI's `validate` and `read-properties`.
2. `Source`, `Dir` with the symlink guard, `Discover`, `Catalog`,
   `Prompt`, `Usage`. The CLI's `to-prompt`. The interop diff against
   `skills-ref`.
3. `Files`, `Open`, `Rules`.
4. `Catalog.Tool`: body and file calls, media handling, size cap,
   errors. Then a run under `agentturn` with the `echo` adapter in a
   test that lives here and imports the loop as a test dependency
   only, over an `embed.FS` source so the run touches no disk.
5. `instructions`: `Chain`, `Render`, the standard-library boundary
   test.
6. dex's `prompt/` piece consumes both packages; its golden prompt test
   covers the wiring.

## Open questions

Resolved:

- `skills-ref` rejects unknown frontmatter keys, so they are errors
  here. Milestone 1 checked: its validator lists them as "Unexpected
  fields in frontmatter", and the interop diff now holds that case.
- The tool serves resources, not only the body. A skill is an `fs.FS`
  and the tool is the model's only required way in, so a skill from an
  embedded bundle or a remote adapter works on a product with no local
  read tool. Levels two and three are one tool with an optional path.

Open:

- Whether a remote `fs.FS` adapter (MCP resources, HTTP) belongs in
  this repository as a nested module, the way `mcpclient` sits in
  `agenttool`, or in the product. Nothing here needs it; reopen when
  a second product wants the same adapter.
- Whether `instructions` should also find files below the path, for a
  product that wants a subtree's rules before it edits inside it.
  `Chain` on the target file covers the case the convention describes;
  reopen if a product wants a whole-tree index.
- The YAML dependency. `go.yaml.in/yaml/v3` is the maintained
  successor of `gopkg.in/yaml.v3`. If a standard-library subset proves
  enough across the fixtures and a corpus of published skills, the
  dependency can be dropped in a minor version without an API change.
- Provenance of what the model was shown. The rendered skill block
  and the instruction chain land in `Config.Instructions` as text, so
  the session's config entry records what the model read but not
  where each piece came from or which version it was. Two additions
  would close that: a `Manifest` on `Catalog` and on the
  `instructions` chain listing each source, each skill or file, its
  location and a SHA-256 of its bytes (the body and every resource
  for a skill, so a changed reference file changes the hash); and a
  place for it in the session, most likely an `agentskill:` slug
  under the config entry's extension keys or a `RequestExtra` value,
  so `agentsession` stores it without learning the type. With that, a
  replay can say "this run had `pdf-processing` at hash `ab12…` from
  `~/.dex/skills`" and an evaluation can group trajectories by skill
  version. Open because it touches the session format: whether the
  RFC needs a named field or the passthrough is enough, and whether
  the hash covers the whole tree or the body alone, is decided with
  `agentsession` once milestone 2 shows what the manifest holds.
  On the `instructions` side, `Result` is already the record of what
  was found, included and omitted; a manifest would add the hash of
  each included file to it.
