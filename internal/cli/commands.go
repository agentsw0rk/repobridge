package cli

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/spf13/cobra"
	"repobridge/internal/cache"
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

func newListCommand(opts Options) *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List cached sources",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			index, err := cache.ReadSources()
			if err != nil {
				return err
			}
			if jsonOutput {
				content, err := json.MarshalIndent(index, "", "  ")
				if err != nil {
					return err
				}
				fmt.Fprintln(out, string(content))
				return nil
			}
			total := len(index.Packages) + len(index.Repos)
			if total == 0 {
				fmt.Fprintln(out, "No sources cached yet.")
				return nil
			}
			if len(index.Packages) > 0 {
				fmt.Fprintln(out, "Packages:")
				for _, pkg := range index.Packages {
					path, err := cache.AbsolutePath(pkg.Path)
					if err != nil {
						return err
					}
					fmt.Fprintf(out, "  %s@%s (%s) %s\n", pkg.Name, pkg.Version, registry.Registry(pkg.Registry).Label(), path)
				}
			}
			if len(index.Repos) > 0 {
				fmt.Fprintln(out, "Repositories:")
				for _, repo := range index.Repos {
					path, err := cache.AbsolutePath(repo.Path)
					if err != nil {
						return err
					}
					fmt.Fprintf(out, "  %s@%s %s\n", repo.Name, repo.Version, path)
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
	cmd := &cobra.Command{
		Use:   "clean",
		Short: "Clean cached sources",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			count, err := cleanSources(cleanOptions{
				packagesOnly: packagesOnly,
				reposOnly:    reposOnly,
				registries:   selectedRegistries(npmOnly, pypiOnly, cratesOnly),
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
	if registry.DetectInputType(spec) == registry.RepoInput {
		parsed, ok := repo.ParseSpec(spec)
		if !ok {
			return false, fmt.Errorf("invalid repository spec: %s", spec)
		}
		displayName := fmt.Sprintf("%s/%s/%s", parsed.Host, parsed.Owner, parsed.Repo)
		var version *string
		if parsed.Ref != "" {
			version = &parsed.Ref
		}
		removed, err := cache.RemoveRepoSource(displayName, version)
		if removed && err == nil {
			fmt.Fprintf(out, "Removed %s\n", displayName)
		}
		return removed, err
	}

	parsed := registry.ParsePackageSpec(spec)
	if parsed.Name == "" {
		return false, fmt.Errorf("package name must not be empty")
	}
	var removed bool
	var err error
	if parsed.Version != "" {
		removed, _, err = cache.RemovePackageSourceVersion(parsed.Name, string(parsed.Registry), parsed.Version)
	} else {
		removed, _, err = cache.RemovePackageSource(parsed.Name, string(parsed.Registry))
	}
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

func selectedRegistries(npmOnly, pypiOnly, cratesOnly bool) map[string]bool {
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
	return registries
}

func cleanSources(opts cleanOptions) (int, error) {
	if opts.reposOnly && len(opts.registries) > 0 {
		return 0, fmt.Errorf("--repos cannot be combined with registry filters")
	}

	packages, repos, err := cache.ListSources()
	if err != nil {
		return 0, err
	}
	if len(packages) == 0 && len(repos) == 0 {
		return 0, nil
	}

	cleanPackages := !opts.reposOnly
	cleanRepos := !opts.packagesOnly && len(opts.registries) == 0
	if opts.packagesOnly {
		cleanPackages = true
		cleanRepos = false
	}
	if opts.reposOnly {
		cleanPackages = false
		cleanRepos = true
	}
	if len(opts.registries) > 0 {
		cleanPackages = true
		cleanRepos = false
	}

	cleaned := 0
	if cleanPackages {
		type packageKey struct {
			name     string
			registry string
		}
		seen := map[packageKey]bool{}
		for _, pkg := range packages {
			if len(opts.registries) > 0 && !opts.registries[pkg.Registry] {
				continue
			}
			key := packageKey{name: pkg.Name, registry: pkg.Registry}
			if seen[key] {
				cleaned++
				continue
			}
			seen[key] = true
			removed, _, err := cache.RemovePackageSource(pkg.Name, pkg.Registry)
			if err != nil {
				return cleaned, err
			}
			if removed {
				cleaned++
			}
		}
	}
	if cleanRepos {
		type repoKey struct {
			name    string
			version string
		}
		seen := map[repoKey]bool{}
		for _, repoEntry := range repos {
			key := repoKey{name: repoEntry.Name, version: repoEntry.Version}
			if seen[key] {
				continue
			}
			seen[key] = true
			version := repoEntry.Version
			removed, err := cache.RemoveRepoSource(repoEntry.Name, &version)
			if err != nil {
				return cleaned, err
			}
			if removed {
				cleaned++
			}
		}
	}
	return cleaned, nil
}
