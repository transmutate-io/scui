package main

import (
	"fmt"
	"os"
	"sort"

	"github.com/c-bata/go-prompt"
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

func methodsMenus(methods map[string]abi.Method) (*menuCompleter, *menuCompleter) {
	constantNode := &menuCompleter{suggestion: &prompt.Suggest{
		Text:        "constant",
		Description: "make a call to a constant method",
	}}
	transactNode := &menuCompleter{suggestion: &prompt.Suggest{
		Text:        "transact",
		Description: "make a transaction to a method",
	}}
	names := make([]string, 0, len(methods))
	for i := range methods {
		names = append(names, i)
	}
	sort.Strings(names)
	for _, name := range names {
		m := methods[name]
		n := &menuCompleter{
			suggestion: &prompt.Suggest{
				Text:        name,
				Description: m.String(),
			},
		}
		if m.IsConstant() {
			n.parent = constantNode
			constantNode.sub = append(constantNode.sub, n)
		} else {
			n.parent = transactNode
			transactNode.sub = append(transactNode.sub, n)
		}
	}
	constantNode.sub = append(constantNode.sub, tailCommands...)
	transactNode.sub = append(transactNode.sub, tailCommands...)
	return constantNode, transactNode
}

func eventsMenu(events map[string]abi.Event) *menuCompleter {
	eventsNode := &menuCompleter{suggestion: &prompt.Suggest{
		Text:        "events",
		Description: "filter/watch events",
	}}
	eventsNames := make([]string, 0, len(events))
	for i := range events {
		eventsNames = append(eventsNames, i)
	}
	sort.Strings(eventsNames)
	for _, name := range eventsNames {
		n := &menuCompleter{
			suggestion: &prompt.Suggest{
				Text:        name,
				Description: events[name].String(),
			},
			parent: eventsNode,
		}
		eventsNode.sub = append(eventsNode.sub, n)
	}
	eventsNode.sub = append(eventsNode.sub, tailCommands...)
	return eventsNode
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
