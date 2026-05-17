package main

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const recordDir = "record"

var accountAddrPattern = regexp.MustCompile(`[0-9a-f]{20,64}`)

type liteTx struct {
	Sender         string
	Recipient      string
	Nonce          uint64
	Signature      []byte
	Value          *big.Int
	TxHash         []byte
	Relayed        bool
	HasBroker      bool
	SenderIsBroker bool
	OriginalSender string
	FinalRecipient string
	RawTxHash      []byte
}

type liteBlockHeader struct {
	ParentBlockHash []byte
	StateRoot       []byte
	TxRoot          []byte
	Number          uint64
	Time            time.Time
	Miner           uint64
}

type liteBlock struct {
	Header liteBlockHeader
	Body   []*liteTx
	Hash   []byte
}

type batchRow struct {
	KeyHex string
	Value  []byte
}

type BlockView struct {
	Height       uint64 `json:"height"`
	TxCount      int    `json:"txCount"`
	Hash         string `json:"hash"`
	HashShort    string `json:"hashShort"`
	ParentHash   string `json:"parentHash"`
	Timestamp    string `json:"timestamp"`
	TimestampRaw int64  `json:"timestampRaw"`
	IsLatest     bool   `json:"isLatest"`
}

type ShardView struct {
	ID           int         `json:"id"`
	Label        string      `json:"label"`
	BlockCount   int         `json:"blockCount"`
	LatestHeight uint64      `json:"latestHeight"`
	LatestHash   string      `json:"latestHash"`
	Blocks       []BlockView `json:"blocks"`
}

type StatView struct {
	BlockchainSizeBytes int64  `json:"blockchainSizeBytes"`
	BlockchainSizeText  string `json:"blockchainSizeText"`
	TotalTransactions   int    `json:"totalTransactions"`
	GenesisBlockTime    string `json:"genesisBlockTime"`
	UserCount           int    `json:"userCount"`
	ShardCount          int    `json:"shardCount"`
	BlockCount          int    `json:"blockCount"`
	LastUpdated         string `json:"lastUpdated"`
}

type TransactionView struct {
	Index          int    `json:"index"`
	Sender         string `json:"sender"`
	Recipient      string `json:"recipient"`
	Value          string `json:"value"`
	Nonce          uint64 `json:"nonce"`
	TxHash         string `json:"txHash"`
	OriginalSender string `json:"originalSender"`
	FinalRecipient string `json:"finalRecipient"`
	Relayed        bool   `json:"relayed"`
	HasBroker      bool   `json:"hasBroker"`
	SenderIsBroker bool   `json:"senderIsBroker"`
}

type Snapshot struct {
	Stat   StatView    `json:"stat"`
	Shards []ShardView `json:"shards"`
}

