package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/alexvanin/monza/chain"
	"github.com/nspcc-dev/neo-go/pkg/neorpc/result"
	"github.com/urfave/cli/v2"
)

func stutter(c *cli.Context) (err error) {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)

	// parse blockchain info
	cacheDir := c.String(cacheFlagKey)
	if len(cacheDir) == 0 {
		cacheDir, err = defaultConfigDir()
		if err != nil {
			return err
		}
	}

	blockchain, err := chain.Open(ctx, cacheDir, c.String(endpointFlagKey))
	if err != nil {
		return fmt.Errorf("cannot initialize remote blockchain client: %w", err)
	}
	defer func() {
		blockchain.Close()
		cancel()
	}()

	// parse block indices
	from, to, err := parseInterval(c.String(fromFlagKey), c.String(toFlagKey), blockchain.Client)
	if err != nil {
		return err
	}

	threshold := c.Duration(stutterThresholdFlagKey)

	// need at least two blocks
	if to-from < 2 {
		return errors.New("range must contain at least two blocks")
	}

	// fetch blocks
	err = cacheBlocks(ctx, &params{
		from:       from,
		to:         to,
		blockchain: blockchain,
		workers:    int(c.Uint64(workersFlagKey)),
		disableBar: c.Bool(disableProgressBarFlagKey),
	})
	if err != nil {
		return err
	}

	// process blocks one by one
	var (
		prev, curr       *result.Block
		prevTS, currTS   time.Time
		lastStutterBlock uint32
	)

	for i := from; i < to; i++ {
		b, err := blockchain.Block(i)
		if err != nil {
			return fmt.Errorf("cannot fetch block %d: %w", i, err)
		}

		prev, prevTS = curr, currTS
		curr = b
		currTS = time.Unix(int64(b.Timestamp/1e3), 0)
		if prev == nil { // first block case
			continue
		}

		blockDelta := currTS.Sub(prevTS)
		if blockDelta <= threshold {
			continue
		}

		skippedBlocks := prev.Index - lastStutterBlock
		if lastStutterBlock > 0 && skippedBlocks > 1 {
			fmt.Printf("-- skipped %d blocks --\n", skippedBlocks-1)
		}

		PrintBlock(prev, "")
		PrintBlock(curr, fmt.Sprintf("<- stutter for %s", blockDelta))
		lastStutterBlock = curr.Index
	}

	return nil
}
