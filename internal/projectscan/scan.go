package projectscan

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type Options struct {
	IncludeImports bool
}

type Result struct {
	Root       string      `json:"root"`
	Candidates []Candidate `json:"candidates"`
	Warnings   []string    `json:"warnings,omitempty"`
}

type Candidate struct {
	Spec       string   `json:"spec"`
	Ecosystem  string   `json:"ecosystem"`
	Confidence int      `json:"confidence"`
	Reasons    []string `json:"reasons"`
	Files      []string `json:"files"`
}

type candidateBuilder struct {
	spec       string
	ecosystem  string
	confidence int
	reasons    map[string]bool
	files      map[string]bool
}

func Scan(root string, opts Options) (Result, error) {
	if root == "" {
		root = "."
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return Result{}, err
	}
	info, err := os.Stat(absRoot)
	if err != nil {
		return Result{}, err
	}
	if !info.IsDir() {
		return Result{}, fmt.Errorf("scan root is not a directory: %s", absRoot)
	}

	state := scanState{
		root:       absRoot,
		candidates: map[string]*candidateBuilder{},
	}
	state.scanPackageJSON()
	state.scanPackageLock()
	state.scanRequirements()
	state.scanGoMod()
	state.scanCargoToml()
	state.scanPom()
	state.scanCSProj()
	if opts.IncludeImports {
		state.scanImports()
	}
	return Result{Root: absRoot, Candidates: state.sortedCandidates(), Warnings: state.warnings}, nil
}

type scanState struct {
	root       string
	candidates map[string]*candidateBuilder
	warnings   []string
}

func (s *scanState) add(spec, ecosystem string, confidence int, reason, file string) {
	spec = strings.TrimSpace(spec)
	if spec == "" || strings.HasPrefix(spec, ".") || strings.HasPrefix(spec, "/") {
		return
	}
	candidate := s.candidates[spec]
	if candidate == nil {
		candidate = &candidateBuilder{
			spec:      spec,
			ecosystem: ecosystem,
			reasons:   map[string]bool{},
			files:     map[string]bool{},
		}
		s.candidates[spec] = candidate
	}
	if confidence > candidate.confidence {
		candidate.confidence = confidence
	}
	candidate.reasons[reason] = true
	candidate.files[file] = true
}

func (s *scanState) sortedCandidates() []Candidate {
	candidates := make([]Candidate, 0, len(s.candidates))
	for _, candidate := range s.candidates {
		candidates = append(candidates, Candidate{
			Spec:       candidate.spec,
			Ecosystem:  candidate.ecosystem,
			Confidence: candidate.confidence,
			Reasons:    sortedKeys(candidate.reasons),
			Files:      sortedKeys(candidate.files),
		})
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Confidence != candidates[j].Confidence {
			return candidates[i].Confidence > candidates[j].Confidence
		}
		return candidates[i].Spec < candidates[j].Spec
	})
	return candidates
}

func sortedKeys(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func (s *scanState) scanPackageJSON() {
	s.walkNamed("package.json", func(path string) {
		content, err := os.ReadFile(path)
		if err != nil {
			s.warnings = append(s.warnings, fmt.Sprintf("could not read %s: %v", path, err))
			return
		}
		var data struct {
			Dependencies         map[string]string `json:"dependencies"`
			DevDependencies      map[string]string `json:"devDependencies"`
			PeerDependencies     map[string]string `json:"peerDependencies"`
			OptionalDependencies map[string]string `json:"optionalDependencies"`
		}
		if err := json.Unmarshal(content, &data); err != nil {
			s.warnings = append(s.warnings, fmt.Sprintf("could not parse %s: %v", path, err))
			return
		}
		s.addNPMDeps(data.Dependencies, 92, "package.json dependencies", path)
		s.addNPMDeps(data.DevDependencies, 76, "package.json devDependencies", path)
		s.addNPMDeps(data.PeerDependencies, 70, "package.json peerDependencies", path)
		s.addNPMDeps(data.OptionalDependencies, 70, "package.json optionalDependencies", path)
	})
}

func (s *scanState) scanPackageLock() {
	s.walkNamed("package-lock.json", func(path string) {
		content, err := os.ReadFile(path)
		if err != nil {
			s.warnings = append(s.warnings, fmt.Sprintf("could not read %s: %v", path, err))
			return
		}
		var data struct {
			Packages map[string]struct {
				Version         string            `json:"version"`
				Dependencies    map[string]string `json:"dependencies"`
				DevDependencies map[string]string `json:"devDependencies"`
			} `json:"packages"`
		}
		if err := json.Unmarshal(content, &data); err != nil {
			s.warnings = append(s.warnings, fmt.Sprintf("could not parse %s: %v", path, err))
			return
		}
		rootPackage, ok := data.Packages[""]
		if !ok {
			return
		}
		direct := map[string]bool{}
		for name := range rootPackage.Dependencies {
			direct[name] = true
		}
		for name := range rootPackage.DevDependencies {
			direct[name] = true
		}
		for pkgPath, meta := range data.Packages {
			if !strings.HasPrefix(pkgPath, "node_modules/") {
				continue
			}
			name := strings.TrimPrefix(pkgPath, "node_modules/")
			if !direct[name] || meta.Version == "" {
				continue
			}
			s.add(name+"@"+meta.Version, "npm", 96, "package-lock.json direct dependency", path)
		}
	})
}

func (s *scanState) addNPMDeps(deps map[string]string, confidence int, reason, path string) {
	for name, version := range deps {
		if strings.HasPrefix(name, "@types/") {
			continue
		}
		if clean := cleanManifestVersion(version); clean != "" {
			s.add(name+"@"+clean, "npm", confidence, reason, path)
			continue
		}
		s.add(name, "npm", confidence-8, reason, path)
	}
}

func (s *scanState) scanRequirements() {
	linePattern := regexp.MustCompile(`^\s*([A-Za-z0-9_.-]+)\s*(?:==\s*([^\s;#]+))?`)
	s.walkNamed("requirements.txt", func(path string) {
		content, err := os.ReadFile(path)
		if err != nil {
			s.warnings = append(s.warnings, fmt.Sprintf("could not read %s: %v", path, err))
			return
		}
		for _, line := range strings.Split(string(content), "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "-") {
				continue
			}
			match := linePattern.FindStringSubmatch(line)
			if len(match) == 0 {
				continue
			}
			name := match[1]
			if len(match) > 2 && match[2] != "" {
				s.add("pypi:"+name+"=="+match[2], "pypi", 90, "requirements.txt", path)
			} else {
				s.add("pypi:"+name, "pypi", 82, "requirements.txt", path)
			}
		}
	})
}

