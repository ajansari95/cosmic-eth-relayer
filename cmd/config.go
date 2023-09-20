package cmd

import (
	"fmt"
	"os"
	"path"

	"github.com/ajansari95/cosmic-relayer/pkg/config"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/types/module"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/strangelove-ventures/lens/client"
	"gopkg.in/yaml.v2"
)

func initConfig(cmd *cobra.Command) error {
	home, err := cmd.PersistentFlags().GetString(flags.FlagHome)
	if err != nil {
		return err
	}

	debug, err := cmd.Flags().GetBool("debug")
	if err != nil {
		return err
	}

	cfg = &config.Config{}
	cfgPath := path.Join(home, "config.yaml")
	_, err = os.Stat(cfgPath)
	if err != nil {
		err = config.CreateConfig(home, debug)
		if err != nil {
			return err
		}
	}
	viper.SetConfigFile(cfgPath)
	err = viper.ReadInConfig()
	if err != nil {
		fmt.Println("Failed to read in config:", err)
		os.Exit(1)
	}

	// read the config file bytes
	file, err := os.ReadFile(viper.ConfigFileUsed())
	if err != nil {
		fmt.Println("Error reading file:", err)
		os.Exit(1)
	}

	// unmarshall them into the struct
	if err = yaml.Unmarshal(file, cfg); err != nil {
		fmt.Println("Error unmarshalling config:", err)
		os.Exit(1)
	}

	cfg.Cl = make(map[string]*client.ChainClient)
	for name, chain := range cfg.Chains {
		chain.Modules = append([]module.AppModuleBasic{}, ModuleBasicsStub...)
		cl, err := client.NewChainClient(nil, chain, home, os.Stdin, os.Stdout)
		if err != nil {
			fmt.Println("Error creating chain client:", err)
			os.Exit(1)
		}
		cfg.Cl[name] = cl
	}

	if cmd.PersistentFlags().Changed("chain") {
		defaultChain, err := cmd.PersistentFlags().GetString("chain")
		if err != nil {
			return err
		}

		cfg.QuerierChain = defaultChain
	}

	if cmd.PersistentFlags().Changed("output") {
		output, err := cmd.PersistentFlags().GetString("output")
		if err != nil {
			return err
		}

		// Should output be a global configuration item?
		for chain := range cfg.Chains {
			cfg.Chains[chain].OutputFormat = output
		}
	}

	// validate configuration
	if err = config.ValidateConfig(cfg); err != nil {
		fmt.Println("Error parsing chain config:", err)
		os.Exit(1)
	}
	return nil

}
