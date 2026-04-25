package session

import (
	"context"
	"errors"
	"fmt"
	"io"
	"voice-chat-sfu/internal/models"

	"github.com/google/uuid"
	"github.com/pion/webrtc/v4"
	sessionv1 "github.com/serega325228/voice-chat-sfu-protos/gen/go/session"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type SessionService interface {
	CreateSession(ctx context.Context, roomID, peerID uuid.UUID) error
	JoinSession(ctx context.Context, roomID, peerID uuid.UUID) error
	LeaveSession(ctx context.Context, roomID, peerID uuid.UUID) error
	ProcessingOffer(ctx context.Context, peer *models.Peer, sdp string) error
	GetCandidate(
		ctx context.Context,
		peer *models.Peer,
		candidate string,
		sdpMid string,
		sdpMlineIndex int32,
		usernameFragment string,
	) error
	GetPeer(roomID, peerID uuid.UUID) (*models.Peer, error)
}

type serverAPI struct {
	sessionv1.UnimplementedSessionServer
	service SessionService
}

func Register(gRPC *grpc.Server, service SessionService) {
	sessionv1.RegisterSessionServer(gRPC, &serverAPI{service: service})
}

func (s *serverAPI) CreateSession(
	ctx context.Context,
	req *sessionv1.CreateSessionRequest,
) (*sessionv1.CreateSessionResponse, error) {
	const op = "serverAPI.CreateSession"

	roomID, peerID, err := parseIDs(req.GetSessionId(), req.GetPeerId())
	if err != nil {
		return nil, err
	}
	if err := s.service.CreateSession(ctx, roomID, peerID); err != nil {
		wrappedErr := fmt.Errorf("%s: %w", op, err)
		return nil, status.Error(codes.Internal, wrappedErr.Error())
	}

	return &sessionv1.CreateSessionResponse{}, nil
}

func (s *serverAPI) JoinSession(
	ctx context.Context,
	req *sessionv1.JoinSessionRequest,
) (*sessionv1.JoinSessionResponse, error) {
	const op = "serverAPI.JoinSession"

	roomID, peerID, err := parseIDs(req.GetSessionId(), req.GetPeerId())
	if err != nil {
		return nil, err
	}
	if err := s.service.JoinSession(ctx, roomID, peerID); err != nil {
		wrappedErr := fmt.Errorf("%s: %w", op, err)
		return nil, status.Error(codes.Internal, wrappedErr.Error())
	}

	return &sessionv1.JoinSessionResponse{}, nil
}

func (s *serverAPI) LeaveSession(
	ctx context.Context,
	req *sessionv1.LeaveSessionRequest,
) (*sessionv1.LeaveSessionResponse, error) {
	const op = "serverAPI.LeaveSession"

	roomID, peerID, err := parseIDs(req.GetSessionId(), req.GetPeerId())
	if err != nil {
		return nil, err
	}
	if err := s.service.LeaveSession(ctx, roomID, peerID); err != nil {
		wrappedErr := fmt.Errorf("%s: %w", op, err)
		return nil, status.Error(codes.Internal, wrappedErr.Error())
	}

	return &sessionv1.LeaveSessionResponse{}, nil
}

func (s *serverAPI) SignalPeer(
	stream sessionv1.Session_SignalPeerServer,
) error {
	const op = "serverAPI.SignalPeer"

	ctx := stream.Context()

	firstMsg, err := stream.Recv()
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	roomID, peerID, err := parseIDs(firstMsg.GetSessionId(), firstMsg.GetPeerId())
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	peer, err := s.service.GetPeer(roomID, peerID)
	if err != nil {
		wrappedErr := fmt.Errorf("%s: %w", op, err)
		return status.Error(codes.NotFound, wrappedErr.Error())
	}

	errCh := make(chan error, 2)

	go func() {
		if err := s.handleIncoming(ctx, peer, firstMsg); err != nil {
			errCh <- err
			return
		}

		for {
			msg, err := stream.Recv()
			if err != nil {
				errCh <- err
				return
			}

			if err := s.handleIncoming(ctx, peer, msg); err != nil {
				errCh <- err
				return
			}
		}
	}()

	go func() {
		for {
			select {
			case <-ctx.Done():
				errCh <- ctx.Err()
				return
			case evt := <-peer.Events:
				if err := stream.Send(s.toSignalMessage(peer, evt)); err != nil {
					errCh <- err
					return
				}
			}
		}
	}()

	err = <-errCh
	if err == io.EOF || errors.Is(err, context.Canceled) {
		return nil
	}
	return fmt.Errorf("%s: %w", op, err)
}

func (s *serverAPI) toSignalMessage(peer *models.Peer, peerEvent *models.PeerEvent) *sessionv1.SignalMessage {
	msg := &sessionv1.SignalMessage{
		SessionId: peer.RoomID.String(),
		PeerId:    peer.ID.String(),
	}

	switch peerEvent.Type {
	case models.SendLocalAnswer:
		msg.Payload = &sessionv1.SignalMessage_LocalAnswer{
			LocalAnswer: &sessionv1.LocalAnswer{
				Answer: &sessionv1.SessionDescription{
					Type: toProtoSDPType(peerEvent.Answer.Type),
					Sdp:  peerEvent.Answer.SDP,
				},
			},
		}
	case models.SendLocalCandidate:
		candidate := peerEvent.Candidate.ToJSON()
		msg.Payload = &sessionv1.SignalMessage_LocalIceCandidate{
			LocalIceCandidate: &sessionv1.LocalIceCandidate{
				Candidate: &sessionv1.IceCandidate{
					Candidate:        candidate.Candidate,
					SdpMid:           valueOrEmpty(candidate.SDPMid),
					SdpMlineIndex:    int32(valueOrZero(candidate.SDPMLineIndex)),
					UsernameFragment: valueOrEmpty(candidate.UsernameFragment),
				},
			},
		}
	}

	return msg
}

func (s *serverAPI) handleIncoming(
	ctx context.Context,
	peer *models.Peer,
	msg *sessionv1.SignalMessage,
) error {
	const op = "serverAPI.handleIncoming"

	switch x := msg.Payload.(type) {
	case *sessionv1.SignalMessage_RemoteOffer:
		return s.handleRemoteOffer(ctx, peer, x.RemoteOffer)
	case *sessionv1.SignalMessage_RemoteIceCandidate:
		return s.handleRemoteIceCandidate(ctx, peer, x.RemoteIceCandidate)
	case *sessionv1.SignalMessage_PeerClosed:
		return s.handlePeerClosed(ctx, peer, x.PeerClosed)
	case *sessionv1.SignalMessage_ConnectionStateChanged:
		return s.handleConnectionStateChanged(ctx, peer, x.ConnectionStateChanged)
	default:
		return fmt.Errorf("%s: %w", op, status.Error(codes.InvalidArgument, "unknown signal message payload"))
	}
}

func (s *serverAPI) handleRemoteOffer(
	ctx context.Context,
	peer *models.Peer,
	offer *sessionv1.RemoteOffer,
) error {
	const op = "serverAPI.handleRemoteOffer"

	if err := s.service.ProcessingOffer(ctx, peer, offer.GetOffer().GetSdp()); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (s *serverAPI) handleRemoteIceCandidate(
	ctx context.Context,
	peer *models.Peer,
	candidate *sessionv1.RemoteIceCandidate,
) error {
	const op = "serverAPI.handleRemoteIceCandidate"

	if err := s.service.GetCandidate(
		ctx,
		peer,
		candidate.GetCandidate().GetCandidate(),
		candidate.GetCandidate().GetSdpMid(),
		candidate.GetCandidate().GetSdpMlineIndex(),
		candidate.GetCandidate().GetUsernameFragment(),
	); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (s *serverAPI) handlePeerClosed(
	ctx context.Context,
	peer *models.Peer,
	_ *sessionv1.PeerClosed,
) error {
	const op = "serverAPI.handlePeerClosed"

	if err := s.service.LeaveSession(ctx, peer.RoomID, peer.ID); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (s *serverAPI) handleConnectionStateChanged(
	ctx context.Context,
	peer *models.Peer,
	_ *sessionv1.PeerConnectionStateChanged,
) error {
	_ = ctx
	_ = peer

	return nil
}

func parseIDs(roomIDRaw, peerIDRaw string) (uuid.UUID, uuid.UUID, error) {
	const op = "session.parseIDs"

	roomID, err := uuid.Parse(roomIDRaw)
	if err != nil {
		wrappedErr := fmt.Errorf("%s: %w", op, err)
		return uuid.Nil, uuid.Nil, status.Error(codes.InvalidArgument, wrappedErr.Error())
	}

	peerID, err := uuid.Parse(peerIDRaw)
	if err != nil {
		wrappedErr := fmt.Errorf("%s: %w", op, err)
		return uuid.Nil, uuid.Nil, status.Error(codes.InvalidArgument, wrappedErr.Error())
	}

	return roomID, peerID, nil
}

func toProtoSDPType(sdpType webrtc.SDPType) sessionv1.SdpType {
	switch sdpType {
	case webrtc.SDPTypeOffer:
		return sessionv1.SdpType_SDP_TYPE_OFFER
	case webrtc.SDPTypeAnswer:
		return sessionv1.SdpType_SDP_TYPE_ANSWER
	default:
		return sessionv1.SdpType_SDP_TYPE_UNSPECIFIED
	}
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}

	return *value
}

func valueOrZero(value *uint16) uint16 {
	if value == nil {
		return 0
	}

	return *value
}
