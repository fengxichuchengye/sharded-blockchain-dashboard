// Here the blockchain structrue is defined
// each node in this system will maintain a blockchain object.

package chain

import (
	"block/core"
	"block/params"
	"block/storage"
	"block/utils"
	"errors"
	"fmt"
	"log"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/trie"
)

type BlockChain struct {
	db            ethdb.Database      // the leveldb database to store in the disk, for status trie
	triedb        *trie.Database      // the trie database which helps to store the status trie
	ChainConfig   *params.ChainConfig // the chain configuration, which can help to identify the chain
	CurrentBlock  *core.Block         // the top block in this blockchain
	Storage       *storage.Storage    // Storage is the bolt-db to store the blocks
	Txpool        *core.TxPool        // the transaction pool
	PartitionMap  map[string]uint64   // 分区映射表   [账户地址] -> [分区ID]   对这里进行账户注入 这个是真正的映射表
	pmlock        sync.RWMutex
	AllStateMapTx map[string]*core.AccountState
	Flag          bool
}

// Get the transaction root, this root can be used to check the transactions
func GetTxTreeRoot(txs []*core.Transaction) []byte {
	// use a memory trie database to do this, instead of disk database
	triedb := trie.NewDatabase(rawdb.NewMemoryDatabase())
	transactionTree := trie.NewEmpty(triedb)
	for _, tx := range txs {
		transactionTree.Update(tx.TxHash, tx.Encode())
	}
	return transactionTree.Hash().Bytes()
}

// 它用于更新BlockChain结构体中的PartitionMap映射表，将指定的key与val关联起来。ralay未使用
func (bc *BlockChain) Update_PartitionMap(key string, val uint64) {
	bc.pmlock.Lock()
	defer bc.pmlock.Unlock()
	bc.PartitionMap[key] = val
}

// 输入一个账户地址，判断该账户是否分配到分片中，若没有分配，则使用默认的分配方法；若已分配，则返回账户所在分片位置
func (bc *BlockChain) Get_PartitionMap(key string) uint64 {
	bc.pmlock.RLock()
	defer bc.pmlock.RUnlock()
	if _, ok := bc.PartitionMap[key]; !ok {
		return uint64(utils.Addr2Shard(key))
	}
	return bc.PartitionMap[key]
}
func (bc *BlockChain) Get_shard(key string) uint64 {
	bc.pmlock.RLock()
	defer bc.pmlock.RUnlock()
	return uint64(utils.Addr2Shard(key))

}

// 向交易池发送事务(需要决定发送哪个池)
func (bc *BlockChain) SendTx2Pool(txs []*core.Transaction) {
	bc.Txpool.AddTxs2Pool(txs)
}

