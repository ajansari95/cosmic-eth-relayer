package cmd

import (
	"github.com/ajansari95/cosmic-relayer/pkg/runner"
	"github.com/spf13/cobra"
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the relayer",
	Long: `Run the relayer. This command will start the relayer and begin relaying data
from one chain to another. The relayer will continue to run until it is stopped.`,
	Run: func(cmd *cobra.Command, args []string) {
		err := runner.Run(cfg, cmd.Flag("home").Value.String())
		if err != nil {
			return
		}
	},
}

func init() {
	rootCmd.AddCommand(runCmd)
}
