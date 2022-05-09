package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alexvanin/monza/chain"
	"github.com/gdamore/tcell/v2"
	"github.com/nspcc-dev/neo-go/pkg/core/state"
	"github.com/nspcc-dev/neo-go/pkg/rpc/response/result"
	"github.com/nspcc-dev/neo-go/pkg/util"
	"github.com/nspcc-dev/neo-go/pkg/vm/stackitem"
	"github.com/rivo/tview"
	"github.com/urfave/cli/v2"
)

type (
	Explorer struct {
		ctx      context.Context
		chain    *chain.Chain
		endpoint string
		app      *tview.Application

		jobCh chan fetchTask
		errCh chan error
		wg    sync.WaitGroup

		searchErrFlag bool
	}

	fetchTask struct {
		txHash *util.Uint256
		index  uint32
	}
)

const defaultExploreWorkers = 100

func explorer(c *cli.Context) (err error) {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)

	// parse blockchain info
	cacheDir := c.String(cacheFlagKey)
	if len(cacheDir) == 0 {
		cacheDir, err = defaultConfigDir()
		if err != nil {
			return err
		}
	}

	endpoint := c.String(endpointFlagKey)
	blockchain, err := chain.Open(ctx, cacheDir, endpoint)
	if err != nil {
		return fmt.Errorf("cannot initialize remote blockchain client: %w", err)
	}
	defer func() {
		blockchain.Close()
		cancel()
	}()

	e := Explorer{
		ctx:      ctx,
		chain:    blockchain,
		endpoint: endpoint,
		app:      tview.NewApplication(),
		jobCh:    make(chan fetchTask),
		errCh:    make(chan error),
	}
	e.startWorkers(defaultExploreWorkers)
	return e.Run()
}

func (e *Explorer) Run() error {
	// UI basic elements
	blockList := tview.NewList()
	searchInput := tview.NewInputField()
	blockInfo := tview.NewTextView()
	txCounter := tview.NewTextView()
	txList := tview.NewList()
	notifications := tview.NewTextView()

	// UI containers
	blockFlex := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(blockInfo, 0, 1, false).
		AddItem(txCounter, 2, 2, false).
		AddItem(txList, 5, 3, false)
	appGrid := tview.NewGrid().
		SetRows(15, -1, 1).
		SetColumns(-1, -2).
		AddItem(blockList, 0, 0, 2, 1, 0, 0, false).
		AddItem(searchInput, 2, 0, 1, 2, 0, 0, false).
		AddItem(blockFlex, 0, 1, 1, 1, 0, 0, false).
		AddItem(notifications, 1, 1, 1, 1, 0, 0, false)

	// Initialize element style
	blockList.ShowSecondaryText(false).SetWrapAround(false)
	blockList.SetBorder(true).SetTitle("Blocks")
	searchInput.SetFieldBackgroundColor(tcell.ColorBlack)
	txList.ShowSecondaryText(false)
	setFocusColorStyle(blockList.Box, blockList.Box)
	setFocusColorStyle(blockFlex.Box, txList.Box)
	setFocusColorStyle(notifications.Box, notifications.Box)

	// Handle redrawing events
	blockList.SetDrawFunc(func(_ tcell.Screen, _, _, _, _ int) (int, int, int, int) {
		posX, posY, width, height := blockList.GetInnerRect()
		e.redrawBlockList(blockList, height)
		return posX, posY, width, height
	})

	// Handle non-default keyboard events
	appGrid.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Rune() == '/' {
			e.searchStatusBar(searchInput)
			e.app.SetFocus(searchInput)
			return nil
		}
		return event
	})
	blockList.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Rune() {
		case 'q':
			e.app.Stop()
			return nil
		case 'r':
			e.refillBlockList(blockList)
			return nil
		default:
			return event
		}
	})
	searchInput.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if !e.searchErrFlag {
			return event
		}
		e.searchErrFlag = false
		e.defaultStatusBar(searchInput, blockList.GetItemCount())
		e.app.SetFocus(blockList)
		return nil
	})
	txList.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyTAB || event.Key() == tcell.KeyEnter {
			e.app.SetFocus(notifications)
			return nil
		}
		if event.Rune() == 'q' {
			e.hideBlockFlex(blockFlex, blockInfo, txCounter, notifications, txList)
			e.app.SetFocus(blockList)
			return nil
		}
		return event
	})
	notifications.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyTAB || event.Rune() == 'q' {
			e.app.SetFocus(txList)
			return nil
		}
		return event
	})

	// Handle search input logic
	searchInput.SetDoneFunc(func(key tcell.Key) {
		e.hideBlockFlex(blockFlex, blockInfo, txCounter, notifications, txList)
		e.refillBlockList(blockList)

		input := searchInput.GetText()
		blockCount := blockList.GetItemCount()

		// Try parse as lookingBlock index
		lookingBlock, err := strconv.Atoi(input)
		if err == nil {
			if lookingBlock < 0 || lookingBlock >= blockCount {
				e.errorStatusBar(searchInput, "invalid block number")
				return
			}
			from, to := blockIndexRange(lookingBlock, blockCount, 50)
			e.cacheBlocks(from, to)
			e.defaultStatusBar(searchInput, blockCount)
			blockList.SetCurrentItem(-lookingBlock - 1)
			e.app.SetFocus(blockList)
			return
		}
		// Try parse as transaction hash
		h, err := util.Uint256DecodeStringLE(input)
		if err == nil {
			appLog, err := e.chain.Client.GetApplicationLog(h, nil)
			if err != nil {
				e.errorStatusBar(searchInput, "tx hash not found")
				return
			}
			block, err := e.chain.BlockByHash(appLog.Container)
			if err != nil {
				e.errorStatusBar(searchInput, "can't get block of specified tx")
				return
			}
			from, to := blockIndexRange(int(block.Index), blockCount, 50)
			e.cacheBlocks(from, to)
			e.defaultStatusBar(searchInput, blockCount)
			blockList.SetCurrentItem(-int(block.Index) - 1)
			e.app.SetFocus(blockList)
			return
		}
		e.errorStatusBar(searchInput, "invalid input, expect valid block number or tx hash")
	})

	// Handle selecting block in block list
	blockList.SetSelectedFunc(func(i int, s1, s2 string, r rune) {
		block, err := e.chain.Block(uint32(blockList.GetItemCount() - i - 1))
		if err != nil {
			panic(err)
		}
		e.cacheNotifications(block)
		e.displayBlockFlex(blockFlex, blockInfo, txCounter, notifications, txList, block)
		e.app.SetFocus(txList)
	})

	// Handle select transaction in block flex
	txList.SetChangedFunc(func(index int, mainText, secondaryText string, shortcut rune) {
		txCounter.SetText(fmt.Sprintf("Transaction %d of %d\n---", index+1, txList.GetItemCount()))
		txHash, err := util.Uint256DecodeStringLE(mainText)
		if err != nil {
			panic(err)
		}
		appLog, err := e.chain.ApplicationLog(txHash)
		if err != nil {
			panic(err)
		}

		events := make([]state.NotificationEvent, 0)
		for _, execution := range appLog.Executions {
			events = append(events, execution.Events...)
		}

		var res string
		for _, event := range events {
			v, err := formatNotification(event)
			if err != nil {
				continue
			}
			res += v
		}
		notifications.SetText(res)
		notifications.ScrollToBeginning()
	})

	// Initialize element data
	e.fillBlockList(blockList)
	e.defaultStatusBar(searchInput, blockList.GetItemCount())

	// Start UI
	e.app.SetRoot(appGrid, true).SetFocus(blockList)
	return e.app.Run()
}

