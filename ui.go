package main

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"

	"github.com/c-bata/go-prompt"
	"github.com/ethereum/go-ethereum/accounts/abi"
)

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

func eventsMenu(events map[string]abi.Event) (*menuCompleter, *menuCompleter, *menuCompleter) {
	eventsNode := &menuCompleter{suggestion: &prompt.Suggest{
		Text:        "events",
		Description: "filter/watch events",
	}}
	listNode := &menuCompleter{parent: eventsNode, suggestion: &prompt.Suggest{
		Text:        "list",
		Description: "list event",
	}}
	watchNode := &menuCompleter{parent: eventsNode, suggestion: &prompt.Suggest{
		Text:        "watch",
		Description: "watch event",
	}}
	eventsNode.sub = append([]*menuCompleter{listNode, watchNode}, tailCommands...)
	eventsNames := make([]string, 0, len(events))
	for i := range events {
		eventsNames = append(eventsNames, i)
	}
	sort.Strings(eventsNames)
	for _, name := range eventsNames {
		sug := &prompt.Suggest{Text: name, Description: events[name].String()}
		listNode.sub = append(listNode.sub, &menuCompleter{suggestion: sug, parent: listNode})
		watchNode.sub = append(watchNode.sub, &menuCompleter{suggestion: sug, parent: watchNode})
	}
	listNode.sub = append(listNode.sub, tailCommands...)
	watchNode.sub = append(watchNode.sub, tailCommands...)
	return eventsNode, listNode, watchNode
}

func inputArguments(args abi.Arguments, isFilter bool) ([]interface{}, error) {
	r := make([]interface{}, 0, len(args))
	for _, i := range args {
		for {
			val := inputText(i.Name + " (" + i.Type.String() + "): ")
			if val == "" {
				fmt.Printf("....\n")
				continue
			}
			v, err := unmarshalValue(val, i.Type.GetType())
			if err != nil {
				return nil, err
			}
			r = append(r, v)
			break
		}
	}
	return r, nil
}

func unmarshalValue(val string, t reflect.Type) (interface{}, error) {
	if t.Elem().Kind() == reflect.Ptr {
		t = t.Elem()
	}
	v := reflect.New(t).Interface()
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	if len(b) > 1 && b[0] == '"' {
		val = "\"" + val + "\""
	}
	if err = json.Unmarshal([]byte(val), v); err != nil {
		return nil, err
	}
	return v, nil
}

func inputFilters(inputs abi.Arguments) ([][]interface{}, error) {
	// get filters
	r := make([][]interface{}, 0, 4)
	for _, i := range inputs {
		if !i.Indexed {
			continue
		}
		pr := fmt.Sprintf("field %s (%s) is indexed. filter? (%%s): ", i.Name, i.Type.String())
		if filterField, ok := inputYesNo(pr, false); !ok {
			return nil, errAborted
		} else if filterField {
			for {
				v := inputText("field value (none): ")
				if v == "" {
					r = append(r, nil)
					break
				}
				fv, err := unmarshalValue(v, i.Type.GetType())
				if err != nil {
					fmt.Printf("can't parse value: %s\n", err)
					continue
				}
				r = append(r, []interface{}{fv})
				break
			}
		}
	}
	return r, nil
}
