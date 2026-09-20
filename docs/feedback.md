# Feedback from the design studies

Findings the design studies under `../examples` raised against this
module. Each was held here as an issue draft until the repository
existed, applied to the plan and the code before the first commit, and
is now filed where the discussion belongs:

- [#1](https://github.com/ChristopherDavenport/agentskill/issues/1)
  Rules: `allowed-tools` was split on whitespace, so the documented
  `Bash(git add *)` could not parse. Tokens now end at whitespace
  outside parentheses.
- [#2](https://github.com/ChristopherDavenport/agentskill/issues/2)
  instructions (now `agentsmd`): `Chain` took every name per directory and capped bytes
  per file. `Names` is now a preference order, one file per directory;
  `Budget` caps the total without error; `Result.Omitted` reports every
  file found and left out.

New findings go straight to the issue tracker.
