package lockfile

import (
	"strings"
)

type pnpmNode struct {
	name    string
	version string
	deps    []string
}

type pnpmGraph struct {
	nodes map[string]*pnpmNode
	roots []string
}

type pnpmOrigin int

const (
	pnpmRoot pnpmOrigin = iota
	pnpmImporter
)

type pnpmFrameKind int

const (
	pnpmFrameRoot pnpmFrameKind = iota
	pnpmFrameImporters
	pnpmFrameImporter
	pnpmFrameDepGroup
	pnpmFrameDepBlock
	pnpmFramePackages
	pnpmFrameSnapshots
	pnpmFramePkgEntry
	pnpmFramePkgDeps
)

type pnpmFrame struct {
	kind    pnpmFrameKind
	base    int
	origin  pnpmOrigin
	pkgName string
	key     string
	owner   string
}

func versionFromPNPMLock(packageName, cwd string) string {
	content, err := readFile(cwd, "pnpm-lock.yaml")
	if err != nil {
		return ""
	}
	return parsePNPMLock(content, packageName)
}

func parsePNPMLock(text, packageName string) string {
	stack := []pnpmFrame{{kind: pnpmFrameRoot, base: -1}}
	graph := pnpmGraph{nodes: map[string]*pnpmNode{}}
	var importerMatch, topMatch, packagesFallback string

	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSuffix(raw, "\r")
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimLeft(line, " \t"), "#") {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " \t"))
		content := line[indent:]

		for len(stack) > 1 && indent <= stack[len(stack)-1].base {
			stack = stack[:len(stack)-1]
		}

		top := stack[len(stack)-1]
		switch top.kind {
		case pnpmFrameRoot:
			if indent != 0 || !strings.HasSuffix(content, ":") {
				continue
			}
			switch strings.TrimSpace(strings.TrimSuffix(content, ":")) {
			case "importers":
				stack = append(stack, pnpmFrame{kind: pnpmFrameImporters, base: indent})
			case "dependencies", "devDependencies", "optionalDependencies":
				stack = append(stack, pnpmFrame{kind: pnpmFrameDepGroup, base: indent, origin: pnpmRoot})
			case "packages":
				stack = append(stack, pnpmFrame{kind: pnpmFramePackages, base: indent})
			case "snapshots":
				stack = append(stack, pnpmFrame{kind: pnpmFrameSnapshots, base: indent})
			}
		case pnpmFrameImporters:
			if strings.HasSuffix(content, ":") {
				stack = append(stack, pnpmFrame{kind: pnpmFrameImporter, base: indent})
			}
		case pnpmFrameImporter:
			key, ok := strings.CutSuffix(content, ":")
			if ok && isDependencyGroup(strings.TrimSpace(key)) {
				stack = append(stack, pnpmFrame{kind: pnpmFrameDepGroup, base: indent, origin: pnpmImporter})
			}
		case pnpmFrameDepGroup:
			name, value, ok := strings.Cut(content, ":")
			if !ok {
				continue
			}
			depName := trimQuotes(strings.TrimSpace(name))
			rawValue := strings.TrimSpace(value)
			if rawValue == "" {
				stack = append(stack, pnpmFrame{kind: pnpmFrameDepBlock, base: indent, origin: top.origin, pkgName: depName})
				continue
			}
			cleaned := cleanValue(rawValue)
			stripped := stripPeerSuffix(cleaned)
			graph.roots = append(graph.roots, depName+"@"+cleaned)
			capturePNPMMatch(depName, packageName, stripped, top.origin, &importerMatch, &topMatch)
		case pnpmFrameDepBlock:
			rest, ok := strings.CutPrefix(content, "version:")
			if !ok {
				continue
			}
			cleaned := cleanValue(rest)
			stripped := stripPeerSuffix(cleaned)
			graph.roots = append(graph.roots, top.pkgName+"@"+cleaned)
			capturePNPMMatch(top.pkgName, packageName, stripped, top.origin, &importerMatch, &topMatch)
			stack = stack[:len(stack)-1]
		case pnpmFramePackages, pnpmFrameSnapshots:
			keyPart, valuePart, ok := strings.Cut(content, ":")
			if !ok {
				continue
			}
			key := trimQuotes(strings.TrimSpace(keyPart))
			name, versionWithPeer, ok := splitPNPMPackageKey(key)
			if !ok {
				continue
			}
			key = name + "@" + versionWithPeer
			version := stripPeerSuffix(versionWithPeer)
			if _, exists := graph.nodes[key]; !exists {
				graph.nodes[key] = &pnpmNode{name: name, version: version}
			}
			if name == packageName && packagesFallback == "" && isRegistryVersion(version) {
				packagesFallback = version
			}
			if strings.TrimSpace(valuePart) == "" {
				stack = append(stack, pnpmFrame{kind: pnpmFramePkgEntry, base: indent, key: key})
			}
		case pnpmFramePkgEntry:
			subKey, ok := strings.CutSuffix(content, ":")
			if ok && (strings.TrimSpace(subKey) == "dependencies" || strings.TrimSpace(subKey) == "optionalDependencies") {
				stack = append(stack, pnpmFrame{kind: pnpmFramePkgDeps, base: indent, owner: top.key})
			}
		case pnpmFramePkgDeps:
			depName, depValue, ok := strings.Cut(content, ":")
			if !ok {
				continue
			}
			cleaned := cleanValue(depValue)
			if cleaned == "" {
				continue
			}
			if node := graph.nodes[top.owner]; node != nil {
				node.deps = append(node.deps, trimQuotes(strings.TrimSpace(depName))+"@"+cleaned)
			}
		}
	}

	if importerMatch != "" {
		return importerMatch
	}
	if topMatch != "" {
		return topMatch
	}
	if version := resolvePNPMTransitive(graph, packageName); version != "" {
		return version
	}
	return packagesFallback
}

