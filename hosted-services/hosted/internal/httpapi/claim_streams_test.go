package httpapi

import (
	"context"
	"fmt"
	"sync"
	"testing"
)

// HOST01: one IPv6 /48 cannot occupy the claim-stream pool by spreading its
// streams over many /64s, while other networks still get their streams.
func TestClaimStreamsBoundEachWiderNetwork(t *testing.T) {
	var c claimStreams
	var closers []func()
	open := func(op, address string) bool {
		_, cancel := context.WithCancel(context.Background())
		closeStream, ok := c.open(op, budgetIP(address), budgetNetwork(address), cancel)
		if ok {
			closers = append(closers, closeStream)
		}
		return ok
	}
	admitted := 0
	for subnet := range 32 {
		for n := range claimStreamsPerPeer {
			if open(fmt.Sprintf("op_%d_%d", subnet, n), fmt.Sprintf("2001:db8:7:%x::%d", subnet, n+1)) {
				admitted++
			}
		}
	}
	if admitted != claimStreamsPerNetwork {
		t.Fatalf("one /48 held %d streams; want %d", admitted, claimStreamsPerNetwork)
	}
	// Another /48 and an IPv4 client are unaffected.
	if !open("other_v6", "2001:db8:8::1") || !open("other_v4", "198.51.100.7") {
		t.Fatal("a separate network was refused")
	}
	// The per-address limit still applies inside a network with spare share.
	for n := range claimStreamsPerPeer - 1 {
		if !open(fmt.Sprintf("v4_%d", n), "198.51.100.7") {
			t.Fatal("address refused under its own limit")
		}
	}
	if open("v4_over", "198.51.100.7") {
		t.Fatal("per-address limit not enforced")
	}
	// A shared IPv4 /24 can use its share from several addresses.
	for n := range claimStreamsPerNetwork - claimStreamsPerPeer {
		if !open(fmt.Sprintf("campus_%d", n), fmt.Sprintf("198.51.100.%d", 20+n)) {
			t.Fatalf("shared /24 refused at %d streams", claimStreamsPerPeer+n)
		}
	}
	if open("campus_over", "198.51.100.200") {
		t.Fatal("IPv4 /24 exceeded its share")
	}
	// Closing a stream returns its slot to the network.
	closers[0]()
	if !open("reuse", "2001:db8:7:ff::1") {
		t.Fatal("released network slot was not reusable")
	}
	for _, closeStream := range closers[1:] {
		closeStream()
	}
	if len(c.byPeer) != 0 || len(c.byNetwork) != 0 || len(c.byOp) != 0 {
		t.Fatalf("counts leaked: peers=%v networks=%v", c.byPeer, c.byNetwork)
	}
}

// A newer stream for the same operation cancels the older one; the older
// stream's close must not clear its successor, and closing twice is harmless.
func TestClaimStreamReplacementKeepsSuccessor(t *testing.T) {
	var c claimStreams
	firstCtx, firstCancel := context.WithCancel(context.Background())
	closeFirst, ok := c.open("op", "198.51.100.7", "198.51.100.0/24", firstCancel)
	if !ok {
		t.Fatal("first stream refused")
	}
	_, secondCancel := context.WithCancel(context.Background())
	closeSecond, ok := c.open("op", "198.51.100.7", "198.51.100.0/24", secondCancel)
	if !ok {
		t.Fatal("replacement refused")
	}
	if firstCtx.Err() == nil {
		t.Fatal("replaced stream was not cancelled")
	}
	second := c.byOp["op"]
	closeFirst()
	closeFirst()
	if c.byOp["op"] != second {
		t.Fatal("the replaced stream's close cleared its successor")
	}
	if c.byPeer["198.51.100.7"] != 1 || c.byNetwork["198.51.100.0/24"] != 1 {
		t.Fatalf("double close miscounted: peers=%v networks=%v", c.byPeer, c.byNetwork)
	}
	// A third stream replaces the second; the second's cancel runs once.
	thirdCtx, thirdCancel := context.WithCancel(context.Background())
	closeThird, _ := c.open("op", "198.51.100.8", "198.51.100.0/24", thirdCancel)
	closeSecond()
	if thirdCtx.Err() != nil || c.byOp["op"] == nil {
		t.Fatal("successor lost after a predecessor closed")
	}
	closeThird()
	closeThird()
	if len(c.byOp) != 0 || len(c.byPeer) != 0 || len(c.byNetwork) != 0 {
		t.Fatalf("leaked: ops=%d peers=%v networks=%v", len(c.byOp), c.byPeer, c.byNetwork)
	}
}

// Concurrent opens, replacements and closes keep every limit and leave
// nothing behind (run with -race).
func TestClaimStreamsConcurrentLifecycle(t *testing.T) {
	var c claimStreams
	var wg sync.WaitGroup
	var mu sync.Mutex
	maxNetwork := 0
	for worker := range 64 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := range 200 {
				address := fmt.Sprintf("2001:db8:7:%x::1", worker%40)
				_, cancel := context.WithCancel(context.Background())
				closeStream, ok := c.open(fmt.Sprintf("op_%d", n%7), budgetIP(address), budgetNetwork(address), cancel)
				if !ok {
					cancel()
					continue
				}
				c.mu.Lock()
				if v := c.byNetwork[budgetNetwork(address)]; v > maxNetwork {
					mu.Lock()
					maxNetwork = v
					mu.Unlock()
				}
				c.mu.Unlock()
				closeStream()
				closeStream()
				cancel()
			}
		}()
	}
	wg.Wait()
	if maxNetwork > claimStreamsPerNetwork {
		t.Fatalf("network held %d streams", maxNetwork)
	}
	if len(c.byOp) != 0 || len(c.byPeer) != 0 || len(c.byNetwork) != 0 {
		t.Fatalf("leaked: ops=%d peers=%v networks=%v", len(c.byOp), c.byPeer, c.byNetwork)
	}
}
