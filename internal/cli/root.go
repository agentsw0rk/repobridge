package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"repobridge/internal/astgraph"
	"repobridge/internal/astgraph/store"
	"repobridge/internal/cache"
	"repobridge/internal/source"
	"repobridge/internal/updatecheck"
)

type Options struct {
	Version       string
	Stdout        io.Writer
	Stderr        io.Writer
	App           App
	Indexer       IndexScheduler
	UpdateChecker UpdateChecker
}

type App interface {
	EnsureCached(spec string, opts source.Options) (source.Outcome, error)
	SearchCode(spec, rawQuery string, opts astgraph.SearchOptions) ([]astgraph.SearchResult, error)
	ASTGraphStatus(spec string, opts astgraph.GraphInspectOptions) (astgraph.GraphInspectStatus, error)
	ASTGraphFiles(spec string, opts astgraph.GraphInspectOptions) (astgraph.GraphFilesResult, error)
	ASTGraphNode(spec, lookup string, opts astgraph.GraphInspectOptions) (astgraph.GraphNodeLookupResult, error)
	ASTGraphCallgraph(spec, symbol string, opts astgraph.CallgraphOptions) (astgraph.CallgraphResult, error)
	ASTGraphContext(spec, query string, opts astgraph.ContextOptions) (astgraph.ContextResult, error)
}

type IndexScheduler interface {
	Schedule(source.Outcome)
}

type UpdateChecker interface {
	Check(context.Context) (updatecheck.CheckResult, error)
	OpportunisticCheck(context.Context) (updatecheck.CheckResult, error)
	Install(context.Context, updatecheck.Release) error
}

const logoGreen = "\x1b[38;2;13;188;121m"
const logoReset = "\x1b[0m"

var rootLogoLines = []string{
	"                                █               █        █                ",
	"▄▄ ▄▄    ▄▄▄    ▄ ▄▄     ▄▄▄    █ ▄▄   ▄▄ ▄▄   ▄▄     ▄▄▄█    ▄▄▄ ▄   ▄▄▄ ",
	" █▀     █   █   █▀  █   █   █   █▀  █   █▀      █    █   █   █   █   █   █",
	" █      █▀▀▀▀   █   █   █   █   █   █   █       █    █   █   ▀▄▄▄▀   █▀▀▀▀",
	" █      ▀▄▄▄▀   █▀▄▄▀   ▀▄▄▄▀   █▀▄▄▀   █      ▄█▄   ▀▄▄▀█    █▄▄    ▀▄▄▄▀",
	"                █                                            █   █        ",
	"                ▀                                             ▀▀▀         ",
}

func printRootLogo(out io.Writer) {
	for _, line := range rootLogoLines {
		for _, r := range line {
			if r == ' ' {
				fmt.Fprint(out, " ")
				continue
			}
			fmt.Fprintf(out, "%s%c%s", logoGreen, r, logoReset)
		}
		fmt.Fprintln(out)
	}
	fmt.Fprintln(out)
}

type defaultApp struct{}

func (defaultApp) EnsureCached(spec string, opts source.Options) (source.Outcome, error) {
	return source.EnsureCached(spec, opts)
}

func (defaultApp) SearchCode(spec, rawQuery string, opts astgraph.SearchOptions) ([]astgraph.SearchResult, error) {
	service := astgraph.NewSearchService(astgraph.SearchServiceOptions{
		Resolver: defaultApp{},
		StoreOpener: func(dir string) (astgraph.GraphStore, error) {
			return store.Open(dir)
		},
	})
	return service.Search(spec, rawQuery, opts)
}

func (defaultApp) ASTGraphStatus(spec string, opts astgraph.GraphInspectOptions) (astgraph.GraphInspectStatus, error) {
	return defaultInspectService().Status(spec, opts)
}

func (defaultApp) ASTGraphFiles(spec string, opts astgraph.GraphInspectOptions) (astgraph.GraphFilesResult, error) {
	return defaultInspectService().Files(spec, opts)
}

func (defaultApp) ASTGraphNode(spec, lookup string, opts astgraph.GraphInspectOptions) (astgraph.GraphNodeLookupResult, error) {
	return defaultInspectService().Node(spec, lookup, opts)
}

func (defaultApp) ASTGraphCallgraph(spec, symbol string, opts astgraph.CallgraphOptions) (astgraph.CallgraphResult, error) {
	return astgraph.NewCallgraphService(astgraph.SearchServiceOptions{
		Resolver: defaultApp{},
		StoreOpener: func(dir string) (astgraph.GraphStore, error) {
			return store.Open(dir)
		},
	}).Callgraph(spec, symbol, opts)
}

func (defaultApp) ASTGraphContext(spec, query string, opts astgraph.ContextOptions) (astgraph.ContextResult, error) {
	return astgraph.NewContextService(astgraph.SearchServiceOptions{
		Resolver: defaultApp{},
		StoreOpener: func(dir string) (astgraph.GraphStore, error) {
			return store.Open(dir)
		},
	}).Context(spec, query, opts)
}

func defaultInspectService() *astgraph.InspectService {
	return astgraph.NewInspectService(astgraph.SearchServiceOptions{
		Resolver: defaultApp{},
		StoreOpener: func(dir string) (astgraph.GraphStore, error) {
			return store.Open(dir)
		},
	})
}

