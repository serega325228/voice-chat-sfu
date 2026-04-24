package models

import "github.com/google/uuid"

type Room struct {
	ID     uuid.UUID
	Peers  map[uuid.UUID]*Peer
	Tracks map[string]*PublishedTrack
}
