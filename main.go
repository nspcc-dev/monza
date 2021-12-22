package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path"
	"sync"
	"time"

	"github.com/nspcc-dev/neo-go/pkg/util"
	"github.com/schollz/progressbar/v3"
	"github.com/urfave/cli/v2"

	"github.com/alexvanin/monza/chain"
)

func main() {
	app := &cli.App{
		Name:  "monza",
		Usage: "monitor notification events in N3 compatible chains",
		Commands: []*cli.Command{
			{
				Name:      "run",
				Usage:     "look up over subset of blocks to find notifications",
				UsageText: "monza run -r [endpoint] --from 101000 --to p1000 -n \"Transfer:gas\" -n \"newEpoch:*\"",
				Action:    monza,
				Flags: []cli.Flag{
					endpointFlag,
					fromFlag,
					toFlag,
					notificationFlag,
					cacheFlag,
					workersFlag,
					disableProgressBarFlag,
				},
			},
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}

}

func monza(c *cli.Context) (err error) {
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

	// parse notifications
	notifications, err := parseNotifications(c.StringSlice(notificationFlagKey), blockchain.Client)
	if err != nil {
		return err
	}

	// start monza
	return run(ctx, &params{
		from:          from,
		to:            to,
		blockchain:    blockchain,
		notifications: notifications,
		workers:       int(c.Uint64(workersFlagKey)),
		disableBar:    c.Bool(disableProgressBarFlagKey),
	})
}

type params struct {
	from, to      uint32
	blockchain    *chain.Chain
	notifications map[string]*util.Uint160
	workers       int
	disableBar    bool
}

func run(ctx context.Context, p *params) error {
	err := cacheBlocks(ctx, p)
	if err != nil {
		return err
	}

	for i := p.from; i < p.to; i++ {
		b, err := p.blockchain.Block(i)
		if err != nil {
			return fmt.Errorf("cannot fetch block %d: %w", i, err)
		}

		notifications, err := p.blockchain.AllNotifications(b)
		if err != nil {
			return fmt.Errorf("cannot fetch notifications from block %d: %w", i, err)
		}

		for _, ev := range notifications {
			contract, ok := p.notifications[ev.Name]
			if !ok {
				continue
			}

			if contract != nil && !contract.Equals(ev.ScriptHash) {
				continue
			}

			switch ev.Name {
			case "Transfer":
				PrintTransfer(b, ev)
			case "NewEpoch":
				PrintNewEpoch(b, ev)
			default:
				PrintEvent(b, ev, "")
			}
		}
	}

	return nil
}

func cacheBlocks(ctx context.Context, p *params) error {
	if p.workers <= 0 {
		return fmt.Errorf("invalid amount of workers %d", p.workers)
	}

	var bar *progressbar.ProgressBar
	if !p.disableBar {
		bar = progressbar.NewOptions(int(p.to-p.from),
			progressbar.OptionSetDescription("syncing"),
			progressbar.OptionSetWriter(os.Stderr),
			progressbar.OptionSetWidth(10),
			progressbar.OptionThrottle(65*time.Millisecond),
			progressbar.OptionShowCount(),
			progressbar.OptionShowIts(),
			progressbar.OptionSetItsString("blocks"),
			progressbar.OptionOnCompletion(func() {
				fmt.Fprint(os.Stderr, "\n")
			}),
			progressbar.OptionSpinnerType(14),
			progressbar.OptionSetWidth(50),
			progressbar.OptionSetTheme(progressbar.Theme{
				Saucer:        "#",
				SaucerHead:    "#",
				SaucerPadding: " ",
				BarStart:      "[",
				BarEnd:        "]",
			}),
		)
	}

	jobCh := make(chan uint32)
	errCh := make(chan error)
	wgCh := make(chan struct{})

	wg := new(sync.WaitGroup)

	for i := 0; i < p.workers; i++ {
		go func(ctx context.Context, ch <-chan uint32, out chan<- error) {
			for {
				select {
				case <-ctx.Done():
					return
				case block, ok := <-ch:
					wg.Add(1)
					if !ok {
						return
					}
					b, err := p.blockchain.Block(block)
					if err != nil {
						out <- err
						return
					}
					_, err = p.blockchain.AllNotifications(b)
					if err != nil {
						out <- err
						return
					}
					if bar != nil {
						bar.Add(1)
					}
					wg.Done()
				}
			}
		}(ctx, jobCh, errCh)
	}

	for i := p.from; i < p.to; i++ {
		select {
		case <-ctx.Done():
			return errors.New("interrupted")
		case err := <-errCh:
			return err
		case jobCh <- i:
		}
	}

	go func() {
		wg.Wait()
		close(wgCh)
	}()

	select {
	case <-ctx.Done():
		return errors.New("interrupted")
	case err := <-errCh:
		return err
	case <-wgCh:
		return nil
	}
}

func defaultConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("cannot determine home dir for default config path: %s", err)
	}

	p := path.Join(home, ".config")
	p = path.Join(p, "monza")

	return p, os.MkdirAll(p, os.ModePerm)
}
