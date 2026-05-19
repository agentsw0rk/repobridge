package source

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"repobridge/internal/cache"
	"repobridge/internal/httpx"
	"repobridge/internal/registry"
	"repobridge/internal/repobridge"
)

const (
	maxSourceArchiveBytes    int64  = 256 * 1024 * 1024
	maxExtractedArchiveBytes uint64 = 512 * 1024 * 1024
	maxSourceArchiveEntries         = 20000
)

type sourceArchiveNotFoundError struct {
	URL string
}

func (e *sourceArchiveNotFoundError) Error() string {
	return fmt.Sprintf("source archive not found: %s", e.URL)
}

func FetchPackageArchive(pkg registry.ResolvedPackage, client *http.Client) FetchResult {
	if pkg.Registry != registry.Maven {
		return FetchResult{Error: fmt.Errorf("source archives are not supported for registry: %s", pkg.Registry)}
	}
	if client == nil {
		client = httpx.NewClient()
	}

	relativePath, target, err := mavenArchiveTarget(pkg)
	if err != nil {
		return FetchResult{Error: err}
	}
	if ok, err := reusableTarget(target); err != nil {
		return FetchResult{Error: err}
	} else if ok {
		return archiveFetchSuccess(pkg, relativePath)
	}

	tmp, err := downloadArchive(pkg.SourceArchiveURL, client)
	if err != nil {
		return FetchResult{Error: err}
	}
	defer os.Remove(tmp)

	if err := extractZipArchive(tmp, target); err != nil {
		_ = os.RemoveAll(target)
		return FetchResult{Error: err}
	}

	return archiveFetchSuccess(pkg, relativePath)
}

func mavenArchiveTarget(pkg registry.ResolvedPackage) (string, string, error) {
	groupID, artifactID, ok := strings.Cut(strings.TrimSpace(pkg.Name), ":")
	groupID = strings.TrimSpace(groupID)
	artifactID = strings.TrimSpace(artifactID)
	if !ok || groupID == "" || artifactID == "" || strings.Contains(artifactID, ":") {
		return "", "", fmt.Errorf("invalid Maven coordinates %q, want groupId:artifactId", pkg.Name)
	}
	version := strings.TrimSpace(pkg.Version)
	if version == "" {
		return "", "", fmt.Errorf("Maven version must not be empty")
	}

	relativePath := filepath.ToSlash(filepath.Join("repos", "maven", groupID, artifactID, version))
	target, err := cache.AbsolutePath(relativePath)
	if err != nil {
		return "", "", err
	}
	return relativePath, target, nil
}

func downloadArchive(url string, client *http.Client) (string, error) {
	if strings.TrimSpace(url) == "" {
		return "", fmt.Errorf("source archive URL must not be empty")
	}

	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return "", &sourceArchiveNotFoundError{URL: url}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", repobridge.HTTPStatusError{Context: "source archive", Status: resp.Status}
	}
	if resp.ContentLength > maxSourceArchiveBytes {
		return "", fmt.Errorf("source archive exceeds maximum size of %d bytes", maxSourceArchiveBytes)
	}

	tmp, err := os.CreateTemp("", "repobridge-source-archive-*.jar")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	written, err := io.Copy(tmp, io.LimitReader(resp.Body, maxSourceArchiveBytes+1))
	if err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return "", err
	}
	if written > maxSourceArchiveBytes {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("source archive exceeds maximum size of %d bytes", maxSourceArchiveBytes)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return "", err
	}
	return tmpPath, nil
}

func extractZipArchive(archivePath, target string) error {
	return extractZipArchiveWithLimit(archivePath, target, maxExtractedArchiveBytes)
}

func extractZipArchiveWithLimit(archivePath, target string, maxExtractedBytes uint64) error {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer reader.Close()

	if err := os.MkdirAll(target, 0o755); err != nil {
		return err
	}
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	if len(reader.File) > maxSourceArchiveEntries {
		return fmt.Errorf("source archive contains too many entries: %d exceeds maximum of %d", len(reader.File), maxSourceArchiveEntries)
	}
	var advertisedUncompressed uint64
	for _, entry := range reader.File {
		if entry.UncompressedSize64 > maxExtractedBytes-advertisedUncompressed {
			return extractedArchiveLimitError(maxExtractedBytes)
		}
		advertisedUncompressed += entry.UncompressedSize64
	}
	extractedBytesRemaining := maxExtractedBytes

	for _, entry := range reader.File {
		path, err := safeArchiveEntryPath(targetAbs, entry.Name)
		if err != nil {
			return err
		}
		mode := entry.FileInfo().Mode()
		if mode&os.ModeSymlink != 0 {
			return fmt.Errorf("source archive contains unsupported symlink: %s", entry.Name)
		}
		if entry.FileInfo().IsDir() {
			if err := os.MkdirAll(path, entryModePerm(mode, 0o755)); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := extractZipFile(entry, path, &extractedBytesRemaining, maxExtractedBytes); err != nil {
			return err
		}
	}
	return nil
}

func safeArchiveEntryPath(targetAbs, name string) (string, error) {
	normalized := strings.ReplaceAll(name, "\\", "/")
	if len(normalized) >= 2 && normalized[1] == ':' {
		return "", fmt.Errorf("source archive contains unsafe path: %s", name)
	}
	for _, part := range strings.Split(normalized, "/") {
		if part == ".." {
			return "", fmt.Errorf("source archive contains unsafe path: %s", name)
		}
	}
	cleanSlash := filepath.ToSlash(filepath.Clean(filepath.FromSlash(normalized)))
	cleanPath := filepath.FromSlash(cleanSlash)
	if cleanSlash == "." || filepath.IsAbs(cleanPath) || filepath.VolumeName(cleanPath) != "" {
		return "", fmt.Errorf("source archive contains unsafe path: %s", name)
	}

	path := filepath.Join(targetAbs, cleanPath)
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(targetAbs, absPath)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("source archive path escapes destination: %s", name)
	}
	return absPath, nil
}

func extractZipFile(entry *zip.File, path string, extractedBytesRemaining *uint64, maxExtractedBytes uint64) error {
	source, err := entry.Open()
	if err != nil {
		return err
	}
	defer source.Close()

	mode := entryModePerm(entry.FileInfo().Mode(), 0o644)
	target, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer target.Close()

	return copyArchiveEntry(source, target, extractedBytesRemaining, maxExtractedBytes)
}

func copyArchiveEntry(source io.Reader, target io.Writer, extractedBytesRemaining *uint64, maxExtractedBytes uint64) error {
	copied, err := io.Copy(target, io.LimitReader(source, int64(*extractedBytesRemaining)+1))
	if uint64(copied) > *extractedBytesRemaining {
		return extractedArchiveLimitError(maxExtractedBytes)
	}
	*extractedBytesRemaining -= uint64(copied)
	return err
}

func extractedArchiveLimitError(maxExtractedBytes uint64) error {
	return fmt.Errorf("extracted source archive exceeds maximum size of %d bytes", maxExtractedBytes)
}

func entryModePerm(mode os.FileMode, fallback os.FileMode) os.FileMode {
	if perm := mode.Perm(); perm != 0 {
		return perm
	}
	return fallback
}

func archiveFetchSuccess(pkg registry.ResolvedPackage, relativePath string) FetchResult {
	return FetchResult{
		Package:  pkg.Name,
		Version:  pkg.Version,
		Path:     relativePath,
		Success:  true,
		Registry: pkg.Registry,
	}
}
