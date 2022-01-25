package utils

import (
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/params"
)

var ether = new(big.Float).SetInt(new(big.Int).SetUint64(params.Ether))

func ToEther(b *big.Int) float64 {
	t := new(big.Float).SetInt(b)
	t.Quo(t, ether)
	f, _ := t.Float64()
	return f
}

func ToWei(amount float64) *big.Int {
	s := fmt.Sprintf("%f", amount)
	a := strings.Split(s, ".")
	for i := 0; i < 18; i++ {
		if len(a) > 1 && len(a[1]) > i {
			a[0] += string(a[1][i])
		} else {
			a[0] += "0"
		}
	}
	b, ok := new(big.Int).SetString(a[0], 10)
	if !ok {
		return new(big.Int)
	}
	return b
}
