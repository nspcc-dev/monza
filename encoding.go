package main

import (
	"encoding/hex"
	"fmt"
	"time"

	"github.com/nspcc-dev/neo-go/pkg/core/state"
	"github.com/nspcc-dev/neo-go/pkg/rpc/response/result"
	"github.com/nspcc-dev/neo-go/pkg/vm/stackitem"
)

func PrintEvent(b *result.Block, n state.NotificationEvent, extra string) {
	d := time.Unix(int64(b.Timestamp/1e3), 0)
	s := fmt.Sprintf("block:%d at:%s name:%s",
		b.Index, d.Format(time.RFC3339), n.Name,
	)

	if len(extra) != 0 {
		s += fmt.Sprintf(" [%s]", extra)
	}

	fmt.Println(s)
}

func PrintTransfer(b *result.Block, n state.NotificationEvent) {
	d := time.Unix(int64(b.Timestamp/1e3), 0)

	items := n.Item.Value().([]stackitem.Item)

	snd, err := items[0].TryBytes()
	if err != nil {
		snd = nil
	}

	rcv, err := items[1].TryBytes()
	if err != nil {
		rcv = nil
	}

	bigAmount, err := items[2].TryInteger()
	if err != nil {
		PrintEvent(b, n, "non NEP-17 compatible")
	}

	var sndStr, rcvStr = "nil", "nil"
	if snd != nil {
		sndStr = hex.EncodeToString(snd)
	}

	if rcv != nil {
		rcvStr = hex.EncodeToString(rcv)
	}

	s := fmt.Sprintf("block:%d at:%s name:%s from:%s to:%s amount:%d",
		b.Index, d.Format(time.RFC3339), n.Name,
		sndStr, rcvStr, bigAmount.Int64(),
	)

	fmt.Println(s)
}
