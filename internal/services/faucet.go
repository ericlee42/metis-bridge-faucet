package services

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"time"

	"github.com/ericlee42/metis-bridge-faucet/internal/goabi/metisl2"
	"github.com/ericlee42/metis-bridge-faucet/internal/repository"
	"github.com/ericlee42/metis-bridge-faucet/internal/utils"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/sirupsen/logrus"
)

type Faucet struct {
	Web3Client *ethclient.Client
	Repositroy repository.Metis
	Uniswap    utils.Uniswaper

	Prvkey       *ecdsa.PrivateKey
	Account      common.Address
	Eip155Signer types.Signer
	nonce        uint64

	DripHeight uint64
	DripAmount *big.Int
	MinUSD     float64
}

func (s *Faucet) Initial(basectx context.Context) (err error) {
	if s.DripAmount == nil || s.DripAmount.Sign() < 1 {
		s.DripAmount = big.NewInt(1e16)
	}

	newctx, cancel := context.WithTimeout(basectx, time.Second)
	defer cancel()
	s.nonce, err = s.Web3Client.NonceAt(newctx, s.Account, nil)
	return
}

func (s *Faucet) SendDrips(basectx context.Context) {
	newctx, cancel := context.WithTimeout(basectx, time.Minute*5)
	defer cancel()
	if err := s.tryToSendDrip(newctx); err != nil {
		logrus.Errorf("failed to transfer drips: %s", err)
	}
}

func (s *Faucet) tryToSendDrip(ctx context.Context) error {
	recset := make(map[string]bool)
	for item := range s.Repositroy.GetDepositTxStream(ctx, repository.DepositStatusUnprocessed) {
		if item.Error != nil {
			return item.Error
		}

		var shouldTransfer = true
		err := s.shouldTransfer(ctx, item.Data, recset)
		if err != nil {
			if v, ok := err.(ErrorNoNeedToTransfer); ok {
				logrus.Infof("Don't need to give a drip to %s: %s", item.Data.To, v.msg)
				shouldTransfer = false
			} else {
				return err
			}
		}

		var drip *repository.Drip
		var tx *types.Transaction
		if shouldTransfer {
			tx, err = s.makeDripTx(ctx, item.Data.To)
			if err != nil {
				return err
			}
			rawtx, err := tx.MarshalBinary()
			if err != nil {
				return err
			}
			drip = &repository.Drip{
				Pid:    item.Data.Id,
				Txid:   tx.Hash().String(),
				From:   s.Account.Hex(),
				To:     item.Data.To,
				Amount: utils.ToEther(s.DripAmount),
				Rawtx:  rawtx,
			}
			recset[item.Data.To] = true
		}
		if err := s.Repositroy.NewDrip(ctx, item.Data, drip); err != nil {
			return err
		}
		if tx != nil && drip != nil {
			s.nonce += 1
			logrus.Infof("Drip: send %f Metis to %s [ Tx %s ]", drip.Amount, drip.To, drip.Txid)
			if err := s.Web3Client.SendTransaction(ctx, tx); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Faucet) shouldTransfer(basectx context.Context, item *repository.Deposit, recset map[string]bool) (err error) {
	if recset[item.To] {
		return ErrorNoNeedToTransfer{msg: "has transfered in current loop"}
	}

	if item.Height < s.DripHeight {
		return ErrorNoNeedToTransfer{msg: "height < dripHeight"}
	}

	newctx, cancel := context.WithTimeout(basectx, time.Second*10)
	defer cancel()

	var rate float64 = 1
	if !utils.IsStableL2Token(item.L2Token) {
		rate, err = s.Uniswap.GetTokenPrice(newctx, item.L1Token)
		if err != nil {
			return err
		}
	}

	l2token, err := metisl2.NewL2StandardERC20(common.HexToAddress(item.L2Token), s.Web3Client)
	if err != nil {
		return err
	}

	decimal, err := l2token.Decimals(&bind.CallOpts{Context: newctx})
	if err != nil {
		return err
	}
	if amount := item.Amount.Readable(int64(decimal)); rate*amount < s.MinUSD {
		return ErrorNoNeedToTransfer{msg: fmt.Sprintf("Amount %f < Min %f", amount, s.MinUSD)}
	}

	first, err := s.Repositroy.HasGotDrip(newctx, item.To)
	if err != nil {
		return err
	}
	if !first {
		return ErrorNoNeedToTransfer{msg: "transfered before"}
	}

	// should not have Metis balance
	balance, err := s.Web3Client.BalanceAt(newctx, common.HexToAddress(item.To), nil)
	if err != nil {
		return err
	}
	if balance.Sign() > 0 {
		return ErrorNoNeedToTransfer{msg: "metis balance > 0"}
	}

	// should be an EOA
	code, err := s.Web3Client.CodeAt(newctx, common.HexToAddress(item.To), nil)
	if err != nil {
		return err
	}
	if len(code) > 0 {
		return ErrorNoNeedToTransfer{msg: "not EOA"}
	}

	// should be a fresh address
	nonce, err := s.Web3Client.NonceAt(newctx, common.HexToAddress(item.To), nil)
	if err != nil {
		return err
	}
	if nonce > 0 {
		return ErrorNoNeedToTransfer{msg: "nonce > 0"}
	}
	return nil
}

func (s *Faucet) makeDripTx(basectx context.Context, toAddr string) (*types.Transaction, error) {
	newctx, cancel := context.WithTimeout(basectx, time.Second*5)
	defer cancel()

	gasPrice, err := s.Web3Client.SuggestGasPrice(newctx)
	if err != nil {
		return nil, err
	}

	receiver := common.HexToAddress(toAddr)
	gas, err := s.Web3Client.EstimateGas(newctx,
		ethereum.CallMsg{From: s.Account, To: &receiver, Value: s.DripAmount})
	if err != nil {
		return nil, err
	}

	rawtx := &types.LegacyTx{
		Nonce:    s.nonce,
		GasPrice: gasPrice,
		Gas:      gas,
		To:       &receiver,
		Value:    s.DripAmount,
	}
	return types.SignNewTx(s.Prvkey, s.Eip155Signer, rawtx)
}

func (s *Faucet) CheckDrips(basectx context.Context) {
	newctx, cancel := context.WithTimeout(basectx, time.Minute*5)
	defer cancel()
	if err := s.tryToCheckDrip(newctx); err != nil {
		logrus.Errorf("failed to check drips: %s", err)
	}
}

func (s *Faucet) tryToCheckDrip(ctx context.Context) error {
	for item := range s.Repositroy.GetPendingDripsStream(ctx) {
		if item.Error != nil {
			return item.Error
		}
		done, err := s.getTxStatus(ctx, item.Data)
		if err != nil {
			return err
		}
		if !done {
			var tx = new(types.Transaction)
			_ = tx.UnmarshalBinary(item.Data.Rawtx)
			_ = s.Web3Client.SendTransaction(ctx, tx)
			continue
		}
		logrus.Infof("Updating deposit %d status [ Tx %s ]", item.Data.Id, item.Data.Txid)
		if err := s.Repositroy.UpdateDripStatus(ctx, item.Data.Id, repository.DepositStatusDone); err != nil {
			return err
		}
	}
	return nil
}

func (s *Faucet) getTxStatus(ctx context.Context, tx *repository.PendingDrip) (bool, error) {
	newctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	_, err := s.Web3Client.TransactionReceipt(newctx, common.HexToHash(tx.Txid))
	if err != nil {
		if err == ethereum.NotFound {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