func (o Options) stdout() io.Writer {
	if o.Stdout != nil {
		return o.Stdout
	}
	return os.Stdout
}

func (o Options) stderr() io.Writer {
	if o.Stderr != nil {
		return o.Stderr
	}
	return os.Stderr
}

func (o Options) app() App {
	if o.App != nil {
		return o.App
	}
	return defaultApp{}
}

func (o Options) indexer() IndexScheduler {
	if o.Indexer != nil {
		return o.Indexer
	}
	return newProcessIndexScheduler()
}

func updateCheckerForOptions(opts Options) UpdateChecker {
	if opts.UpdateChecker != nil {
		return opts.UpdateChecker
	}
	return updatecheck.Service{CurrentVersion: opts.Version}
}

func maybeRunUpdateCheck(cmd *cobra.Command, opts Options) {
	if os.Getenv("REPOBRIDGE_NO_UPDATE_CHECK") == "1" {
		return
	}
	if !updatecheck.IsReleaseVersion(opts.Version) {
		return
	}
	if cmd.Name() == "self-update" || strings.HasPrefix(cmd.Name(), "__") {
		return
	}

	result, err := updateCheckerForOptions(opts).OpportunisticCheck(cmd.Context())
	if err != nil || !result.Available {
		return
	}
	latest := selfUpdateLatestVersion(result)
	if latest == "" {
		return
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "RepoBridge %s is available; run `repobridge self-update`\n", latest)
}

func indexOutcome(outcome source.Outcome) error {
	return indexOutcomePathWithGraphDir(outcome.Path, outcome.GraphPath)
}

func indexOutcomePath(sourcePath string) error {
	return indexOutcomePathWithGraphDir(sourcePath, "")
}

func indexOutcomePathWithGraphDir(sourcePath, graphDir string) error {
	startedAt := time.Now().UTC()
	if graphDir == "" {
		var err error
		graphDir, err = cache.GraphDirForSource(sourcePath)
		if err != nil {
			return err
		}
	}
	graph, err := store.Open(graphDir)
	if err != nil {
		return err
	}
	defer graph.Close()

	index, err := indexSourcePath(sourcePath)
	if err != nil {
		_ = graph.MarkFailed(sourcePath, startedAt, err)
		return err
	}
	return graph.Replace(index)
}

var indexSourcePath = func(sourcePath string) (astgraph.IndexResult, error) {
	return astgraph.NewIndexer(astgraph.IndexOptions{}).Index(sourcePath)
}

type processIndexScheduler struct {
	executable string
}

func newProcessIndexScheduler() IndexScheduler {
	executable, err := os.Executable()
	if err != nil {
		return processIndexScheduler{}
	}
	return processIndexScheduler{executable: executable}
}

func (s processIndexScheduler) Schedule(outcome source.Outcome) {
	if strings.TrimSpace(s.executable) == "" || strings.TrimSpace(outcome.Path) == "" {
		return
	}
	args := []string{"__astgraph-index", outcome.Path}
	if strings.TrimSpace(outcome.GraphPath) != "" {
		args = []string{"__astgraph-index", "--graph-dir", outcome.GraphPath, outcome.Path}
	}
	cmd := exec.Command(s.executable, args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		return
	}
	if err := cmd.Process.Release(); err != nil {
		_ = cmd.Process.Kill()
	}
}

func NewRootCommand(opts Options) *cobra.Command {
	version := opts.Version
	if version == "" {
		version = "dev"
	}
	opts.Version = version

	cmd := &cobra.Command{
		Use:           "repobridge",
		Short:         "Fetch source code for packages and repositories",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			maybeRunUpdateCheck(cmd, opts)
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			printRootLogo(cmd.OutOrStdout())
			return cmd.Help()
		},
	}
	cmd.SetOut(opts.stdout())
	cmd.SetErr(opts.stderr())
	cmd.SetVersionTemplate(fmt.Sprintf("repobridge %s\n", version))

	cmd.AddCommand(newFetchCommand(opts))
	cmd.AddCommand(newPathCommand(opts))
	cmd.AddCommand(newScanCommand(opts))
	cmd.AddCommand(newSearchCommand(opts))
	cmd.AddCommand(newGraphStatusCommand(opts))
	cmd.AddCommand(newGraphFilesCommand(opts))
	cmd.AddCommand(newGraphNodeCommand(opts))
	cmd.AddCommand(newCallersCommand(opts))
	cmd.AddCommand(newCalleesCommand(opts))
	cmd.AddCommand(newImpactCommand(opts))
	cmd.AddCommand(newContextCommand(opts))
	cmd.AddCommand(newExploreCommand(opts))
	cmd.AddCommand(newInstallAgentCommand(opts))
	cmd.AddCommand(newSelfUpdateCommand(opts))
	cmd.AddCommand(newListCommand(opts))
	cmd.AddCommand(newRemoveCommand(opts))
	cmd.AddCommand(newCleanCommand(opts))
	cmd.AddCommand(newASTGraphIndexCommand())

	return cmd
}

func newASTGraphIndexCommand() *cobra.Command {
	var graphDir string
	cmd := &cobra.Command{
		Use:    "__astgraph-index <source-path>",
		Hidden: true,
		Args:   cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return indexOutcomePathWithGraphDir(args[0], graphDir)
		},
	}
	cmd.Flags().StringVar(&graphDir, "graph-dir", "", "override AST graph storage directory")
	return cmd
}