func main() {
	gob.RegisterName("core.Transaction", &liteTx{})
	gob.RegisterName("[]*core.Transaction", []*liteTx{})
	gob.RegisterName("core.BlockHeader", liteBlockHeader{})
	gob.RegisterName("core.Block", liteBlock{})

	mux := http.NewServeMux()
	mux.HandleFunc("/api/stat", func(w http.ResponseWriter, r *http.Request) {
		snapshot, err := loadSnapshot(recordDir)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, snapshot.Stat)
	})
	mux.HandleFunc("/api/shards", func(w http.ResponseWriter, r *http.Request) {
		snapshot, err := loadSnapshot(recordDir)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, map[string]any{"shards": snapshot.Shards})
	})
	mux.HandleFunc("/api/blocks", func(w http.ResponseWriter, r *http.Request) {
		snapshot, err := loadSnapshot(recordDir)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, map[string]any{"shards": snapshot.Shards})
	})
	mux.HandleFunc("/api/block-transactions", func(w http.ResponseWriter, r *http.Request) {
		shardID, err := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("shard")))
		if err != nil {
			http.Error(w, `{"error":"invalid shard"}`, http.StatusBadRequest)
			return
		}
		hash := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("hash")))
		if hash == "" {
			http.Error(w, `{"error":"missing hash"}`, http.StatusBadRequest)
			return
		}
		transactions, err := loadBlockTransactions(recordDir, shardID, hash)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, map[string]any{
			"shard":        shardID,
			"hash":         hash,
			"transactions": transactions,
		})
	})
	serveIndex := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		http.ServeFile(w, r, "index.html")
	}
	mux.HandleFunc("/index.html", serveIndex)
	staticFS := http.FileServer(http.Dir("."))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			serveIndex(w, r)
			return
		}
		staticFS.ServeHTTP(w, r)
	})

	addr := serverAddr()
	log.Printf("server listening on http://localhost%s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

func serverAddr() string {
	port := strings.TrimSpace(os.Getenv("PORT"))
	if port == "" {
		port = "8080"
	}
	if !strings.HasPrefix(port, ":") {
		port = ":" + port
	}
	return port
}

func loadSnapshot(root string) (Snapshot, error) {
	sizeBytes, err := dirSize(root)
	if err != nil {
		return Snapshot{}, err
	}

	shards, err := loadShardBlocks(root)
	if err != nil {
		return Snapshot{}, err
	}

	userCount, err := loadUserCount(filepath.Join(root, "ldb"))
	if err != nil {
		return Snapshot{}, err
	}

	totalTx := 0
	totalBlocks := 0
	var genesis time.Time
	for i := range shards {
		totalBlocks += len(shards[i].Blocks)
		for j := range shards[i].Blocks {
			totalTx += shards[i].Blocks[j].TxCount
			ts := time.Unix(0, shards[i].Blocks[j].TimestampRaw).UTC()
			if shards[i].Blocks[j].TimestampRaw == 0 {
				continue
			}
			if genesis.IsZero() || ts.Before(genesis) {
				genesis = ts
			}
		}
	}

	return Snapshot{
		Stat: StatView{
			BlockchainSizeBytes: sizeBytes,
			BlockchainSizeText:  humanSize(sizeBytes),
			TotalTransactions:   totalTx,
			GenesisBlockTime:    formatTime(genesis),
			UserCount:           userCount,
			ShardCount:          len(shards),
			BlockCount:          totalBlocks,
			LastUpdated:         time.Now().UTC().Format(time.RFC3339),
		},
		Shards: shards,
	}, nil
}

func loadShardBlocks(root string) ([]ShardView, error) {
	chosen, err := chooseShardDatabases(root)
	if err != nil {
		return nil, err
	}

	shardIDs := make([]int, 0, len(chosen))
	for shardID := range chosen {
		shardIDs = append(shardIDs, shardID)
	}
	sort.Ints(shardIDs)

	shards := make([]ShardView, 0, len(shardIDs))
	for _, shardID := range shardIDs {
		blocks, err := parseShardDatabase(chosen[shardID])
		if err != nil {
			return nil, fmt.Errorf("parse shard %d: %w", shardID, err)
		}
		latestHeight := uint64(0)
		latestHash := ""
		for i := range blocks {
			if blocks[i].Height >= latestHeight {
				latestHeight = blocks[i].Height
				latestHash = blocks[i].Hash
			}
		}
		for i := range blocks {
			blocks[i].IsLatest = blocks[i].Height == latestHeight && latestHash == blocks[i].Hash
		}
		shards = append(shards, ShardView{
			ID:           shardID,
			Label:        fmt.Sprintf("Shard %d", shardID),
			BlockCount:   len(blocks),
			LatestHeight: latestHeight,
			LatestHash:   latestHash,
			Blocks:       blocks,
		})
	}

	return shards, nil
}

func chooseShardDatabases(root string) (map[int]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}

	chosen := make(map[int]string)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), "_database") {
			continue
		}
		shardID, nodeID, ok := parseDatabaseName(entry.Name())
		if !ok {
			continue
		}
		current, exists := chosen[shardID]
		if !exists || nodeID < parseNodeFromPath(current) {
			chosen[shardID] = filepath.Join(root, entry.Name())
		}
	}
	return chosen, nil
}

func parseDatabaseName(name string) (int, int, bool) {
	base := strings.TrimSuffix(name, "_database")
	parts := strings.Split(base, "_")
	if len(parts) != 2 {
		return 0, 0, false
	}
	shardID, err1 := strconv.Atoi(parts[0])
	nodeID, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return shardID, nodeID, true
}

func parseNodeFromPath(path string) int {
	_, nodeID, ok := parseDatabaseName(filepath.Base(path))
	if !ok {
		return 1 << 30
	}
	return nodeID
}

