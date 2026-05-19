package lockfile

import "strings"

func versionFromYarnLock(packageName, cwd string) string {
	content, err := readFile(cwd, "yarn.lock")
	if err != nil {
		return ""
	}
	return parseYarnLock(content, packageName)
}

func parseYarnLock(text, packageName string) string {
	for _, block := range yarnBlocks(text) {
		header := ""
		body := make([]string, 0, len(block))
		for _, line := range block {
			if strings.HasPrefix(strings.TrimLeft(line, " \t"), "#") {
				continue
			}
			if header == "" && !startsWithWhitespace(line) {
				header = line
			} else {
				body = append(body, line)
			}
		}
		if header == "" || strings.HasPrefix(header, "__metadata:") || !strings.HasSuffix(header, ":") {
			continue
		}
		headerBody := strings.TrimSuffix(header, ":")
		matched := false
		for _, specPart := range strings.Split(headerBody, ", ") {
			spec := trimQuotes(strings.TrimSpace(specPart))
			name, _, ok := splitPackageSpec(spec)
			if ok && name == packageName {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		for _, line := range body {
			trimmed := strings.TrimLeft(line, " \t")
			rest, ok := strings.CutPrefix(trimmed, "version")
			if !ok {
				continue
			}
			if rest == "" {
				continue
			}
			next := rest[0]
			if next != ':' && next != ' ' && next != '\t' {
				continue
			}
			rest = strings.TrimLeft(rest, " \t")
			rest = strings.TrimPrefix(rest, ":")
			version := stripPeerSuffix(cleanValue(rest))
			if isRegistryVersion(version) {
				return version
			}
		}
	}
	return ""
}

func yarnBlocks(text string) [][]string {
	var blocks [][]string
	var current []string
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSuffix(raw, "\r")
		if strings.TrimSpace(line) == "" {
			if len(current) > 0 {
				blocks = append(blocks, current)
				current = nil
			}
			continue
		}
		current = append(current, line)
	}
	if len(current) > 0 {
		blocks = append(blocks, current)
	}
	return blocks
}

func startsWithWhitespace(value string) bool {
	if value == "" {
		return false
	}
	return value[0] == ' ' || value[0] == '\t'
}
