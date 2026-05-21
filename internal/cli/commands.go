package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"repobridge/internal/agentinstall"
	"repobridge/internal/astgraph"
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
			indexer := opts.indexer()
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
				indexer.Schedule(outcome)
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
			indexer := opts.indexer()
			for _, spec := range args {
				outcome, err := opts.app().EnsureCached(spec, source.Options{CWD: cwd, Verbose: verbose})
				if err != nil {
					return err
				}
				indexer.Schedule(outcome)
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
				indexer := opts.indexer()
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
					indexer.Schedule(outcome)
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

func newSearchCommand(opts Options) *cobra.Command {
	var cwd string
	var jsonOutput bool
	var limit int
	var kinds []string
	var languages []string
	var paths []string
	var calls []string
	var noSyncIndex bool

	cmd := &cobra.Command{
		Use:   "search <spec> <query>",
		Short: "Search cached source with the AST-Graph Engine",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			query := appendSearchFilters(args[1], kinds, languages, paths, calls)
			results, err := opts.app().SearchCode(args[0], query, astgraph.SearchOptions{
				CWD:       cwd,
				SyncIndex: !noSyncIndex,
				Limit:     limit,
			})
			if err != nil {
				return err
			}
			if jsonOutput {
				content, err := json.MarshalIndent(results, "", "  ")
				if err != nil {
					return err
				}
				fmt.Fprintln(out, string(content))
				return nil
			}
			if len(results) == 0 {
				fmt.Fprintln(out, "No AST-Graph Engine results found.")
				return nil
			}
			for _, result := range results {
				printSearchResult(out, result)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&cwd, "cwd", ".", "working directory for lockfile version detection")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "print search results as JSON")
	cmd.Flags().IntVar(&limit, "limit", 0, "limit number of search results")
	cmd.Flags().StringArrayVar(&kinds, "kind", nil, "filter by node kind")
	cmd.Flags().StringArrayVar(&languages, "lang", nil, "filter by language")
	cmd.Flags().StringArrayVar(&paths, "path", nil, "filter by path substring")
	cmd.Flags().StringArrayVar(&calls, "calls", nil, "filter by called symbol")
	cmd.Flags().BoolVar(&noSyncIndex, "no-sync-index", false, "do not build a missing or stale AST-Graph Engine index")
	return cmd
}

func newGraphStatusCommand(opts Options) *cobra.Command {
	var cwd string
	var jsonOutput bool
	var noSyncIndex bool

	cmd := &cobra.Command{
		Use:   "status <spec>",
		Short: "Show cached AST-Graph Engine status",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			result, err := opts.app().ASTGraphStatus(args[0], astgraph.GraphInspectOptions{
				CWD:       cwd,
				SyncIndex: !noSyncIndex,
			})
			if err != nil {
				return err
			}
			if jsonOutput {
				return printJSON(out, result)
			}
			printGraphStatus(out, result)
			return nil
		},
	}
	cmd.Flags().StringVar(&cwd, "cwd", ".", "working directory for lockfile version detection")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "print graph status as JSON")
	cmd.Flags().BoolVar(&noSyncIndex, "no-sync-index", false, "do not build a missing or stale AST-Graph Engine index")
	return cmd
}

func newGraphFilesCommand(opts Options) *cobra.Command {
	var cwd string
	var jsonOutput bool
	var noSyncIndex bool
	var limit int
	var pathFilter string

	cmd := &cobra.Command{
		Use:   "files <spec>",
		Short: "List files in the cached AST-Graph Engine index",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			result, err := opts.app().ASTGraphFiles(args[0], astgraph.GraphInspectOptions{
				CWD:        cwd,
				SyncIndex:  !noSyncIndex,
				Limit:      limit,
				PathFilter: pathFilter,
			})
			if err != nil {
				return err
			}
			if jsonOutput {
				return printJSON(out, result)
			}
			printGraphFiles(out, result)
			return nil
		},
	}
	cmd.Flags().StringVar(&cwd, "cwd", ".", "working directory for lockfile version detection")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "print graph files as JSON")
	cmd.Flags().BoolVar(&noSyncIndex, "no-sync-index", false, "do not build a missing or stale AST-Graph Engine index")
	cmd.Flags().IntVar(&limit, "limit", 0, "limit number of files")
	cmd.Flags().StringVar(&pathFilter, "path", "", "filter files by path substring")
	return cmd
}

