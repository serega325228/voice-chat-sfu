package models

import "github.com/pion/webrtc/v4"

type PeerEvent struct {
	Type      PeerEventType
	Candidate *webrtc.ICECandidate
	Answer    *webrtc.SessionDescription
}

type PeerEventType string

const (
	SendLocalAnswer    PeerEventType = "send_local_answer"
	SendLocalCandidate PeerEventType = "send_local_candidate"
)
