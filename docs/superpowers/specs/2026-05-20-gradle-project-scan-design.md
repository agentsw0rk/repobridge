# Gradle Project Scan Design

## Problem

`repobridge scan` erkennt Maven-Abhaengigkeiten aus `pom.xml`, aber Gradle-Projekte mit `build.gradle` oder `build.gradle.kts` liefern aktuell keine Maven-Kandidaten. Dadurch fehlen haeufige JVM/Kotlin-Projekte im Project-Scan.

## Scope

Diese Iteration erkennt nur direkte, statisch angegebene Maven-Koordinaten in:

- `build.gradle`
- `build.gradle.kts`

Nicht enthalten:

- `gradle/libs.versions.toml` Version Catalog Aufloesung
- dynamische Variablen wie `$kotlinVersion` oder `${versions.kotlin}`
- Ausfuehren von Gradle oder Build-Skripten

## Design

Die bestehende `internal/projectscan`-Struktur wird erweitert. `Scan` ruft eine neue `scanGradle`-Methode auf, die Gradle-Build-Dateien ohne Ausfuehrung liest und klare Dependency-Deklarationen in RepoBridge-Specs uebersetzt.

Erkannte Formen:

- Kotlin DSL: `implementation("org.jetbrains.kotlin:kotlin-stdlib:2.1.0")`
- Groovy DSL: `implementation "com.google.guava:guava:33.4.0-jre"`
- Wrapper mit String-Koordinaten: `api(platform("org.springframework.boot:spring-boot-dependencies:3.4.1"))`
- Map-Notation: `implementation group: "org.slf4j", name: "slf4j-api", version: "2.0.16"`

Unterstuetzte Konfigurationen:

- `implementation`
- `api`
- `compileOnly`
- `runtimeOnly`
- `testImplementation`
- `testRuntimeOnly`
- `annotationProcessor`
- `kapt`

Jede erkannte Abhaengigkeit wird als `maven:<groupId>:<artifactId>@<version>` mit Ecosystem `maven` hinzugefuegt. Der Grund ist `Gradle dependency`.

## Safety

Der Scanner fuehrt keine Gradle-Dateien aus. Er akzeptiert nur String-Koordinaten mit drei statischen Teilen und lehnt dynamische Versionen, Projekt-Abhaengigkeiten, lokale Dateien und unvollstaendige Angaben ab.

## Tests

Tests erweitern `internal/projectscan/scan_test.go` um:

- `build.gradle.kts` mit Kotlin-DSL-Stringnotation, Wrappern und dynamischer Version, die ignoriert wird
- `build.gradle` mit Groovy-Stringnotation und Map-Notation
- Sicherstellung, dass `project(...)`, `files(...)` und dynamische Versionen keine Kandidaten erzeugen

