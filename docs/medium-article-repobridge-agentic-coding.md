---
title: "Warum AI-Agenten Quellcode brauchen: Context Engineering mit RepoBridge"
subtitle: "Ein Agent soll React, FastAPI oder Cobra verstehen - aber welches Wissen bekommt er wirklich?"
tags: [AI Agents, Context Engineering, Developer Tools, Source Code, Agentic Coding]
---

# Warum AI-Agenten Quellcode brauchen

*Ein Agent kennt die Namen deiner Frameworks. Aber kennt er auch den Code,
auf dem dein Projekt wirklich laeuft? Genau hier beginnt Context Engineering.*

---

Viele Fehler von Coding-Agenten entstehen nicht beim Tippen von Code. Sie
entstehen frueher: Der Agent muss entscheiden, welchem Wissen er vertraut.

Ein typischer Auftrag klingt harmlos:

> "Fuege ein neues Feature hinzu und nutze die bestehenden Framework-Patterns."

Das Projekt verwendet bekannte Bausteine:

```text
react
vite
github.com/spf13/cobra
fastapi
org.springframework:spring-core
Newtonsoft.Json
```

Ein erfahrener Entwickler wuerde jetzt nicht aus dem Gedaechtnis loslegen.
Er wuerde Projektmuster suchen, Versionen pruefen und bei Bedarf in den
Source Code der Libraries springen.

Ein AI-Agent arbeitet dagegen oft aus Trainingswissen, kurzen Docs-Snippets
oder zufaellig vorhandenem Prompt-Kontext. Das funktioniert manchmal. Bis es
zu Versionsfehlern, falschen Imports oder erfundenen Framework-Patterns fuehrt.

**RepoBridge** setzt genau dort an: Es macht die echten Source Trees der
verwendeten Frameworks und Libraries lokal auffindbar, damit Agenten sie wie
Projektcode durchsuchen koennen.

---

## Zwei Codebasen, ein Problem

In AI-Coding-Demos sieht es oft so aus, als gaebe es nur eine Codebasis: dein
Projekt. In realen Projekten gibt es aber immer zwei Ebenen.

### 1. Dein Projektcode

Das sind Components, Services, CLI-Commands, Tests, Konfiguration und
Domain-Logik. Hier ist ein Agent meistens stark: Er kann Dateien lesen,
Symbole suchen, Aenderungen schreiben und Tests ausfuehren.

### 2. Der Code unter deinem Projekt

React rendert deine UI. Vite baut deine App. Cobra strukturiert deine CLI.
FastAPI verarbeitet Requests. Spring loest Dependencies auf. Newtonsoft.Json
entscheidet, wie JSON serialisiert wird.

Dieser Code liegt selten direkt im Repository. Er steckt in Package-Caches,
Registries, Source-Artefakten oder GitHub-Repos. Wenn ein Agent diese Ebene
nicht lesen kann, arbeitet er mit einer unscharfen Vorstellung davon, wie dein
Projekt wirklich funktioniert.

**Context Engineering** heisst deshalb nicht: mehr Text in den Prompt. Es
heisst: dem Agenten die richtigen Quellen und Suchwege geben.

---

## Was RepoBridge macht

RepoBridge ist ein kleines Go-CLI-Tool. Die Idee:

> Aus Package- oder Repository-Spezifikationen werden stabile lokale
> Source-Pfade.

Einzelne Quellen lassen sich direkt aufloesen:

```bash
repobridge path react
repobridge path pypi:requests==2.32.3
repobridge path crates:serde@1.0.217
repobridge path maven:org.jetbrains.kotlin:kotlin-stdlib@2.1.0
repobridge path nuget:Newtonsoft.Json@13.0.3
repobridge path github.com/spf13/cobra
```

Fuer Agentic Coding ist der Projektscan wichtiger:

```bash
repobridge scan --cwd . --json
repobridge scan --cwd . --fetch --limit 10
```

RepoBridge liest Manifest-/Lockfiles und Imports, zum Beispiel:

- `package.json`
- `package-lock.json`
- `requirements.txt`
- `go.mod`
- `Cargo.toml`
- `pom.xml`
- `.csproj`
- JavaScript-/TypeScript-Imports
- Go-Imports

Daraus entstehen Kandidaten:

```json
{
  "spec": "react@19.0.0",
  "ecosystem": "npm",
  "confidence": 96,
  "reasons": ["package-lock.json direct dependency"],
  "files": ["package-lock.json"]
}
```

Der Agent kann danach gezielt suchen:

```bash
REACT_PATH="$(repobridge path --cwd . react)"
rg "useSyncExternalStore" "$REACT_PATH"
```

Das ist der Unterschied: Der Agent bekommt nicht nur eine Beschreibung von
React. Er bekommt einen lokalen, versionierten, durchsuchbaren Source Tree.

---

## Der Agent muss nicht alles wissen

Viele Diskussionen ueber Coding-Agenten drehen sich um Modellgroesse:

- Kennt das Modell die Library?
- Ist sein Wissen aktuell?
- Reicht das Kontextfenster?

Das ist relevant, aber nicht genug.

Ein Agent, der "React kennt", weiss nicht automatisch, welche React-Version
in deinem Projekt installiert ist. Er weiss auch nicht, welches Pattern dein
Projekt nutzt oder ob ein Blog-Beispiel noch zur aktuellen API passt.

Ein guter Agent muss nicht alles im Modellkopf tragen. Er muss richtig
nachschlagen koennen.

