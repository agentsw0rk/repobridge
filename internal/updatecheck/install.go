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
	if err := replaceExtractedBinary(sourceBinary, currentExecutable, goos); err != nil {
		return err
	}

	for _, pattern := range []string{"libobjectbox.*", "objectbox.dll"} {
		matches, err := filepath.Glob(filepath.Join(extractedDir, pattern))
		if err != nil {
			return err
		}
		for _, match := range matches {
			if err := copyFile(match, filepath.Join(destDir, filepath.Base(match)), 0o644); err != nil {
				return err
			}
		}
	}
	return nil
}

var replaceExtractedBinary = replaceFile

func replaceFile(src, dst, goos string) error {
	destDir := filepath.Dir(dst)
	mode := os.FileMode(0o755)
	if info, err := os.Stat(src); err != nil {
		return fmt.Errorf("stat replacement source %s: %w", src, err)
	} else {
		mode = info.Mode().Perm()
	}

	staged, err := os.CreateTemp(destDir, filepath.Base(dst)+".new-*")
	if err != nil {
		return fmt.Errorf("stage replacement for %s: %w", dst, err)
	}
	stagedPath := staged.Name()
	if err := staged.Close(); err != nil {
		_ = os.Remove(stagedPath)
		return fmt.Errorf("stage replacement for %s: %w", dst, err)
	}
	if err := copyFile(src, stagedPath, mode); err != nil {
		_ = os.Remove(stagedPath)
		return fmt.Errorf("stage replacement for %s: %w", dst, err)
	}

	if goos != "windows" {
		if err := os.Rename(stagedPath, dst); err != nil {
			_ = os.Remove(stagedPath)
			return fmt.Errorf("replace %s: %w", dst, err)
		}
		if err := os.Remove(src); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove replaced source %s: %w", src, err)
		}
		return nil
	}

	backupPath := ""
	if _, err := os.Stat(dst); err == nil {
		backup, err := os.CreateTemp(destDir, filepath.Base(dst)+".old-*")
		if err != nil {
			_ = os.Remove(stagedPath)
			return fmt.Errorf("backup existing executable %s: %w", dst, err)
		}
		backupPath = backup.Name()
		if err := backup.Close(); err != nil {
			_ = os.Remove(stagedPath)
			_ = os.Remove(backupPath)
			return fmt.Errorf("backup existing executable %s: %w", dst, err)
		}
		if err := os.Remove(backupPath); err != nil {
			_ = os.Remove(stagedPath)
			return fmt.Errorf("backup existing executable %s: %w", dst, err)
		}
		if err := os.Rename(dst, backupPath); err != nil {
			_ = os.Remove(stagedPath)
			return fmt.Errorf("backup existing executable %s: %w", dst, err)
		}
	} else if !os.IsNotExist(err) {
		_ = os.Remove(stagedPath)
		return fmt.Errorf("stat existing executable %s: %w", dst, err)
	}

	if err := os.Rename(stagedPath, dst); err != nil {
		if backupPath != "" {
			if rollbackErr := os.Rename(backupPath, dst); rollbackErr != nil {
				return fmt.Errorf("replace %s: %w; rollback failed: %v", dst, err, rollbackErr)
			}
		}
		_ = os.Remove(stagedPath)
		return fmt.Errorf("replace %s: %w", dst, err)
	}
	if backupPath != "" {
		if err := os.Remove(backupPath); err != nil {
			return fmt.Errorf("remove backup for %s: %w", dst, err)
		}
	}
	if err := os.Remove(src); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove replaced source %s: %w", src, err)
	}
	return nil
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
