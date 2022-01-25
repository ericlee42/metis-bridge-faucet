package utils

import (
	"context"
	"testing"
)

func TestUniswap_GetTokenPrice(t *testing.T) {
	type args struct {
		tokenAddress string
	}
	tests := []struct {
		name string
		args args
	}{
		{"Metis", args{"0x9e32b13ce7f2e80a01932b42553652e053d6ed8e"}},
		{"Link", args{"0x514910771af9ca656af840dff83e8264ecf986ca"}},
		{"Ether", args{"0x0000000000000000000000000000000000000000"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewUniswap()
			got, err := c.GetTokenPrice(context.Background(), tt.args.tokenAddress)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("price %f", got)
		})
	}
}
