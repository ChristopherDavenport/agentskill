# Changelog

All user-visible changes to this library. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project
uses [Semantic Versioning](https://semver.org/); before v1.0.0 minor
versions may break the API.

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
