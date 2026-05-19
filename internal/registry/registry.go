package registry

import (
	"fmt"
	"regexp"
	"strings"
)

type Registry string

const (
	NPM    Registry = "npm"
	PyPI   Registry = "pypi"
	Crates Registry = "crates"
)

func (r Registry) Label() string {
	switch r {
	case NPM:
		return "npm"
	case PyPI:
		return "PyPI"
	case Crates:
		return "crates.io"
	default:
		return string(r)
	}
}

type InputType string

const (
	PackageInput InputType = "package"
	RepoInput    InputType = "repo"
)

type DetectedRegistry struct {
	Registry  Registry
	CleanSpec string
}

type PackageSpec struct {
	Registry Registry
	Name     string
	Version  string
}

type ResolvedPackage struct {
	Registry      Registry
	Name          string
	Version       string
	RepoURL       string
	RepoDirectory string
	GitTag        string
}

var scopedNPM = regexp.MustCompile(`^(@[^/]+/[^@]+)(?:@(.+))?$`)
var pypiEq = regexp.MustCompile(`^([^=<>!~]+)==(.+)$`)

var prefixes = []struct {
	Prefix   string
	Registry Registry
}{
	{"npm:", NPM},
	{"pypi:", PyPI},
	{"pip:", PyPI},
	{"python:", PyPI},
	{"crates:", Crates},
	{"cargo:", Crates},
	{"rust:", Crates},
}

func DetectRegistry(spec string) DetectedRegistry {
	trimmed := strings.TrimSpace(spec)
	lower := strings.ToLower(trimmed)
	for _, item := range prefixes {
		if strings.HasPrefix(lower, item.Prefix) {
			return DetectedRegistry{Registry: item.Registry, CleanSpec: trimmed[len(item.Prefix):]}
		}
	}
	return DetectedRegistry{Registry: NPM, CleanSpec: trimmed}
}

func ParsePackageSpec(spec string) PackageSpec {
	detected := DetectRegistry(spec)
	name, version := parseByRegistry(detected.Registry, detected.CleanSpec)
	return PackageSpec{Registry: detected.Registry, Name: name, Version: version}
}

func parseByRegistry(reg Registry, spec string) (string, string) {
	switch reg {
	case NPM:
		return ParseNPMSpec(spec)
	case PyPI:
		return ParsePyPISpec(spec)
	case Crates:
		return ParseCratesSpec(spec)
	default:
		return strings.TrimSpace(spec), ""
	}
}

func ParseNPMSpec(spec string) (string, string) {
	if strings.HasPrefix(spec, "@") {
		if matches := scopedNPM.FindStringSubmatch(spec); matches != nil {
			version := ""
			if len(matches) > 2 {
				version = matches[2]
			}
			return matches[1], version
		}
	}
	if at := strings.LastIndex(spec, "@"); at > 0 {
		return spec[:at], spec[at+1:]
	}
	return spec, ""
}

func ParsePyPISpec(spec string) (string, string) {
	trimmed := strings.TrimSpace(spec)
	if matches := pypiEq.FindStringSubmatch(trimmed); matches != nil {
		return strings.TrimSpace(matches[1]), strings.TrimSpace(matches[2])
	}
	if at := strings.LastIndex(trimmed, "@"); at > 0 {
		return strings.TrimSpace(trimmed[:at]), strings.TrimSpace(trimmed[at+1:])
	}
	return trimmed, ""
}

func ParseCratesSpec(spec string) (string, string) {
	trimmed := strings.TrimSpace(spec)
	if at := strings.LastIndex(trimmed, "@"); at > 0 {
		return strings.TrimSpace(trimmed[:at]), strings.TrimSpace(trimmed[at+1:])
	}
	return trimmed, ""
}

func DetectInputType(spec string) InputType {
	trimmed := strings.TrimSpace(spec)
	lower := strings.ToLower(trimmed)
	for _, item := range prefixes {
		if strings.HasPrefix(lower, item.Prefix) {
			return PackageInput
		}
	}
	if IsRepoLike(trimmed) {
		return RepoInput
	}
	return PackageInput
}

func IsRepoLike(spec string) bool {
	lower := strings.ToLower(spec)
	if strings.HasPrefix(lower, "github:") || strings.HasPrefix(lower, "gitlab:") || strings.HasPrefix(lower, "bitbucket:") {
		return true
	}
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return true
	}
	if strings.HasPrefix(lower, "@") {
		return false
	}
	parts := strings.Split(spec, "/")
	if len(parts) > 3 && strings.EqualFold(parts[0], "gitlab.com") {
		for _, part := range parts[1:] {
			if part == "" || strings.Contains(part, ":") {
				return false
			}
		}
		return true
	}
	if len(parts) == 3 && isSupportedRepoHost(parts[0]) && parts[1] != "" && parts[2] != "" {
		return true
	}
	return len(parts) == 2 && parts[0] != "" && parts[1] != "" && !strings.Contains(parts[1], ":")
}

func isSupportedRepoHost(host string) bool {
	switch strings.ToLower(host) {
	case "github.com", "gitlab.com", "bitbucket.org":
		return true
	default:
		return false
	}
}

func SupportedRegistry(reg Registry) error {
	switch reg {
	case NPM, PyPI, Crates:
		return nil
	default:
		return fmt.Errorf("unsupported registry: %s", reg)
	}
}
