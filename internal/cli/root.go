package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"repobridge/internal/source"
)

type Options struct {
	Version string
	Stdout  io.Writer
	Stderr  io.Writer
	App     App
}

type App interface {
	EnsureCached(spec string, opts source.Options) (source.Outcome, error)
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
