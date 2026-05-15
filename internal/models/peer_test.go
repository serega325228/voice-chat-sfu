package models

import (
	"testing"
	"time"
)

func TestPeerAttachCancelsPreviousAttachment(t *testing.T) {
	peer := &Peer{}

	firstGeneration, firstCtx := peer.Attach()
	secondGeneration, secondCtx := peer.Attach()

	if secondGeneration <= firstGeneration {
		t.Fatalf("expected attachment generation to increase, got first=%d second=%d", firstGeneration, secondGeneration)
	}

	select {
	case <-firstCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("expected first attachment context to be canceled")
	}

	select {
	case <-secondCtx.Done():
		t.Fatal("second attachment context should stay active")
	default:
	}
}

func TestPeerDetachOnlyCancelsCurrentAttachment(t *testing.T) {
	peer := &Peer{}

	firstGeneration, firstCtx := peer.Attach()
	secondGeneration, secondCtx := peer.Attach()

	peer.Detach(firstGeneration, time.Minute, func() {})
	select {
	case <-secondCtx.Done():
		t.Fatal("stale detach should not cancel current attachment")
	default:
	}

	peer.Detach(secondGeneration, time.Minute, func() {})
	select {
	case <-secondCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("expected current attachment context to be canceled")
	}

	select {
	case <-firstCtx.Done():
	default:
		t.Fatal("first attachment context should have been canceled by second attach")
	}
}
