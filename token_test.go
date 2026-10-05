package solanatools

import (
	"encoding/binary"
	"github.com/gagliardetto/solana-go"
	"testing"
)

func TestATAAndTransfer(t *testing.T) {
	from := PrivateKeyFromSeed("test-from").PublicKey()
	to := PrivateKeyFromSeed("test-to").PublicKey()
	mint := solana.WrappedSol
	legacy, _, err := solana.FindAssociatedTokenAddress(from, mint)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := AssociatedTokenAddress(from, mint, solana.TokenProgramID)
	if err != nil || actual != legacy {
		t.Fatal("legacy ATA mismatch", err)
	}
	extended, err := AssociatedTokenAddress(from, mint, solana.Token2022ProgramID)
	if err != nil || extended == legacy {
		t.Fatal("Token-2022 ATA mismatch", err)
	}
	ix, err := NewCreateATAIdempotentInstruction(from, to, mint, solana.Token2022ProgramID)
	if err != nil {
		t.Fatal(err)
	}
	ata, _ := AssociatedTokenAddress(to, mint, solana.Token2022ProgramID)
	if ix.Accounts()[1].PublicKey != ata || ix.Accounts()[5].PublicKey != solana.Token2022ProgramID {
		t.Fatal("wrong ATA accounts")
	}
	transfer, err := NewTransferCheckedInstruction(TransferParams{Mint: mint, FromOwner: from, ToOwner: to, TokenProgram: solana.Token2022ProgramID, Amount: 123, Decimals: 6})
	if err != nil {
		t.Fatal(err)
	}
	data, err := transfer.Data()
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 10 || data[0] != 12 || data[9] != 6 || binary.LittleEndian.Uint64(data[1:9]) != 123 {
		t.Fatal("incorrect TransferChecked encoding")
	}
	if transfer.Accounts()[1].PublicKey != mint || !transfer.Accounts()[3].IsSigner {
		t.Fatal("incorrect transfer accounts")
	}
	if _, err := AssociatedTokenAddress(from, mint, solana.SystemProgramID); err == nil {
		t.Fatal("invalid program accepted")
	}
}

func TestPriorityFee(t *testing.T) {
	if _, err := NewPriorityFeeInstruction(1, 0); err == nil {
		t.Fatal("zero CU accepted")
	}
	if _, err := NewPriorityFeeInstruction(^uint64(0), 1); err == nil {
		t.Fatal("overflow accepted")
	}
	ix, err := NewPriorityFeeInstruction(100000, 200000)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := ix.Data()
	if binary.LittleEndian.Uint64(data[1:]) != 500000 {
		t.Fatal("wrong CU price")
	}
}
