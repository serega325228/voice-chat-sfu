package models

import (
	"github.com/google/uuid"
	"github.com/pion/webrtc/v4"
)

type Peer struct {
	ID     uuid.UUID
	RoomID uuid.UUID
	Conn   *webrtc.PeerConnection
	Events chan *PeerEvent
}
