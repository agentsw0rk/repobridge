package updatecheck

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func VerifyArchiveChecksum(name string, content []byte, checksums map[string]string) error {
	want := strings.ToLower(strings.TrimSpace(checksums[name]))
	if want == "" {
		return fmt.Errorf("checksum for %s not found", name)
	}

	sum := sha256.Sum256(content)
	got := hex.EncodeToString(sum[:])
	if got != want {
		return fmt.Errorf("checksum mismatch for %s", name)
	}
	return nil
}

func ExtractArchive(name string, content []byte, targetDir string) (string, error) {
	switch {
	case strings.HasSuffix(name, ".tar.gz"):
		return extractTarGz(content, targetDir)
	case strings.HasSuffix(name, ".zip"):
		return extractZip(content, targetDir)
	default:
		return "", fmt.Errorf("unsupported release archive %s", name)
	}
}

func InstallExtractedRelease(extractedDir, currentExecutable, goos string) error {
	binaryName := "repobridge"
	if goos == "windows" {
		binaryName = "repobridge.exe"
	}

	sourceBinary := filepath.Join(extractedDir, binaryName)
	if _, err := os.Stat(sourceBinary); err != nil {
		return fmt.Errorf("release binary not found: %w", err)
	}
	if goos != "windows" {
		if err := os.Chmod(sourceBinary, 0o755); err != nil {
			return fmt.Errorf("chmod release binary: %w", err)
		}
	}

	destDir := filepath.Dir(currentExecutable)
	files := []releaseInstallFile{{
		source: sourceBinary,
		target: currentExecutable,
		mode:   0o755,
		binary: true,
	}}
	for _, pattern := range []string{"libobjectbox.*", "objectbox.dll"} {
		matches, err := filepath.Glob(filepath.Join(extractedDir, pattern))
		if err != nil {
			return err
		}
		for _, match := range matches {
			files = append(files, releaseInstallFile{
				source: match,
				target: filepath.Join(destDir, filepath.Base(match)),
				mode:   0o644,
			})
		}
	}
	return installReleaseFiles(files, goos)
}

var replaceStagedBinary = replaceStagedFile

type releaseInstallFile struct {
	source string
	target string
	mode   os.FileMode
	binary bool
}

type stagedInstallFile struct {
	releaseInstallFile
	staged string
	backup string
}

func installReleaseFiles(files []releaseInstallFile, goos string) error {
	var staged []stagedInstallFile
	for _, file := range files {
		stagedFile, err := stageInstallFile(file)
		if err != nil {
			removeStagedFiles(staged)
			return err
		}
		staged = append(staged, stagedFile)
	}
	defer removeStagedFiles(staged)

	var binary *stagedInstallFile
	var installedLibraries []*stagedInstallFile
	for i := range staged {
		if staged[i].binary {
			binary = &staged[i]
			continue
		}
		if err := replaceLibraryWithRollback(&staged[i]); err != nil {
			rollbackLibraries(installedLibraries)
			return err
		}
		installedLibraries = append(installedLibraries, &staged[i])
	}
	if binary == nil {
		rollbackLibraries(installedLibraries)
		return fmt.Errorf("release binary not staged")
	}
	if err := replaceStagedBinary(binary.staged, binary.target, goos); err != nil {
		rollbackLibraries(installedLibraries)
		return err
	}
	binary.staged = ""
	removeLibraryBackups(installedLibraries)
	return nil
}

func stageInstallFile(file releaseInstallFile) (stagedInstallFile, error) {
	staged, err := os.CreateTemp(filepath.Dir(file.target), filepath.Base(file.target)+".new-*")
	if err != nil {
		return stagedInstallFile{}, fmt.Errorf("stage replacement for %s: %w", file.target, err)
	}
	stagedPath := staged.Name()
	if err := staged.Close(); err != nil {
		_ = os.Remove(stagedPath)
		return stagedInstallFile{}, fmt.Errorf("stage replacement for %s: %w", file.target, err)
	}
	if err := copyFile(file.source, stagedPath, file.mode); err != nil {
		_ = os.Remove(stagedPath)
		return stagedInstallFile{}, fmt.Errorf("stage replacement for %s: %w", file.target, err)
	}
	return stagedInstallFile{releaseInstallFile: file, staged: stagedPath}, nil
}

