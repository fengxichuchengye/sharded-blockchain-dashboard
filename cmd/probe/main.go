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
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func main() {
	gob.RegisterName("Time", Time(0))
	gob.RegisterName("core.Transaction", &liteTx{})
	gob.RegisterName("[]*core.Transaction", []*liteTx{})
	gob.RegisterName("core.BlockHeader", liteBlockHeader{})
	gob.RegisterName("core.Block", liteBlock{})

	root := filepath.Join("record", "ldb")
	shards, err := os.ReadDir(root)
	if err != nil {
		log.Fatal(err)
	}

	for _, shard := range shards {
		if !shard.IsDir() {
			continue
		}
		nodePath := filepath.Join(root, shard.Name(), "n0", "000001.log")
		fmt.Printf("== %s ==\n", nodePath)
		rows, err := parseLevelDBLog(nodePath)
		if err != nil {
			log.Fatal(err)
		}

		sort.Slice(rows, func(i, j int) bool { return rows[i].Hex < rows[j].Hex })
		limit := min(len(rows), 20)
		for _, row := range rows[:limit] {
			fmt.Printf("key=%q hex=%s len=%d\n", row.Key, row.Hex, len(row.Value))
			describeValue(row.Value)
			fmt.Println()
		}
	}

	fmt.Println("== flat database sample ==")
	if err := inspectFlatDatabase(filepath.Join("record", "0_0_database")); err != nil {
		fmt.Println("flat database decode failed:", err)
	}
	debugKnownOffset(filepath.Join("record", "0_0_database"), 28784)
}

type row struct {
	Key   string
	Hex   string
	Value []byte
}

type Time int64

type liteTx struct{}

type liteBlockHeader struct {
	ParentBlockHash []byte
	StateRoot       []byte
	TxRoot          []byte
	Number          uint64
	Time            any
	Miner           uint64
}

type liteBlock struct {
	Header liteBlockHeader
	Body   []*liteTx
	Hash   []byte
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

func describeValue(v []byte) {
	trim := v
	if len(trim) > 120 {
		trim = trim[:120]
	}
	fmt.Printf("value_hex=%s\n", hex.EncodeToString(trim))

	var asAny any
	if json.Unmarshal(v, &asAny) == nil {
		enc, _ := json.MarshalIndent(asAny, "", "  ")
		fmt.Printf("json=%s\n", string(enc))
	}

	text := printable(trim)
	fmt.Printf("text=%s\n", text)
}

func parseLevelDBLog(path string) ([]row, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var (
		offset   int
		fragment []byte
		rows     []row
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

func parseBatch(payload []byte) ([]row, error) {
	if len(payload) < 12 {
		return nil, fmt.Errorf("batch too short")
	}
	count := int(binary.LittleEndian.Uint32(payload[8:12]))
	pos := 12
	rows := make([]row, 0, count)
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
		rows = append(rows, row{
			Key:   printable(key),
			Hex:   hex.EncodeToString(key),
			Value: value,
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func inspectFlatDatabase(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	needle := []byte("Block\x01")
	found := 0
	for idx := 0; idx < len(data) && found < 10; {
		pos := bytes.Index(data[idx:], needle)
		if pos < 0 {
			break
		}
		pos += idx
		for back := 40; back >= 0; back-- {
			start := pos - back
			if start < 0 {
				continue
			}
			reader := bytes.NewReader(data[start:])
			dec := gob.NewDecoder(reader)
			var block liteBlock
			if err := dec.Decode(&block); err == nil && looksLikeBlock(block) {
				fmt.Printf("offset=%d consumed=%d block number=%d txs=%d hash=%x parent=%x\n",
					start, len(data[start:])-reader.Len(), block.Header.Number,
					len(block.Body), block.Hash, block.Header.ParentBlockHash,
				)
				found++
				idx = start + 1
				break
			}
			reader = bytes.NewReader(data[start:])
			dec = gob.NewDecoder(reader)
			var header liteBlockHeader
			if err := dec.Decode(&header); err == nil && header.Number > 0 {
				fmt.Printf("offset=%d consumed=%d header number=%d parent=%x txroot=%x\n",
					start, len(data[start:])-reader.Len(), header.Number, header.ParentBlockHash, header.TxRoot,
				)
				found++
				idx = start + 1
				break
			}
		}
		idx = pos + len(needle)
	}
	return nil
}

func looksLikeBlock(block liteBlock) bool {
	return block.Header.Number > 0 || len(block.Body) > 0 || len(block.Hash) > 0
}

func debugKnownOffset(path string, offset int) {
	data, err := os.ReadFile(path)
	if err != nil || offset >= len(data) {
		return
	}
	fmt.Println("== debug known offset ==")
	reader := bytes.NewReader(data[offset:])
	dec := gob.NewDecoder(reader)
	var block liteBlock
	if err := dec.Decode(&block); err != nil {
		fmt.Println("block err:", err)
	}
	fmt.Printf("block=%+v body=%d consumed=%d\n", block.Header, len(block.Body), len(data[offset:])-reader.Len())
}
