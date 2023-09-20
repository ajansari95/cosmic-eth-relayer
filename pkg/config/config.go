package config

import (
	"fmt"
	"os"
	"path"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/strangelove-ventures/lens/client"
	"gopkg.in/yaml.v2"
)

func CreateConfig(home string, debug bool) error {
	cfgPath := path.Join(home, "config.yaml")

	// If the config doesn't exist...
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		// And the config folder doesn't exist...
		// And the home folder doesn't exist
		if _, err := os.Stat(home); os.IsNotExist(err) {
			// Create the home folder
			if err = os.Mkdir(home, os.ModePerm); err != nil {
				return err
			}
		}
	}

	// Then create the file...
	f, err := os.Create(cfgPath)
	if err != nil {
		return err
	}
	defer f.Close()

	// And write the default config to that location...
	if _, err = f.Write(defaultConfig(path.Join(home, "keys"), debug)); err != nil {
		return err
	}
	return nil
}

type Config struct {
	QuerierChain string                               `yaml:"querier_chain" json:"querier_chain"`
	EthRPC       string                               `yaml:"eth_rpc" json:"eth_rpc"`
	Chains       map[string]*client.ChainClientConfig `yaml:"chains" json:"chains"`
	Cl           map[string]*client.ChainClient       `yaml:",omitempty" json:",omitempty"`
	EthClient    *ethclient.Client                    `yaml:",omitempty" json:",omitempty"`
}

func (c *Config) GetDefaultClient() *client.ChainClient {
	return c.GetClient(c.QuerierChain)
}

func (c *Config) GetClient(chainID string) *client.ChainClient {
	if v, ok := c.Cl[chainID]; ok {
		return v
	}
	return nil
}

// Called to initialize the relayer.Chain types on Config
func ValidateConfig(c *Config) error {
	for _, chain := range c.Chains {
		if err := chain.Validate(); err != nil {
			return err
		}
	}
	if c.GetDefaultClient() == nil {
		return fmt.Errorf("default chain (%s) configuration not found", c.QuerierChain)
	}
	return nil
}

// MustYAML returns the yaml string representation of the Paths
func (c Config) MustYAML() []byte {
	out, err := yaml.Marshal(c)
	if err != nil {
		panic(err)
	}
	return out
}

func defaultConfig(keyHome string, debug bool) []byte {
	return Config{
		QuerierChain: "test-1",
		EthRPC:       "https://mainnet.infura.io/v3/<API_KEY>",
		Chains: map[string]*client.ChainClientConfig{
			"test-1": GetQuerierChain(keyHome, debug),
		},
	}.MustYAML()
}

func GetQuerierChain(keyHome string, debug bool) *client.ChainClientConfig {
	return &client.ChainClientConfig{
		Key:            "default",
		ChainID:        "test-1",
		RPCAddr:        "http://localhost:26657",
		GRPCAddr:       "http://localhost:9090",
		AccountPrefix:  "cosmos",
		KeyringBackend: "test",
		GasAdjustment:  1.2,
		GasPrices:      "0.01stake",
		MinGasAmount:   0,
		KeyDirectory:   keyHome,
		Debug:          debug,
		Timeout:        "20s",
		BlockTimeout:   "10s",
		OutputFormat:   "json",
		SignModeStr:    "direct",
	}
}
