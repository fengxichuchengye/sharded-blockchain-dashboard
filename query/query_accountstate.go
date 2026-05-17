package query

import "block/core"

// QueryAccountState is kept for compatibility with the original test helpers.
// The current dashboard backend only relies on block/transaction queries.
func QueryAccountState(ShardID, NodeID uint64, address string) *core.AccountState {
	return nil
}

// QueryAllAccountStates is kept for compatibility with the original test helpers.
func QueryAllAccountStates(ShardID, NodeID uint64) map[string]*core.AccountState {
	return map[string]*core.AccountState{}
}
