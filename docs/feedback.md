# Feedback from the design studies

Findings the design studies under `../examples` raised against this
module, held here because it has no issue tracker yet. Each section is
one finding in the shape of an issue: why it matters, the evidence, a
failure scenario and the smallest fix. The plan in `plans/` is the
document these correct; apply them there before the code is written,
and move each section to an issue when a repository exists.

Generated 2026-09-20 from the studies' issue drafts.

## 1. Rules: allowed-tools is split on whitespace, so a specifier with a space cannot parse

*Applied 2026-09-20: `rules.go` splits at whitespace outside
parentheses and the plan's validation and `allowed-tools` sections say
so; the `allowed-tools-spaces` fixture carries the documented line
through the interop diff.*

### Why this is necessary

`allowed-tools` exists so a skill can pre-approve the tools it needs. The
reference's own documented example for the field uses specifiers that
contain spaces, and the field is split on whitespace, so that example
does not parse: `Rules` returns an error and `Validate` reports the skill
as broken. A product loading a skill written against the reference either
refuses it or gets no rules at all, and the user is prompted for every
command the skill was written to pre-approve. Any skill that follows the
published guidance hits this.

### Evidence

The module is not yet under version control, so these are against its
current tree. `Skill.Rules` splits with `strings.Fields(s.AllowedTools)`
(`rules.go:33`), `parseRule` rejects a token whose parenthesis is
unmatched (`rules.go:48-62`), and `Validate` turns that error into an
`Error` problem on the `allowed-tools` field (`validate.go:60-64`), which
fails validation for the CLI and for strict discovery. The plan specifies
the same behaviour: "each space-separated token parses as a rule"
(`docs/plans/skill-layer.md:173`).

Claude Code documents this field with the example
`allowed-tools: Bash(git add *) Bash(git commit *) Bash(git status *)`,
and states that "Parentheses inside the specifier are literal, so a
command or path that contains them needs no escaping", with
`Edit(./Finance (2024)/**)` given as a path that needs no escaping. So
both spaces and parentheses are expected inside a specifier.

A probe that parses that frontmatter through this module reports
`Skill.Rules() = [], err = allowed-tools token "Bash(git": unmatched
parenthesis` and one `Error` problem from `Validate`.

### Failure scenario

A repository ships a `commit` skill whose frontmatter is the documented
line above. `Load` succeeds, `Validate` returns one error, and the skill
is reported as broken by the CLI and skipped by strict discovery. A
product that ignores the error calls `Rules`, gets nil, applies no
grants, and prompts the user for `git add`, `git commit` and `git status`
every time the skill runs.

### Suggested fix

In `rules.go`, split on whitespace outside parentheses instead of
`strings.Fields`: track parenthesis depth and end a token only at
whitespace at depth zero. A probe shows that splitter returning the three
intended rules from the documented field.

This must not start interpreting the specifier. The plan is right that
"the specifier's syntax belongs to the product", so the change is purely
where a token ends. Unbalanced parentheses should stay an error, since a
specifier cannot then be delimited unambiguously.

Found by the codex-permissions design study against Codex CLI and Claude Code.

## 2. instructions: Chain takes every name per directory and caps bytes per file

*Applied 2026-09-20: `Names` is a per-directory preference order and
`Options.Budget` ends the chain without error; the plan's
`instructions` section says so. `Budget` defaults to none rather than
the reference's 32 KiB, because this package does not truncate and a
default that drops whole files is the product's call. Neither
omission is silent: `Chain` returns a `Result` whose `Omitted` lists
every file found and left out, shadowed or over budget, with its
size, so the product and the session know what the model was not
given.*

### Why this is necessary

`Chain` implements the AGENTS.md convention, whose point is that a nearer
file refines or replaces a farther one. The reference takes at most one
file per directory, preferring an override name over the base name, so a
developer can shadow a committed file without deleting it. `Chain`'s
`Names` collects every name it finds in a directory, so both go into the
prompt and the shadowing does not happen. Its byte limit is the wrong shape
too, a per-file cap whose breach is an error, where the reference has a
small total budget and stops adding files when it is reached.

### Evidence

The module is not yet under version control, so this is against the plan
and the current tree. `docs/plans/skill-layer.md:303-309` defines `Chain`
with `Names []string // default: AGENTS.md` and `MaxBytes int64 // per
file; default 1 MiB, larger files are an error`.

Codex reads, per directory, `AGENTS.override.md`, then `AGENTS.md`, then
`project_doc_fallback_filenames`, and "Codex includes at most one file per
directory", with a global `~/.codex/AGENTS.md` or its override first and
files concatenated "from the root down". On the budget its own docs are
inconsistent. The AGENTS.md page says Codex "stops adding files once the
combined size reaches the limit defined by `project_doc_max_bytes` (32 KiB
by default)", while the configuration reference describes the same key per
file. Either reading disagrees with a 1 MiB per-file error. Codex also
treats the name list as closed: "Filenames not on this list are ignored
for instruction discovery."

### Failure scenario

A repository root holds both `AGENTS.md` and `AGENTS.override.md`, the
override written to replace the committed guidance for one developer.
`Chain` returns both, so the overridden guidance is still in the prompt and
whichever name is later in `Names` wins by position rather than intent.
Separately, a monorepo with a 40 KiB root `AGENTS.md` fails to load where
the reference would have included what fits.

### Suggested fix

Make `Names` a per-directory preference list, first match wins, and add a
cumulative budget beside the per-file cap:

```go
type Options struct {
    Names    []string // tried in order per directory; first found wins
    Root     string
    Extra    []string
    MaxBytes int64 // per file; a larger file is an error
    Budget   int64 // stop adding files once the total reaches this
}
```

This must not change the root-down order, which is already right, and must
not make a breached `Budget` an error. The reference stops adding and keeps
what it has, so a large file deep in a tree degrades the prompt rather than
failing the run.

Found by the codex-permissions design study against Codex CLI and Claude Code.

