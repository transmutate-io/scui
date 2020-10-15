package main

import (
	"crypto/ecdsa"
	"errors"
	"fmt"
	"io/ioutil"
	"math/big"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/c-bata/go-prompt"
	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/ssh/terminal"
)

var (
	upCommand = &menuCompleter{
		suggestion: &prompt.Suggest{Text: "..",
			Description: "move to the parent menu",
		}}
	helpCommand = &menuCompleter{suggestion: &prompt.Suggest{
		Text:        "help",
		Description: "show the help for the current menu",
	}}
	exitCommand = &menuCompleter{suggestion: &prompt.Suggest{
		Text:        "exit",
		Description: "exit the interactive console",
	}}
	tailCommands = []*menuCompleter{upCommand, helpCommand, exitCommand}
)

type menuCompleter struct {
	suggestion *prompt.Suggest
	sub        []*menuCompleter
	parent     *menuCompleter
}

func newRootNode(entries []*menuCompleter) *menuCompleter {
	r := &menuCompleter{sub: append(make([]*menuCompleter, 0, len(entries)+3), entries...)}
	for _, i := range r.sub {
		i.parent = r
	}
	r.sub = append(r.sub, newSignerMenu(r), helpCommand, exitCommand)
	return r
}

func newMenuCompleter(cmd *cobra.Command, parent *menuCompleter) *menuCompleter {
	name := strings.SplitN(cmd.Use, " ", 2)[0]
	cmds := cmd.Commands()
	r := &menuCompleter{
		suggestion: &prompt.Suggest{
			Text:        name,
			Description: cmd.Short,
		},
		parent: parent,
		sub:    make([]*menuCompleter, 0, len(cmds)+3),
	}
	for _, i := range cmd.Commands() {
		r.sub = append(r.sub, newMenuCompleter(i, r))
	}
	r.sub = append(r.sub, tailCommands...)
	return r
}

const nameSep = "/"

func (cc *menuCompleter) name() string {
	c := cc
	parts := make([]string, 0, 8)
	for {
		if c == nil {
			break
		}
		if c.suggestion != nil {
			parts = append(parts, c.suggestion.Text)
		}
		c = c.parent
	}
	sz := len(parts)
	for i := 0; i < sz/2; i++ {
		parts[i], parts[sz-i-1] = parts[sz-i-1], parts[i]
	}
	return strings.Join(parts, nameSep)
}

func (cc *menuCompleter) prompt(p string) string {
	return cc.name() + p + " "
}

func (cc *menuCompleter) completer(doc prompt.Document) []prompt.Suggest {
	r := make([]prompt.Suggest, 0, len(cc.sub))
	for _, i := range cc.sub {
		r = append(r, *i.suggestion)
	}
	return prompt.FilterHasPrefix(r, doc.GetWordBeforeCursor(), false)
}

func newSignerMenu(parent *menuCompleter) *menuCompleter {
	r := &menuCompleter{parent: parent, suggestion: &prompt.Suggest{
		Text:        "signer",
		Description: "configure signer",
	}}
	sigKey := &menuCompleter{parent: r, suggestion: &prompt.Suggest{
		Text:        "key",
		Description: "sign with a key",
	}}
	sigLedger := &menuCompleter{parent: r, suggestion: &prompt.Suggest{
		Text:        "ledger",
		Description: "sign with ledger",
	}}
	r.sub = append([]*menuCompleter{sigKey, sigLedger}, tailCommands...)
	return r
}

func inputMultiChoice(pr string, def string, choices []prompt.Suggest, helpFunc func(c []prompt.Suggest)) (string, bool) {
	choices = append(choices, *tailCommands[0].suggestion, *tailCommands[1].suggestion)
	for {
		input := prompt.Input(fmt.Sprintf(pr, def), func(doc prompt.Document) []prompt.Suggest {
			return prompt.FilterHasPrefix(choices, doc.GetWordBeforeCursor(), false)
		})
		switch ii := strings.TrimSpace(input); ii {
		case "":
			return def, true
		case "..":
			fmt.Println("aborted")
			return "", false
		case "help":
			helpFunc(choices[:len(choices)-2])
		default:
			for _, i := range choices[:len(choices)-2] {
				if input == i.Text {
					return input, true
				}
			}
			fmt.Printf("invalid choice: %s\n", input)
		}
	}
}

func inputMultiChoiceString(pr string, def string, choices []string, helpFunc func(c []prompt.Suggest)) (string, bool) {
	c := make([]prompt.Suggest, 0, len(choices))
	for _, i := range choices {
		c = append(c, prompt.Suggest{Text: i})
	}
	return inputMultiChoice(pr, def, c, helpFunc)
}

var yesNoMap = map[string]bool{"yes": true, "no": false}

