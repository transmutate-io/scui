package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"reflect"
	"strings"
	"syscall"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

var menuCommands = map[string]func(){
	"signer/key": cmdConfigSignerKey,
	// "signer/ledger": cmdConfigSignerLedger,
}

func cmdConfigSignerKey() {
	key, err := inputKeyFile()
	if err != nil {
		fmt.Printf("can't read key file: %s\n", err)
		return
	}
	txSigner = newKeySigner(key)
}

func cmdConfigSignerLedger() {}

var (
	errNotConstant = errors.New("method is not constant")
	errConstant    = errors.New("method is constant")
	errAborted     = errors.New("aborted")
)

func executeConstantMethod(cl *ethclient.Client, addr *common.Address, abi *abi.ABI, name string) ([]interface{}, error) {
	fmt.Printf("constant call arguments:\n")
	args, err := inputArguments(abi.Methods[name].Inputs, false)
	if err != nil {
		return nil, err
	}
	bc := bind.NewBoundContract(*addr, *abi, cl, cl, cl)
	method := abi.Methods[name]
	if !method.IsConstant() {
		return nil, errNotConstant
	}
	// call method
	res := newCallResult(method.Outputs)
	if err := bc.Call(nil, res.res, name, args...); err != nil {
		return nil, err
	}
	return res.results(), nil
}

type callResult struct {
	mo  abi.Arguments
	res interface{}
}

func newCallResult(mo abi.Arguments) *callResult {
	switch len(mo) {
	case 0:
		return nil
	case 1:
		return &callResult{mo: mo, res: reflect.New(mo[0].Type.GetType()).Interface()}
	}
	r := make([]interface{}, 0, len(mo))
	for _, i := range mo {
		r = append(r, reflect.New(i.Type.GetType()).Interface())
	}
	return &callResult{mo: mo, res: &r}
}

func indirectInterface(v interface{}) interface{} {
	return reflect.Indirect(reflect.ValueOf(v)).Interface()
}

func (cr *callResult) results() []interface{} {
	if s, ok := cr.res.(*[]interface{}); ok {
		r := make([]interface{}, 0, 4)
		for _, i := range *s {
			r = append(r, indirectInterface(i))
		}
		return r
	}
	return []interface{}{indirectInterface(cr.res)}
}

func executeTransactMethod(cl *ethclient.Client, addr *common.Address, abi *abi.ABI, name string) (*types.Transaction, error) {
	method := abi.Methods[name]
	if method.IsConstant() {
		return nil, errConstant
	}
	fmt.Printf("transaction arguments:\n")
	args, err := inputArguments(abi.Methods[name].Inputs, false)
	if err != nil {
		return nil, err
	}
	bc := bind.NewBoundContract(*addr, *abi, cl, cl, cl)
	opts := bind.NewKeyedTransactor(txSigner.key)
	if abi.Methods[name].IsPayable() {
		send, ok := inputYesNo("method is payable. send amount with transaction? (%s): ", false)
		if !ok {
			return nil, errAborted
		}
		if send {
			amount := inputBigInt("amount: ")
			opts.Value = amount
		}
	}
	if estimateGasPrice, ok := inputYesNo("estimate gas price? (%s): ", true); !ok {
		return nil, errAborted
	} else if !estimateGasPrice {
		sugg, err := cl.SuggestGasPrice(context.Background())
		if err != nil {
			return nil, err
		}
		opts.GasPrice = inputBigIntWithDefault("gas price (%s): ", sugg)
	}
	if estimateGasLimit, ok := inputYesNo("estimate gas limit? (%s): ", true); !ok {
		return nil, errAborted
	} else if !estimateGasLimit {
		if gl, ok := inputIntWithDefault("gas limit (%d): ", 0); ok {
			opts.GasLimit = uint64(gl)
		}
	}
	return bc.Transact(opts, name, args...)
}

func listEvents(cl *ethclient.Client, addr *common.Address, abi *abi.ABI, name string) {
	filters, err := inputFilters(abi.Events[name].Inputs)
	if err != nil {
		fmt.Printf("error parsing filter fields: %s\n", err)
		return
	}
	opts := &bind.FilterOpts{}
	startBlock, ok := inputIntWithDefault("start block (%d): ", 0)
	if !ok {
		fmt.Printf("aborted\n")
		return
	}
	opts.Start = uint64(startBlock)
	if lastBlock, ok := inputIntWithDefault("end block (last, %d): ", -1); !ok {
		fmt.Printf("aborted\n")
		return
	} else if lastBlock >= 0 {
		lb := uint64(lastBlock)
		opts.End = &lb
	}
	bc := bind.NewBoundContract(*addr, *abi, cl, cl, cl)
	logs, sub, err := bc.FilterLogs(opts, name, filters...)
	if err != nil {
		fmt.Printf("error listing logs: %s\n", err)
		return
	}
	defer close(logs)
	defer sub.Unsubscribe()
	for {
		if err := <-sub.Err(); err != nil {
			fmt.Printf("error listing logs: %s\n", err)
			return
		}
		select {
		case l := <-logs:
			eventData := make(map[string]interface{}, 8)
			if err := bc.UnpackLogIntoMap(eventData, name, l); err != nil {
				fmt.Printf("error listing logs: %s\n", err)
				return
			}
			fmt.Print(formatEvent(abi.Events[name].Inputs, eventData, l.BlockNumber))
		default:
			return
		}
	}

}

func formatEvent(inputs abi.Arguments, eventData map[string]interface{}, blockNumber uint64) string {
	var values []string
	for _, i := range inputs {
		b, err := json.Marshal(eventData[i.Name])
		if err != nil {
			fmt.Printf("can't marshal value: %s\n", err)
			continue
		}
		values = append(values, fmt.Sprintf("%s=%s", i.Name, string(b)))
	}
	return fmt.Sprintf("  block %d: %s\n", blockNumber, strings.Join(values, " "))
}

func watchEvents(cl *ethclient.Client, addr *common.Address, abi *abi.ABI, name string) {
	filters, err := inputFilters(abi.Events[name].Inputs)
	if err != nil {
		fmt.Printf("error parsing filter fields: %s\n", err)
		return
	}
	bc := bind.NewBoundContract(*addr, *abi, cl, cl, cl)
	logs, sub, err := bc.WatchLogs(nil, name, filters...)
	if err != nil {
		fmt.Printf("error watching logs: %s\n", err)
		return
	}
	defer close(logs)
	defer sub.Unsubscribe()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	for {
		select {
		case <-sig:
			return
		case err := <-sub.Err():
			if err != nil {
				fmt.Printf("error watching logs: %s\n", err)
				return
			}
		case l := <-logs:
			eventData := make(map[string]interface{}, 8)
			if err := bc.UnpackLogIntoMap(eventData, name, l); err != nil {
				fmt.Printf("error watching logs: %s\n", err)
				return
			}
			fmt.Print(formatEvent(abi.Events[name].Inputs, eventData, l.BlockNumber))
		}
	}
}