func cleanManifestVersion(version string) string {
	version = strings.TrimSpace(version)
	version = strings.TrimLeft(version, "^~>=<")
	if version == "" || version == "*" || version == "latest" || version == "next" {
		return ""
	}
	if strings.ContainsAny(version, " |") || strings.Contains(version, ":") {
		return ""
	}
	if regexp.MustCompile(`^\d+\.\d+\.\d+`).MatchString(version) {
		return version
	}
	return ""
}

func (s *scanState) scanGoMod() {
	requireLine := regexp.MustCompile(`^\s*([A-Za-z0-9_.~/-]+\.[A-Za-z0-9_.~/-]+)\s+(v[^\s]+)`)
	s.walkNamed("go.mod", func(path string) {
		content, err := os.ReadFile(path)
		if err != nil {
			s.warnings = append(s.warnings, fmt.Sprintf("could not read %s: %v", path, err))
			return
		}
		inRequire := false
		for _, line := range strings.Split(string(content), "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "require (") {
				inRequire = true
				continue
			}
			if inRequire && trimmed == ")" {
				inRequire = false
				continue
			}
			if strings.HasPrefix(trimmed, "require ") {
				trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "require "))
			} else if !inRequire {
				continue
			}
			match := requireLine.FindStringSubmatch(trimmed)
			if len(match) != 3 {
				continue
			}
			module, version := match[1], match[2]
			if strings.HasPrefix(module, "github.com/") || strings.HasPrefix(module, "gitlab.com/") || strings.HasPrefix(module, "bitbucket.org/") {
				s.add(module+"@"+version, "go", 90, "go.mod require", path)
			}
		}
	})
}

func (s *scanState) scanCargoToml() {
	headerPattern := regexp.MustCompile(`^\[(dependencies|dev-dependencies|build-dependencies)]`)
	depPattern := regexp.MustCompile(`^([A-Za-z0-9_-]+)\s*=\s*(?:"([^"]+)"|\{([^}]+)})`)
	versionPattern := regexp.MustCompile(`version\s*=\s*"([^"]+)"`)
	s.walkNamed("Cargo.toml", func(path string) {
		content, err := os.ReadFile(path)
		if err != nil {
			s.warnings = append(s.warnings, fmt.Sprintf("could not read %s: %v", path, err))
			return
		}
		inDeps := false
		for _, line := range strings.Split(string(content), "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "[") {
				inDeps = headerPattern.MatchString(trimmed)
				continue
			}
			if !inDeps || trimmed == "" || strings.HasPrefix(trimmed, "#") {
				continue
			}
			match := depPattern.FindStringSubmatch(trimmed)
			if len(match) == 0 {
				continue
			}
			name := match[1]
			version := match[2]
			if version == "" && match[3] != "" {
				if versionMatch := versionPattern.FindStringSubmatch(match[3]); len(versionMatch) == 2 {
					version = versionMatch[1]
				}
			}
			version = strings.TrimLeft(strings.TrimSpace(version), "^~>=<")
			if version != "" {
				s.add("crates:"+name+"@"+version, "crates", 88, "Cargo.toml dependency", path)
			} else {
				s.add("crates:"+name, "crates", 72, "Cargo.toml dependency", path)
			}
		}
	})
}

type pomProject struct {
	Dependencies []pomDependency `xml:"dependencies>dependency"`
}

type pomDependency struct {
	GroupID    string `xml:"groupId"`
	ArtifactID string `xml:"artifactId"`
	Version    string `xml:"version"`
}