// 处理交易更新状态树 *******************************************************
func (bc *BlockChain) GetUpdateStatusTrie(txs []*core.Transaction) common.Hash {
	bc.PartitionMap = bc.MapInfo()
	fmt.Printf("The len of Map is %d\n", len(bc.PartitionMap))

	if len(txs) == 0 {
		return common.BytesToHash(bc.CurrentBlock.Header.StateRoot)
	}

	st, err := trie.New(trie.TrieID(common.BytesToHash(bc.CurrentBlock.Header.StateRoot)), bc.triedb)
	if err != nil {
		log.Panic(err)
	}
	cnt := 0

	for i, tx := range txs {

		if !tx.Relayed && (bc.Get_PartitionMap(tx.Sender) == bc.ChainConfig.ShardID) {

			s_state_enc, _ := st.Get([]byte(tx.Sender))
			var s_state *core.AccountState
			if s_state_enc == nil {

				ib := new(big.Int)
				ib.Add(ib, params.Init_Balance)
				s_state = &core.AccountState{
					Nonce:   uint64(i),
					Balance: ib,
				}
			} else {
				s_state = core.DecodeAS(s_state_enc)
			}
			s_balance := s_state.Balance
			if s_balance.Cmp(tx.Value) == -1 {
				fmt.Printf("the balance is less than the transfer amount\n")
				continue
			}
			s_state.Deduct(tx.Value)
			st.Update([]byte(tx.Sender), s_state.Encode())
			//bc.AllStateMapTx[tx.Sender] = s_state
			cnt++
		}
		// recipientIn := false
		if bc.Get_PartitionMap(tx.Recipient) == bc.ChainConfig.ShardID || tx.HasBroker {

			r_state_enc, _ := st.Get([]byte(tx.Recipient))
			var r_state *core.AccountState
			if r_state_enc == nil {
				// fmt.Println("missing account RECIPIENT, now adding account")
				ib := new(big.Int)
				ib.Add(ib, params.Init_Balance)
				r_state = &core.AccountState{
					Nonce:   uint64(i),
					Balance: ib,
				}
			} else {
				r_state = core.DecodeAS(r_state_enc)
			}
			r_state.Deposit(tx.Value)
			st.Update([]byte(tx.Recipient), r_state.Encode())
			//bc.AllStateMapTx[tx.Recipient] = r_state
			cnt++
		} else if bc.Get_PartitionMap(tx.Recipient) != bc.ChainConfig.ShardID || tx.HasBroker {
			mainAccountStateShardID := utils.Addr2Shard(tx.Sender)
			VirtualRecipient := utils.AddrVirtualAccount(tx.Recipient, mainAccountStateShardID)
			r_state_enc, _ := st.Get([]byte(VirtualRecipient))
			var r_state *core.AccountState
			if r_state_enc == nil {
				// fmt.Println("missing account RECIPIENT, now adding account")
				ib := new(big.Int)
				ib.Add(ib, params.VirtualInit_Balance)
				r_state = &core.AccountState{
					Nonce:   uint64(i),
					Balance: ib,
				}
			} else {
				r_state = core.DecodeAS(r_state_enc)
			}
			r_state.Deposit(tx.Value)
			st.Update([]byte(VirtualRecipient), r_state.Encode())
			bc.AllStateMapTx[VirtualRecipient] = r_state
			cnt++
		}

	}
	// commit the memory trie to the database in the disk
	if cnt == 0 {
		return common.BytesToHash(bc.CurrentBlock.Header.StateRoot)
	}
	rt, ns := st.Commit(false)
	err = bc.triedb.Update(trie.NewWithNodeSet(ns))
	if err != nil {
		log.Panic()
	}
	err = bc.triedb.Commit(rt, false)
	if err != nil {
		log.Panic(err)
	}
	bc.Flag = true
	fmt.Println("modified account number is ", cnt)
	return rt
}

// generate (mine) a block, this function return a block
func (bc *BlockChain) GenerateBlock() *core.Block {
	// pack the transactions from the txpool
	txs := bc.Txpool.PackTxs(bc.ChainConfig.BlockSize)
	bh := &core.BlockHeader{
		ParentBlockHash: bc.CurrentBlock.Hash,
		Number:          bc.CurrentBlock.Header.Number + 1,
		Time:            time.Now(),
	}
	// handle transactions to build root
	rt := bc.GetUpdateStatusTrie(txs)
	bh.TxNum = uint64(len(txs))
	bh.StateRoot = rt.Bytes()
	bh.TxRoot = GetTxTreeRoot(txs)
	b := core.NewBlock(bh, txs)
	b.Header.Miner = 0
	b.Hash = b.Header.Hash()
	return b
}

// new a genisis block, this func will be invoked only once for a blockchain object
func (bc *BlockChain) NewGenisisBlock() *core.Block {
	body := make([]*core.Transaction, 0)
	bh := &core.BlockHeader{
		Number: 0,
	}
	// build a new trie database by db
	triedb := trie.NewDatabaseWithConfig(bc.db, &trie.Config{
		Cache:     0,
		Preimages: true,
	})
	bc.triedb = triedb
	statusTrie := trie.NewEmpty(triedb)
	bh.StateRoot = statusTrie.Hash().Bytes()
	bh.TxRoot = GetTxTreeRoot(body)
	b := core.NewBlock(bh, body)
	b.Hash = b.Header.Hash()
	return b
}

// add the genisis block in a blockchain
func (bc *BlockChain) AddGenisisBlock(gb *core.Block) {
	bc.Storage.AddBlock(gb)
	newestHash, err := bc.Storage.GetNewestBlockHash()
	if err != nil {
		log.Panic()
	}
	curb, err := bc.Storage.GetBlock(newestHash)
	if err != nil {
		log.Panic()
	}
	bc.CurrentBlock = curb
}

