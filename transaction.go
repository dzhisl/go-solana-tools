package solanatools

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"fmt"

	"github.com/gagliardetto/solana-go"
)

// PrivateKeyFromSeed hashes seed with SHA-256, preserving bundler's derivation.
// The seed must be secret and high entropy: public or guessable seeds expose the key.
func PrivateKeyFromSeed(seed string) solana.PrivateKey {
	hash := sha256.Sum256([]byte(seed))
	return solana.PrivateKey(ed25519.NewKeyFromSeed(hash[:]))
}

func signingKeys(keys []solana.PrivateKey) (map[solana.PublicKey]solana.PrivateKey, error) {
	result := make(map[solana.PublicKey]solana.PrivateKey, len(keys))
	for _, key := range keys {
		if len(key) != ed25519.PrivateKeySize {
			return nil, fmt.Errorf("invalid private key length")
		}
		result[key.PublicKey()] = key
	}
	return result, nil
}

// Sign mutates tx by signing with all required keys; missing signers cause an error.
func Sign(tx *solana.Transaction, keys ...solana.PrivateKey) ([]solana.Signature, error) {
	if tx == nil {
		return nil, fmt.Errorf("transaction is nil")
	}
	indexed, err := signingKeys(keys)
	if err != nil {
		return nil, err
	}
	return tx.Sign(func(public solana.PublicKey) *solana.PrivateKey {
		key, ok := indexed[public]
		if !ok {
			return nil
		}
		return &key
	})
}

// PartialSign mutates tx by adding available signatures while preserving other signers.
func PartialSign(tx *solana.Transaction, keys ...solana.PrivateKey) ([]solana.Signature, error) {
	if tx == nil {
		return nil, fmt.Errorf("transaction is nil")
	}
	indexed, err := signingKeys(keys)
	if err != nil {
		return nil, err
	}
	return tx.PartialSign(func(public solana.PublicKey) *solana.PrivateKey {
		key, ok := indexed[public]
		if !ok {
			return nil
		}
		return &key
	})
}

// EncodeTransaction serializes the complete transaction as base64 without submitting it.
func EncodeTransaction(tx *solana.Transaction) (string, error) {
	if tx == nil {
		return "", fmt.Errorf("transaction is nil")
	}
	data, err := tx.MarshalBinary()
	if err != nil {
		return "", fmt.Errorf("serialize transaction: %w", err)
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

// EncodeTransactions serializes transactions as base64 in their original order.
func EncodeTransactions(txs []*solana.Transaction) ([]string, error) {
	result := make([]string, len(txs))
	for i, tx := range txs {
		value, err := EncodeTransaction(tx)
		if err != nil {
			return nil, fmt.Errorf("transaction %d: %w", i, err)
		}
		result[i] = value
	}
	return result, nil
}

// IsSigner reports whether the transaction requires account's signature.
func IsSigner(tx *solana.Transaction, account solana.PublicKey) (bool, error) {
	if tx == nil {
		return false, fmt.Errorf("transaction is nil")
	}
	metas, err := tx.AccountMetaList()
	if err != nil {
		return false, err
	}
	for _, meta := range metas {
		if meta.IsSigner && meta.PublicKey == account {
			return true, nil
		}
	}
	return false, nil
}
