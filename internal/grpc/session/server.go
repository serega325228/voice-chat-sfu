package session

import (
	"context"
	"errors"
	"io"
	"voice-chat-sfu/internal/models"
	sessionv1 "voice-chat-sfu/protos/gen/go/session"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type SessionService interface {
	//rpc
	CreateSession(ctx context.Context, roomID, peerID uuid.UUID) error
	JoinSession(ctx context.Context, roomID, peerID uuid.UUID) error
	LeaveSession(ctx context.Context, roomID, peerID uuid.UUID) error

	//stream
	ProcessingOffer(ctx context.Context, peer *models.Peer, sdp string) error
	GetCandidate(
		ctx context.Context,
		peer *models.Peer,
		candidate string,
		sdpMid string,
		sdpMlineIndex int32,
		usernameFragment string,
	) error

	//helping
	GetPeer(roomID, peerID uuid.UUID) (*models.Peer, error)
}

type serverAPI struct {
	sessionv1.UnimplementedSessionServer
	service SessionService
}

func Register(gRPC *grpc.Server) {
	sessionv1.RegisterSessionServer(gRPC, &serverAPI{})
}

func (s *serverAPI) CreateSession(
	ctx context.Context,
	req *sessionv1.CreateSessionRequest,
) (*sessionv1.CreateSessionResponse, error) {
	//TODO add in response grpc error status
	sessionID, err := uuid.Parse(req.GetSessionId())
	if err != nil {
		return nil, err
	}
	peerID, err := uuid.Parse(req.GetPeerId())
	if err != nil {
		return nil, err
	}
	if err := s.service.CreateSession(ctx, sessionID, peerID); err != nil {
		return nil, err
	}

	return &sessionv1.CreateSessionResponse{}, nil
}

func (s *serverAPI) JoinSession(
	ctx context.Context,
	req *sessionv1.JoinSessionRequest,
) (*sessionv1.JoinSessionResponse, error) {
	//TODO add in response grpc error status
	sessionID, err := uuid.Parse(req.GetSessionId())
	if err != nil {
		return nil, err
	}
	peerID, err := uuid.Parse(req.GetPeerId())
	if err != nil {
		return nil, err
	}
	if err := s.service.JoinSession(ctx, sessionID, peerID); err != nil {
		return nil, err
	}

	return &sessionv1.JoinSessionResponse{}, nil
}

func (s *serverAPI) LeaveSession(
	ctx context.Context,
	req *sessionv1.LeaveSessionRequest,
) (*sessionv1.LeaveSessionResponse, error) {
	//TODO add in response grpc error status
	sessionID, err := uuid.Parse(req.GetSessionId())
	if err != nil {
		return nil, err
	}
	peerID, err := uuid.Parse(req.GetPeerId())
	if err != nil {
		return nil, err
	}
	if err := s.service.CreateSession(ctx, sessionID, peerID); err != nil {
		return nil, err
	}

	return &sessionv1.LeaveSessionResponse{}, nil
}

func (s *serverAPI) SignalPeer(
	stream sessionv1.Session_SignalPeerServer,
) error {
	ctx := stream.Context()

	firstMsg, err := stream.Recv()
	if err != nil {
		return err
	}

	sessionID, err := uuid.Parse(firstMsg.GetSessionId())
	if err != nil {
		return err
	}
	peerID, err := uuid.Parse(firstMsg.GetPeerId())
	if err != nil {
		return err
	}

	peer, err := s.service.GetPeer(sessionID, peerID)
	if err != nil {
		return nil
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
	return err
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
					//TODO
				},
			},
		}
	case models.SendLocalCandidate:
		msg.Payload = &sessionv1.SignalMessage_LocalIceCandidate{
			LocalIceCandidate: &sessionv1.LocalIceCandidate{
				Candidate: &sessionv1.IceCandidate{
					//TODO
				},
			},
		}
	}
	return msg
}

// TODO split from SignalMessasge to SfuToClient and ClientToSfu
func (s *serverAPI) handleIncoming(
	ctx context.Context,
	peer *models.Peer,
	msg *sessionv1.SignalMessage,
) error {
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
		return status.Error(codes.InvalidArgument, "unknown signal message payload")
	}
}

// TODO
func (s *serverAPI) handleRemoteOffer(
	ctx context.Context,
	peer *models.Peer,
	offer *sessionv1.RemoteOffer,
) error {
	err := s.service.ProcessingOffer(ctx, peer, offer.GetOffer().GetSdp())
	if err != nil {
		return err
	}
	return nil
}

func (s *serverAPI) handleRemoteIceCandidate(
	ctx context.Context,
	peer *models.Peer,
	candidate *sessionv1.RemoteIceCandidate,
) error {
	err := s.service.GetCandidate(
		ctx,
		peer,
		candidate.GetCandidate().GetCandidate(),
		candidate.GetCandidate().GetSdpMid(),
		candidate.GetCandidate().GetSdpMlineIndex(),
		candidate.GetCandidate().GetUsernameFragment(),
	)
	if err != nil {
		return err
	}
	return nil
}

func (s *serverAPI) handlePeerClosed(
	ctx context.Context,
	peer *models.Peer,
	offer *sessionv1.PeerClosed,
) error {
	return nil
}

func (s *serverAPI) handleConnectionStateChanged(
	ctx context.Context,
	peer *models.Peer,
	offer *sessionv1.PeerConnectionStateChanged,
) error {
	return nil
}
