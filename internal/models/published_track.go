package models

import (
	"github.com/google/uuid"
	"github.com/pion/webrtc/v4"
)

type PublishedTrack struct {
	ID          string
	PublisherID uuid.UUID
	Track       *webrtc.TrackLocalStaticRTP
}
