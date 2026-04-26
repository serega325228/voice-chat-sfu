package models

import (
	"context"
	"sync"

	"github.com/google/uuid"
	"github.com/pion/webrtc/v4"
)

type Peer struct {
	ID           uuid.UUID
	RoomID       uuid.UUID
	Conn         *webrtc.PeerConnection
	Events       chan *PeerEvent
	HandlersOnce sync.Once

	LifetimeCtx          context.Context
	Cancel               context.CancelFunc
	sendersMu            sync.Mutex
	TrackSenders         map[string]*webrtc.RTPSender
	renegotiationPending bool
}

func (p *Peer) HasTrackSender(trackID string) bool {
	p.sendersMu.Lock()
	defer p.sendersMu.Unlock()

	_, ok := p.TrackSenders[trackID]
	return ok
}

func (p *Peer) BindTrackSender(trackID string, sender *webrtc.RTPSender) {
	p.sendersMu.Lock()
	defer p.sendersMu.Unlock()

	p.TrackSenders[trackID] = sender
}

func (p *Peer) RemoveTrackSender(trackID string) *webrtc.RTPSender {
	p.sendersMu.Lock()
	defer p.sendersMu.Unlock()

	sender := p.TrackSenders[trackID]
	delete(p.TrackSenders, trackID)

	return sender
}

func (p *Peer) MarkRenegotiationNeeded() bool {
	p.sendersMu.Lock()
	defer p.sendersMu.Unlock()

	if p.renegotiationPending {
		return false
	}

	p.renegotiationPending = true
	return true
}

func (p *Peer) ClearRenegotiationNeeded() {
	p.sendersMu.Lock()
	defer p.sendersMu.Unlock()

	p.renegotiationPending = false
}