func newGraphNodeCommand(opts Options) *cobra.Command {
	var cwd string
	var jsonOutput bool
	var noSyncIndex bool
	var limit int
	var sourceLines int

	cmd := &cobra.Command{
		Use:   "node <spec> <id-or-name>",
		Short: "Show details for one AST-Graph Engine node",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			result, err := opts.app().ASTGraphNode(args[0], args[1], astgraph.GraphInspectOptions{
				CWD:         cwd,
				SyncIndex:   !noSyncIndex,
				Limit:       limit,
				SourceLines: sourceLines,
			})
			if err != nil {
				return err
			}
			if jsonOutput {
				return printJSON(out, result)
			}
			printGraphNode(out, args[1], result)
			return nil
		},
	}
	cmd.Flags().StringVar(&cwd, "cwd", ".", "working directory for lockfile version detection")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "print graph node as JSON")
	cmd.Flags().BoolVar(&noSyncIndex, "no-sync-index", false, "do not build a missing or stale AST-Graph Engine index")
	cmd.Flags().IntVar(&limit, "limit", 0, "limit ambiguous node matches")
	cmd.Flags().IntVar(&sourceLines, "source-lines", 0, "include up to this many source lines")
	return cmd
}

func newCallersCommand(opts Options) *cobra.Command {
	return newCallgraphCommand(opts, astgraph.CallgraphDirectionCallers)
}

func newCalleesCommand(opts Options) *cobra.Command {
	return newCallgraphCommand(opts, astgraph.CallgraphDirectionCallees)
}

func newImpactCommand(opts Options) *cobra.Command {
	return newCallgraphCommand(opts, astgraph.CallgraphDirectionImpact)
}

func newContextCommand(opts Options) *cobra.Command {
	return newContextLikeCommand(opts, astgraph.ContextModeContext)
}

func newExploreCommand(opts Options) *cobra.Command {
	return newContextLikeCommand(opts, astgraph.ContextModeExplore)
}

func newContextLikeCommand(opts Options, mode astgraph.ContextMode) *cobra.Command {
	var cwd string
	var jsonOutput bool
	var noSyncIndex bool
	var limit int
	var depth int
	var budget string

	short := "Build focused task context from the cached AST-Graph Engine"
	if mode == astgraph.ContextModeExplore {
		short = "Explore broader AST-Graph Engine context for symbols and tasks"
	}

	cmd := &cobra.Command{
		Use:   string(mode) + " <spec> <query>",
		Short: short,
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			result, err := opts.app().ASTGraphContext(args[0], args[1], astgraph.ContextOptions{
				CWD:       cwd,
				SyncIndex: !noSyncIndex,
				Limit:     limit,
				Depth:     depth,
				Budget:    budget,
				Mode:      mode,
			})
			if err != nil {
				return err
			}
			if jsonOutput {
				return printJSON(out, result)
			}
			printContextResult(out, result)
			return nil
		},
	}
	cmd.Flags().StringVar(&cwd, "cwd", ".", "working directory for lockfile version detection")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "print context result as JSON")
	cmd.Flags().BoolVar(&noSyncIndex, "no-sync-index", false, "do not build a missing or stale AST-Graph Engine index")
	cmd.Flags().IntVar(&limit, "limit", 0, "override budget search result limit")
	cmd.Flags().IntVar(&depth, "depth", 0, "override budget relationship traversal depth")
	cmd.Flags().StringVar(&budget, "budget", "", "context budget: small, medium, or large")
	return cmd
}

