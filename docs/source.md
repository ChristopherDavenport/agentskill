# Skill sources

What this module reads a skill from, and the rules it applies while
reading. The first half restates the parts of the Agent Skills
documents the module relies on, so the rules below can be checked
against them. The second half is what the module adds: the source as
three operations, the path rules, and the symlink guard. The format
does not specify any of these, but a reader of untrusted skills needs
all of them.

The Go binding is `fs.FS` together with `Dir`'s guard. A reader in
another language implements the same operations and rules with its own
filesystem API.

## What the format defines

Restated from the [specification](https://agentskills.io/specification)
and the [client implementation guide](https://agentskills.io/client-implementation/adding-skills-support),
as published on 2026-10-01. Where a document's words decide a question,
they are quoted.

**A skill is a directory.** "A skill is a directory containing, at
minimum, a `SKILL.md` file". The `SKILL.md` "must contain YAML
frontmatter followed by Markdown content". Beyond it, "a skill
directory may contain any files and directories". `scripts/`,
`references/` and `assets/` are recommendations, not rules.

**The skill file's name.** The guide says to look for "subdirectories
containing a file named exactly `SKILL.md`". The reference reader,
`skills-ref`, also accepts `skill.md`, and so does this module, which
prefers `SKILL.md` when both exist. Names are compared exactly against
the directory listing, never by asking the filesystem whether a name
exists. The two differ on a case-insensitive filesystem, so a probe
would make the same tree hold a skill on macOS and none on Linux (#11).
A file named `SKILL.md` in some other case, such as `Skill.md`, is not
a skill file on any host. `Discover` reports it as a warning.

**Frontmatter.** `name` and `description` are required. `license`,
`compatibility`, `metadata` and `allowed-tools` are optional. The
constraints are those of the specification's table: a name of 1–64
lowercase letters, digits and single hyphens that matches the parent
directory; a description of 1–1024 characters; a compatibility of at
most 500 characters; `metadata` a map from string to string; and
`allowed-tools` a space-separated string, marked experimental.
`Validate` reports each rule with `skills-ref`'s verdict and wording.
The README's Conformance section lists where the two differ.

**Lenient loading.** The guide says to warn and still load a skill
whose name is wrong, and to skip one with no description or with YAML
that cannot be parsed. This module loads the first kind and reports it
in `Catalog.Problems`. A skill with no description is loaded and
reported but never listed. A file whose YAML cannot be parsed fails to
load, and that error goes into `Problems`.

**Where skills live.** The specification "does not mandate where skill
directories live (it only defines what goes inside them)". The guide's
convention is project-level over user-level, and within one scope
"either first-found or last-found is acceptable — pick one and be
consistent". `Discover` takes its sources in order and the first one
wins. Choosing the directories is the product's job.

**Location and relative paths.** A skill's location is the path of its
`SKILL.md`. Its base directory, "the parent directory of `location`",
is what the body's references are resolved against: "use relative paths
from the skill root". `Skill.Location` is that path, and every resource
path this module takes or returns is relative to the skill root.

**Progressive disclosure.** There are three levels: metadata at
startup, the body when the skill is activated, and resources "only when
required". A tool that activates a skill "can also enumerate supporting
files … but it should **not eagerly read them**". The `skill` tool
follows this: called with a name it returns the body and the list of
files, and called with a path it returns that one file.

**Trust.** Project-level skills "may be untrusted (e.g., a freshly
cloned open-source project)", and the guide suggests gating them on a
trust check. That gate is the product's job. Neither document says
anything about what a skill's files may point at, which is why the
rest of this document exists.

## What a source is

A source is a tree of skill directories that can be reached by three
operations:

1. **List a directory**: the names of its entries, as the source spells
   them, each marked as a file or a directory.
2. **Read a file**: its bytes.
3. **Stat a path**: whether it exists, whether it is a directory, and
   its size. The tool checks the size before reading.

Opening a file as a stream is optional; reading it whole is enough. A
source that cannot list still serves a bare `Load`, which falls back to
probing the two names of the skill file. `Discover` and `Skill.Files`
need to list.

In Go these operations are `fs.ReadDir`, `fs.ReadFile` and `fs.Stat`
over an `fs.FS`. Any `fs.FS` will do: a directory, an `embed.FS`, a zip
archive, or an adapter over a remote store.

## Paths

Every path given to a source, and every path the skill tool accepts
from the model, MUST be:

- separated by forward slashes, whatever the host's separator;
- relative to the root it is resolved against, which is the skill
  directory for resources and the source for discovery;
- free of empty elements and of `.` and `..` elements, except for the
  path `.` that names the root itself;
- not absolute: no leading slash.

This is Go's `fs.ValidPath`. A path that breaks a rule is refused before
the source sees it, so no spelling of a path can name something above
the root. A path naming a directory is refused when a file was asked
for.

## Symlinks

The source does not define the boundary on its own. A symlink inside a
skill can name any file the reading process can open, and a skill tree
may have been written by someone the user does not trust. A skill that
ships `reference.md` as a link to `~/.ssh/id_ed25519` turns the skill
tool into a way to read that key. The key goes into the transcript,
the session file, and anywhere the session is exported.

So a source over a real filesystem MUST apply this rule:

> A path that resolves, through any symlink in any of its elements, to
> a place outside the source's root MUST be refused, for every
> operation: list, read and stat.

Resolving means following every link to the real path and comparing it
with the root's real path. Checking the path's text is not enough,
because the link is what leaves the root. The root is itself resolved
once, when the source is made, so a root reached through a link is not
a mistake.

A path that does not exist passes the check, so the operation reports
"not found" as usual. A refused path looks like a missing one to the
model: the skill tool answers that the skill has no such file.

A link that resolves inside the root is followed. `Discover` treats a
child that links to a directory inside the source as a skill directory.
A skill directory that is itself a link elsewhere is outside the root
and refused. To serve it, the product adds the link's target as its own
source.

`Dir` is this module's source over a local directory. It wraps
`os.DirFS`, which does not apply the rule, with a check on every method
that takes a path. A refusal returns `ErrOutside`. A source with no
symlinks, such as an `embed.FS`, a zip archive or an `fstest.MapFS`,
needs no guard. A source over a remote store MUST apply the same rule
to whatever the store has that can point outside a tree.

Known gap: `Skill.Files` lists a link that the guard will refuse to
read, so the tool offers a path it then refuses (#26). No content
leaks; only the link's name is shown.
