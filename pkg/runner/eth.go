package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"

	ethTypes "github.com/ajansari95/cosmicether/eth_types"
	ethquerytypes "github.com/ajansari95/cosmicether/x/ethquery/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/go-kit/log"
)

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

// GetBlockHeader constructs and executes an RPC call to retrieve block header data
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