func newCallgraphCommand(opts Options, direction astgraph.CallgraphDirection) *cobra.Command {
	var cwd string
	var jsonOutput bool
	var noSyncIndex bool
	var includeUnresolved bool
	var limit int
	var depth int
	var kinds []string
	var languages []string
	var paths []string

	cmd := &cobra.Command{
		Use:   string(direction) + " <spec> <symbol>",
		Short: "Traverse cached AST-Graph Engine " + string(direction),
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			result, err := opts.app().ASTGraphCallgraph(args[0], args[1], astgraph.CallgraphOptions{
				CWD:               cwd,
				SyncIndex:         !noSyncIndex,
				Limit:             limit,
				Depth:             depth,
				Kinds:             parseNodeKinds(kinds),
				Languages:         parseLanguages(languages),
				PathFilters:       paths,
				IncludeUnresolved: includeUnresolved,
				Direction:         direction,
			})
			if err != nil {
				return err
			}
			if jsonOutput {
				return printJSON(out, result)
			}
			printCallgraphResult(out, result)
			return nil
		},
	}
	cmd.Flags().StringVar(&cwd, "cwd", ".", "working directory for lockfile version detection")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "print call graph results as JSON")
	cmd.Flags().BoolVar(&noSyncIndex, "no-sync-index", false, "do not build a missing or stale AST-Graph Engine index")
	cmd.Flags().BoolVar(&includeUnresolved, "include-unresolved", false, "include unresolved call references")
	cmd.Flags().IntVar(&limit, "limit", 0, "limit number of call graph edges")
	cmd.Flags().IntVar(&depth, "depth", 1, "call graph traversal depth")
	cmd.Flags().StringArrayVar(&kinds, "kind", nil, "filter by node kind")
	cmd.Flags().StringArrayVar(&languages, "lang", nil, "filter by language")
	cmd.Flags().StringArrayVar(&paths, "path", nil, "filter by path substring")
	return cmd
}

func parseNodeKinds(values []string) []astgraph.NodeKind {
	kinds := make([]astgraph.NodeKind, 0, len(values))
	for _, value := range values {
		kinds = append(kinds, astgraph.NodeKind(value))
	}
	return kinds
}

func parseLanguages(values []string) []astgraph.Language {
	languages := make([]astgraph.Language, 0, len(values))
	for _, value := range values {
		languages = append(languages, astgraph.Language(value))
	}
	return languages
}

func appendSearchFilters(query string, kinds, languages, paths, calls []string) string {
	filters := make([]string, 0, len(kinds)+len(languages)+len(paths)+len(calls))
	for _, kind := range kinds {
		filters = append(filters, "kind:"+kind)
	}
	for _, language := range languages {
		filters = append(filters, "lang:"+language)
	}
	for _, path := range paths {
		filters = append(filters, "path:"+path)
	}
	for _, call := range calls {
		filters = append(filters, "calls:"+call)
	}
	if len(filters) == 0 {
		return query
	}
	if query == "" {
		return strings.Join(filters, " ")
	}
	return query + " " + strings.Join(filters, " ")
}

func printJSON(out io.Writer, value any) error {
	content, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintln(out, string(content))
	return nil
}

func printGraphStatus(out io.Writer, result astgraph.GraphInspectStatus) {
	source := result.Source
	if source == "" {
		source = "(unknown source)"
	}
	fmt.Fprintln(out, source)
	if result.GraphPath != "" {
		fmt.Fprintf(out, "  graph: %s\n", result.GraphPath)
	}
	fmt.Fprintf(out, "  status: %s\n", result.Status)
	if result.SourcePath != "" {
		fmt.Fprintf(out, "  source: %s\n", result.SourcePath)
	}
	if result.SchemaVersion != 0 {
		fmt.Fprintf(out, "  schemaVersion: %d\n", result.SchemaVersion)
	}
	fmt.Fprintf(out, "  files: %d\n", result.Counts.Files)
	fmt.Fprintf(out, "  nodes: %d\n", result.Counts.Nodes)
	fmt.Fprintf(out, "  edges: %d\n", result.Counts.Edges)
	fmt.Fprintf(out, "  unresolved: %d\n", result.Counts.Unresolved)
	fmt.Fprintf(out, "  warnings: %d\n", result.Counts.Warnings)
	if !result.IndexedAt.IsZero() {
		fmt.Fprintf(out, "  indexedAt: %s\n", result.IndexedAt.Format(time.RFC3339))
	}
	if result.ErrorText != "" {
		fmt.Fprintf(out, "  error: %s\n", result.ErrorText)
	}
}

