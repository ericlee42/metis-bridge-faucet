package utils

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/ericlee42/metis-bridge-faucet/internal/graphql"
)

type Uniswap struct {
	client *graphql.Client
}

func NewUniswap() *Uniswap {
	const uniswapv2_subgraph_api = "https://api.thegraph.com/subgraphs/name/uniswap/uniswap-v2"
	return &Uniswap{graphql.New(uniswapv2_subgraph_api)}
}

type Uniswaper interface {
	GetTokenPrice(ctx context.Context, tokenAddress string) (float64, error)
}

func (c Uniswap) GetTokenPrice(ctx context.Context, tokenAddress string) (float64, error) {
	newctx, cancel := context.WithTimeout(ctx, time.Second*5)
	defer cancel()

	const query = `query TokenPrice($tokenId: ID!){
		bundles{
		  ethPrice
		}
		tokens(where: {id: $tokenId}) {
		  derivedETH
		  symbol
		  decimals
		}
	  }`

	var result struct {
		Bundles []struct {
			EthPrice string `json:"ethPrice"`
		} `json:"bundles"`
		Tokens []struct {
			DerivedETH string `json:"derivedETH"`
			Symbol     string `json:"symbol"`
			Decimal    int    `json:"decimal"`
		} `json:"tokens"`
	}

	vars := map[string]interface{}{"tokenId": tokenAddress}
	if err := c.client.CallContext(newctx, &result, query, vars); err != nil {
		return 0, err
	}
	if len(result.Bundles) == 0 {
		return 0, errors.New("no eth price result")
	}

	ethPrice, ok := new(big.Float).SetString(result.Bundles[0].EthPrice)
	if !ok {
		return 0, fmt.Errorf("failed to parse eth price")
	}

	if tokenAddress == EtherL1Address {
		price, _ := ethPrice.Float64()
		return price, nil
	}

	if len(result.Tokens) == 0 {
		return 0, errors.New("no token price result")
	}

	toknePrice, ok := new(big.Float).SetString(result.Tokens[0].DerivedETH)
	if !ok {
		return 0, fmt.Errorf("failed to parse token price")
	}
	toknePrice.Mul(toknePrice, ethPrice)

	price, _ := toknePrice.Float64()
	return price, nil
}
