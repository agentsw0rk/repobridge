package parser

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"repobridge/internal/astgraph/model"
)

type Options struct {
	MaxFileSize int64
}

type SourceFile struct {
	AbsolutePath string
	RelativePath string
	Language     model.Language
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
		if language == model.LanguageUnknown {
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

func DetectLanguage(path string) model.Language {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		return model.LanguageGo
	case ".java":
		return model.LanguageJava
	case ".kt", ".kts":
		return model.LanguageKotlin
	case ".cs":
		return model.LanguageCSharp
	case ".js", ".jsx", ".mjs", ".cjs":
		return model.LanguageJavaScript
	case ".ts", ".tsx", ".mts", ".cts":
		return model.LanguageTypeScript
	case ".py":
		return model.LanguagePython
	case ".rs":
		return model.LanguageRust
	default:
		return model.LanguageUnknown
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
