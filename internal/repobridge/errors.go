package repobridge

import "fmt"

type PackageNotFoundError struct {
	Name     string
	Registry string
}

func (e PackageNotFoundError) Error() string {
	return fmt.Sprintf("Package %q not found on %s", e.Name, e.Registry)
}

type VersionNotFoundError struct {
	Message string
}

func (e VersionNotFoundError) Error() string { return e.Message }

type NoRepoURLError struct {
	Message string
}

func (e NoRepoURLError) Error() string { return e.Message }

type RepoNotFoundError struct {
	Message string
}

func (e RepoNotFoundError) Error() string { return e.Message }

type RateLimitExceededError struct{}

func (e RateLimitExceededError) Error() string {
	return "GitHub API rate limit exceeded. Try again later or set GITHUB_TOKEN."
}

type InvalidRepoSpecError struct {
	Spec string
}

func (e InvalidRepoSpecError) Error() string {
	return fmt.Sprintf("Invalid repository format: %s", e.Spec)
}

type CloneFailedError struct {
	Message string
}

func (e CloneFailedError) Error() string { return e.Message }

type HomeDirNotFoundError struct{}

func (e HomeDirNotFoundError) Error() string {
	return "Could not determine home directory. Set the REPOBRIDGE_HOME environment variable."
}

type HTTPStatusError struct {
	Context string
	Status  string
}

func (e HTTPStatusError) Error() string {
	return fmt.Sprintf("Failed to fetch %s: %s", e.Context, e.Status)
}