func inputYesNo(pr string, def bool) (bool, bool) {
	choices := make([]prompt.Suggest, 0, 2)
	for _, i := range []string{"no", "yes"} {
		choices = append(choices, prompt.Suggest{Text: i})
	}
	var d string
	if def {
		d = choices[1].Text
	} else {
		d = choices[0].Text
	}
	r, ok := inputMultiChoice(pr, d, choices, func(_ []prompt.Suggest) {
		fmt.Printf("\nchoose yes or no\n")
	})
	if !ok {
		return false, false
	}
	return yesNoMap[r], true
}

var pathSep = string([]rune{filepath.Separator})

func inputPath(pr string, rootPath string, mustExist bool, pathToSuggestionFn func(path string, text string) (prompt.Suggest, bool)) (string, error) {
	for {
		input := prompt.Input(pr, func(doc prompt.Document) []prompt.Suggest {
			r := make([]prompt.Suggest, 0, 0)
			text := doc.TextBeforeCursor()
			var fullPath string
			if filepath.IsAbs(text) {
				fullPath = text
			} else {
				fullPath = filepath.Join(rootPath, text)
			}
			var dirName string
			if text == "" {
				dirName = rootPath
			} else if filepath.IsAbs(text) {
				dirName, _ = filepath.Split(text)
			} else {
				if strings.HasSuffix(text, pathSep) {
					dirName = fullPath
				} else {
					dirName, _ = filepath.Split(fullPath)
				}
			}
			err := listPath(dirName, func(path string) {
				sug, ok := pathToSuggestionFn(path, text)
				if !ok {
					return
				}
				r = append(r, sug)
			})
			if err != nil {
				return nil
			}
			return r
		})
		if strings.TrimSpace(input) == "" {
			return "", nil
		}
		var r string
		if filepath.IsAbs(input) {
			r = input
		} else {
			r = filepath.Join(rootPath, input)
		}
		if mustExist {
			info, err := os.Stat(r)
			if err != nil {
				return "", err
			}
			if info.IsDir() {
				return "", errors.New("not a file")
			}
		}
		return r, nil
	}
}

func listPath(rootPath string, entryFn func(string)) error {
	return filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			e, ok := err.(*os.PathError)
			if !ok {
				return err
			}
			if e.Err != syscall.ENOENT && e.Err != syscall.EACCES {
				return err
			}
			return nil
		}
		if info.IsDir() {
			if filepath.Clean(rootPath) == filepath.Clean(path) {
				return nil
			}
		}
		entryFn(path)
		if info.IsDir() {
			return filepath.SkipDir
		}
		return nil
	})
}

func trimPath(s, p string) string {
	return strings.TrimPrefix(strings.TrimPrefix(s, p), pathSep)
}

func newAbsolutePathFilter(rootPath string) func(string, string) (prompt.Suggest, bool) {
	return func(path string, text string) (prompt.Suggest, bool) {
		if filepath.IsAbs(text) && strings.HasPrefix(path, text) {
			return prompt.Suggest{Text: path}, true
		} else {
			if strings.HasPrefix(filepath.Clean(text), "..") {
				_, pathName := filepath.Split(path)
				textDir, textName := filepath.Split(text)
				if strings.HasPrefix(pathName, textName) {
					return prompt.Suggest{Text: textDir + pathName}, true
				}
			} else {
				if relPath := trimPath(path, rootPath); strings.HasPrefix(relPath, text) {
					return prompt.Suggest{Text: relPath}, true
				}
			}
		}
		return prompt.Suggest{}, false
	}
}

func inputFilename(pr string, rootPath string, mustExit bool) (string, error) {
	return inputPath(pr, rootPath, mustExit, newAbsolutePathFilter(rootPath))
}

func showHelp(node *menuCompleter) {
	maxSz := 0
	for _, i := range node.sub {
		if newSz := len(i.suggestion.Text); newSz > maxSz {
			maxSz = newSz
		}
	}
	maxSz += 4
	for _, i := range node.sub {
		fmt.Printf(
			"%s%s%s\n",
			i.suggestion.Text,
			strings.Repeat(" ", maxSz-len(i.suggestion.Text)),
			i.suggestion.Description,
		)
	}
}

func inputKeyFile() (*ecdsa.PrivateKey, error) {
	p, err := filepath.Abs(".")
	if err != nil {
		return nil, err
	}
	keyFile, err := inputFilename("key file: ", p, true)
	if err != nil {
		return nil, err
	}
	b, err := ioutil.ReadFile(keyFile)
	if err != nil {
		return nil, err
	}
	var k *ecdsa.PrivateKey
	encrypted, ok := inputYesNo("encrypted? (%s): ", false)
	if !ok {
		return nil, errors.New("bad response")
	}
	if encrypted {
		password, err := inputPassword()
		if err != nil {
			return nil, err
		}
		ksk, err := keystore.DecryptKey(b, password)
		if err != nil {
			return nil, err
		}
		k = ksk.PrivateKey
	} else if k, err = crypto.HexToECDSA(string(b)); err != nil {
		return nil, err
	}
	return k, nil
}