func replaceLibraryWithRollback(file *stagedInstallFile) error {
	if _, err := os.Stat(file.target); err == nil {
		backup, err := os.CreateTemp(filepath.Dir(file.target), filepath.Base(file.target)+".old-*")
		if err != nil {
			return fmt.Errorf("backup existing library %s: %w", file.target, err)
		}
		file.backup = backup.Name()
		if err := backup.Close(); err != nil {
			_ = os.Remove(file.backup)
			return fmt.Errorf("backup existing library %s: %w", file.target, err)
		}
		if err := os.Remove(file.backup); err != nil {
			return fmt.Errorf("backup existing library %s: %w", file.target, err)
		}
		if err := os.Rename(file.target, file.backup); err != nil {
			return fmt.Errorf("backup existing library %s: %w", file.target, err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat existing library %s: %w", file.target, err)
	}

	if err := os.Rename(file.staged, file.target); err != nil {
		if file.backup != "" {
			_ = os.Rename(file.backup, file.target)
		}
		return fmt.Errorf("replace library %s: %w", file.target, err)
	}
	file.staged = ""
	return nil
}

func rollbackLibraries(files []*stagedInstallFile) {
	for i := len(files) - 1; i >= 0; i-- {
		file := files[i]
		_ = os.Remove(file.target)
		if file.backup != "" {
			_ = os.Rename(file.backup, file.target)
			file.backup = ""
		}
	}
}

func removeLibraryBackups(files []*stagedInstallFile) {
	for _, file := range files {
		if file.backup != "" {
			_ = os.Remove(file.backup)
			file.backup = ""
		}
	}
}

func removeStagedFiles(files []stagedInstallFile) {
	for _, file := range files {
		if file.staged != "" {
			_ = os.Remove(file.staged)
		}
	}
}

func replaceFile(src, dst, goos string) error {
	mode := os.FileMode(0o755)
	if info, err := os.Stat(src); err != nil {
		return fmt.Errorf("stat replacement source %s: %w", src, err)
	} else {
		mode = info.Mode().Perm()
	}

	staged, err := stageInstallFile(releaseInstallFile{source: src, target: dst, mode: mode})
	if err != nil {
		return err
	}
	defer removeStagedFiles([]stagedInstallFile{staged})

	if err := replaceStagedFile(staged.staged, dst, goos); err != nil {
		return err
	}
	if err := os.Remove(src); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove replaced source %s: %w", src, err)
	}
	return nil
}

func replaceStagedFile(stagedPath, dst, goos string) error {
	if goos != "windows" {
		if err := os.Rename(stagedPath, dst); err != nil {
			return fmt.Errorf("replace %s: %w", dst, err)
		}
		return nil
	}

	destDir := filepath.Dir(dst)
	backupPath := ""
	if _, err := os.Stat(dst); err == nil {
		backup, err := os.CreateTemp(destDir, filepath.Base(dst)+".old-*")
		if err != nil {
			return windowsManualReplaceError(dst, fmt.Errorf("backup existing executable %s: %w", dst, err))
		}
		backupPath = backup.Name()
		if err := backup.Close(); err != nil {
			_ = os.Remove(backupPath)
			return windowsManualReplaceError(dst, fmt.Errorf("backup existing executable %s: %w", dst, err))
		}
		if err := os.Remove(backupPath); err != nil {
			return windowsManualReplaceError(dst, fmt.Errorf("backup existing executable %s: %w", dst, err))
		}
		if err := os.Rename(dst, backupPath); err != nil {
			return windowsManualReplaceError(dst, fmt.Errorf("backup existing executable %s: %w", dst, err))
		}
	} else if !os.IsNotExist(err) {
		return windowsManualReplaceError(dst, fmt.Errorf("stat existing executable %s: %w", dst, err))
	}

	if err := os.Rename(stagedPath, dst); err != nil {
		if backupPath != "" {
			if rollbackErr := os.Rename(backupPath, dst); rollbackErr != nil {
				return windowsManualReplaceError(dst, fmt.Errorf("replace %s: %w; rollback failed: %v", dst, err, rollbackErr))
			}
		}
		return windowsManualReplaceError(dst, fmt.Errorf("replace %s: %w", dst, err))
	}
	if backupPath != "" {
		if err := os.Remove(backupPath); err != nil {
			return windowsManualReplaceError(dst, fmt.Errorf("remove backup for %s: %w", dst, err))
		}
	}
	return nil
}

func windowsManualReplaceError(dst string, err error) error {
	return fmt.Errorf("%w; close running RepoBridge processes and replace %s manually from the downloaded release archive, or rerun self-update", err, dst)
}

func extractTarGz(content []byte, targetDir string) (string, error) {
	gr, err := gzip.NewReader(bytes.NewReader(content))
	if err != nil {
		return "", err
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	var root string
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}

		switch header.Typeflag {
		case tar.TypeDir:
			target, top, err := safeExtractPath(targetDir, header.Name)
			if err != nil {
				return "", err
			}
			if root, err = updateExtractRoot(root, targetDir, top); err != nil {
				return "", err
			}
			if err := os.MkdirAll(target, os.FileMode(header.Mode)); err != nil {
				return "", err
			}
		case tar.TypeReg:
			target, top, err := safeExtractPath(targetDir, header.Name)
			if err != nil {
				return "", err
			}
			if root, err = updateExtractRoot(root, targetDir, top); err != nil {
				return "", err
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return "", err
			}
			if err := writeReaderToFile(target, tr, os.FileMode(header.Mode)); err != nil {
				return "", err
			}
		}
	}
	if root == "" {
		return "", fmt.Errorf("release archive is empty")
	}
	return root, nil
}