// add a block
func (bc *BlockChain) AddBlock(b *core.Block) {
	if b.Header.Number != bc.CurrentBlock.Header.Number+1 {
		fmt.Println("the block height is not correct")
		return
	}
	// if this block is mined by the node, the transactions is no need to be handled again
	if b.Header.Miner != bc.ChainConfig.NodeID {
		rt := bc.GetUpdateStatusTrie(b.Body)
		fmt.Println(bc.CurrentBlock.Header.Number+1, "the root = ", rt.Bytes())
	}
	bc.CurrentBlock = b
	bc.Storage.AddBlock(b)
}

// new a blockchain.
// the ChainConfig is pre-defined to identify the blockchain; the db is the status trie database in disk
func NewBlockChain(cc *params.ChainConfig, db ethdb.Database) (*BlockChain, error) {
	fmt.Println("Generating a new blockchain", db)
	bc := &BlockChain{
		db:            db,
		ChainConfig:   cc,
		Txpool:        core.NewTxPool(),
		Storage:       storage.NewStorage(cc),
		AllStateMapTx: make(map[string]*core.AccountState),
		PartitionMap:  make(map[string]uint64),
	}

	curHash, err := bc.Storage.GetNewestBlockHash()
	if err != nil {
		fmt.Println("Get newest block hash err")
		// if the Storage bolt database cannot find the newest blockhash,
		// it means the blockchain should be built in height = 0
		if err.Error() == "cannot find the newest block hash" {
			genisisBlock := bc.NewGenisisBlock()
			bc.AddGenisisBlock(genisisBlock)
			fmt.Println("New genisis block")
			return bc, nil
		}
		log.Panic()
	}

	// there is a blockchain in the storage
	fmt.Println("Existing blockchain found")
	curb, err := bc.Storage.GetBlock(curHash)
	if err != nil {
		log.Panic()
	}

	bc.CurrentBlock = curb
	triedb := trie.NewDatabaseWithConfig(db, &trie.Config{
		Cache:     0,
		Preimages: true,
	})
	bc.triedb = triedb
	// check the existence of the trie database
	_, err = trie.New(trie.TrieID(common.BytesToHash(curb.Header.StateRoot)), triedb)
	if err != nil {
		log.Panic()
	}
	fmt.Println("The status trie can be built")
	fmt.Println("Generated a new blockchain successfully")
	return bc, nil
}

// check a block is valid or not in this blockchain config
func (bc *BlockChain) IsValidBlock(b *core.Block) error {
	if string(b.Header.ParentBlockHash) != string(bc.CurrentBlock.Hash) {
		fmt.Println("the parentblock hash is not equal to the current block hash")
		return errors.New("the parentblock hash is not equal to the current block hash")
	} else if string(GetTxTreeRoot(b.Body)) != string(b.Header.TxRoot) {
		fmt.Println("the transaction root is wrong")
		return errors.New("the transaction root is wrong")
	}
	return nil
}

// add accounts
func (bc *BlockChain) AddAccounts(ac []string, as []*core.AccountState) {
	fmt.Printf("The len of accounts is %d, now adding the accounts\n", len(ac))

	bh := &core.BlockHeader{
		ParentBlockHash: bc.CurrentBlock.Hash,
		Number:          bc.CurrentBlock.Header.Number + 1,
		Time:            time.Time{},
	}
	// handle transactions to build root
	rt := common.BytesToHash(bc.CurrentBlock.Header.StateRoot)
	if len(ac) != 0 {
		st, err := trie.New(trie.TrieID(common.BytesToHash(bc.CurrentBlock.Header.StateRoot)), bc.triedb)
		if err != nil {
			log.Panic(err)
		}
		for i, addr := range ac {
			if bc.Get_PartitionMap(addr) == bc.ChainConfig.ShardID {
				ib := new(big.Int)
				ib.Add(ib, as[i].Balance)
				new_state := &core.AccountState{
					Balance: ib,
					Nonce:   as[i].Nonce,
				}
				st.Update([]byte(addr), new_state.Encode())
			}
		}
		rrt, ns := st.Commit(false)
		err = bc.triedb.Update(trie.NewWithNodeSet(ns))
		if err != nil {
			log.Panic(err)
		}
		err = bc.triedb.Commit(rt, false)
		if err != nil {
			log.Panic(err)
		}
		rt = rrt
	}

	emptyTxs := make([]*core.Transaction, 0)
	bh.StateRoot = rt.Bytes()
	bh.TxRoot = GetTxTreeRoot(emptyTxs)
	b := core.NewBlock(bh, emptyTxs)
	b.Header.Miner = 0
	b.Hash = b.Header.Hash()

	bc.CurrentBlock = b
	bc.Storage.AddBlock(b)
}