Das ist normale Entwicklerarbeit: Niemand merkt sich jede Library komplett.
Gute Entwickler wissen, wo sie suchen muessen. RepoBridge macht diesen Schritt
agentenfreundlich.

---

## Der kompakte Workflow

Der Skill laesst sich so installieren:

```bash
npx skills add agentsw0rk/repobridge
```

Danach kann ein Agent diesen Ablauf nutzen:

```bash
repobridge scan --cwd . --json
repobridge scan --cwd . --fetch --limit 10
repobridge path --cwd . react
```

Was passiert dabei?

1. **Projektwurzel finden.** Dependency-Kontext ist immer projektbezogen.
2. **Manifest- und Lockfile-Signale lesen.** Versionen schlagen Allgemeinwissen.
3. **Imports als Zusatzsignal nutzen.** Nicht jede transitive Dependency ist relevant.
4. **Source Code holen.** RepoBridge cached Source Snapshots lokal.
5. **Lesen und suchen.** Der Agent nutzt `rg`, Dateizugriff und Symbolsuche.

Beispiel:

```bash
rg "createRoot|useEffect" "$(repobridge path --cwd . react)"
rg "PersistentPreRunE" "$(repobridge path --cwd . github.com/spf13/cobra)"
```

Jetzt kann der Agent die Library wie Projektcode lesen. Er muss nicht raten,
wie das Framework arbeitet.

---

## Warum Context Engineering wichtiger ist als Prompt Engineering

Prompt Engineering fragt:

> Wie formuliere ich die Aufgabe, damit das Modell gut antwortet?

Context Engineering fragt:

> Welche Quellen, Werkzeuge und Zugriffswege braucht der Agent, damit er die
> Aufgabe wirklich loesen kann?

In Softwareentwicklung ist die zweite Frage oft wichtiger.

Ein guter Prompt hilft wenig, wenn der Agent die falsche Version einer Library
annimmt. Ein kurzer Prompt reicht eher, wenn der Agent den echten Source Code
lesen kann.

Schlechter Kontext:

```text
Nutze React und implementiere es sauber.
```

Besserer Kontext:

```bash
REACT_PATH="$(repobridge path --cwd . react)"
rg "useSyncExternalStore" "$REACT_PATH"
```

Kontext ist dann nicht nur Text. Kontext ist eine navigierbare Ressource.

---

## Welche Signale zaehlen

RepoBridge ist kein Observability-Dashboard, aber der Workflow liefert
wichtige Hinweise:

| Signal | Warum es wichtig ist |
| --- | --- |
| Candidates | Welche Dependencies wurden erkannt? |
| Confidence | Wie stark ist das Signal? |
| Files | Woher stammt die Erkennung? |
| Fetch limit | Wie viel Kontext wird wirklich geholt? |
| Path resolution | Kann der Agent den Source Tree stabil wiederfinden? |
| rg hits | Findet der Agent die relevante Stelle im Code? |

Diese Signale helfen, Kontext bewusst zu steuern. Ein Agent braucht nicht den
kompletten Dependency-Baum. Er braucht die richtigen Quellen fuer die aktuelle
Aufgabe.

---

## Warum nicht einfach Dokumentation?

Dokumentation bleibt wichtig. Aber Dokumentation ist nicht Source Code.

Docs zeigen Absicht, Beispiele und Happy Paths. Source Code zeigt Verhalten,
Default-Werte, Tests, Edge Cases und echte Implementierungsdetails.

Gerade fuer Agenten macht das einen Unterschied:

- Docs passen nicht immer zur installierten Version.
- Beispiele zeigen selten Grenzfaelle.
- Interne Defaults stehen oft nur im Code.
- Tests erklaeren Verhalten, das in Guides nicht auftaucht.

Das Ziel ist nicht, Dokumentation zu ersetzen. Das Ziel ist, die Luecke
zwischen Dokumentation, Projektcode und tatsaechlicher Implementierung zu
schliessen.

---

## Was du mitnehmen kannst

1. **Kontext ist ein Designproblem.** Gute Agenten brauchen Dateien, Befehle,
   Suchwerkzeuge, Versionswissen und klare Grenzen.

2. **Versionen schlagen Allgemeinwissen.** "React" ist zu ungenau. "React in
   der Version dieses Projekts" ist handlungsfaehiger Kontext.

3. **Nicht alles ist Kontext.** Ein fokussierter Source Tree ist besser als
   ein riesiger Dump transitive Dependencies.

4. **Suchbarkeit zaehlt.** Ein Pfad plus `rg` ist oft wertvoller als tausend
   Tokens Beschreibung.

5. **Read-only schuetzt.** Dependency Source sollte Referenz sein, nicht
   Patch-Ziel.

---

## Schluss: Weniger Raten, mehr Quellen

Agentic Coding wird nicht verlaesslicher, weil Prompts laenger werden. Es wird
verlaesslicher, wenn Agenten in einer Entwicklungsumgebung arbeiten, die ihnen
die richtigen Quellen zur richtigen Zeit gibt.

RepoBridge ist ein kleiner Baustein dafuer: Es scannt ein Projekt, erkennt
verwendete Frameworks und Libraries, holt deren Source Code lokal und macht
ihn durchsuchbar.

Gute Softwareentwicklung ist selten ein Ratespiel. Sie ist Recherche,
Verstehen, Aendern und Testen.

Ein AI-Agent sollte genauso arbeiten.

---

*Wenn dir der Artikel gefallen hat, lass ein Klatschen da. Mich wuerde
interessieren: Welche Quellen gibst du deinen Coding-Agenten heute schon -
und wo raten sie noch zu oft?*