func (s *scanState) scanPom() {
	s.walkNamed("pom.xml", func(path string) {
		content, err := os.ReadFile(path)
		if err != nil {
			s.warnings = append(s.warnings, fmt.Sprintf("could not read %s: %v", path, err))
			return
		}
		var project pomProject
		if err := xml.Unmarshal(content, &project); err != nil {
			s.warnings = append(s.warnings, fmt.Sprintf("could not parse %s: %v", path, err))
			return
		}
		for _, dep := range project.Dependencies {
			if dep.GroupID == "" || dep.ArtifactID == "" || dep.Version == "" || strings.HasPrefix(dep.Version, "${") {
				continue
			}
			s.add("maven:"+dep.GroupID+":"+dep.ArtifactID+"@"+dep.Version, "maven", 88, "pom.xml dependency", path)
		}
	})
}

func (s *scanState) scanCSProj() {
	refPattern := regexp.MustCompile(`<PackageReference[^>]*Include="([^"]+)"[^>]*Version="([^"]+)"`)
	s.walkSuffix([]string{".csproj"}, func(path string) {
		content, err := os.ReadFile(path)
		if err != nil {
			s.warnings = append(s.warnings, fmt.Sprintf("could not read %s: %v", path, err))
			return
		}
		for _, match := range refPattern.FindAllStringSubmatch(string(content), -1) {
			s.add("nuget:"+match[1]+"@"+match[2], "nuget", 88, ".csproj PackageReference", path)
		}
	})
}

func (s *scanState) scanImports() {
	jsImport := regexp.MustCompile(`(?:from\s+|import\s*\(|require\()\s*['"]([^'"]+)['"]`)
	goImport := regexp.MustCompile(`"([^"]+\.[^"]+)"`)
	s.walkSuffix([]string{".js", ".jsx", ".ts", ".tsx", ".mjs", ".cjs"}, func(path string) {
		content, err := os.ReadFile(path)
		if err != nil {
			s.warnings = append(s.warnings, fmt.Sprintf("could not read %s: %v", path, err))
			return
		}
		for _, match := range jsImport.FindAllStringSubmatch(string(content), -1) {
			imported := match[1]
			if strings.HasPrefix(imported, ".") || strings.HasPrefix(imported, "/") || strings.HasPrefix(imported, "node:") {
				continue
			}
			s.add(npmPackageFromImport(imported), "npm", 52, "JavaScript/TypeScript import", path)
		}
	})
	s.walkSuffix([]string{".go"}, func(path string) {
		content, err := os.ReadFile(path)
		if err != nil {
			s.warnings = append(s.warnings, fmt.Sprintf("could not read %s: %v", path, err))
			return
		}
		for _, imported := range parseGoImports(string(content), goImport) {
			if strings.HasPrefix(imported, "github.com/") || strings.HasPrefix(imported, "gitlab.com/") || strings.HasPrefix(imported, "bitbucket.org/") {
				s.add(imported, "go", 55, "Go import", path)
			}
		}
	})
}

func npmPackageFromImport(imported string) string {
	if strings.HasPrefix(imported, "@") {
		parts := strings.Split(imported, "/")
		if len(parts) >= 2 {
			return parts[0] + "/" + parts[1]
		}
		return imported
	}
	return strings.Split(imported, "/")[0]
}

func parseGoImports(content string, importRegexp *regexp.Regexp) []string {
	imports := []string{}
	inBlock := false
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "import (") {
			inBlock = true
			continue
		}
		if inBlock && trimmed == ")" {
			inBlock = false
			continue
		}
		if strings.HasPrefix(trimmed, "import ") || inBlock {
			for _, match := range importRegexp.FindAllStringSubmatch(trimmed, -1) {
				imports = append(imports, match[1])
			}
		}
	}
	return imports
}

func (s *scanState) walkNamed(name string, visit func(path string)) {
	_ = filepath.WalkDir(s.root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			s.warnings = append(s.warnings, fmt.Sprintf("could not scan %s: %v", path, err))
			return nil
		}
		if entry.IsDir() {
			if shouldSkipDir(entry.Name()) && path != s.root {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Name() == name {
			visit(path)
		}
		return nil
	})
}

func (s *scanState) walkSuffix(suffixes []string, visit func(path string)) {
	_ = filepath.WalkDir(s.root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			s.warnings = append(s.warnings, fmt.Sprintf("could not scan %s: %v", path, err))
			return nil
		}
		if entry.IsDir() {
			if shouldSkipDir(entry.Name()) && path != s.root {
				return filepath.SkipDir
			}
			return nil
		}
		for _, suffix := range suffixes {
			if strings.HasSuffix(entry.Name(), suffix) {
				visit(path)
				break
			}
		}
		return nil
	})
}

func shouldSkipDir(name string) bool {
	switch name {
	case ".git", ".hg", ".svn", ".idea", ".vscode", ".agents", ".superpowers", ".worktrees", "node_modules", "vendor", "dist", "build", ".next", "target", "bin", "obj", "__pycache__", ".venv", "venv":
		return true
	default:
		return false
	}
}