// 根据地址获取账户状态
func (bc *BlockChain) FetchAccounts(addrs []string) []*core.AccountState {
	res := make([]*core.AccountState, 0)
	st, err := trie.New(trie.TrieID(common.BytesToHash(bc.CurrentBlock.Header.StateRoot)), bc.triedb)
	if err != nil {
		log.Panic(err)
	}
	for _, addr := range addrs {
		asenc, _ := st.Get([]byte(addr))
		var state_a *core.AccountState
		if asenc == nil {
			ib := new(big.Int)
			ib.Add(ib, params.Init_Balance)
			state_a = &core.AccountState{
				Nonce:   uint64(0),
				Balance: ib,
			}
		} else {
			state_a = core.DecodeAS(asenc)
		}
		res = append(res, state_a)
	}
	return res
}

func (bc *BlockChain) AddAccount(ac []string, as []*core.AccountState) {
	bh := &core.BlockHeader{
		ParentBlockHash: bc.CurrentBlock.Hash,
		Number:          bc.CurrentBlock.Header.Number + 1,
		Time:            time.Time{},
	}
	rt := common.BytesToHash(bc.CurrentBlock.Header.StateRoot)

	fmt.Println("当前区块状态根：", rt)
	if len(ac) != 0 {
		st, err := trie.New(trie.TrieID(common.BytesToHash(bc.CurrentBlock.Header.StateRoot)), bc.triedb)
		if err != nil {
			log.Panic(err)
		}
		for i, addr := range ac {
			//fmt.Println(addr)
			r_state_enc, _ := st.Get([]byte(addr))
			var r_state *core.AccountState
			if r_state_enc == nil {
				ib := new(big.Int)
				ib.Add(ib, as[i].Balance)
				ib.Add(ib, params.Init_Balance)
				new_state := &core.AccountState{
					Balance: ib,
					Nonce:   as[i].Nonce,
				}
				st.Update([]byte(addr), new_state.Encode())
			} else {
				r_state = core.DecodeAS(r_state_enc)
				r_state.Deposit(as[i].Balance)
				st.Update([]byte(addr), r_state.Encode())
			}
		}
		rrt, ns := st.Commit(false)
		err = bc.triedb.Update(trie.NewWithNodeSet(ns)) //使用节点集创建新Trie，并将其提交到数据库中
		if err != nil {
			log.Panic(err)
		}
		err = bc.triedb.Commit(rt, false)
		if err != nil {
			log.Panic(err)
		}
		rt = rrt
	}
	fmt.Println("新的状态根：", rt)
	emptyTxs := make([]*core.Transaction, 0)
	bh.StateRoot = rt.Bytes()
	bh.TxRoot = GetTxTreeRoot(emptyTxs)
	b := core.NewBlock(bh, emptyTxs)
	b.Header.Miner = 0
	b.Hash = b.Header.Hash()

	bc.AddBlock(b)
	//bc.CurrentBlock = b
	//bc.Storage.AddBlock(b)
	println("账户状态添加完成")
}

// 关闭一个区块链，关闭数据库接口
func (bc *BlockChain) CloseBlockChain() {
	bc.Storage.DataBase.Close()
	bc.triedb.CommitPreimages()
}

