package updatecheck

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"time"
)

const DefaultCheckInterval = 24 * time.Hour

type Service struct {
	CurrentVersion string
	Client         Client
	StateStore     StateStore
	CheckInterval  time.Duration
	Now            func() time.Time
	ExecutablePath func() (string, error)
	GOOS           string
	GOARCH         string
}

type CheckResult struct {
	Available      bool
	CurrentVersion string
	LatestVersion  string
	ReleaseURL     string
	Release        Release
}

func (s Service) Check(ctx context.Context) (CheckResult, error) {
	if !IsReleaseVersion(s.CurrentVersion) {
		return CheckResult{CurrentVersion: s.CurrentVersion}, nil
	}

	release, err := s.Client.LatestRelease(ctx)
	if err != nil {
		return CheckResult{}, err
	}
	cmp, err := CompareVersions(release.TagName, s.CurrentVersion)
	if err != nil {
		return CheckResult{}, err
	}
	return CheckResult{
		Available:      cmp > 0,
		CurrentVersion: s.CurrentVersion,
		LatestVersion:  release.TagName,
		ReleaseURL:     release.HTMLURL,
		Release:        release,
	}, nil
}

func (s Service) OpportunisticCheck(ctx context.Context) (CheckResult, error) {
	if !IsReleaseVersion(s.CurrentVersion) {
		return CheckResult{CurrentVersion: s.CurrentVersion}, nil
	}

	store := s.StateStore
	if store.HomeDir == "" {
		defaultStore, err := DefaultStateStore()
		if err != nil {
			return CheckResult{CurrentVersion: s.CurrentVersion}, nil
		}
		store = defaultStore
	}

	now := s.now()
	interval := s.CheckInterval
	if interval <= 0 {
		interval = DefaultCheckInterval
	}

	if state, err := store.Read(); err == nil && state.IsFresh(now, interval) {
		return s.checkResultFromState(state), nil
	}

	result, err := s.Check(ctx)
	if err != nil {
		_ = store.Write(State{CheckedAt: now})
		return CheckResult{CurrentVersion: s.CurrentVersion}, nil
	}
	_ = store.Write(State{
		CheckedAt:  now,
		Latest:     result.LatestVersion,
		ReleaseURL: result.ReleaseURL,
	})
	return result, nil
}

func (s Service) Install(ctx context.Context, release Release) error {
	goos := s.GOOS
	if goos == "" {
		goos = runtime.GOOS
	}
	goarch := s.GOARCH
	if goarch == "" {
		goarch = runtime.GOARCH
	}

	archiveAsset, checksumAsset, err := release.SelectAssets(goos, goarch)
	if err != nil {
		return err
	}
	checksumContent, err := s.Client.Download(ctx, checksumAsset.DownloadURL)
	if err != nil {
		return err
	}
	checksums, err := ParseChecksums(checksumContent)
	if err != nil {
		return err
	}
	archiveContent, err := s.Client.Download(ctx, archiveAsset.DownloadURL)
	if err != nil {
		return err
	}
	if err := VerifyArchiveChecksum(archiveAsset.Name, archiveContent, checksums); err != nil {
		return err
	}

	tempDir, err := os.MkdirTemp("", "repobridge-update-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	extractedDir, err := ExtractArchive(archiveAsset.Name, archiveContent, tempDir)
	if err != nil {
		return err
	}
	executablePath := os.Executable
	if s.ExecutablePath != nil {
		executablePath = s.ExecutablePath
	}
	currentExecutable, err := executablePath()
	if err != nil {
		return err
	}
	if currentExecutable == "" {
		return fmt.Errorf("could not locate current executable")
	}
	return InstallExtractedRelease(extractedDir, currentExecutable, goos)
}

func (s Service) checkResultFromState(state State) CheckResult {
	result := CheckResult{
		CurrentVersion: s.CurrentVersion,
		LatestVersion:  state.Latest,
		ReleaseURL:     state.ReleaseURL,
	}
	if !IsReleaseVersion(s.CurrentVersion) || !IsReleaseVersion(state.Latest) {
		return result
	}
	cmp, err := CompareVersions(state.Latest, s.CurrentVersion)
	if err == nil && cmp > 0 {
		result.Available = true
	}
	return result
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now().UTC()
}
