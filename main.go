package main

import (
	"fmt"
	"os"

	"github.com/c-bata/go-prompt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

func main() {
	if len(os.Args) != 4 {
		errorExit(-1, "missing arguments: usage: %s <client_url> <address> <abi_file>\n", os.Args[0])
	}
	// dial client
	cl, err := ethclient.Dial(os.Args[1])
	if err != nil {
		errorExit(-2, "can't dial client: %s\n", err)
	}
	defer cl.Close()
	// parse contract address
	contractAddr := common.HexToAddress(os.Args[2])
	// read and parse abi file
	contractABI, err := readABI(os.Args[3])
	if err != nil {
		errorExit(-3, "can't read abi: %s\n", err)
	}

	_ = contractAddr
	// setup constant and transaction method calls
	constantNode, transactNode := methodsMenus(contractABI.Methods)
	// setup events
	eventsNode := eventsMenu(contractABI.Events)
	// setup root node
	rootNode := newRootNode([]*menuCompleter{constantNode, transactNode, eventsNode})
	// out := make(chan string, 1024)
	// go findNotDefinedCommands(rootNode, out, true)
	// for i := range out {
	// 	if _, ok := menuCommands[i]; !ok {
	// 		fmt.Printf("WARNING: command not defined: %s\n", i)
	// 	}
	// }
	curNode := rootNode
	for {
		inp := prompt.Input(curNode.prompt(">"), curNode.completer)
	Outer:
		switch inp {
		case "exit":
			os.Exit(0)
		case "help":
			showHelp(curNode)
		case "..":
			if curNode != rootNode {
				curNode = curNode.parent
			}
		case "":
		default:
			for _, i := range curNode.sub {
				if i.suggestion.Text == inp {
					if i.sub == nil {
						switch i.parent {
						case constantNode:
							fmt.Printf("constant method: %s\n", i.name())
						case transactNode:
							fmt.Printf("transaction: %s\n", i.name())
						case eventsNode:
							fmt.Printf("event: %s\n", i.name())
						default:
							cmd := i.name()
							cmdFunc, ok := menuCommands[cmd]
							if ok {
								cmdFunc()
							} else {
								fmt.Printf("command not defined: %s\n", cmd)
							}
						}
					} else {
						curNode = i
						break Outer
					}
				}
			}
		}
	}
}
