package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/nspcc-dev/neo-go/pkg/core/native/nativenames"
	"github.com/nspcc-dev/neo-go/pkg/rpcclient"
	"github.com/nspcc-dev/neo-go/pkg/util"
	"github.com/urfave/cli/v2"
)

const (
	endpointFlagKey           = "rpc-endpoint"
	fromFlagKey               = "from"
	toFlagKey                 = "to"
	notificationFlagKey       = "notification"
	cacheFlagKey              = "cache"
	workersFlagKey            = "workers"
	disableProgressBarFlagKey = "disable-progress-bar"
	stutterThresholdFlagKey   = "threshold"
)

var (
	endpointFlag = &cli.StringFlag{
		Name:     endpointFlagKey,
		Aliases:  []string{"r"},
		Usage:    "N3 RPC endpoint",
		Required: true,
	}

	fromFlag = &cli.StringFlag{
		Name:     fromFlagKey,
		Usage:    "starting block (can be relative value with minus prefix, e.g. 'm100')",
		Required: true,
		Value:    "",
	}

	toFlag = &cli.StringFlag{
		Name:     toFlagKey,
		Usage:    "ending block (can be relative value with plus prefix, e.g. 'p100' or omitted for latest block in chain)",
		Required: false,
		Value:    "",
	}

	notificationFlag = &cli.StringSliceFlag{
		Name:     notificationFlagKey,
		Aliases:  []string{"n"},
		Usage:    "'notification:contract' pair (specify LE script hash, '*' for any contract or 'gas' and 'neo' strings)",
		Required: true,
		Value:    nil,
	}

	cacheFlag = &cli.StringFlag{
		Name:    cacheFlagKey,
		Aliases: []string{"c"},
		Usage:   "path to the blockchain cache (default: $HOME/.config/monza)",
		Value:   "",
	}

	workersFlag = &cli.Uint64Flag{
		Name:    workersFlagKey,
		Aliases: []string{"w"},
		Usage:   "amount of workers for parallel block fetch",
		Value:   3,
	}

	disableProgressBarFlag = &cli.BoolFlag{
		Name:  disableProgressBarFlagKey,
		Usage: "disable progress bar output",
	}

	stutterThresholdFlag = &cli.DurationFlag{
		Name:    stutterThresholdFlagKey,
		Aliases: []string{"t"},
		Usage:   "duration limit between block timestamps",
		Value:   20 * time.Second,
	}
)

func parseNotifications(notifications []string, cli *rpcclient.Client) (map[string]*util.Uint160, error) {
	res := make(map[string]*util.Uint160, len(notifications))

	for _, n := range notifications {
		pair := strings.Split(n, ":")
		if len(pair) != 2 {
			return nil, fmt.Errorf("invalid notification %s", n)
		}

		name := pair[0]

		switch contractName := strings.ToLower(pair[1]); contractName {
		case "*":
			res[name] = nil
		case "gas":
			u160, err := cli.GetNativeContractHash(nativenames.Gas)
			if err != nil {
				return nil, fmt.Errorf("invalid contract name %s", contractName)
			}
			res[name] = &u160
		case "neo":
			u160, err := cli.GetNativeContractHash(nativenames.Neo)
			if err != nil {
				return nil, fmt.Errorf("invalid contract name %s", contractName)
			}
			res[name] = &u160
		default:
			u160, err := util.Uint160DecodeStringLE(contractName)
			if err != nil {
				return nil, fmt.Errorf("invalid contract name %s", contractName)
			}
			res[name] = &u160
		}
	}

	return res, nil
}

func parseInterval(fromStr, toStr string, cli *rpcclient.Client) (from, to uint32, err error) {
	switch { // parse from value and return result if it is relative
	case len(fromStr) == 0:
		return 0, 0, ErrInvalidInterval(fromStr, toStr)
	case fromStr[0] == 'm':
		v, err := strconv.Atoi(fromStr[1:])
		if err != nil || v <= 0 {
			return 0, 0, ErrInvalidInterval(fromStr, toStr)
		}
		h, err := cli.GetBlockCount()
		if err != nil {
			return 0, 0, fmt.Errorf("latest block index unavailable: %w", err)
		}
		if uint32(v) >= h {
			return 0, 0, fmt.Errorf("latest block is less than from value, from:%s, to:%d", fromStr, h)
		}
		return h - uint32(v), h, nil
	default:
		v, err := strconv.Atoi(fromStr)
		if err != nil || v <= 0 {
			return 0, 0, ErrInvalidInterval(fromStr, toStr)
		}
		from = uint32(v)
	}

	switch { // parse to value
	case len(toStr) == 0:
		h, err := cli.GetBlockCount()
		if err != nil {
			return 0, 0, fmt.Errorf("latest block index unavailable: %w", err)
		}
		if h <= from {
			return 0, 0, fmt.Errorf("latest block is less than from value, from:%d, to:%d", from, h)
		}
		return from, h, nil
	case toStr[0] == 'p':
		v, err := strconv.Atoi(toStr[1:])
		if err != nil || v <= 0 {
			return 0, 0, ErrInvalidInterval(fromStr, toStr)
		}
		return from, from + uint32(v), nil
	default:
		v, err := strconv.Atoi(toStr)
		if err != nil || v <= 0 {
			return 0, 0, ErrInvalidInterval(fromStr, toStr)
		}
		if uint32(v) <= from {
			return 0, 0, ErrInvalidInterval(fromStr, toStr)
		}
		return from, uint32(v), nil
	}
}

func ErrInvalidInterval(from, to string) error {
	return fmt.Errorf("invalid block interval from:%s to:%s", from, to)
}