func capturePNPMMatch(depName, packageName, version string, origin pnpmOrigin, importerMatch, topMatch *string) {
	if depName != packageName || !isRegistryVersion(version) {
		return
	}
	switch origin {
	case pnpmImporter:
		if *importerMatch == "" {
			*importerMatch = version
		}
	case pnpmRoot:
		if *topMatch == "" {
			*topMatch = version
		}
	}
}

func resolvePNPMTransitive(graph pnpmGraph, packageName string) string {
	if len(graph.nodes) == 0 || len(graph.roots) == 0 {
		return ""
	}
	visited := map[string]bool{}
	queue := append([]string(nil), graph.roots...)
	for len(queue) > 0 {
		key := queue[0]
		queue = queue[1:]
		if visited[key] {
			continue
		}
		visited[key] = true
		node := graph.nodes[key]
		if node == nil {
			continue
		}
		if node.name == packageName && isRegistryVersion(node.version) {
			return node.version
		}
		for _, dep := range node.deps {
			if !visited[dep] {
				queue = append(queue, dep)
			}
		}
	}
	return ""
}

func isDependencyGroup(key string) bool {
	return key == "dependencies" || key == "devDependencies" || key == "optionalDependencies"
}

func stripPeerSuffix(version string) string {
	if before, _, ok := strings.Cut(version, "("); ok {
		return strings.TrimRight(before, " \t")
	}
	trimmed := strings.TrimRight(version, " \t")
	if before, after, ok := strings.Cut(trimmed, "_"); ok && strings.Contains(after, "@") {
		return before
	}
	return trimmed
}

func stripInlineComment(line string) string {
	if index := strings.Index(line, " #"); index >= 0 {
		return strings.TrimRight(line[:index], " \t")
	}
	return line
}

func cleanValue(value string) string {
	return trimQuotes(stripInlineComment(strings.TrimSpace(value)))
}

func trimQuotes(value string) string {
	return strings.Trim(value, `"'`)
}

func splitPackageSpec(spec string) (name string, rest string, ok bool) {
	atPos := -1
	if strings.HasPrefix(spec, "@") {
		if idx := strings.Index(spec[1:], "@"); idx >= 0 {
			atPos = idx + 1
		}
	} else {
		atPos = strings.Index(spec, "@")
	}
	if atPos < 0 {
		return "", "", false
	}
	return spec[:atPos], spec[atPos+1:], true
}

func splitPNPMPackageKey(key string) (name string, rest string, ok bool) {
	if !strings.HasPrefix(key, "/") {
		return splitPackageSpec(key)
	}

	trimmed := strings.TrimPrefix(key, "/")
	if strings.HasPrefix(trimmed, "@") || !strings.Contains(trimmed, "/") {
		if name, rest, ok := splitPackageSpec(trimmed); ok {
			return name, rest, true
		}
	}

	parts := strings.SplitN(trimmed, "/", 3)
	if len(parts) < 2 {
		return "", "", false
	}
	if strings.HasPrefix(trimmed, "@") {
		if len(parts) < 3 {
			return "", "", false
		}
		return parts[0] + "/" + parts[1], parts[2], true
	}
	return parts[0], parts[1], true
}