func parseShardDatabase(path string) ([]BlockView, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	needle := []byte("Block\x01")
	seen := make(map[string]BlockView)
	for idx := 0; idx < len(data); {
		pos := bytes.Index(data[idx:], needle)
		if pos < 0 {
			break
		}
		pos += idx
		matched := false
		for back := 40; back >= 0; back-- {
			start := pos - back
			if start < 0 {
				continue
			}
			reader := bytes.NewReader(data[start:])
			dec := gob.NewDecoder(reader)
			var block liteBlock
			if err := dec.Decode(&block); err != nil || len(block.Hash) == 0 {
				continue
			}
			consumed := len(data[start:]) - reader.Len()
			if consumed <= 0 || start+consumed > len(data) {
				continue
			}

			hash := hex.EncodeToString(block.Hash)
			record := data[start : start+consumed]
			ts := block.Header.Time
			if ts.IsZero() {
				ts = extractTimestamp(record)
			}
			blockView := BlockView{
				Height:       block.Header.Number,
				TxCount:      len(block.Body),
				Hash:         hash,
				HashShort:    shortHash(hash),
				ParentHash:   hex.EncodeToString(block.Header.ParentBlockHash),
				Timestamp:    formatTime(ts),
				TimestampRaw: ts.UnixNano(),
			}

			existing, exists := seen[hash]
			if !exists || blockView.TxCount > existing.TxCount || blockView.Height > existing.Height {
				seen[hash] = blockView
			}
			idx = start + max(consumed, 1)
			matched = true
			break
		}
		if !matched {
			idx = pos + len(needle)
		}
	}

	blocks := make([]BlockView, 0, len(seen))
	for _, block := range seen {
		blocks = append(blocks, block)
	}
	sort.Slice(blocks, func(i, j int) bool {
		if blocks[i].Height == blocks[j].Height {
			return blocks[i].Hash < blocks[j].Hash
		}
		return blocks[i].Height < blocks[j].Height
	})
	return blocks, nil
}

func loadBlockTransactions(root string, shardID int, hash string) ([]TransactionView, error) {
	chosen, err := chooseShardDatabases(root)
	if err != nil {
		return nil, err
	}

	path, ok := chosen[shardID]
	if !ok {
		return []TransactionView{}, nil
	}

	return parseBlockTransactions(path, hash)
}

func parseBlockTransactions(path, hash string) ([]TransactionView, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	targetHash := strings.ToLower(strings.TrimSpace(hash))
	needle := []byte("Block\x01")
	for idx := 0; idx < len(data); {
		pos := bytes.Index(data[idx:], needle)
		if pos < 0 {
			break
		}
		pos += idx
		matched := false
		for back := 40; back >= 0; back-- {
			start := pos - back
			if start < 0 {
				continue
			}
			reader := bytes.NewReader(data[start:])
			dec := gob.NewDecoder(reader)
			var block liteBlock
			if err := dec.Decode(&block); err != nil || len(block.Hash) == 0 {
				continue
			}
			consumed := len(data[start:]) - reader.Len()
			if consumed <= 0 || start+consumed > len(data) {
				continue
			}
			if hex.EncodeToString(block.Hash) == targetHash {
				return buildTransactionViews(block.Body, 100), nil
			}
			idx = start + max(consumed, 1)
			matched = true
			break
		}
		if !matched {
			idx = pos + len(needle)
		}
	}

	return []TransactionView{}, nil
}

func buildTransactionViews(body []*liteTx, limit int) []TransactionView {
	if len(body) == 0 {
		return []TransactionView{}
	}
	if limit <= 0 || limit > len(body) {
		limit = len(body)
	}

	transactions := make([]TransactionView, 0, limit)
	for i := 0; i < limit; i++ {
		tx := body[i]
		if tx == nil {
			transactions = append(transactions, TransactionView{Index: i + 1})
			continue
		}
		transactions = append(transactions, TransactionView{
			Index:          i + 1,
			Sender:         strings.TrimSpace(tx.Sender),
			Recipient:      strings.TrimSpace(tx.Recipient),
			Value:          bigIntText(tx.Value),
			Nonce:          tx.Nonce,
			TxHash:         encodeHash(tx.TxHash, tx.RawTxHash),
			OriginalSender: strings.TrimSpace(tx.OriginalSender),
			FinalRecipient: strings.TrimSpace(tx.FinalRecipient),
			Relayed:        tx.Relayed,
			HasBroker:      tx.HasBroker,
			SenderIsBroker: tx.SenderIsBroker,
		})
	}
	return transactions
}

func encodeHash(primary, fallback []byte) string {
	if len(primary) > 0 {
		return hex.EncodeToString(primary)
	}
	if len(fallback) > 0 {
		return hex.EncodeToString(fallback)
	}
	return ""
}

func bigIntText(value *big.Int) string {
	if value == nil {
		return ""
	}
	return value.String()
}

func extractTimestamp(record []byte) time.Time {
	const (
		minUnixNano = uint64(1577836800) * uint64(time.Second)
		maxUnixNano = uint64(1798761600) * uint64(time.Second)
	)

	type candidate struct {
		count int
		pos   int
	}

	counts := make(map[uint64]candidate)
	for i := 0; i+8 <= len(record); i++ {
		v := binary.LittleEndian.Uint64(record[i : i+8])
		if v < minUnixNano || v > maxUnixNano {
			continue
		}
		c := counts[v]
		if c.count == 0 {
			c.pos = i
		}
		c.count++
		counts[v] = c
	}

	bestValue := uint64(0)
	bestCount := -1
	bestPos := len(record) + 1
	for value, c := range counts {
		if c.count > bestCount || (c.count == bestCount && c.pos < bestPos) {
			bestValue = value
			bestCount = c.count
			bestPos = c.pos
		}
	}
	if bestValue == 0 {
		return time.Time{}
	}
	return time.Unix(0, int64(bestValue)).UTC()
}

