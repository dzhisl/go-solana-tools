package solanatools

import (
	"context"
	"encoding/binary"
	"fmt"
	"sort"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
)

// TokenAccount describes an owner's SPL Token or Token-2022 account and raw balance.
type TokenAccount struct {
	Address   solana.PublicKey
	Mint      solana.PublicKey
	Owner     solana.PublicKey
	Program   solana.PublicKey
	RawAmount uint64
}

// TokenAccounts scans legacy SPL Token and Token-2022 accounts of owner.
// Results are returned only when both scans succeed; malformed accounts are errors.
func (c *Client) TokenAccounts(ctx context.Context, owner solana.PublicKey) ([]TokenAccount, error) {
	if err := c.validate(); err != nil {
		return nil, err
	}
	if c.tokenAccountsRPC == nil {
		return nil, fmt.Errorf("token accounts RPC client is required")
	}
	accounts := make([]TokenAccount, 0)
	for _, program := range []solana.PublicKey{solana.TokenProgramID, solana.Token2022ProgramID} {
		res, err := c.tokenAccountsRPC.GetTokenAccountsByOwner(ctx, owner, &rpc.GetTokenAccountsConfig{ProgramId: &program}, &rpc.GetTokenAccountsOpts{Commitment: c.commitment, Encoding: solana.EncodingBase64})
		if err != nil {
			return nil, fmt.Errorf("scan token accounts for %s: %w", program, err)
		}
		if res == nil {
			return nil, fmt.Errorf("empty token accounts response")
		}
		for _, entry := range res.Value {
			if entry == nil || entry.Account.Data == nil {
				return nil, fmt.Errorf("empty token account entry")
			}
			data := entry.Account.Data.GetBinary()
			if len(data) < 165 || entry.Account.Owner != program {
				return nil, fmt.Errorf("invalid token account %s", entry.Pubkey)
			}
			wallet := solana.PublicKeyFromBytes(data[32:64])
			if wallet != owner {
				return nil, fmt.Errorf("token account %s owner mismatch", entry.Pubkey)
			}
			accounts = append(accounts, TokenAccount{Address: entry.Pubkey, Mint: solana.PublicKeyFromBytes(data[:32]), Owner: wallet, Program: program, RawAmount: binary.LittleEndian.Uint64(data[64:72])})
		}
	}
	sort.Slice(accounts, func(i, j int) bool { return accounts[i].Address.String() < accounts[j].Address.String() })
	return accounts, nil
}

// NonZeroTokenMints returns sorted unique mints with a nonzero raw balance.
func (c *Client) NonZeroTokenMints(ctx context.Context, owner solana.PublicKey) ([]solana.PublicKey, error) {
	accounts, err := c.TokenAccounts(ctx, owner)
	if err != nil {
		return nil, err
	}
	unique := make(map[solana.PublicKey]struct{})
	for _, account := range accounts {
		if account.RawAmount > 0 {
			unique[account.Mint] = struct{}{}
		}
	}
	mints := make([]solana.PublicKey, 0, len(unique))
	for mint := range unique {
		mints = append(mints, mint)
	}
	sort.Slice(mints, func(i, j int) bool { return mints[i].String() < mints[j].String() })
	return mints, nil
}

// TokenAccountCounts separates zero-balance and nonzero-balance token accounts.
type TokenAccountCounts struct {
	Empty    int
	NonEmpty int
}

// TokenAccountCounts scans both token programs and counts empty/nonempty accounts.
func (c *Client) TokenAccountCounts(ctx context.Context, owner solana.PublicKey) (TokenAccountCounts, error) {
	accounts, err := c.TokenAccounts(ctx, owner)
	if err != nil {
		return TokenAccountCounts{}, err
	}
	var counts TokenAccountCounts
	for _, account := range accounts {
		if account.RawAmount == 0 {
			counts.Empty++
		} else {
			counts.NonEmpty++
		}
	}
	return counts, nil
}
