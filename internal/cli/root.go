package cli

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"repobridge/internal/cache"
	"repobridge/internal/codegraph"
	"repobridge/internal/codegraph/store"
	"repobridge/internal/source"
)

type Options struct {
	Version string
	Stdout  io.Writer
	Stderr  io.Writer
	App     App
	Indexer IndexScheduler
}

type App interface {
	EnsureCached(spec string, opts source.Options) (source.Outcome, error)
	SearchCode(spec, rawQuery string, opts codegraph.SearchOptions) ([]codegraph.SearchResult, error)
	CodeGraphStatus(spec string, opts codegraph.GraphInspectOptions) (codegraph.GraphInspectStatus, error)
	CodeGraphFiles(spec string, opts codegraph.GraphInspectOptions) (codegraph.GraphFilesResult, error)
	CodeGraphNode(spec, lookup string, opts codegraph.GraphInspectOptions) (codegraph.GraphNodeLookupResult, error)
	CodeGraphCallgraph(spec, symbol string, opts codegraph.CallgraphOptions) (codegraph.CallgraphResult, error)
	CodeGraphContext(spec, query string, opts codegraph.ContextOptions) (codegraph.ContextResult, error)
}

type IndexScheduler interface {
	Schedule(source.Outcome)
}

type defaultApp struct{}

func (defaultApp) EnsureCached(spec string, opts source.Options) (source.Outcome, error) {
	return source.EnsureCached(spec, opts)
}

func (defaultApp) SearchCode(spec, rawQuery string, opts codegraph.SearchOptions) ([]codegraph.SearchResult, error) {
	service := codegraph.NewSearchService(codegraph.SearchServiceOptions{
		Resolver: defaultApp{},
		StoreOpener: func(dir string) (codegraph.GraphStore, error) {
			return store.Open(dir)
		},
	})
	return service.Search(spec, rawQuery, opts)
}

func (defaultApp) CodeGraphStatus(spec string, opts codegraph.GraphInspectOptions) (codegraph.GraphInspectStatus, error) {
	return defaultInspectService().Status(spec, opts)
}

func (defaultApp) CodeGraphFiles(spec string, opts codegraph.GraphInspectOptions) (codegraph.GraphFilesResult, error) {
	return defaultInspectService().Files(spec, opts)
}

func (defaultApp) CodeGraphNode(spec, lookup string, opts codegraph.GraphInspectOptions) (codegraph.GraphNodeLookupResult, error) {
	return defaultInspectService().Node(spec, lookup, opts)
}

func (defaultApp) CodeGraphCallgraph(spec, symbol string, opts codegraph.CallgraphOptions) (codegraph.CallgraphResult, error) {
	return codegraph.NewCallgraphService(codegraph.SearchServiceOptions{
		Resolver: defaultApp{},
		StoreOpener: func(dir string) (codegraph.GraphStore, error) {
			return store.Open(dir)
		},
	}).Callgraph(spec, symbol, opts)
}

func (defaultApp) CodeGraphContext(spec, query string, opts codegraph.ContextOptions) (codegraph.ContextResult, error) {
	return codegraph.NewContextService(codegraph.SearchServiceOptions{
		Resolver: defaultApp{},
		StoreOpener: func(dir string) (codegraph.GraphStore, error) {
			return store.Open(dir)
		},
	}).Context(spec, query, opts)
}

func defaultInspectService() *codegraph.InspectService {
	return codegraph.NewInspectService(codegraph.SearchServiceOptions{
		Resolver: defaultApp{},
		StoreOpener: func(dir string) (codegraph.GraphStore, error) {
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

func indexOutcome(outcome source.Outcome) error {
	return indexOutcomePath(outcome.Path)
}

func indexOutcomePath(sourcePath string) error {
	startedAt := time.Now().UTC()
	graphDir, err := cache.GraphDirForSource(sourcePath)
	if err != nil {
		return err
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

var indexSourcePath = func(sourcePath string) (codegraph.IndexResult, error) {
	return codegraph.NewIndexer(codegraph.IndexOptions{}).Index(sourcePath)
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
	cmd := exec.Command(s.executable, "__codegraph-index", outcome.Path)
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

	cmd := &cobra.Command{
		Use:           "repobridge",
		Short:         "Fetch source code for packages and repositories",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
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
	cmd.AddCommand(newListCommand(opts))
	cmd.AddCommand(newRemoveCommand(opts))
	cmd.AddCommand(newCleanCommand(opts))
	cmd.AddCommand(newCodegraphIndexCommand())

	return cmd
}

func newCodegraphIndexCommand() *cobra.Command {
	return &cobra.Command{
		Use:    "__codegraph-index <source-path>",
		Hidden: true,
		Args:   cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return indexOutcomePath(args[0])
		},
	}
}
