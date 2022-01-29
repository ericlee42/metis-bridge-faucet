package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ericlee42/metis-bridge-faucet/internal/goabi/metisl2"
	"github.com/ericlee42/metis-bridge-faucet/internal/repository"
	"github.com/ericlee42/metis-bridge-faucet/internal/services"
	"github.com/ericlee42/metis-bridge-faucet/internal/utils"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

func main() {
	var (
		MinUSD        float64
		DripAmount    float64
		Web3Endpoint  string
		MysqlEndpoint string
		RangeSync     uint64
		DripHeight    uint64
		KeyPath       string
		OpenFaucet    bool
	)

	flag.Float64Var(&MinUSD, "minusd", 500, "min usd value")
	flag.Float64Var(&DripAmount, "drip", 0.01, "metis amount to transfer")
	flag.StringVar(&Web3Endpoint, "rpc", "wss://andromeda-ws.metis.io", "rpc endpoint")
	flag.StringVar(&MysqlEndpoint, "mysql", "root:Pa$$w0rd@tcp(127.0.0.1:3306)/metis?parseTime=true", "mysql endpoint")
	flag.Uint64Var(&RangeSync, "range", 20, "range sync at once")
	flag.Uint64Var(&DripHeight, "height", 100, "height to transfer a drip")
	flag.StringVar(&KeyPath, "key", "key.txt", "private key path")
	flag.BoolVar(&OpenFaucet, "faucet", false, "faucet")
	flag.Parse()

	if RangeSync < 20 {
		RangeSync = 20
	}
	if DripAmount <= 0 {
		DripAmount = 0.01
	}

	// connect to rpc
	rpc, err := ethclient.Dial(Web3Endpoint)
	if err != nil {
		logrus.Fatalf("unable to connect to rpc: %s", err)
	}
	defer rpc.Close()

	chainId, err := rpc.ChainID(context.Background())
	if err != nil {
		logrus.Fatalf("unable to get chain id: %s", err)
	}

	if id := chainId.Uint64(); id != utils.AndromedaChainId && id != utils.StardustChainId {
		logrus.Fatalf("wrong network: %d", id)
	}

	bridge, err := metisl2.NewL2StandardBridge(common.HexToAddress(utils.BridgeAddress), rpc)
	if err != nil {
		logrus.Fatalf("unable to create bridge instance: %s", err)
	}

	// connect to mysql
	mysql, err := repository.Connect(MysqlEndpoint)
	if err != nil {
		logrus.Fatalf("unable to connect to mysql: %s", err)
	}
	defer mysql.Close()

	basectx, cancel := context.WithCancel(context.Background())
	go func() {
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
		for range stop {
			cancel()
		}
	}()

	eg, egctx := errgroup.WithContext(basectx)

	eg.Go(func() error {
		syncer := &services.DataSync{
			Web3Client: rpc,
			Repositroy: repository.NewMetis(mysql),
			Bridge:     bridge,
			RangeSync:  RangeSync,
			DripHeight: DripHeight,
		}
		if err := syncer.Prefight(egctx); err != nil {
			return err
		}
		logrus.Info("fetching new events")
		timer := time.NewTimer(0)
		for {
			select {
			case <-egctx.Done():
				return nil
			case <-timer.C:
				syncer.Run(basectx)
				timer.Reset(time.Minute / 2)
			}
		}
	})

	eg.Go(func() error {
		if !OpenFaucet {
			return nil
		}

		prvkey, wallet, err := utils.ReadPrvkey(KeyPath)
		if err != nil {
			return fmt.Errorf("unable to read pricate key: %s", err)
		}
		logrus.Infof("Current wallet address is %s", wallet)

		faucet := &services.Faucet{
			Web3Client:   rpc,
			Repositroy:   repository.NewMetis(mysql),
			Uniswap:      utils.NewUniswap(),
			Prvkey:       prvkey,
			Account:      wallet,
			Eip155Signer: types.NewEIP155Signer(chainId),
			DripHeight:   DripHeight,
			DripAmount:   utils.ToWei(DripAmount),
			MinUSD:       MinUSD,
		}
		if err := faucet.Initial(egctx); err != nil {
			return err
		}

		timer := time.NewTimer(0)
		for {
			select {
			case <-egctx.Done():
				return nil
			case <-timer.C:
				faucet.SendDrips(egctx)
				select {
				case <-egctx.Done():
					return nil
				case <-time.After(time.Second * 5):
					faucet.CheckDrips(egctx)
				}
				timer.Reset(time.Minute)
			}
		}
	})

	if err := eg.Wait(); err != nil {
		logrus.Fatal(err)
	}
}
