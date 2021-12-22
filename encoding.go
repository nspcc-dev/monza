package main

import (
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/nspcc-dev/neo-go/pkg/core/state"
	"github.com/nspcc-dev/neo-go/pkg/rpc/response/result"
	"github.com/nspcc-dev/neo-go/pkg/vm/stackitem"
	netmap "github.com/nspcc-dev/neofs-api-go/v2/netmap/grpc"
	"google.golang.org/protobuf/proto"
)

const nonCompatibleMsg = "not NeoFS compatible"

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
	const nonCompatibleMsg = "not NEP-17 compatible"

	items, ok := n.Item.Value().([]stackitem.Item)
	if !ok {
		PrintEvent(b, n, nonCompatibleMsg)
		return
	}

	if len(items) != 3 {
		PrintEvent(b, n, nonCompatibleMsg)
		return
	}

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
		PrintEvent(b, n, nonCompatibleMsg)
		return
	}

	var sndStr, rcvStr = "nil", "nil"
	if snd != nil {
		sndStr = hex.EncodeToString(snd)
	}

	if rcv != nil {
		rcvStr = hex.EncodeToString(rcv)
	}

	d := time.Unix(int64(b.Timestamp/1e3), 0)

	s := fmt.Sprintf("block:%d at:%s name:%s from:%s to:%s amount:%d",
		b.Index, d.Format(time.RFC3339), n.Name,
		sndStr, rcvStr, bigAmount.Int64(),
	)

	fmt.Println(s)
}

func PrintNewEpoch(b *result.Block, n state.NotificationEvent) {
	items, ok := n.Item.Value().([]stackitem.Item)
	if !ok {
		PrintEvent(b, n, nonCompatibleMsg)
		return
	}

	if len(items) != 1 {
		PrintEvent(b, n, nonCompatibleMsg)
		return
	}

	epoch, err := items[0].TryInteger()
	if err != nil {
		PrintEvent(b, n, nonCompatibleMsg)
		return
	}

	d := time.Unix(int64(b.Timestamp/1e3), 0)

	s := fmt.Sprintf("block:%d at:%s name:%s epoch:%d",
		b.Index, d.Format(time.RFC3339), n.Name, epoch,
	)

	fmt.Println(s)
}

func PrintAddPeer(b *result.Block, n state.NotificationEvent) {
	items, ok := n.Item.Value().([]stackitem.Item)
	if !ok {
		PrintEvent(b, n, nonCompatibleMsg)
		return
	}

	if len(items) != 1 {
		PrintEvent(b, n, nonCompatibleMsg)
		return
	}

	data, err := items[0].TryBytes()
	if err != nil {
		PrintEvent(b, n, nonCompatibleMsg)
		return
	}

	info := new(netmap.NodeInfo)
	err = proto.Unmarshal(data, info)
	if err != nil {
		PrintEvent(b, n, nonCompatibleMsg)
		return
	}

	d := time.Unix(int64(b.Timestamp/1e3), 0)

	s := fmt.Sprintf("block:%d at:%s name:%s pubkey:[..%s] endpoints:[%s]",
		b.Index, d.Format(time.RFC3339), n.Name,
		hex.EncodeToString(info.PublicKey[len(info.PublicKey)-3:]),
		strings.Join(info.Addresses, ", "),
	)

	fmt.Println(s)
}
