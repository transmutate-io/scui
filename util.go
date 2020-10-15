package main

import (
	"fmt"
	"os"

	"github.com/ethereum/go-ethereum/accounts/abi"
)

func readABI(fn string) (*abi.ABI, error) {
	f, err := os.Open(fn)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r, err := abi.JSON(f)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func errorExit(code int, f string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, f, a...)
	os.Exit(code)
}

// func findNotDefinedCommands(node *menuCompleter, out chan string, isRoot bool) {
// 	if isRoot {
// 		defer close(out)
// 	}
// 	if node.sub == nil {
// 		switch s := node.name(); s {
// 		case "..", "help", "exit":
// 		default:
// 			if strings.HasPrefix(s, "constant/") ||
// 				strings.HasPrefix(s, "transact/") ||
// 				strings.HasPrefix(s, "events") {
// 				return
// 			}
// 			out <- s
// 		}
// 		return
// 	}
// 	for _, i := range node.sub {
// 		findNotDefinedCommands(i, out, false)
// 	}
// }
