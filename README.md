# Cosmic Relayer

Cosmic Relayer is a command-line utility designed to relay Ethereum requests created on a Cosmos chain equipped with the ethQueryModule, and includes supported callbacks for the same.

## Getting Started

### Building

You can build the Cosmic Relayer both as a standalone binary or as a Docker container.

1. **Standalone Binary:**
   ```bash
   make build
   ```

2. **Docker Image:**
   ```bash
   make docker
   ```

# Installation

After building the `cosmic-relayer` binary using the `make build` command, you can install it to your system's binary directory for easy access. Here's how to do it:

1. Navigate to the directory where the `cosmic-relayer` binary is located.

2. Make the binary executable:
   ```bash
   chmod +x cosmic-relayer
   ```

3. Move the binary to your system's binary directory, usually `/usr/local/bin/`:
   ```bash
   sudo mv cosmic-relayer /usr/local/bin/
   ```

4. Now, you should be able to run `cosmic-relayer` from any location in your terminal.

### Configuration

Before you start the relayer, you need to create a configuration in your app home directory (as specified by the `--home` flag). By default, the configuration file is named `config.yaml` and looks like this:

```yaml
querier_chain: test-1
eth_rpc: https://mainnet.infura.io/v3/<APIKEY>
chains:
  test-1:
    key: default
    chain-id: test-1
    rpc-addr: http://localhost:26657
    grpc-addr: http://localhost:9090
    account-prefix: cosmos
    keyring-backend: test
    gas-adjustment: 1.5
    gas-prices: 0.0001stake
    min-gas-amount: 0
    key-directory: ./cosmicrelayer/keys
    debug: true
    timeout: 20s
    block-timeout: ""
    output-format: json
    sign-mode: dir
```

### Adding or Restoring Keys

After setting up the configuration, you need to add a key. The key name should match the one mentioned in `config.yaml`:

1. **Add a Key:**
   ```bash
   cosmic-relayer keys add default --home .cosmicrelayer
   ```

2. **Restore a Key from Mnemonic:**
   ```bash
   cosmic-relayer keys restore default --home .cosmicrelayer
   ```

Ensure that the corresponding account has an adequate balance on the Cosmos side.

## Usage

```
Cosmic Relayer is a CLI tool for relaying data from one chain to another.

Usage:
  cosmic-relayer [command]

Available Commands:
  keys        Manage keys held by the relayer for each chain
  run         Run the relayer
  help        Help about any command

Flags:
  -d, --debug           Debug output
  -h, --help            Help for cosmic-relayer
      --home string     Home directory for config and data (default "/Users/ajansari/.cosmic-relayer")
  -o, --output string   Output format (json, indent, yaml) (default "json")
  -t, --toggle          Help message for toggle
```

To run the relayer, simply use:

```bash
cosmic-relayer --home RELAYER_HOME_DIR run 
```

