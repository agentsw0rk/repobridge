package parser

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Options struct {
	MaxFileSize int64
}

type SourceFile struct {
	AbsolutePath string
	RelativePath string
	Language     Language
	Size         int64
	ModifiedAt   time.Time
}

func ScanSourceFiles(root string, opts Options) ([]SourceFile, error) {
	if opts.MaxFileSize <= 0 {
		opts.MaxFileSize = 1024 * 1024
	}
	var files []SourceFile
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			if path == root {
				return walkErr
			}
			return nil
		}
		if entry.IsDir() {
			if shouldSkipDir(entry.Name()) && path != root {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() || info.Size() > opts.MaxFileSize {
			return nil
		}
		language := DetectLanguage(path)
		if language == LanguageUnknown {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		files = append(files, SourceFile{
			AbsolutePath: path,
			RelativePath: filepath.ToSlash(rel),
			Language:     language,
			Size:         info.Size(),
			ModifiedAt:   info.ModTime(),
		})
		return nil
	})
	sort.Slice(files, func(i, j int) bool { return files[i].RelativePath < files[j].RelativePath })
	return files, err
}

func DetectLanguage(path string) Language {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		return LanguageGo
	case ".java":
		return LanguageJava
	case ".kt", ".kts":
		return LanguageKotlin
	case ".cs":
		return LanguageCSharp
	case ".js", ".jsx", ".mjs", ".cjs":
		return LanguageJavaScript
	case ".ts", ".tsx", ".mts", ".cts":
		return LanguageTypeScript
	case ".py":
		return LanguagePython
	case ".rs":
		return LanguageRust
	default:
		return LanguageUnknown
	}
}

func shouldSkipDir(name string) bool {
	switch name {
	case ".git", ".repobridge-graph", "node_modules", "vendor", "dist", "build", "target", "bin", "obj", "__pycache__":
		return true
	default:
		return false
	}
}
