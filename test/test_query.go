package test

import (
	"block/query"
	"fmt"
)

func TestQueryBlocks() { //查询链的长度
	blocks := query.QueryBlocks(0, 0)
	fmt.Println(len(blocks))
}

func TestQueryBlock() { //查询分片中特定区块长度
	block := query.QueryBlock(0, 1, 22)
	fmt.Println(len(block.Body))
}

func TestQueryNewestBlock() {
	blocks := query.QueryNewestBlock(0, 0)
	fmt.Println(blocks.Header.Number)
}

func TestQueryBlockTxs() { //查询分片中特定区块的txs
	txs := query.QueryBlockTxs(0, 0, 9)
	fmt.Println(len(txs))
}
func TestQueryBlockHash() {
	Hash := query.QueryBlockHash(0, 0, 9)
	fmt.Println(Hash)
}

func TestQueryAccountState() { //查询分片中特定账户地址的余额
	accountState := query.QueryAccountState(3, 0, "0x32be343b94f860124dc4fee278fdcbd38c102d88")
	fmt.Println(accountState.Balance)
}
func QueryAllAccountStates() { //查询分片中全部的账户地址和余额
	query.QueryAllAccountStates(3, 0)
}
