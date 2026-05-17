package utils

import (
	"block/params"
	"log"
	"strconv"
)

// the default method
func Addr2Shard(addr Address) int {
	last16_addr := addr[len(addr)-8:]
	num, err := strconv.ParseUint(last16_addr, 16, 64)
	if err != nil {
		log.Panic(err)
	}
	return int(num) % params.ShardNum
}
func AddrVirtualAccount(addr Address, shardID int) Address {
	// 将 shardID 转换为字符串
	shardIDStr := strconv.Itoa(shardID)
	// 将 addr 和 shardIDStr 拼接
	xuniAddress := addr + shardIDStr
	return xuniAddress
}
func RestoreAccount(xuniaddr Address) Address {
	restoreAddress := xuniaddr[:40]
	return restoreAddress
}
func RestoreAccountShard(xuniaddr Address) (Address, int) {
	restoreAddress := xuniaddr[:40]
	shardid := int(xuniaddr[41])
	return restoreAddress, shardid
}