func printGraphFiles(out io.Writer, result astgraph.GraphFilesResult) {
	source := result.Source
	if source == "" {
		source = "(unknown source)"
	}
	fmt.Fprintln(out, source)
	if len(result.Files) == 0 {
		fmt.Fprintln(out, "  No indexed files found.")
		return
	}
	for _, file := range result.Files {
		fmt.Fprintf(out, "  %s %s nodes:%d bytes:%d\n", file.Path, file.Language, file.NodeCount, file.Size)
	}
}

func printGraphNode(out io.Writer, lookup string, result astgraph.GraphNodeLookupResult) {
	source := result.Source
	if source == "" {
		source = "(unknown source)"
	}
	fmt.Fprintln(out, source)
	switch {
	case result.Node != nil:
		printGraphNodeDetail(out, "  ", *result.Node)
	case len(result.Matches) > 0:
		fmt.Fprintf(out, "  Multiple AST-Graph Engine nodes matched %s:\n", lookup)
		for _, match := range result.Matches {
			location := formatSearchLocation(match.Path, match.StartLine)
			fmt.Fprintf(out, "    %s %s %s %s\n", match.ID, match.Kind, bestNodeDisplayName(match), location)
		}
	default:
		fmt.Fprintf(out, "  No AST-Graph Engine node matched %s.\n", lookup)
	}
}

func printGraphNodeDetail(out io.Writer, prefix string, node astgraph.GraphNodeDetail) {
	location := formatSearchLocation(node.Path, node.StartLine)
	descriptor := strings.TrimSpace(fmt.Sprintf("%s %s", node.Kind, node.Name))
	if location == "" {
		fmt.Fprintf(out, "%s%s\n", prefix, descriptor)
	} else {
		fmt.Fprintf(out, "%s%s %s\n", prefix, descriptor, location)
	}
	if node.ID != "" {
		fmt.Fprintf(out, "%s  id: %s\n", prefix, node.ID)
	}
	if node.QualifiedName != "" {
		fmt.Fprintf(out, "%s  qualified: %s\n", prefix, node.QualifiedName)
	}
	if node.Language != "" {
		fmt.Fprintf(out, "%s  language: %s\n", prefix, node.Language)
	}
	if node.Signature != "" {
		fmt.Fprintf(out, "%s  signature: %s\n", prefix, node.Signature)
	}
	if len(node.Calls) > 0 {
		fmt.Fprintf(out, "%s  calls: %s\n", prefix, strings.Join(node.Calls, ", "))
	}
	if len(node.Source) > 0 {
		fmt.Fprintf(out, "%s  source:\n", prefix)
		for _, line := range node.Source {
			fmt.Fprintf(out, "%s    %d: %s\n", prefix, line.Line, line.Text)
		}
	}
}

