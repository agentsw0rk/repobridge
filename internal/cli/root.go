package cli

import (
	"fmt"
	"io"
	"os"

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
}

type IndexScheduler interface {
	Schedule(source.Outcome)
}

type waitableIndexScheduler interface {
	Wait()
}

type defaultApp struct{}

func (defaultApp) EnsureCached(spec string, opts source.Options) (source.Outcome, error) {
	return source.EnsureCached(spec, opts)
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
	return codegraph.NewScheduler(codegraph.SchedulerOptions{IndexFunc: indexOutcome})
}

func indexOutcome(outcome source.Outcome) error {
	graphDir, err := cache.GraphDirForSource(outcome.Path)
	if err != nil {
		return err
	}
	graph, err := store.Open(graphDir)
	if err != nil {
		return err
	}
	defer graph.Close()

	index, err := codegraph.NewIndexer(codegraph.IndexOptions{}).Index(outcome.Path)
	if err != nil {
		return err
	}
	return graph.Replace(index)
}

func waitForIndexer(indexer IndexScheduler) {
	waiter, ok := indexer.(waitableIndexScheduler)
	if !ok {
		return
	}
	waiter.Wait()
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
	cmd.AddCommand(newListCommand(opts))
	cmd.AddCommand(newRemoveCommand(opts))
	cmd.AddCommand(newCleanCommand(opts))

	return cmd
}
