package cli

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/spf13/cobra"
	"repobridge/internal/cache"
	"repobridge/internal/projectscan"
	"repobridge/internal/registry"
	"repobridge/internal/registry/repo"
	"repobridge/internal/source"
)

func newFetchCommand(opts Options) *cobra.Command {
	var cwd string
	var quiet bool
	cmd := &cobra.Command{
		Use:   "fetch <spec...>",
		Short: "Fetch source code into the cache",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			errOut := cmd.ErrOrStderr()
			fetched, cached, failed := 0, 0, 0
			for _, spec := range args {
				outcome, err := opts.app().EnsureCached(spec, source.Options{CWD: cwd, Verbose: !quiet})
				if err != nil {
					failed++
					fmt.Fprintf(errOut, "Failed %s: %v\n", spec, err)
					continue
				}
				if outcome.FromCache {
					cached++
					if !quiet {
						fmt.Fprintf(out, "Cached %s\n", formatOutcome(outcome, spec))
					}
				} else {
					fetched++
					if !quiet {
						fmt.Fprintf(out, "Fetched %s\n", formatOutcome(outcome, spec))
					}
				}
				if outcome.Warning != "" && !quiet {
					fmt.Fprintf(errOut, "Warning for %s: %s\n", spec, outcome.Warning)
				}
			}
			if !quiet {
				fmt.Fprintf(out, "Fetched %d source(s), %d already cached\n", fetched, cached)
			}
			if failed > 0 {
				return fmt.Errorf("%d source(s) failed", failed)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&cwd, "cwd", ".", "working directory for lockfile version detection")
	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "suppress progress output")
	return cmd
}

