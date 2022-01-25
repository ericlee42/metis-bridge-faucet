package services

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/ericlee42/metis-bridge-faucet/internal/goabi/metisl2"
	"github.com/ericlee42/metis-bridge-faucet/internal/repository"
	"github.com/ericlee42/metis-bridge-faucet/internal/utils"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/sirupsen/logrus"
)

type DataSync struct {
	Web3Client *ethclient.Client
	Bridge     *metisl2.L2StandardBridge
	Repositroy repository.Metis
	RangeSync  uint64
	DripHeight uint64

	height uint64
}

func (s *DataSync) Prefight(basectx context.Context) (err error) {
	newctx, cancel := context.WithTimeout(basectx, time.Second*5)
	defer cancel()
	s.height, err = s.Repositroy.InitHeight(newctx)
	return
}

func (s *DataSync) Run(basectx context.Context) {
	if err := s.tryToSync(basectx); err != nil {
		if errors.Is(err, context.Canceled) {
			return
		}
		logrus.Errorf("sync fail: %s", err)
	}
}

func (s *DataSync) tryToSync(basectx context.Context) error {
	latestBlock, err := func() (uint64, error) {
		newctx, cancle := context.WithTimeout(basectx, time.Second*10)
		defer cancle()
		return s.Web3Client.BlockNumber(newctx)
	}()
	if err != nil {
		return err
	}

	for targetHeight := latestBlock; s.height < targetHeight; {
		startHeight, endHeight := s.height, s.height+s.RangeSync
		if endHeight > targetHeight {
			endHeight = targetHeight
		}
		if err := s.syncWithRange(basectx, startHeight, endHeight); err != nil {
			return err
		}
		s.height = endHeight + 1
	}
	return nil
}

func (s *DataSync) syncWithRange(basectx context.Context, startHeight, endHeight uint64) error {
	logrus.Infof("Syncing from %d to %d", startHeight, endHeight)

	newctx, cancel := context.WithTimeout(basectx, time.Minute)
	defer cancel()

	formatEvent := func(event *metisl2.L2StandardBridgeDepositFinalized) *repository.Deposit {
		var status = repository.DepositStatusUnprocessed

		var l2token = strings.ToLower(event.L2Token.Hex())
		if s.DripHeight > event.Raw.BlockNumber || l2token == utils.MetisL2Address {
			status = repository.DepositStatusIgnore
		}

		return &repository.Deposit{
			Height:  event.Raw.BlockNumber,
			Txid:    event.Raw.TxHash.Hex(),
			L1Token: strings.ToLower(event.L1Token.Hex()),
			L2Token: l2token,
			From:    strings.ToLower(event.From.Hex()),
			To:      strings.ToLower(event.To.Hex()),
			Amount:  utils.ToEther(event.Amount),
			Status:  status,
		}
	}

	header, err := s.Web3Client.HeaderByNumber(newctx, big.NewInt(int64(endHeight)))
	if err != nil {
		return fmt.Errorf("syncWithHeight: get tail header: %w", err)
	}
	var tail = &repository.Height{Number: endHeight, Blockhash: header.Hash().String()}

	iter, err := s.Bridge.FilterDepositFinalized(&bind.FilterOpts{Context: newctx, Start: startHeight, End: &endHeight}, nil, nil, nil)
	if err != nil {
		return fmt.Errorf("syncWithHeight: filter deposit event: %w", err)
	}
	defer iter.Close()

	var deposits []*repository.Deposit
	for iter.Next() {
		deposits = append(deposits, formatEvent(iter.Event))
	}

	if err := iter.Error(); err != nil {
		return fmt.Errorf("syncWithHeight: filter deposit event: %w", err)
	}

	if err := s.Repositroy.SaveSyncedData(newctx, deposits, tail); err != nil {
		return fmt.Errorf("syncWithHeight: %w", err)
	}

	logrus.Infof("Done: NewDeposits %d BlockTime %s", len(deposits), time.Unix(int64(header.Time), 0))
	return nil
}