func extractZip(content []byte, targetDir string) (string, error) {
	zr, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		return "", err
	}

	var root string
	for _, file := range zr.File {
		target, top, err := safeExtractPath(targetDir, file.Name)
		if err != nil {
			return "", err
		}
		if root, err = updateExtractRoot(root, targetDir, top); err != nil {
			return "", err
		}

		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(target, file.Mode()); err != nil {
				return "", err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return "", err
		}
		in, err := file.Open()
		if err != nil {
			return "", err
		}
		err = writeReaderToFile(target, in, file.Mode())
		closeErr := in.Close()
		if err != nil {
			return "", err
		}
		if closeErr != nil {
			return "", closeErr
		}
	}
	if root == "" {
		return "", fmt.Errorf("release archive is empty")
	}
	return root, nil
}

func safeExtractPath(targetDir, archivePath string) (string, string, error) {
	clean := filepath.Clean(filepath.FromSlash(archivePath))
	if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return "", "", fmt.Errorf("unsafe archive path %s", archivePath)
	}
	parts := strings.Split(clean, string(os.PathSeparator))
	if len(parts) == 0 || parts[0] == "" || parts[0] == "." || parts[0] == ".." {
		return "", "", fmt.Errorf("unsafe archive path %s", archivePath)
	}

	target := filepath.Join(targetDir, clean)
	rel, err := filepath.Rel(targetDir, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", "", fmt.Errorf("unsafe archive path %s", archivePath)
	}
	return target, parts[0], nil
}

func updateExtractRoot(current, targetDir, top string) (string, error) {
	root := filepath.Join(targetDir, top)
	if current == "" {
		return root, nil
	}
	if current != root {
		return "", fmt.Errorf("release archive contains multiple top-level directories")
	}
	return current, nil
}

func writeReaderToFile(path string, r io.Reader, mode os.FileMode) error {
	out, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, r)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Chmod(path, mode)
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Chmod(dst, mode)
}
