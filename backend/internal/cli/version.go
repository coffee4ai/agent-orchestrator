package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// Build metadata. Release tooling can override these with -ldflags.
var (
	Version = "dev"
	Commit  = ""
	Date    = ""
)

// VersionString renders the build metadata as "<version> commit <c> built <d>",
// omitting the commit/date parts when they are unset.
func VersionString() string {
	parts := []string{Version}
	if Commit != "" {
		parts = append(parts, "commit "+Commit)
	}
	if Date != "" {
		parts = append(parts, "built "+Date)
	}
	return strings.Join(parts, " ")
}

// forkCommit is the build commit, falling back to the version when the build
// stamped no commit.
func forkCommit() string {
	if Commit != "" {
		return Commit
	}
	return Version
}

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := fmt.Fprintln(cmd.OutOrStdout(), VersionString())
			return err
		},
	}
}
