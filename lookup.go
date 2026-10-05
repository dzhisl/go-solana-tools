package solanatools

import (
	"context"
	"fmt"

	"github.com/gagliardetto/solana-go"
	addresslookuptable "github.com/gagliardetto/solana-go/programs/address-lookup-table"
	"github.com/gagliardetto/solana-go/rpc"
)

// AddressLookupTables fetches address lists for unique table IDs without caching.
func (c *Client) AddressLookupTables(ctx context.Context, ids ...solana.PublicKey) (map[solana.PublicKey]solana.PublicKeySlice, error) {
	if err := c.validate(); err != nil {
		return nil, err
	}
	tables := make(map[solana.PublicKey]solana.PublicKeySlice, len(ids))
	for _, id := range ids {
		if _, exists := tables[id]; exists {
			continue
		}
		state, err := addresslookuptable.GetAddressLookupTableStateWithOpts(ctx, c.rpc, id, &rpc.GetAccountInfoOpts{Commitment: c.commitment})
		if err != nil {
			return nil, fmt.Errorf("lookup table %s: %w", id, err)
		}
		if state == nil {
			return nil, fmt.Errorf("lookup table %s: %w", id, ErrAccountNotFound)
		}
		tables[id] = append(solana.PublicKeySlice(nil), state.Addresses...)
	}
	return tables, nil
}

// TransactionAddressTables fetches tables and builds an SDK transaction option.
func (c *Client) TransactionAddressTables(ctx context.Context, ids ...solana.PublicKey) (solana.TransactionOption, error) {
	tables, err := c.AddressLookupTables(ctx, ids...)
	if err != nil {
		return nil, err
	}
	return solana.TransactionAddressTables(tables), nil
}
