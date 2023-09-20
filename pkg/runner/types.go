package runner

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
