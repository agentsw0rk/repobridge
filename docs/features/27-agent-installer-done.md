# Feature 27 Done: Agent Installer

## Zusammenfassung

RepoBridge hat jetzt `install-agent`, um den gebuendelten RepoBridge-Skill fuer Codex, Claude, Cursor und opencode zu installieren. Der Installer unterstuetzt `--target all`, `--dry-run`, `--print-config` und optionale Versionsmetadaten ueber `--version`.

Die Implementierung liegt in `internal/agentinstall` und wird ueber `internal/cli` als Cobra-Kommando angebunden. Bestehende nicht identische Dateien werden vor dem Ersetzen mit `.bak` oder einem nummerierten Backup gesichert. Identische Dateien bleiben unveraendert.

## Abweichungen

Der urspruengliche Feature-Text nennt MCP-Konfiguration. Diese Implementierung installiert bewusst nur Skills und Agent-Instructions. MCP-Konfiguration und `serve --mcp` bleiben ausserhalb des Scopes.

## Offene Punkte

- MCP-Agent-Konfiguration kann spaeter nach Feature 23 separat ergaenzt werden.
- Zielpfade fuer Cursor und opencode sind konservative Skill-Verzeichnisse und koennen angepasst werden, wenn diese Agenten verbindliche Skill-Spezifikationen stabilisieren.
