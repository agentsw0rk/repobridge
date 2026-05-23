# RepoBridge Self-Update Design

## Goal

RepoBridge should help users stay on the latest released version without disrupting scripted, LLM, or agent-driven invocations. Every normal command should opportunistically check whether a newer GitHub release exists. Human users at an interactive terminal may approve an update inline; non-interactive callers must never block on a prompt.

## User Experience

For normal commands, RepoBridge performs a lightweight update check before command execution when all of these are true:

- The current build version is a release version, not `dev`.
- Update checks are not disabled with `REPOBRIDGE_NO_UPDATE_CHECK=1`.
- The command is not `--version`, `self-update`, or an internal hidden command such as `__astgraph-index`.
- The last recorded check is older than the configured interval, initially 24 hours.

If the latest release is newer and the process is attached to an interactive terminal, RepoBridge prompts on stderr:

```text
RepoBridge v0.10.5 is available. Update from v0.10.4 now? [y/N]
```

The default is no. Pressing Enter, answering anything other than yes, or failing to receive input leaves the running command untouched. RepoBridge then executes the originally requested command.

If the latest release is newer but the process is not interactive, RepoBridge does not ask. It writes at most one short stderr hint and continues:

```text
RepoBridge v0.10.5 is available; run `repobridge self-update`.
```

Network failures, malformed release responses, missing assets, checksum failures during the check phase, and cache write failures must not fail the user's original command. They are silent by default during opportunistic checks, except for the available-update hint.

## Commands And Flags

Add a public command:

```text
repobridge self-update
```

`self-update` always performs an explicit update check and may return errors. It supports:

- `--check-only`: reports whether an update is available and exits without modifying files.
- `--yes`: installs without prompting. This is intended for scripts and agents that explicitly choose update behavior.
- `--force`: reinstalls the current latest version even if it matches the running version. This is useful for repairing a partial install.

The existing `--version` behavior remains unchanged and never triggers update checks.

## Architecture

Introduce an `internal/updatecheck` package with these responsibilities:

- Compare semantic release tags such as `v0.10.4` and `0.10.4`.
- Query `https://api.github.com/repos/agentsw0rk/repobridge/releases/latest`.
- Select the matching asset for `runtime.GOOS` and `runtime.GOARCH`.
- Download and parse `checksums.txt`.
- Verify the selected archive's SHA256 before installation.
- Extract the release archive to a temporary directory.
- Locate the currently running executable with `os.Executable`.
- Replace the executable, and keep bundled native library files such as `libobjectbox.*` or `objectbox.dll` next to it.

The CLI package owns when checks are invoked. It should call a small update-check hook from Cobra pre-run setup after command resolution but before the target command's `RunE`. The hook must be injectable in tests so command tests do not perform real network calls.

## State And Caching

Persist update-check state in the existing RepoBridge home/cache area. The state file should contain:

- Last successful or attempted check timestamp.
- Latest version observed.
- Optional latest release URL.

The state file is best-effort. If it cannot be read or written, RepoBridge should continue normally.

The initial interval is 24 hours. The interval can be overridden in tests through package-level options or injected clocks, not through a user-facing flag.

## Interactivity Rules

Interactive prompting is allowed only when stdin is a terminal and stderr or stdout is a terminal. Non-interactive detection protects these cases:

- Agent tools invoking `repobridge` and capturing stdout/stderr.
- Shell scripts.
- CI jobs.
- Commands with piped input or redirected output.

Prompt text should go to stderr. Normal command output should remain clean for machine consumers.

## Installation Details

Release artifacts are named like:

```text
repobridge_v0.10.4_linux_amd64.tar.gz
repobridge_v0.10.4_darwin_arm64.tar.gz
repobridge_v0.10.4_windows_amd64.zip
```

The updater selects the current OS and architecture. Unsupported platforms should report a clear error for explicit `self-update`, and silently skip opportunistic prompts.

For Unix-like systems, installation should write files to a temporary directory, verify the extracted `repobridge` binary exists, chmod it executable, then replace the current executable path. For Windows, installation should use a rename-and-replace strategy that works within Windows file locking constraints where possible; if direct replacement fails, the command reports manual instructions.

If the current executable path is not writable, `self-update` should fail with a concise permission message and the release URL. Opportunistic update prompts should only offer installation when the command can reasonably attempt it; permission errors are shown only after the user approves.

## Error Handling

Opportunistic checks must be fail-open. They must not change exit status, stdout data, or normal command semantics.

Explicit `self-update` must fail closed. It returns non-zero for network errors, missing platform assets, checksum mismatches, extraction failures, or install failures.

Checksum mismatches must never install files.

## Testing

Use local fake HTTP servers and injected CLI/update dependencies. Do not call GitHub in tests.

Coverage should include:

- Version comparison with stable, equal, older, newer, malformed, and `dev` versions.
- Skipping checks for `--version`, `self-update`, hidden internal commands, disabled env var, and fresh cache.
- Non-interactive command invocation never prompts.
- Interactive invocation prompts only when a newer release exists.
- Prompt decline continues the original command.
- `self-update --check-only` reports current and outdated states.
- Asset selection for Linux, macOS, and Windows naming.
- Checksum verification accepts matching hashes and rejects mismatches.
- Installation uses temporary files and does not replace the current binary when verification fails.

## Out Of Scope

- Package-manager integration such as Homebrew, npm, or apt.
- Background daemon checks.
- Automatic updates without an explicit user prompt or `--yes`.
- Cryptographic signature verification beyond the existing release checksum and SLSA provenance assets.