func (e *Explorer) fillBlockList(list *tview.List) {
	to, err := e.chain.Client.GetBlockCount()
	if err != nil {
		panic(err)
	}

	// adding is much faster than inserting
	// so we reverse the order to use that
	for i := int64(to) - 1; i >= 0; i-- {
		line := fmt.Sprintf("#%d", i)
		list.AddItem(line, "", 0, nil)
	}

	// cache some blocks upfront for smoother scrolling
	var from uint32
	if to > 100 {
		from = to - 100
	}
	e.cacheBlocks(from, to-1)
}

func (e *Explorer) refillBlockList(list *tview.List) {
	from := uint32(list.GetItemCount())

	to, err := e.chain.Client.GetBlockCount()
	if err != nil {
		panic(err)
	}

	for i := from; i < to; i++ {
		line := fmt.Sprintf("#%d", i)
		list.InsertItem(0, line, "", 0, nil)
	}

	e.cacheBlocks(from, to-1)
}

func (e *Explorer) redrawBlockList(list *tview.List, height int) {
	currentItemIndex := list.GetCurrentItem()
	itemCount := list.GetItemCount()

	// range of blocks for detailed info in listbox
	// to avoid whole blockchain fetching, app works
	// with small ranges of visible blocks on the screen
	fromIndex := currentItemIndex - height
	if fromIndex < 0 {
		fromIndex = 0
	}

	toIndex := currentItemIndex + height
	if toIndex > itemCount {
		toIndex = itemCount
	}

	e.cacheBlocks(toBlockIndex(toIndex, itemCount), toBlockIndex(fromIndex, itemCount))
	for i := fromIndex; i < toIndex; i++ {
		elem, _ := list.GetItemText(i)
		// ignore blocks that has been parsed
		if strings.Contains(elem, "tx") {
			continue
		}
		blockIndex := toBlockIndex(i, itemCount)
		block, err := e.chain.Block(blockIndex)
		if err != nil {
			panic(err)
		}
		ts := time.Unix(int64(block.Timestamp/1e3), 0)
		richText := fmt.Sprintf("#%d [%s] txs:%d", blockIndex, ts.Format(time.RFC3339), len(block.Transactions))
		list.SetItemText(i, richText, "")
	}
}

