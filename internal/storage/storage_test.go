package storage

import (
	"testing"
	"time"
	"voice-chat-sfu/internal/models"

	"github.com/google/uuid"
)

func TestRemovePeerKeepsRoomUntilTTL(t *testing.T) {
	t.Parallel()

	st := NewSFUStorage(50 * time.Millisecond)
	roomID := uuid.New()
	peerID := uuid.New()

	if err := st.CreateRoom(roomID, &models.Peer{ID: peerID, RoomID: roomID}); err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}

	if _, _, err := st.RemovePeer(roomID, peerID); err != nil {
		t.Fatalf("RemovePeer() error = %v", err)
	}

	if _, err := st.Tracks(roomID); err != nil {
		t.Fatalf("room should stay available before TTL expires: %v", err)
	}

	time.Sleep(90 * time.Millisecond)

	if _, err := st.Tracks(roomID); err == nil {
		t.Fatal("room should be deleted after TTL expires")
	}
}

func TestAddPeerCancelsPendingRoomCleanup(t *testing.T) {
	t.Parallel()

	st := NewSFUStorage(80 * time.Millisecond)
	roomID := uuid.New()
	firstPeerID := uuid.New()
	secondPeerID := uuid.New()

	if err := st.CreateRoom(roomID, &models.Peer{ID: firstPeerID, RoomID: roomID}); err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}

	if _, _, err := st.RemovePeer(roomID, firstPeerID); err != nil {
		t.Fatalf("RemovePeer() error = %v", err)
	}

	time.Sleep(20 * time.Millisecond)

	if err := st.AddPeer(roomID, &models.Peer{ID: secondPeerID, RoomID: roomID}); err != nil {
		t.Fatalf("AddPeer() error = %v", err)
	}

	time.Sleep(90 * time.Millisecond)

	if _, err := st.GetPeer(roomID, secondPeerID); err != nil {
		t.Fatalf("room cleanup should be cancelled after peer rejoins: %v", err)
	}
}
