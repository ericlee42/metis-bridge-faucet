package utils

import (
	"crypto/ecdsa"
	"io/ioutil"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

const (
	BridgeAddress    = "0x4200000000000000000000000000000000000010"
	EtherL1Address   = "0x0000000000000000000000000000000000000000"
	MetisL2Address   = "0xdeaddeaddeaddeaddeaddeaddeaddeaddead0000"
	MetisUSDTAddress = "0xbb06dca3ae6887fabf931640f67cab3e3a16f4dc"
	MetisUSDCAddress = "0xea32a96608495e54156ae48931a7c20f0dcc1a21"
)

func IsStableL2Token(u string) bool {
	return strings.EqualFold(u, MetisUSDCAddress) || strings.EqualFold(u, MetisUSDTAddress)
}

func ReadPrvkey(keyPath string) (*ecdsa.PrivateKey, common.Address, error) {
	data, err := ioutil.ReadFile(keyPath)
	if err != nil {
		return nil, common.Address{}, err
	}

	rawkey := strings.TrimSpace(string(data))
	prvkey, err := crypto.HexToECDSA(strings.TrimPrefix(rawkey, "0x"))
	if err != nil {
		return nil, common.Address{}, err
	}
	address := crypto.PubkeyToAddress(*prvkey.Public().(*ecdsa.PublicKey))
	return prvkey, address, nil
}