func inputPassword() (string, error) {
	fmt.Print("password: ")
	bytePassword, err := terminal.ReadPassword(int(syscall.Stdin))
	if err != nil {
		return "", err
	}
	return string(bytePassword), nil
}

func inputText(pr string) string {
	return prompt.Input(
		fmt.Sprintf("%s", pr),
		func(prompt.Document) []prompt.Suggest { return nil },
	)
}

func inputBigInt(pr string) *big.Int {
	for {
		v := inputText(pr)
		if v == "" {
			continue
		}
		if r, ok := new(big.Int).SetString(v, 10); ok {
			return r
		}
	}
}

func inputBigIntWithDefault(pr string, d *big.Int) *big.Int {
	for {
		v := inputText(fmt.Sprintf(pr, d))
		if v == "" {
			return d
		}
		fmt.Printf("::: %#v\n", v)
		if r, ok := new(big.Int).SetString(v, 10); ok {
			return r
		}
	}
}

func inputIntWithDefault(pr string, def int) (int, bool) {
	for {
		v := inputText(fmt.Sprintf(pr, def))
		if v == "" {
			return def, true
		} else if v == ".." {
			fmt.Println("aborted")
			return 0, false
		}
		i, err := strconv.Atoi(v)
		if err != nil {
			fmt.Printf("%#v is not a number: %s\n", v, err)
			continue
		}
		return i, true
	}
}

// func newConfigMenu(parent *menuCompleter) *menuCompleter {
// 	r := &menuCompleter{
// 		parent: parent,
// 		suggestion: &prompt.Suggest{
// 			Text:        "config",
// 			Description: "configuration",
// 		},
// 	}
// 	r.sub = append(r.sub, tailCommands...)
// 	return r
// 			// 	configClientsCommand := &menuCompleter{
// 			// 		Parent: r,
// 			// 		Suggestion: &prompt.Suggest{
// 			// 			Text:        "clients",
// 			// 			Description: "configure cryptocurrency clients",
// 			// 		},
// 			// 	}
// 			// 	cc := make([]string, 0, len(cryptos.Cryptos))
// 			// 	for c := range cryptos.Cryptos {
// 			// 		cc = append(cc, c)
// 			// 	}
// 			// 	sort.Strings(cc)
// 			// 	for _, c := range cc {
// 			// 		configClientsCommand.Sub = append(configClientsCommand.Sub, newClientConfigMenu(c, configClientsCommand))
// 			// 	}
// 			// 	configClientsCommand.Sub = append(configClientsCommand.Sub, tailCommands...)
// 			// 	r.Sub = append(r.Sub,
// 			// 		configClientsCommand,
// 			// 		&menuCompleter{
// 			// 			Parent: r,
// 			// 			Suggestion: &prompt.Suggest{
// 			// 				Text:        "save",
// 			// 				Description: "save configuration",
// 			// 			},
// 			// 		},
// 			// 		&menuCompleter{
// 			// 			Parent: r,
// 			// 			Suggestion: &prompt.Suggest{
// 			// 				Text:        "load",
// 			// 				Description: "load configuration",
// 			// 			},
// 			// 		},
// 			// 		&menuCompleter{
// 			// 			Parent: r,
// 			// 			Suggestion: &prompt.Suggest{
// 			// 				Text:        "show",
// 			// 				Description: "show the current configuration",
// 			// 			},
// 			// 		},
// 			// 	)
// }

// func newClientConfigMenu(name string, parent *menuCompleter) *menuCompleter {
// 	r := &menuCompleter{
// 		Parent: parent,
// 		Suggestion: &prompt.Suggest{
// 			Text:        name,
// 			Description: fmt.Sprintf("configure %s client", name),
// 		},
// 	}
// 	r.Sub = append(r.Sub,
// 		&menuCompleter{
// 			Parent: r,
// 			Suggestion: &prompt.Suggest{
// 				Text:        "address",
// 				Description: "configure the address",
// 			},
// 		},
// 		&menuCompleter{
// 			Parent: r,
// 			Suggestion: &prompt.Suggest{
// 				Text:        "username",
// 				Description: "configure the username",
// 			},
// 		},
// 		&menuCompleter{
// 			Parent: r,
// 			Suggestion: &prompt.Suggest{
// 				Text:        "password",
// 				Description: "configure the password",
// 			},
// 		},
// 		newClientTLSConfigMenu(r),
// 		&menuCompleter{
// 			Parent: r,
// 			Suggestion: &prompt.Suggest{
// 				Text:        "show",
// 				Description: "show configuration",
// 			},
// 		},
// 	)
// 	r.Sub = append(r.Sub, tailCommands...)
// 	return r
// }

// func inputTextWithDefault(pr, def string) string {
// 	r := inputText(fmt.Sprintf("%s (%s)", pr, def))
// 	if strings.TrimSpace(r) == "" {
// 		return def
// 	}
// 	return r
// }
