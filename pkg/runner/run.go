package runner

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/ajansari95/cosmic-relayer/pkg/config"
	ethquerytypes "github.com/ajansari95/cosmicether/x/ethquery/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/go-kit/log"
	lensclient "github.com/strangelove-ventures/lens/client"
)

const VERSION = "v0.0.2b"

// TODO: Move to Config
var (
	WaitInterval          = time.Second * 3
	HistoricQueryInterval = time.Second * 10
	ctx                   = context.Background()
	globalCfg             *config.Config
	sendQueue             = map[string]chan sdk.Msg{}
	MaxHistoricQueries    = 10
	MaxTxMsgs             = 5
)

func Run(cfg *config.Config, home string) error {
	globalCfg = cfg
	var err error
	logger := log.NewLogfmtLogger(log.NewSyncWriter(os.Stderr))
	logger = log.With(logger, "ts", log.DefaultTimestampUTC, "caller", log.DefaultCaller)

	_ = logger.Log("msg", "starting relayer", "version", VERSION)

	for _, c := range cfg.Chains {
		cfg.Cl[c.ChainID], err = lensclient.NewChainClient(nil, c, home, os.Stdin, os.Stdout)
		if err != nil {
			return err
		}

		err = logger.Log("worker", "init", "msg", "configured chain", "chain", c.ChainID)
		if err != nil {
			return err
		}
		sendQueue[c.ChainID] = make(chan sdk.Msg)
	}
	wg := &sync.WaitGroup{}
	defer wg.Wait()

	QuerierClient, ok := cfg.Cl[cfg.QuerierChain]
	if !ok {
		panic("unable to create default chainClient; Client is nil")
	}

	ethClient, err := ethclient.Dial(globalCfg.EthRPC)
	if err != nil {
		panic(err)
	}

	globalCfg.EthClient = ethClient

	err = QuerierClient.RPCClient.Start()
	if err != nil {
		_ = logger.Log("error", err.Error())
	}

	_ = logger.Log("worker", "init", "msg", "configuring subscription on default chainClient", "chain", QuerierClient.Config.ChainID)

	wg.Add(1)
	go func() {
		defer wg.Done()
		err := FlushSendQueue(QuerierClient.Config.ChainID, log.With(logger, "worker", "flusher", "chain", QuerierClient.Config.ChainID))
		if err != nil {
			_ = logger.Log("Flush Go-routine Bailing")
			panic(err)
		}
	}()

	wg.Add(1)
	go func(querierClient *lensclient.ChainClient, globalCfg *config.Config, logger log.Logger) {
		defer wg.Done()
	LOOP:
		for {
			time.Sleep(HistoricQueryInterval)
			req := &ethquerytypes.QueryRequestsRequest{}

			bz := querierClient.Codec.Marshaler.MustMarshal(req)
			res, err := querierClient.RPCClient.ABCIQuery(ctx, "/cosmicether.ethquery.Query/Queries", bz)
			if err != nil {
				if strings.Contains(err.Error(), "Client.Timeout") {
					err := logger.Log("error", fmt.Sprintf("timeout: %s", err.Error()))
					if err != nil {
						return
					}
					continue LOOP
				}
				panic(fmt.Sprintf("panic(3): %v", err))
			}

			out := &ethquerytypes.QueryRequestsResponse{}
			err = querierClient.Codec.Marshaler.Unmarshal(res.Response.Value, out)
			if err != nil {
				err := logger.Log("msg", "Error: Unable to unmarshal: ", "error", err)
				if err != nil {
					return
				}
				continue LOOP
			}

			_ = logger.Log("worker", "chainClient", "msg", "fetched historic queries for chain", "count", len(out.Quereis))
			if len(out.Quereis) > 0 {
				go handleHistoricRequests(out.Quereis, querierClient.Config.ChainID, log.With(logger, "worker", "historic"))
			}

		}

	}(QuerierClient, globalCfg, log.With(logger, "worker", "historic-queries", "chain", QuerierClient.Config.ChainID))
	return nil
}