// 打印区块链的详细信息
func (bc *BlockChain) PrintBlockChain() string {
	vals := []interface{}{
		bc.CurrentBlock.Header.Number,
		bc.CurrentBlock.Hash,
		bc.CurrentBlock.Header.StateRoot,
		bc.CurrentBlock.Header.Time,
		bc.triedb,
		// len(bc.Txpool.RelayPool[1]),
	}
	res := fmt.Sprintf("%v\n", vals)
	fmt.Println(res)
	return res
}
func (bc *BlockChain) MapInfo() map[string]uint64 {
	Map := make(map[string]uint64)
	shardData := []struct {
		Account  string
		ShardNum uint64
	}{
		{"0xea674fdde714fd979de3edf0f56aa9716b898ec8", 1}, {"0xa42af2c70d316684e57aefcc6e393fecb1c7e84e", 1}, {"0x6c7f03ddfdd8a37ca267c88630a4fee958591de0", 1}, {"0x70faa28a6b8d6829a4b1e649d26ec9a2a39ba413", 1}, {"0xfbb1b73c4f0bda4f67dca266ce6ef42f520fbb98", 1}, {"0xcaa216e03ee4932941ef0729f250e297fd5655ad", 1}, {"0xfbe26da0e985087d28228defbdaa394713b0865f", 1}, {"0xe9eeaec75883f0e389a78e2260bfac1776df2f1d", 1}, {"0x517c265f45eac01389dbb3045206c1d07c7a178c", 1}, {"0xc4df9a95d783c1d181b81c69370d201e595acea8", 1}, {"0x556b5712c2a7cc5506fbfa785b52536fb8243765", 1}, {"0x3c0aab77940d82dd73d6eb2a67682e0c4095e0ad", 1}, {"0x45857089358bc54891067319e8c0b60a5a552bfe", 1}, {"0x167a9333bf582556f35bd4d16a7e80e191aa6476", 1}, {"0xfde9fa07b01ece3e51a3e509cc30bc8d6380b45b", 1}, {"0x24f21c22f0e641e2371f04a7bb8d713f89f53550", 1}, {"0x754c50465885f1ed1fa1a55b95ee8ecf3f1f4324 ", 1},
		{"0x2a65aca4d5fc5b5c859090a6c34d164135398226", 2}, {"0x61c808d82a3ac53231750dadc13c777b59310bd9", 2}, {"0x27bebc44c6f60f99d558422288d616ee15dcb36d", 2}, {"0x32be343b94f860124dc4fee278fdcbd38c102d88", 2}, {"0x267be1c1d684f78cb4f6a176c4911b741e4ffdc0", 2}, {"0x96fc4553a00c117c5b0bed950dd625d1c16dc894", 2}, {"0x09820a2fae3bd73b5009e89ccb7bb236ff59b744", 2}, {"0x81f631b8615eab75d38dac4d4bce4a5b63e10310", 2}, {"0x1151314c646ce4e0efd76d1af4760ae66a9fe30f", 2}, {"0x999d1ce359692aebc26cd969a31d47d150128600", 2}, {"0x304cc179719bc5b05418d6f7f6783abe45d83090", 2}, {"0xcf40d0d2b44f2b66e07cace1372ca42b73cf21a3", 2}, {"0x8b2232c5006a7c9bcb7b52d07ad429bb71ae2c1c", 2}, {"0xab11204cfeaccffa63c2d23aef2ea9accdb0a0d5", 2}, {"0xb36cbe7f95a39984384e6aa4068b02c1697ef80e", 2}, {"0x4cd3fce91781904ac10e88619045719dee6869ee", 2}, {"0xa2a8f158aed54ce9a73d41eeec23bf3a51b5654d", 2}, {"0xacb0c84dbca1d25fe97d5513456b926abd758dbe", 2}, {"0xdf21fa922215b1a56f5a6d6294e6e36c85a0acfb", 2},
		{"0x9e0b9ddba97dd4f7addab0b5f67036eebe687606", 3}, {"0x1e9939daaad6924ad004c2560e90804164900341", 3}, {"0xb2d955733e6a470533f68f72d0af442070f24f55", 3}, {"0xb2930b35844a230f00e51431acae96fe543a0347", 3}, {"0xa8f769b88d6d74fb2bd3912f6793f75625228baf", 3}, {"0x9e6316f44baeeee5d41a1070516cc5fa47baf227", 3}, {"0xfd4ef6b7b52ec618881636c851dd9f28683d4d47", 3}, {"0x96338149e9f6c262d4cb7aeec1cf4c652079a11c", 3}, {"0xa027231f42c80ca4125b5cb962a21cd4f812e88f", 3}, {"0x7ed1e469fcb3ee19c0366d829e291451be638e59", 3}, {"0x00a63d34051602b2cb268ea344d4b8bc4767f2d4", 3}, {"0xdfad8c8a6172d33b95038ec5bb067239b71f70d0", 3}, {"0x02459d2ea9a008342d8685dae79d213f14a87d43", 3}, {"0x1fd6267f0d86f62d88172b998390afee2a1f54b6", 3}, {"0x40ce7569d555dbf939e58867be78fd76142df821", 3}, {"0xc49a0f384cdfbe5cef14dc663639af7d11ec11d0", 3}, {"0x9c67e141c0472115aa1b98bd0088418be68fd249", 3}, {"0x9e9d1613f966936f74534ba71614350d1d87c4b1", 3}, {"0x594ec26a4ac4ee03f625357af76f033e4ea2b53c", 3}, {"0xfcdc0ba0c152ed4b9e7add6f73896c18af636565", 3}, {"0x049029dd41661e58f99271a0112dfd34695f7000 ", 3}, {"0xf34a762291e2578b79646cbf296abf4f5a242b3d", 3}, {"0xd3d038bcb4f2450c592381f8bb4bee860532ee9d", 3}, {"0xcd88e0e0c455345833ce31c5452c1c37f4b4c438", 3}, {"0x86c9981b0d85e1cd6f42b10b8305ffd88f64f55e", 3}, {"0x138ad256e99d8818fd4305d12e6f7ea0aac09d59", 3}, {"0x16545fb79dbee1ad3a7f868b7661c023f372d5de", 3}, {"0x3143e1cabc3547acde8b630e0dea3b1dfebb3cb0", 3}, {"0x6483d4dbbeae052f69c90b0bd26ccff2a44ada13", 3}, {"0x211e96f7a2dfbc2512331f69976445b28f8e63e6", 3}, {"0x9ad47b64f333fb2e313ee7828e11bef01aa8c2de", 3}, {"0xac4361f56c82ed59d533d45129f407015d84702a", 3},
		{"0xd34da389374caad1a048fbdc4569aae33fd5a375", 0}, {"0x52bc44d5378309ee2abf1539bf71de1b7d7be3b5", 0}, {"0x9535b2e7faaba5288511d89341d94a38063a349b", 0}, {"0x4bb96091ee9d802ed039c4d1a5f6216f90f81b01", 0}, {"0xf8165fe1c2cc5360049e2b9c6bd88432a01c0d24", 0}, {"0x7b818b805ac3a94e74e5d417f5871ca0a53fd04d ", 0}, {"0x7baf96ee63017cc63d7da8df51fb04d4c3a7ef7b", 0}, {"0x26588a9301b0428d95e6fc3a5024fce8bec12d51", 0}, {"0x42da8a05cb7ed9a43572b5ba1b8f82a0a6e263dc", 0}, {"0x91337a300e0361bddb2e377dd4e88ccb7796663d", 0}, {"0x215c86bc952b0d98c4b2313a0a9ae56fa33c7f5d", 0}, {"0x3a088473a45ef73c40add2d2af2b5b0106fe2e8f", 0}, {"0xd94c9ff168dc6aebf9b6cc86deff54f3fb0afc33", 0}, {"0x4334275bca3eea692973a3f2de1a783cb59148d8", 0}, {"0xef846f473ef73c34d75712babd7d04bfabd7b56c", 0}, {"0xf90c9ac616ecfefb3860aaa5bc33caf9bc606441", 0}, {"0xbbabf639cd15a5692e237c2412bfe1afe7a37d08", 0}, {"0xed059bc543141c8c93031d545079b3da0233b27f", 0}, {"0xb84961e1ffae1f131fba36a6a90c03e124f970ee", 0}, {"0x2754d69d250c71b88f0d72246fb235ac38e39550", 0}, {"0x06a2cf3f024d8871baedd0f7bd9340001aada574", 0}, {"0x5ce174e21ec9e29af1132c606f41305e2905f1db", 0},
	}
	for _, data := range shardData {
		Map[data.Account] = data.ShardNum
	}
	return Map
}
