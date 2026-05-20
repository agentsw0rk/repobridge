# Gradle Project Scan Done

## Zusammenfassung

`repobridge scan` erkennt jetzt direkte Maven-Abhaengigkeiten aus `build.gradle` und `build.gradle.kts`. Der Scanner arbeitet statisch, fuehrt keine Gradle-Skripte aus und erzeugt RepoBridge-Specs im Format `maven:<groupId>:<artifactId>@<version>`.

Unterstuetzt werden direkte String-Koordinaten in ueblichen Gradle-Konfigurationen wie `implementation`, `api`, `compileOnly`, `runtimeOnly`, `testImplementation`, `testRuntimeOnly`, `annotationProcessor` und `kapt`. Groovy-Map-Notation wie `group: "...", name: "...", version: "..."` wird ebenfalls erkannt.

## Abweichungen vom Feature-Wunsch

Version Catalogs aus `gradle/libs.versions.toml` werden in dieser Iteration bewusst nicht aufgeloest. Dynamische Versionen wie `$postgresVersion` oder `${commonsLangVersion}` werden ignoriert, weil der Scanner keine Build-Skripte ausfuehrt.

## Offene Punkte

- Version-Catalog-Aufloesung kann spaeter als separates Feature ergaenzt werden.
- Mehrzeilige Gradle-Dependency-Deklarationen sind nicht im aktuellen Scope.

