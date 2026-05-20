# Agent Skill Installer Design

## Problem

RepoBridge has a project skill under `skills/repobridge`, but users still need to copy it into each agent's local skill directory manually. Manual setup is easy to skip, hard to reproduce, and risky when an existing local skill file already contains user changes.

Feature 27 originally mentions MCP configuration, but the approved scope for this implementation is skill installation only. MCP server setup, MCP tool registration, and `serve --mcp` configuration are out of scope.

## Goal

Add a CLI command that installs the bundled RepoBridge skill into supported agent skill directories:

```bash
repobridge install-agent --target codex
repobridge install-agent --target claude
repobridge install-agent --target cursor
repobridge install-agent --target opencode
repobridge install-agent --target all --version v0.3.0
```

The command should support dry-run and print-only modes so users and automation can inspect the rendered files without writing to disk.

## Scope

In scope:

- `install-agent` Cobra command.
- Target selection for `codex`, `claude`, `cursor`, `opencode`, and `all`.
- Rendering the bundled `skills/repobridge/SKILL.md` and `skills/repobridge/agents/openai.yaml`.
- Adding visible pinned-version metadata to rendered skill content when `--version` is supplied.
- Controlled writes with idempotent behavior.
- Backups before replacing existing non-identical files.
- `--dry-run` output with planned actions and paths.
- `--print-config` output with rendered target files and no writes.
- Unit tests for target selection, rendering, idempotence, dry-run, print-only mode, and backups.

Out of scope:

- MCP server implementation.
- MCP client configuration for any agent.
- Installing or downloading the RepoBridge binary.
- Modifying shell startup files or `PATH`.
- Detecting secrets inside agent configuration files. The installer should avoid printing file contents from existing user files.

## Agent Targets

The installer writes the RepoBridge skill as a normal skill folder for each supported target:

| Target | Default destination |
|---|---|
| `codex` | `$HOME/.agents/skills/repobridge` |
| `claude` | `$HOME/.claude/skills/repobridge` |
| `cursor` | `$HOME/.cursor/skills/repobridge` |
| `opencode` | `$HOME/.config/opencode/skills/repobridge` |

The implementation should support an internal base-home override for tests so unit tests never write to a real home directory.

## CLI

Command:

```bash
repobridge install-agent --target <target> [--version <version>] [--dry-run] [--print-config]
```

Flags:

- `--target`: required target. Allowed values: `codex`, `claude`, `cursor`, `opencode`, `all`.
- `--version`: optional pinned RepoBridge version string to document inside rendered skill files.
- `--dry-run`: calculate and report actions without writing.
- `--print-config`: print rendered files without writing.

`--dry-run` and `--print-config` are both non-writing modes. If both are supplied, `--print-config` wins because it is the more detailed output.

## Rendering

The renderer reads templates from the repository's bundled `skills/repobridge` directory. For each target it emits:

- `SKILL.md`
- `agents/openai.yaml` when the source file exists

When `--version` is non-empty, `SKILL.md` receives a small generated metadata block near the top:

```markdown
## Installed Version

This skill was installed by RepoBridge for version `<version>`.
```

When no version is supplied, no version block is added.

The installer must not render MCP instructions or MCP command snippets.

## Write Behavior

For each rendered file:

1. If the destination file does not exist, create parent directories and write it.
2. If the destination file already has identical content, report it as unchanged.
3. If the destination file exists and differs, copy it to `<filename>.bak` before replacing it.
4. If a backup path already exists, append a numeric suffix such as `.bak.1`.

The command output should report only target names, paths, and action names. It should not print existing file contents.

## Architecture

Create `internal/agentinstall` for the installer logic:

- target definitions and validation
- source template discovery
- rendering
- dry-run planning
- idempotent writes and backups

The CLI command in `internal/cli` should only parse flags, call the installer, and print results. This keeps filesystem behavior independently testable.

## Error Handling

- Unknown targets return a clear error.
- Missing bundled `SKILL.md` returns a clear error.
- Filesystem write or backup failures return errors with the affected path.
- `--print-config` should fail if rendering fails, but it should never inspect or write destination files.

## Testing

Use Go's built-in `testing` package:

- Target validation expands `all` to all concrete targets.
- Rendering includes version metadata only when requested.
- `--print-config` prints rendered files and writes nothing.
- `--dry-run` reports planned creates/replaces and writes nothing.
- Apply creates missing files.
- Apply is idempotent for identical files.
- Apply backs up and replaces non-identical files.
- CLI tests verify flags and output through Cobra writers.

## External References

Context7 Cobra guidance confirmed that local flags, `RunE`, `Args` validation, and command output writers are the right patterns for this codebase.
