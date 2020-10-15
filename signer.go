package main

import "crypto/ecdsa"

type signerKind int

const (
	signerNone signerKind = iota
	signerKey
	signerLedger
)

type signer struct {
	key    *ecdsa.PrivateKey
	ledger interface{}
}

func newKeySigner(key *ecdsa.PrivateKey) signer { return signer{key: key} }
func newLedgerSigner(v interface{}) signer      { return signer{ledger: v} }

func (s signer) kind() signerKind {
	if s.key != nil {
		return signerKey
	}
	if s.ledger != nil {
		return signerLedger
	}
	return signerNone
}

var txSigner = signer{}
