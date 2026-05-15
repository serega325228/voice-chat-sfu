package models

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/pion/webrtc/v4"
)

type PeerStatus string

const (
	PeerConnected    PeerStatus = "connected"
	PeerReconnecting PeerStatus = "reconnecting"
	PeerDisconnected PeerStatus = "disconnected"
)

type Peer struct {
	ID           uuid.UUID
	RoomID       uuid.UUID
	Conn         *webrtc.PeerConnection
	Events       chan *PeerEvent
	HandlersOnce sync.Once
	Status       PeerStatus

	LifetimeCtx          context.Context
	Cancel               context.CancelFunc
	sendersMu            sync.Mutex
	TrackSenders         map[string]*webrtc.RTPSender
	renegotiationPending bool

	attachmentMu         sync.Mutex
	attachmentGeneration uint64
	attached             bool
	detachTimer          *time.Timer
	attachmentCancel     context.CancelFunc
}

func (p *Peer) MarkConnected() {
	p.attachmentMu.Lock()
	defer p.attachmentMu.Unlock()

	p.Status = PeerConnected
}

func (p *Peer) MarkReconnecting() {
	p.attachmentMu.Lock()
	defer p.attachmentMu.Unlock()

	if p.Status != PeerDisconnected {
		p.Status = PeerReconnecting
	}
}

func (p *Peer) MarkDisconnected() {
	p.attachmentMu.Lock()
	defer p.attachmentMu.Unlock()

	p.attached = false
	p.Status = PeerDisconnected
}

func (p *Peer) IsConnected() bool {
	p.attachmentMu.Lock()
	defer p.attachmentMu.Unlock()

	return p.Status == PeerConnected
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

func (p *Peer) Attach() (uint64, context.Context) {
	p.attachmentMu.Lock()
	defer p.attachmentMu.Unlock()

	if p.detachTimer != nil {
		p.detachTimer.Stop()
		p.detachTimer = nil
	}
	if p.attachmentCancel != nil {
		p.attachmentCancel()
	}

	attachmentCtx, cancel := context.WithCancel(context.Background())
	p.attachmentGeneration++
	p.attached = true
	p.Status = PeerConnected
	p.attachmentCancel = cancel

	return p.attachmentGeneration, attachmentCtx
}

func (p *Peer) IsCurrentAttachment(generation uint64) bool {
	p.attachmentMu.Lock()
	defer p.attachmentMu.Unlock()

	return p.attached && p.attachmentGeneration == generation
}

func (p *Peer) Detach(generation uint64, gracePeriod time.Duration, onExpire func()) {
	p.attachmentMu.Lock()
	defer p.attachmentMu.Unlock()

	if p.attachmentGeneration != generation || !p.attached {
		return
	}

	p.attached = false
	p.Status = PeerReconnecting
	if p.attachmentCancel != nil {
		p.attachmentCancel()
		p.attachmentCancel = nil
	}

	if p.detachTimer != nil {
		p.detachTimer.Stop()
		p.detachTimer = nil
	}

	if gracePeriod <= 0 {
		go onExpire()
		return
	}

	currentGeneration := generation
	p.detachTimer = time.AfterFunc(gracePeriod, func() {
		p.attachmentMu.Lock()
		defer p.attachmentMu.Unlock()

		if p.attached || p.attachmentGeneration != currentGeneration {
			return
		}

		p.detachTimer = nil
		go onExpire()
	})
}
