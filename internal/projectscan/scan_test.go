package projectscan

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanProjectDetectsManifestAndImportSpecs(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "package.json", `{
		"dependencies": {
			"react": "^19.0.0"
		},
		"devDependencies": {
			"vite": "~5.0.1"
		}
	}`)
	writeFile(t, root, "src/main.tsx", `import React from "react";
import { createRoot } from "react-dom/client";
import { QueryClient } from "@tanstack/react-query";
import local from "./local";
`)

	result, err := Scan(root, Options{IncludeImports: true})
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}

	assertCandidate(t, result, "react@19.0.0", "npm", "package.json dependencies")
	assertCandidate(t, result, "vite@5.0.1", "npm", "package.json devDependencies")
	assertCandidate(t, result, "react-dom", "npm", "JavaScript/TypeScript import")
	assertCandidate(t, result, "@tanstack/react-query", "npm", "JavaScript/TypeScript import")
	if hasCandidate(result, "./local") {
		t.Fatalf("relative import was reported as external candidate: %#v", result.Candidates)
	}
}

func TestScanProjectDetectsGoModuleAndImports(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", `module example.com/app

go 1.22

require (
	github.com/spf13/cobra v1.9.1
	github.com/stretchr/testify v1.10.0
)
`)
	writeFile(t, root, "main.go", `package main

import (
	"fmt"

	"github.com/spf13/cobra"
)
`)

	result, err := Scan(root, Options{IncludeImports: true})
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}

	assertCandidate(t, result, "github.com/spf13/cobra@v1.9.1", "go", "go.mod require")
	assertCandidate(t, result, "github.com/stretchr/testify@v1.10.0", "go", "go.mod require")
	assertCandidate(t, result, "github.com/spf13/cobra", "go", "Go import")
	if hasCandidate(result, "fmt") {
		t.Fatalf("standard library import was reported as external candidate: %#v", result.Candidates)
	}
}

func TestScanProjectDetectsPackageLockAndOtherManifestSpecs(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "package-lock.json", `{
		"packages": {
			"": {
				"dependencies": {
					"zod": "^3.22.4"
				}
			},
			"node_modules/zod": {
				"version": "3.22.4"
			}
		}
	}`)
	writeFile(t, root, "requirements.txt", "fastapi==0.115.0\n")
	writeFile(t, root, "Cargo.toml", `[dependencies]
serde = "1.0.217"
`)
	writeFile(t, root, "pom.xml", `<project>
	<dependencies>
		<dependency>
			<groupId>org.jetbrains.kotlin</groupId>
			<artifactId>kotlin-stdlib</artifactId>
			<version>2.1.0</version>
		</dependency>
	</dependencies>
</project>`)
	writeFile(t, root, "App.csproj", `<Project Sdk="Microsoft.NET.Sdk">
	<ItemGroup>
		<PackageReference Include="Newtonsoft.Json" Version="13.0.3" />
	</ItemGroup>
</Project>`)

	result, err := Scan(root, Options{})
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}

	assertCandidate(t, result, "zod@3.22.4", "npm", "package-lock.json direct dependency")
	assertCandidate(t, result, "pypi:fastapi==0.115.0", "pypi", "requirements.txt")
	assertCandidate(t, result, "crates:serde@1.0.217", "crates", "Cargo.toml dependency")
	assertCandidate(t, result, "maven:org.jetbrains.kotlin:kotlin-stdlib@2.1.0", "maven", "pom.xml dependency")
	assertCandidate(t, result, "nuget:Newtonsoft.Json@13.0.3", "nuget", ".csproj PackageReference")
}

func writeFile(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertCandidate(t *testing.T, result Result, spec, ecosystem, reason string) {
	t.Helper()
	for _, candidate := range result.Candidates {
		if candidate.Spec != spec {
			continue
		}
		if candidate.Ecosystem != ecosystem {
			t.Fatalf("candidate %s ecosystem = %q, want %q", spec, candidate.Ecosystem, ecosystem)
		}
		for _, gotReason := range candidate.Reasons {
			if gotReason == reason {
				return
			}
		}
		t.Fatalf("candidate %s reasons = %#v, want %q", spec, candidate.Reasons, reason)
	}
	t.Fatalf("candidate %s not found in %#v", spec, result.Candidates)
}

func hasCandidate(result Result, spec string) bool {
	for _, candidate := range result.Candidates {
		if candidate.Spec == spec {
			return true
		}
	}
	return false
}
