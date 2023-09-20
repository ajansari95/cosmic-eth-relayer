package cmd

import (
	"os"

	"github.com/ajansari95/cosmic-relayer/pkg/config"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	homePath    string
	debug       bool
	cfg         *config.Config
	defaultHome = os.ExpandEnv("$HOME/.cosmic-relayer")
	appName     = "cosmic-relayer"
)

var rootCmd = &cobra.Command{
	Use:   appName,
	Short: "Cosmic Relayer",
	Long:  `Cosmic Relayer is a CLI tool for relaying data from one chain to another.`,
}

func Execute() {
	cobra.EnableCommandSorting = false

	rootCmd.SilenceUsage = true
	rootCmd.CompletionOptions.DisableDefaultCmd = true
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error {
		// reads `homeDir/config.yaml` into `var config *Config` before each command
		if err := initConfig(rootCmd); err != nil {
			return err
		}
		return nil
	}

	// --home flag
	rootCmd.PersistentFlags().StringVar(&homePath, "home", defaultHome, "home directory for config and data")
	if err := viper.BindPFlag(flags.FlagHome, rootCmd.PersistentFlags().Lookup(flags.FlagHome)); err != nil {
		panic(err)
	}

	// --debug flag
	rootCmd.PersistentFlags().BoolVarP(&debug, "debug", "d", false, "debug output")
	if err := viper.BindPFlag("debug", rootCmd.PersistentFlags().Lookup("debug")); err != nil {
		panic(err)
	}

	rootCmd.PersistentFlags().StringP("output", "o", "json", "output format (json, indent, yaml)")
	if err := viper.BindPFlag("output", rootCmd.PersistentFlags().Lookup("output")); err != nil {
		panic(err)
	}

	rootCmd.AddCommand(keysCmd())
}
