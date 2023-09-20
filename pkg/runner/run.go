package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/ajansari95/cosmic-relayer/pkg/config"
	ethTypes "github.com/ajansari95/cosmicether/eth_types"
	ethquerytypes "github.com/ajansari95/cosmicether/x/ethquery/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/go-kit/log"
	lensclient "github.com/strangelove-ventures/lens/client"
)

type client []*lensclient.ChainClient

const VERSION = "v0.0.1b"

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

	ethclient, err := ethclient.Dial(globalCfg.EthRPC)
	if err != nil {
		fmt.Println("error", err)
		panic(err)
	}

	globalCfg.EthClient = ethclient

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
				go handleHistoricRequests(out.Quereis, querierClient.Config.ChainID, globalCfg.EthRPC, log.With(logger, "worker", "historic"))
			}

		}

	}(QuerierClient, globalCfg, log.With(logger, "worker", "historic-queries", "chain", QuerierClient.Config.ChainID))
	return nil
}

//{"quereis":[{"id":"a11618f23718567b2db1c2a97f575d8634778771b7c044627e838b4b49b9d8e6","query_type":"eth_getStorageAt","request":"eyJjb250cmFjdEFkZHJlc3MiOiIweDRkMjI0NDUyODAxYWNlZDhiMmYwYWViZTE1NTM3OWJiNWQ1OTQzODEiLCJzbG90IjoiMHgwMiIsImhlaWdodCI6MTIzNDQzNTR9","callback_id":"getstorageat","block_height":"5"},
//{"id":"c8cd721af3c759568869ddb1cdb97c28554149b71961d78900002c0554458ec5","query_type":"eth_getBlockByNumber","request":"eyJoZWlnaHQiOjEyMzQ0MzU0fQ==","callback_id":"getblock","block_height":"5"}]}
//

type Query struct {
	QueryId     string `json:"id"`
	QueryType   string `json:"query_type"`
	Request     []byte `json:"request"`
	CallbackId  string `json:"callback_id"`
	BlockHeight uint64 `json:"block_height"`
}

type queryGetStorageAt struct {
	ContractAddress string `json:"contractAddress"`
	Slot            string `json:"slot"`
	Height          uint64 `json:"height"`
}

type queryGetBlockHeader struct {
	Height uint64 `json:"height"`
}

func handleHistoricRequests(queries []ethquerytypes.EthQuery, chainID string, ethRPC string, logger log.Logger) {
	if len(queries) == 0 {
		return
	}

	for _, qry := range queries[0:int(math.Min(float64(len(queries)), float64(MaxHistoricQueries)))] {
		_, ok := globalCfg.Cl[chainID]
		if !ok {
			continue
		}

		q := Query{}
		q.QueryId = qry.Id
		q.QueryType = qry.QueryType
		q.Request = qry.Request
		q.CallbackId = qry.CallbackId
		q.BlockHeight = qry.BlockHeight

		time.Sleep(WaitInterval)
		go RequestOnETH(q, logger)
	}
}

func RequestOnETH(query Query, logger log.Logger) {
	var err error
	var res []byte
	ethClient := globalCfg.EthClient
	if ethClient == nil {
		return
	}
	_ = logger.Log("msg", "Handling request", "type", query.QueryType, "id", query.QueryId, "height", query.BlockHeight)
	submitClient := globalCfg.Cl[globalCfg.QuerierChain]
	switch query.QueryType {
	case "eth_getStorageAt":
		request := queryGetStorageAt{}

		err = json.Unmarshal(query.Request, &request)
		if err != nil {
			_ = logger.Log("msg", "Error: Failed in Unmarshalling Request", "type", query.QueryType, "id", query.QueryId, "height", query.BlockHeight)
			panic(fmt.Sprintf("panic(7a): %v", err))
		}

		_ = logger.Log("msg", "Requesting StorageAt", "type", query.QueryType, "id", query.QueryId, "height", query.BlockHeight, " contract-", request.ContractAddress, "slot-", request.Slot)
		res, err = GetStorageData(ethClient, request)
		if err != nil {
			_ = logger.Log("msg", "Error: Failed in RequestOnETH ", "type", query.QueryType, "id", query.QueryId, "height", query.BlockHeight)
			panic(fmt.Sprintf("panic(7c): %v", err))
		}

	case "eth_getBlockByNumber":
		request := queryGetBlockHeader{}

		err = json.Unmarshal(query.Request, &request)
		if err != nil {
			_ = logger.Log("msg", "Error: Failed in Unmarshalling Request", "type", query.QueryType, "id", query.QueryId, "height", query.BlockHeight)
			panic(fmt.Sprintf("panic(7b): %v", err))
		}

		_ = logger.Log("msg", "Requesting BlockHeader", "type", query.QueryType, "id", query.QueryId, "height", query.BlockHeight, "block-", request.Height)
		res, err = GetBlockHeader(ethClient, request)
		if err != nil {
			_ = logger.Log("msg", "Error: Failed in RequestOnETH ", "type", query.QueryType, "id", query.QueryId, "height", query.BlockHeight)
			panic(fmt.Sprintf("panic(7d): %v", err))
		}
	}

	from, err := submitClient.GetKeyAddress()
	if err != nil {
		_ = logger.Log("msg", "Error: Failed in RequestOnETH ", "type", query.QueryType, "id", query.QueryId, "height", query.BlockHeight)
		panic(fmt.Sprintf("panic(7e): %v", err))
	}

	msg := &ethquerytypes.MsgSubmitQueryResponse{
		QueryId:     query.QueryId,
		Result:      res,
		Height:      int64(query.BlockHeight),
		FromAddress: submitClient.MustEncodeAccAddr(from),
	}
	sendQueue[globalCfg.QuerierChain] <- msg

}

