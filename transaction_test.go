package solanatools

import (
	"encoding/base64"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/programs/system"
	"testing"
)

func TestSignEncode(t *testing.T) {
	key := PrivateKeyFromSeed("test-signing-seed")
	tx, err := solana.NewTransaction([]solana.Instruction{system.NewTransferInstruction(123, key.PublicKey(), solana.SystemProgramID).Build()}, solana.Hash{1}, solana.TransactionPayer(key.PublicKey()))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Sign(tx, key); err != nil {
		t.Fatal(err)
	}
	if err := tx.VerifySignatures(); err != nil {
		t.Fatal(err)
	}
	if signer, err := IsSigner(tx, key.PublicKey()); err != nil || !signer {
		t.Fatal(signer, err)
	}
	encoded, err := EncodeTransaction(tx)
	if err != nil {
		t.Fatal(err)
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := solana.TransactionFromBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	if err := decoded.VerifySignatures(); err != nil {
		t.Fatal(err)
	}
	if _, err := Sign(tx, solana.PrivateKey{1}); err == nil {
		t.Fatal("malformed key accepted")
	}
}