func printCallgraphResult(out io.Writer, result astgraph.CallgraphResult) {
	fmt.Fprintf(out, "%s of %s\n", result.Direction, result.Symbol)
	if len(result.Matches) > 0 {
		fmt.Fprintf(out, "  Multiple AST-Graph Engine nodes matched %s:\n", result.Symbol)
		for _, match := range result.Matches {
			location := formatSearchLocation(match.Path, match.StartLine)
			fmt.Fprintf(out, "    %s %s %s %s\n", match.ID, match.Kind, bestNodeDisplayName(match), location)
		}
		return
	}
	if len(result.Edges) == 0 {
		fmt.Fprintln(out, "  No call graph results found.")
		return
	}
	for _, edge := range result.Edges {
		target := callgraphDisplayNode(result.Direction, edge)
		location := formatSearchLocation(target.Path, target.StartLine)
		fmt.Fprintf(out, "  %s %s %s\n", target.Kind, bestNodeDisplayName(target), location)
		reference := bestCallgraphReference(result.Direction, edge)
		if edge.Unresolved {
			fmt.Fprintf(out, "    unresolved %s at line %d\n", reference, edge.Line)
			continue
		}
		if edge.Line > 0 {
			fmt.Fprintf(out, "    %s %s at line %d\n", edge.Kind, reference, edge.Line)
		} else {
			fmt.Fprintf(out, "    %s %s\n", edge.Kind, reference)
		}
	}
}

func printContextResult(out io.Writer, result astgraph.ContextResult) {
	fmt.Fprintf(out, "%s: %s\n", result.Mode, result.Query)
	source := result.Source
	if source == "" {
		source = "(unknown source)"
	}
	fmt.Fprintf(out, "source: %s\n", source)
	fmt.Fprintf(out, "budget: %s search:%d snippets:%d lines:%d depth:%d\n",
		result.Budget.Name,
		result.Budget.SearchLimit,
		result.Budget.SnippetCount,
		result.Budget.SourceLines,
		result.Budget.Depth,
	)

	fmt.Fprintln(out, "Entry points")
	if len(result.EntryPoints) == 0 {
		fmt.Fprintln(out, "  No entry points found.")
	} else {
		for _, entry := range result.EntryPoints {
			location := formatSearchLocation(entry.Path, entry.StartLine)
			fmt.Fprintf(out, "  %s %s %s\n", entry.Kind, bestNodeDisplayName(entry), location)
		}
	}

	fmt.Fprintln(out, "Relationships")
	if len(result.Relationships) == 0 {
		fmt.Fprintln(out, "  No relationships found.")
	} else {
		for _, edge := range result.Relationships {
			left := bestNodeDisplayName(edge.From)
			right := bestNodeDisplayName(edge.To)
			if edge.Unresolved && edge.ReferenceName != "" {
				right = edge.ReferenceName
			}
			if edge.Line > 0 {
				fmt.Fprintf(out, "  %s -> %s %s at line %d\n", left, right, edge.Kind, edge.Line)
			} else {
				fmt.Fprintf(out, "  %s -> %s %s\n", left, right, edge.Kind)
			}
		}
	}

	fmt.Fprintln(out, "Snippets")
	if len(result.Snippets) == 0 {
		fmt.Fprintln(out, "  No snippets found.")
	} else {
		for _, snippet := range result.Snippets {
			fmt.Fprintf(out, "  %s:%d-%d\n", snippet.Path, snippet.StartLine, snippet.EndLine)
			for _, line := range snippet.Lines {
				fmt.Fprintf(out, "    %d: %s\n", line.Line, line.Text)
			}
		}
	}

	fmt.Fprintln(out, "Related files")
	if len(result.RelatedFiles) == 0 {
		fmt.Fprintln(out, "  No related files found.")
	} else {
		for _, file := range result.RelatedFiles {
			fmt.Fprintf(out, "  %s %s nodes:%d\n", file.Path, file.Language, file.NodeCount)
		}
	}

	if len(result.Warnings) > 0 {
		fmt.Fprintln(out, "Warnings")
		for _, warning := range result.Warnings {
			fmt.Fprintf(out, "  %s\n", warning)
		}
	}
	fmt.Fprintf(out, "Stats terms:%d entryPoints:%d relationships:%d snippets:%d relatedFiles:%d\n",
		result.Stats.Terms,
		result.Stats.EntryPoints,
		result.Stats.Relationships,
		result.Stats.Snippets,
		result.Stats.RelatedFiles,
	)
}