func (e *Explorer) startWorkers(amount int) {
	worker := func(ctx context.Context, ch <-chan fetchTask, out chan<- error) {
		for {
			select {
			case <-ctx.Done():
				return
			case task, ok := <-ch:
				if !ok {
					return
				}
				var err error
				if task.txHash != nil {
					_, err = e.chain.ApplicationLog(*task.txHash)
				} else {
					_, err = e.chain.Block(task.index)
				}
				if err != nil {
					out <- err
					return
				}
				e.wg.Done()
			}
		}
	}

	for i := 0; i < amount; i++ {
		go worker(e.ctx, e.jobCh, e.errCh)
	}
}

func (e *Explorer) cacheBlocks(from, to uint32) {
	for i := from; i <= to; i++ {
		e.wg.Add(1)
		select {
		case <-e.ctx.Done():
			return
		case err := <-e.errCh:
			panic(err)
		case e.jobCh <- fetchTask{index: i}:
		}
	}

	wgCh := make(chan struct{})

	go func() {
		e.wg.Wait()
		close(wgCh)
	}()

	select {
	case <-e.ctx.Done():
		return
	case err := <-e.errCh:
		panic(err)
	case <-wgCh:
		return
	}
}

func (e *Explorer) cacheNotifications(block *result.Block) {
	for _, tx := range block.Transactions {
		h := tx.Hash()
		e.wg.Add(1)
		select {
		case <-e.ctx.Done():
			return
		case err := <-e.errCh:
			panic(err)
		case e.jobCh <- fetchTask{txHash: &h}:
		}
	}

	wgCh := make(chan struct{})

	go func() {
		e.wg.Wait()
		close(wgCh)
	}()

	select {
	case <-e.ctx.Done():
		return
	case err := <-e.errCh:
		panic(err)
	case <-wgCh:
		return
	}
}

func (e *Explorer) defaultStatusBar(input *tview.InputField, blocks int) {
	message := fmt.Sprintf("Endpoint: %s Blocks: %d | Press q to back, / to search, r to resync.",
		e.endpoint,
		blocks)
	input.SetText("").
		SetLabelColor(tcell.ColorWhite).
		SetLabel(message)
}

func (e *Explorer) searchStatusBar(input *tview.InputField) {
	input.SetText("").
		SetLabelColor(tcell.ColorGreen).
		SetLabel("Search block or transaction: ")
}

func (e *Explorer) errorStatusBar(input *tview.InputField, message string) {
	e.searchErrFlag = true
	input.SetText("").
		SetLabelColor(tcell.ColorRed).
		SetLabel(message)
}

func (e *Explorer) displayBlockFlex(f *tview.Flex, info, counter, notif *tview.TextView, list *tview.List, b *result.Block) {
	ln := min(len(b.Transactions), 7)
	for _, tx := range b.Transactions {
		list.AddItem(tx.Hash().StringLE(), "", 0, nil)
	}
	info.SetText(fmt.Sprintf("Exploring block #%d", b.Index))
	counter.SetText(fmt.Sprintf("Transaction %d of %d\n---", min(1, ln), list.GetItemCount()))
	notif.SetBorder(true).SetTitle("Notifications")
	f.Clear()
	f.SetBorder(true)
	f.AddItem(info, 0, 1, false)
	f.AddItem(counter, 2, 2, false)
	f.AddItem(list, ln, 3, false)
}

func (e *Explorer) hideBlockFlex(flex *tview.Flex, info, counter, notif *tview.TextView, list *tview.List) {
	info.SetText("")
	counter.SetText("")
	list.Clear()
	flex.SetBorder(false)
	notif.SetText("").SetBorder(false).SetTitle("")
}

func toBlockIndex(index, length int) uint32 {
	return uint32(length - index - 1)
}

func blockIndexRange(index, length, delta int) (uint32, uint32) {
	from := index - delta
	to := index + delta
	if from < 0 {
		from = 0
	}
	if to >= length {
		to = length - 1
	}
	return uint32(from), uint32(to)
}

func formatNotification(event state.NotificationEvent) (string, error) {
	data, err := stackitem.ToJSONWithTypes(event.Item)
	if err != nil {
		return "", err
	}
	var formatted bytes.Buffer
	err = json.Indent(&formatted, data, "", "   ")
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s\n---\n%s\n\n", event.Name, formatted.String()), nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func setFocusColorStyle(target, focus *tview.Box) {
	focus.SetFocusFunc(func() {
		target.SetBorderColor(tcell.ColorGreen)
		target.SetBorderAttributes(tcell.AttrBold)
	})
	focus.SetBlurFunc(func() {
		target.SetBorderColor(tcell.ColorDefault)
		target.SetBorderAttributes(tcell.AttrNone)
	})
}
