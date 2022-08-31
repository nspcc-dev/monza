package chain

import (
	"context"
	"encoding/binary"
	"fmt"
	"path"
	"strconv"

	"github.com/nspcc-dev/neo-go/pkg/core/state"
	"github.com/nspcc-dev/neo-go/pkg/io"
	"github.com/nspcc-dev/neo-go/pkg/neorpc/result"
	"github.com/nspcc-dev/neo-go/pkg/rpcclient"
	"github.com/nspcc-dev/neo-go/pkg/util"
	"go.etcd.io/bbolt"
)

type Chain struct {
	db     *bbolt.DB
	Client *rpcclient.Client
}

var (
	blocksBucket = []byte("blocks")
	logsBucket   = []byte("logs")
)

func Open(ctx context.Context, dir, endpoint string) (*Chain, error) {
	cli, err := rpcclient.New(ctx, endpoint, rpcclient.Options{})
	if err != nil {
		return nil, fmt.Errorf("rpc connection: %w", err)
	}

	err = cli.Init()
	if err != nil {
		return nil, fmt.Errorf("rpc client initialization: %w", err)
	}

	n, err := cli.GetNetwork()
	if err != nil {
		return nil, fmt.Errorf("rpc network state: %w", err)
	}

	dbPath := path.Join(dir, strconv.Itoa(int(n))+".db")

	db, err := bbolt.Open(dbPath, 0600, nil)
	if err != nil {
		return nil, fmt.Errorf("database [%s] init: %w", dbPath, err)
	}

	return &Chain{db, cli}, nil
}

func (d *Chain) Block(i uint32) (res *result.Block, err error) {
	cached, err := d.block(i)
	if err != nil {
		return nil, err
	}

	if cached != nil {
		return cached, nil
	}

	block, err := d.Client.GetBlockByIndexVerbose(i)
	if err != nil {
		return nil, fmt.Errorf("block %d fetch: %w", i, err)
	}

	return block, d.addBlock(block)
}

func (d *Chain) BlockByHash(h util.Uint256) (res *result.Block, err error) {
	rev := h.Reverse()
	block, err := d.Client.GetBlockByHashVerbose(rev)
	if err != nil {
		return nil, fmt.Errorf("block %s fetch: %w", h.StringLE(), err)
	}

	return block, d.addBlock(block)
}

func (d *Chain) block(i uint32) (res *result.Block, err error) {
	err = d.db.View(func(tx *bbolt.Tx) error {
		key := make([]byte, 4)
		binary.LittleEndian.PutUint32(key, i)

		bkt := tx.Bucket(blocksBucket)
		if bkt == nil {
			return nil
		}

		data := bkt.Get(key)
		if len(data) == 0 {
			return nil
		}

		res = new(result.Block)
		r := io.NewBinReaderFromBuf(data)
		res.DecodeBinary(r)

		return r.Err
	})
	if err != nil {
		return nil, fmt.Errorf("cannot read block %d from cache: %w", i, err)
	}

	return res, nil
}

func (d *Chain) addBlock(block *result.Block) error {
	err := d.db.Batch(func(tx *bbolt.Tx) error {
		key := make([]byte, 4)
		binary.LittleEndian.PutUint32(key, block.Index)

		w := io.NewBufBinWriter()
		block.EncodeBinary(w.BinWriter)
		if w.Err != nil {
			return w.Err
		}

		bkt, err := tx.CreateBucketIfNotExists(blocksBucket)
		if err != nil {
			return err
		}

		return bkt.Put(key, w.Bytes())
	})
	if err != nil {
		return fmt.Errorf("cannot add block %d to cache: %w", block.Index, err)
	}

	return nil
}

func (d *Chain) ApplicationLog(txHash util.Uint256) (*result.ApplicationLog, error) {
	cached, err := d.applicationLog(txHash)
	if err != nil {
		return nil, err
	}

	if cached != nil {
		return cached, nil
	}

	appLog, err := d.Client.GetApplicationLog(txHash, nil)
	if err != nil {
		return nil, fmt.Errorf("app log of tx %s fetch: %w", txHash.StringLE(), err)
	}

	return appLog, d.addApplicationLog(txHash, appLog)
}

func (d *Chain) applicationLog(txHash util.Uint256) (res *result.ApplicationLog, err error) {
	err = d.db.View(func(tx *bbolt.Tx) error {
		bkt := tx.Bucket(logsBucket)
		if bkt == nil {
			return nil
		}

		data := bkt.Get(txHash.BytesLE())
		if len(data) == 0 {
			return nil
		}

		res = new(result.ApplicationLog)
		return res.UnmarshalJSON(bkt.Get(txHash.BytesLE()))
	})
	if err != nil {
		return nil, fmt.Errorf("cannot read tx %s from cache: %w", txHash.StringLE(), err)
	}

	return res, nil
}

func (d *Chain) addApplicationLog(txHash util.Uint256, appLog *result.ApplicationLog) error {
	err := d.db.Batch(func(tx *bbolt.Tx) error {
		val, err := appLog.MarshalJSON()
		if err != nil {
			return err
		}

		bkt, err := tx.CreateBucketIfNotExists(logsBucket)
		if err != nil {
			return err
		}

		return bkt.Put(txHash.BytesLE(), val)
	})
	if err != nil {
		return fmt.Errorf("cannot add tx %s to cache: %w", txHash.StringLE(), err)
	}

	return nil
}

func (d *Chain) Notifications(txHash util.Uint256) ([]state.NotificationEvent, error) {
	appLog, err := d.ApplicationLog(txHash)
	if err != nil {
		return nil, err
	}

	res := make([]state.NotificationEvent, 0, 0)
	for _, execution := range appLog.Executions {
		res = append(res, execution.Events...)
	}

	return res, nil
}

func (d *Chain) AllNotifications(b *result.Block) ([]state.NotificationEvent, error) {
	res := make([]state.NotificationEvent, 0, 0)

	appLog, err := d.ApplicationLog(b.Hash())
	if err != nil {
		return nil, err
	}

	for _, execution := range appLog.Executions {
		res = append(res, execution.Events...)
	}

	for _, tx := range b.Transactions {
		appLog, err = d.ApplicationLog(tx.Hash())
		if err != nil {
			return nil, err
		}
		for _, execution := range appLog.Executions {
			res = append(res, execution.Events...)
		}
	}

	return res, nil
}

func (d Chain) Close() {
	_ = d.db.Close()
}