// GetBlockHeader constructs and executes a RPC call to retrieve block header data
func GetBlockHeader(ethClient *ethclient.Client, request queryGetBlockHeader) ([]byte, error) {
	var BlockHeader ethTypes.QueryBlockHeaderResponseData
	height := new(big.Int).SetUint64(request.Height)

	block, err := ethClient.BlockByNumber(context.Background(), height)
	if err != nil {
		return nil, err
	}

	BlockHeader.Root = block.Root()
	BlockHeader.Height = request.Height

	return json.Marshal(BlockHeader)
}

// GetStorageData constructs and executes a batched RPC call to retrieve storage and block data
// for a given Ethereum contract address and slot.
func GetStorageData(ethClient *ethclient.Client, request queryGetStorageAt) ([]byte, error) {
	var responseData ethTypes.QueryRespoonseData
	var blockData ethTypes.Block
	height := new(big.Int).SetUint64(request.Height)

	if err := executeBatchCall(ethClient, request, height, &responseData, &blockData); err != nil {
		return nil, err
	}

	populateResponseData(&responseData, blockData, height, request.Slot)

	return json.Marshal(responseData)
}

func executeBatchCall(ethClient *ethclient.Client, request queryGetStorageAt, height *big.Int, responseData *ethTypes.QueryRespoonseData, blockData *ethTypes.Block) error {
	// Prepare the batch of RPC calls
	batch := []rpc.BatchElem{
		{
			Method: "eth_getStorageAt",
			Args:   []interface{}{common.HexToAddress(request.ContractAddress), request.Slot, hexutil.EncodeBig(height)},
			Result: &responseData.StorageData,
		},
		{
			Method: "eth_getProof",
			Args:   []interface{}{common.HexToAddress(request.ContractAddress), []string{request.Slot}, hexutil.EncodeBig(height)},
			Result: &responseData.StorageProofData,
		},
		{
			Method: "eth_getBlockByNumber",
			Args:   []interface{}{hexutil.EncodeBig(height), true},
			Result: blockData,
		},
	}

	// Execute the batch
	if err := ethClient.Client().BatchCall(batch); err != nil {
		return err
	}

	// Check each RPC call in the batch for errors
	for _, batchElem := range batch {
		if batchElem.Error != nil {
			return errors.New("error in batch RPC call: " + batchElem.Method + " - " + batchElem.Error.Error())
		}
	}

	return nil
}

func populateResponseData(responseData *ethTypes.QueryRespoonseData, blockData ethTypes.Block, height *big.Int, slot string) {
	responseData.StorageProofData.StateRoot = blockData.Root
	responseData.BlockRootHash = blockData.Root
	responseData.StorageProofData.Height = height
	responseData.Slot = slot
}

func FlushSendQueue(chainID string, logger log.Logger) error {
	time.Sleep(WaitInterval)
	toSend := []sdk.Msg{}
	ch := sendQueue[chainID]

	for {
		if len(toSend) > MaxTxMsgs {
			flush(chainID, toSend, logger)
			toSend = []sdk.Msg{}
		}
		select {
		case msg := <-ch:
			toSend = append(toSend, msg)
		case <-time.After(WaitInterval):
			flush(chainID, toSend, logger)
			toSend = []sdk.Msg{}
		}
	}
}

