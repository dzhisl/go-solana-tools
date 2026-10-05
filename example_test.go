package solanatools_test

import (
	"fmt"
	tools "github.com/dzhisl/go-solana-tools"
)

func ExampleParseSOL() {
	lamports, err := tools.ParseSOL("0.125")
	if err != nil {
		panic(err)
	}
	fmt.Println(lamports)
	fmt.Println(tools.FormatSOL(lamports))
	// Output:
	// 125000000
	// 0.125
}