func newPathCommand(opts Options) *cobra.Command {
	var cwd string
	var verbose bool
	cmd := &cobra.Command{
		Use:   "path <spec...>",
		Short: "Print the absolute path to cached source",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			for _, spec := range args {
				outcome, err := opts.app().EnsureCached(spec, source.Options{CWD: cwd, Verbose: verbose})
				if err != nil {
					return err
				}
				fmt.Fprintln(out, outcome.Path)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&cwd, "cwd", ".", "working directory for lockfile version detection")
	cmd.Flags().BoolVar(&verbose, "verbose", false, "show fetch progress")
	return cmd
}

func newScanCommand(opts Options) *cobra.Command {
	var cwd string
	var jsonOutput bool
	var fetch bool
	var includeImports bool
	var noImports bool
	var limit int
	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Scan a project for dependency source specs",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			errOut := cmd.ErrOrStderr()
			if noImports {
				includeImports = false
			}
			result, err := projectscan.Scan(cwd, projectscan.Options{IncludeImports: includeImports})
			if err != nil {
				return err
			}
			if limit > 0 && len(result.Candidates) > limit {
				result.Candidates = result.Candidates[:limit]
			}
			if fetch {
				for _, candidate := range result.Candidates {
					outcome, err := opts.app().EnsureCached(candidate.Spec, source.Options{CWD: cwd, Verbose: !jsonOutput})
					if err != nil {
						fmt.Fprintf(errOut, "Failed %s: %v\n", candidate.Spec, err)
						continue
					}
					if !jsonOutput {
						if outcome.FromCache {
							fmt.Fprintf(out, "Cached %s\n", formatOutcome(outcome, candidate.Spec))
						} else {
							fmt.Fprintf(out, "Fetched %s\n", formatOutcome(outcome, candidate.Spec))
						}
					}
				}
			}
			if jsonOutput {
				content, err := json.MarshalIndent(result, "", "  ")
				if err != nil {
					return err
				}
				fmt.Fprintln(out, string(content))
				return nil
			}
			if len(result.Candidates) == 0 {
				fmt.Fprintln(out, "No dependency source specs found.")
				return nil
			}
			fmt.Fprintf(out, "Dependency source specs for %s:\n", result.Root)
			for _, candidate := range result.Candidates {
				fmt.Fprintf(out, "  %s (%s, confidence %d)\n", candidate.Spec, candidate.Ecosystem, candidate.Confidence)
				for _, reason := range candidate.Reasons {
					fmt.Fprintf(out, "    - %s\n", reason)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&cwd, "cwd", ".", "project directory to scan")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "print scan result as JSON")
	cmd.Flags().BoolVar(&fetch, "fetch", false, "fetch detected source specs into the cache")
	cmd.Flags().BoolVar(&includeImports, "include-imports", true, "include import hints from source files")
	cmd.Flags().BoolVar(&includeImports, "imports", true, "include import hints from source files")
	cmd.Flags().BoolVar(&noImports, "no-imports", false, "disable import hints from source files")
	cmd.Flags().IntVar(&limit, "limit", 0, "limit number of reported or fetched specs")
	return cmd
}

func newListCommand(opts Options) *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List cached sources",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			store := cache.NewSourceCache()
			result, err := store.List(cache.ListOptions{})
			if err != nil {
				return err
			}
			if jsonOutput {
				content, err := json.MarshalIndent(result.Index, "", "  ")
				if err != nil {
					return err
				}
				fmt.Fprintln(out, string(content))
				return nil
			}
			total := len(result.Sources)
			if total == 0 {
				fmt.Fprintln(out, "No sources cached yet.")
				return nil
			}
			hasPackages := false
			for _, source := range result.Sources {
				if source.Kind == cache.PackageSource {
					hasPackages = true
					break
				}
			}
			if hasPackages {
				fmt.Fprintln(out, "Packages:")
				for _, source := range result.Sources {
					if source.Kind != cache.PackageSource {
						continue
					}
					fmt.Fprintf(out, "  %s@%s (%s) %s\n", source.Name, source.Version, registry.Registry(source.Registry).Label(), source.Path)
				}
			}
			hasRepos := false
			for _, source := range result.Sources {
				if source.Kind == cache.RepoSource {
					hasRepos = true
					break
				}
			}
			if hasRepos {
				fmt.Fprintln(out, "Repositories:")
				for _, source := range result.Sources {
					if source.Kind != cache.RepoSource {
						continue
					}
					fmt.Fprintf(out, "  %s@%s %s\n", source.Name, source.Version, source.Path)
				}
			}
			fmt.Fprintf(out, "Total: %d source(s)\n", total)
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "print cache index as JSON")
	return cmd
}

func newRemoveCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "remove <spec...>",
		Aliases: []string{"rm"},
		Short:   "Remove cached sources",
		Args:    cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			errOut := cmd.ErrOrStderr()
			removed, missing, failed := 0, 0, 0
			for _, spec := range args {
				ok, err := removeSource(spec, out)
				if err != nil {
					failed++
					fmt.Fprintf(errOut, "Failed to remove %s: %v\n", spec, err)
					continue
				}
				if !ok {
					missing++
					fmt.Fprintf(errOut, "No cached source found for %s\n", spec)
					continue
				}
				removed++
			}
			if removed > 0 {
				fmt.Fprintf(out, "Removed %d source(s)\n", removed)
			}
			if failed > 0 {
				return fmt.Errorf("%d source(s) failed to remove", failed)
			}
			if missing > 0 && removed == 0 {
				return fmt.Errorf("%d source(s) not found", missing)
			}
			return nil
		},
	}
	return cmd
}