func flush(chainID string, toSend []sdk.Msg, logger log.Logger) {
	if len(toSend) > 0 {
		_ = logger.Log("msg", "flushing", "count", len(toSend))
		chainClient := globalCfg.Cl[chainID]
		if chainClient == nil {
			return
		}

		msgs := unique(toSend, logger)

		if len(msgs) > 0 {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			fmt.Println(msgs)
			resp, err := chainClient.SendMsgs(ctx, msgs, VERSION)
			if err != nil {
				if resp != nil && resp.Code == 19 && resp.Codespace == "sdk" {
					//if err.Error() == "transaction failed with code: 19" {
					_ = logger.Log("msg", "Tx already in mempool")
				} else if resp != nil && resp.Code == 12 && resp.Codespace == "sdk" {
					//if err.Error() == "transaction failed with code: 19" {
					_ = logger.Log("msg", "Not enough gas")
				} else if err.Error() == "context deadline exceeded" {
					_ = logger.Log("msg", "Failed to submit in time, retrying")
					resp, err := chainClient.SendMsgs(ctx, msgs, VERSION)
					if err != nil {
						if resp != nil && resp.Code == 19 && resp.Codespace == "sdk" {
							//if err.Error() == "transaction failed with code: 19" {
							_ = logger.Log("msg", "Tx already in mempool")
						} else if resp != nil && resp.Code == 12 && resp.Codespace == "sdk" {
							//if err.Error() == "transaction failed with code: 19" {
							_ = logger.Log("msg", "Not enough gas")
						} else if err.Error() == "context deadline exceeded" {
							_ = logger.Log("msg", "Failed to submit in time, bailing")
							return
						} else {
							//panic(fmt.Sprintf("panic(1): %v", err))
							_ = logger.Log("msg", "Failed to submit after retry; nevermind, we'll try again!", "err", err)
						}
					}

				} else {
					// for some reason the submission failed; but we should be able to continue here.
					// panic(fmt.Sprintf("panic(2): %v", err))
					_ = logger.Log("msg", "Failed to submit; nevermind, we'll try again!", "err", err)

				}
			}
			_ = logger.Log("msg", fmt.Sprintf("Sent batch of %d (deduplicated) messages", len(msgs)))

		}
	}
}

func unique(msgSlice []sdk.Msg, logger log.Logger) []sdk.Msg {
	keys := make(map[string]bool)

	list := []sdk.Msg{}
	for _, entry := range msgSlice {
		msg2, ok2 := entry.(*ethquerytypes.MsgSubmitQueryResponse)
		if ok2 {
			if _, value := keys[msg2.QueryId]; !value {
				keys[msg2.QueryId] = true
				list = append(list, entry)
				_ = logger.Log("msg", "Added SubmitResponse message", "id", msg2.QueryId)
			}
		}
	}
	if len(keys) == 0 {
		return []sdk.Msg{}
	}
	return list
}

//fmt.Println(result)
//batch := []rpc.BatchElem{
//{
//Method: "eth_getStorageAt",
//Args:   []interface{}{common.HexToAddress(contractAddress), storagePosition, fmt.Sprintf("0x%x", blockNumber)},
//Result: &json.RawMessage{},
//},
//{
//Method: "eth_getProof",
//Args:   []interface{}{common.HexToAddress(contractAddress), []string{storagePosition}, hexutil.EncodeBig(blockNumber)},
//Result: &result,
//},
//{
//Method: "eth_getBlockByNumber",
//Args:   []interface{}{hexutil.EncodeBig(blockNumber), true},
//Result: &block,
//},
//}
//
//err = client.Client().BatchCall(batch)
//if err != nil {
//panic(err)
//}
//
//result.StateRoot = block.Root
////result.Height = common.HexToHash(block.Height)
//height := common.HexToHash(block.Height)
//fmt.Println(height)
//bh, _ := hexutil.DecodeBig(block.Height)
//result.Height = bh
//fmt.Println(bh)
//
//data := QueryRespoonseData{
//StorageData:      *batch[0].Result.(*json.RawMessage),
//StorageProofData: result,
//BlockRootHash:    block.Root,
//}
//
//fmt.Println(data)
//marhaaled_Data, err := json.Marshal(data)
//fmt.Println(string(marhaaled_Data))
//
//err = ioutil.WriteFile("data.json", marhaaled_Data, 0644)
//if err != nil {
//log.Fatalf("Failed to write to file: %v", err)
//}
//
//receivedData, err := ioutil.ReadFile("data.json")
//if err != nil {
//log.Fatalf("Failed to read from file: %v", err)
