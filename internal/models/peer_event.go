package models

import "github.com/pion/webrtc/v4"

type PeerEvent struct {
	Type      PeerEventType
	Candidate *webrtc.ICECandidate
	Answer    *webrtc.SessionDescription
	State     string
}

type PeerEventType string

const (
	SendLocalAnswer    PeerEventType = "send_local_answer"
	SendLocalCandidate PeerEventType = "send_local_candidate"
	RequestRenegotiate PeerEventType = "request_renegotiate"
)