func callgraphDisplayNode(direction astgraph.CallgraphDirection, edge astgraph.CallgraphEdge) astgraph.GraphNodeDetail {
	if direction == astgraph.CallgraphDirectionCallees {
		return edge.To
	}
	return edge.From
}

func bestCallgraphReference(direction astgraph.CallgraphDirection, edge astgraph.CallgraphEdge) string {
	if edge.ReferenceName != "" {
		return edge.ReferenceName
	}
	if direction == astgraph.CallgraphDirectionCallees {
		return bestNodeDisplayName(edge.To)
	}
	return bestNodeDisplayName(edge.To)
}

func bestNodeDisplayName(node astgraph.GraphNodeDetail) string {
	if node.QualifiedName != "" {
		return node.QualifiedName
	}
	return node.Name
}

func printSearchResult(out io.Writer, result astgraph.SearchResult) {
	source := result.Source
	if source == "" {
		source = "(unknown source)"
	}
	descriptor := strings.TrimSpace(fmt.Sprintf("%s %s", result.Kind, result.Name))
	if descriptor == "" {
		descriptor = "(unknown)"
	}
	location := formatSearchLocation(result.Path, result.StartLine)
	fmt.Fprintln(out, source)
	if location == "" {
		fmt.Fprintf(out, "  %s\n", descriptor)
	} else {
		fmt.Fprintf(out, "  %s %s\n", descriptor, location)
	}
	if len(result.Calls) > 0 {
		fmt.Fprintf(out, "    calls: %s\n", strings.Join(result.Calls, ", "))
	}
}

func newInstallAgentCommand(opts Options) *cobra.Command {
	var target string
	var version string
	var dryRun bool
	var printConfig bool
	var home string

	cmd := &cobra.Command{
		Use:   "install-agent",
		Short: "Install RepoBridge skill files for coding agents",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			options := agentinstall.Options{
				Target:  target,
				Version: version,
				HomeDir: home,
				DryRun:  dryRun || printConfig,
			}
			if printConfig {
				result, err := agentinstall.PrintConfig(options)
				if err != nil {
					return err
				}
				printAgentInstallConfig(out, result)
				return nil
			}
			result, err := agentinstall.Apply(options)
			if err != nil {
				return err
			}
			printAgentInstallResult(out, result, dryRun)
			return nil
		},
	}
	cmd.Flags().StringVar(&target, "target", "", "agent target: codex, claude, cursor, opencode, or all")
	cmd.Flags().StringVar(&version, "version", "", "pinned RepoBridge version to document in installed skill")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show planned skill installation changes without writing")
	cmd.Flags().BoolVar(&printConfig, "print-config", false, "print rendered skill files without writing")
	cmd.Flags().StringVar(&home, "home", "", "override home directory for installation")
	return cmd
}

func printAgentInstallResult(out io.Writer, result agentinstall.Result, dryRun bool) {
	agentinstall.SortFiles(result.Files)
	prefix := "Installed"
	if dryRun {
		prefix = "Would install"
	}
	changed := 0
	for _, file := range result.Files {
		if file.Action != "unchanged" {
			changed++
		}
		if file.BackupPath != "" {
			fmt.Fprintf(out, "%s %s %s backup:%s\n", file.Target, file.Action, file.Path, file.BackupPath)
			continue
		}
		fmt.Fprintf(out, "%s %s %s\n", file.Target, file.Action, file.Path)
	}
	fmt.Fprintf(out, "%s %d file(s)\n", prefix, changed)
}

func printAgentInstallConfig(out io.Writer, result agentinstall.Result) {
	agentinstall.SortFiles(result.Files)
	for _, file := range result.Files {
		fmt.Fprintf(out, "---\ntarget: %s\nfile: %s\n", file.Target, file.Path)
		fmt.Fprintln(out, string(file.Content))
	}
}

func formatSearchLocation(path string, startLine int) string {
	if startLine <= 0 {
		return path
	}
	return fmt.Sprintf("%s:%d", path, startLine)
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