func loadUserCount(ldbRoot string) (int, error) {
	shards, err := os.ReadDir(ldbRoot)
	if err != nil {
		return 0, err
	}

	accounts := make(map[string]struct{})
	for _, shard := range shards {
		if !shard.IsDir() {
			continue
		}
		logPath := filepath.Join(ldbRoot, shard.Name(), "n0", "000001.log")
		rows, err := parseLevelDBLog(logPath)
		if err != nil {
			return 0, err
		}
		for _, row := range rows {
			if !bytes.Contains(row.Value, []byte("AccountState")) {
				continue
			}
			accountID := extractAccountID(row)
			accounts[accountID] = struct{}{}
		}
	}
	return len(accounts), nil
}

func extractAccountID(row batchRow) string {
	text := printable(row.Value)
	if match := accountAddrPattern.FindString(text); match != "" {
		return match
	}
	return row.KeyHex
}

func parseLevelDBLog(path string) ([]batchRow, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var (
		offset   int
		fragment []byte
		rows     []batchRow
	)
	for offset < len(data) {
		blockEnd := min(((offset/32768)+1)*32768, len(data))
		for offset+7 <= blockEnd {
			if blockEnd-offset < 7 {
				offset = blockEnd
				break
			}
			length := int(binary.LittleEndian.Uint16(data[offset+4 : offset+6]))
			recType := data[offset+6]
			offset += 7
			if length == 0 && recType == 0 {
				offset = blockEnd
				break
			}
			if offset+length > len(data) {
				return nil, io.ErrUnexpectedEOF
			}
			payload := append([]byte(nil), data[offset:offset+length]...)
			offset += length
			switch recType {
			case 1:
				batchRows, err := parseBatch(payload)
				if err == nil {
					rows = append(rows, batchRows...)
				}
			case 2:
				fragment = append(fragment[:0], payload...)
			case 3:
				fragment = append(fragment, payload...)
			case 4:
				fragment = append(fragment, payload...)
				batchRows, err := parseBatch(fragment)
				if err == nil {
					rows = append(rows, batchRows...)
				}
				fragment = nil
			default:
				fragment = nil
			}
		}
		offset = blockEnd
	}
	return rows, nil
}

func parseBatch(payload []byte) ([]batchRow, error) {
	if len(payload) < 12 {
		return nil, fmt.Errorf("batch too short")
	}
	count := int(binary.LittleEndian.Uint32(payload[8:12]))
	pos := 12
	rows := make([]batchRow, 0, count)
	for i := 0; i < count && pos < len(payload); i++ {
		kind := payload[pos]
		pos++
		key, next, err := readVarString(payload, pos)
		if err != nil {
			return rows, err
		}
		pos = next
		var value []byte
		if kind == 1 {
			value, next, err = readVarString(payload, pos)
			if err != nil {
				return rows, err
			}
			pos = next
		}
		rows = append(rows, batchRow{
			KeyHex: hex.EncodeToString(key),
			Value:  value,
		})
	}
	return rows, nil
}

func readVarString(data []byte, pos int) ([]byte, int, error) {
	length, n := binary.Uvarint(data[pos:])
	if n <= 0 {
		return nil, pos, fmt.Errorf("bad varint")
	}
	pos += n
	end := pos + int(length)
	if end > len(data) {
		return nil, pos, io.ErrUnexpectedEOF
	}
	return data[pos:end], end, nil
}

func dirSize(root string) (int64, error) {
	var total int64
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	return total, err
}

func shortHash(hash string) string {
	if len(hash) <= 12 {
		return hash
	}
	return hash[:6] + "..." + hash[len(hash)-6:]
}

func formatTime(ts time.Time) string {
	if ts.IsZero() {
		return "Unavailable"
	}
	return ts.Local().Format("2006-01-02 15:04:05")
}

func humanSize(size int64) string {
	units := []string{"B", "KB", "MB", "GB", "TB"}
	value := float64(size)
	unit := units[0]
	for i := 0; i < len(units)-1 && value >= 1024; i++ {
		value /= 1024
		unit = units[i+1]
	}
	return fmt.Sprintf("%.2f %s", value, unit)
}

func writeError(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusInternalServerError)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func printable(b []byte) string {
	var sb strings.Builder
	for _, c := range b {
		if c >= 32 && c <= 126 {
			sb.WriteByte(c)
			continue
		}
		sb.WriteString(fmt.Sprintf("\\x%02x", c))
	}
	return sb.String()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
