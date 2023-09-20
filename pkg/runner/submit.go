package runner

import (
	"context"
	"fmt"
	"math"
	"time"

	ethquerytypes "github.com/ajansari95/cosmicether/x/ethquery/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/go-kit/log"
)

func handleHistoricRequests(queries []ethquerytypes.EthQuery, chainID string, logger log.Logger) {
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
			resp, err := chainClient.SendMsgs(ctx, msgs, VERSION)
			if err != nil {
				if resp != nil && resp.Code == 19 && resp.Codespace == "sdk" {
					//if err.Error() == "transaction failed with code: 19" {
					_ = logger.Log("msg", "Tx already in mem-pool")
				} else if resp != nil && resp.Code == 12 && resp.Codespace == "sdk" {
					//if err.Error() == "transaction failed with code: 19" {
					_ = logger.Log("msg", "Not enough gas")
				} else if err.Error() == "context deadline exceeded" {
					_ = logger.Log("msg", "Failed to submit in time, retrying")
					resp, err := chainClient.SendMsgs(ctx, msgs, VERSION)
					if err != nil {
						if resp != nil && resp.Code == 19 && resp.Codespace == "sdk" {
							//if err.Error() == "transaction failed with code: 19" {
							_ = logger.Log("msg", "Tx already in mem-pool")
						} else if resp != nil && resp.Code == 12 && resp.Codespace == "sdk" {
							//if err.Error() == "transaction failed with code: 19" {
							_ = logger.Log("msg", "Not enough gas")
						} else if err.Error() == "context deadline exceeded" {
							_ = logger.Log("msg", "Failed to submit in time, bailing")
							return
						} else {
							_ = logger.Log("msg", "Failed to submit after retry; never-mind, we'll try again!", "err", err)
						}
					}

				} else {
					_ = logger.Log("msg", "Failed to submit; never-mind, we'll try again!", "err", err)

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