func newCleanCommand(opts Options) *cobra.Command {
	var packagesOnly bool
	var reposOnly bool
	var npmOnly bool
	var pypiOnly bool
	var cratesOnly bool
	var mavenOnly bool
	var nugetOnly bool
	cmd := &cobra.Command{
		Use:   "clean",
		Short: "Clean cached sources",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			count, err := cleanSources(cleanOptions{
				packagesOnly: packagesOnly,
				reposOnly:    reposOnly,
				registries:   selectedRegistries(npmOnly, pypiOnly, cratesOnly, mavenOnly, nugetOnly),
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(out, "Cleaned %d source(s)\n", count)
			return nil
		},
	}
	cmd.Flags().BoolVar(&packagesOnly, "packages", false, "only remove package sources")
	cmd.Flags().BoolVar(&reposOnly, "repos", false, "only remove repository sources")
	cmd.Flags().BoolVar(&npmOnly, "npm", false, "only remove npm package sources")
	cmd.Flags().BoolVar(&pypiOnly, "pypi", false, "only remove PyPI package sources")
	cmd.Flags().BoolVar(&cratesOnly, "crates", false, "only remove crates.io package sources")
	cmd.Flags().BoolVar(&mavenOnly, "maven", false, "only remove Maven package sources")
	cmd.Flags().BoolVar(&nugetOnly, "nuget", false, "only remove NuGet package sources")
	return cmd
}

func formatOutcome(outcome source.Outcome, fallback string) string {
	name := outcome.Name
	if name == "" {
		name = fallback
	}
	version := outcome.Version
	label := outcome.SourceLabel
	display := name
	if version != "" {
		display += "@" + version
	}
	if label != "" {
		display += " from " + label
	}
	return display
}

func removeSource(spec string, out io.Writer) (bool, error) {
	store := cache.NewSourceCache()
	if registry.DetectInputType(spec) == registry.RepoInput {
		parsed, ok := repo.ParseSpec(spec)
		if !ok {
			return false, fmt.Errorf("invalid repository spec: %s", spec)
		}
		displayName := fmt.Sprintf("%s/%s/%s", parsed.Host, parsed.Owner, parsed.Repo)
		result, err := store.Remove(cache.RemoveSelector{
			Kind: cache.RepoSource,
			Repo: cache.RepoSelector{
				DisplayName: displayName,
				Version:     parsed.Ref,
			},
		})
		removed := result.Matched > 0
		if removed && err == nil {
			fmt.Fprintf(out, "Removed %s\n", displayName)
		}
		return removed, err
	}

	parsed := registry.ParsePackageSpec(spec)
	if parsed.Name == "" {
		return false, fmt.Errorf("package name must not be empty")
	}
	result, err := store.Remove(cache.RemoveSelector{
		Kind: cache.PackageSource,
		Package: cache.PackageSelector{
			Name:     parsed.Name,
			Registry: string(parsed.Registry),
			Version:  parsed.Version,
		},
	})
	removed := result.Matched > 0
	if removed && err == nil {
		display := parsed.Name
		if parsed.Version != "" {
			display += "@" + parsed.Version
		}
		fmt.Fprintf(out, "Removed %s from %s\n", display, parsed.Registry.Label())
	}
	return removed, err
}

type cleanOptions struct {
	packagesOnly bool
	reposOnly    bool
	registries   map[string]bool
}

func selectedRegistries(npmOnly, pypiOnly, cratesOnly, mavenOnly, nugetOnly bool) map[string]bool {
	registries := map[string]bool{}
	if npmOnly {
		registries[string(registry.NPM)] = true
	}
	if pypiOnly {
		registries[string(registry.PyPI)] = true
	}
	if cratesOnly {
		registries[string(registry.Crates)] = true
	}
	if mavenOnly {
		registries[string(registry.Maven)] = true
	}
	if nugetOnly {
		registries[string(registry.NuGet)] = true
	}
	return registries
}

func cleanSources(opts cleanOptions) (int, error) {
	if opts.reposOnly && len(opts.registries) > 0 {
		return 0, fmt.Errorf("--repos cannot be combined with registry filters")
	}
	kinds := map[cache.SourceKind]bool{}
	if opts.packagesOnly {
		kinds[cache.PackageSource] = true
	}
	if opts.reposOnly {
		kinds[cache.RepoSource] = true
	}
	if len(opts.registries) > 0 {
		kinds[cache.PackageSource] = true
	}
	result, err := cache.NewSourceCache().Clean(cache.CleanOptions{
		Kinds:      kinds,
		Registries: opts.registries,
	})
	return result.Removed, err
}
