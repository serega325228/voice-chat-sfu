package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"voice-chat-sfu/internal/models"

	"github.com/google/uuid"
	"github.com/pion/interceptor"
	"github.com/pion/webrtc/v4"
)

type SFUStorage interface {
	CreateRoom(roomID uuid.UUID, creatorPeer *models.Peer) error
	AddPeer(roomID uuid.UUID, peer *models.Peer) error
	GetPeer(roomID, peerID uuid.UUID) (*models.Peer, error)
	RemovePeer(roomID, peerID uuid.UUID) (*models.Peer, error)
	PeersExcept(roomID, peerID uuid.UUID) ([]*models.Peer, error)
	AddTrack(roomID uuid.UUID, track *models.PublishedTrack) error
	RemoveTrack(roomID uuid.UUID, trackID string) error
	Tracks(roomID uuid.UUID) ([]*models.PublishedTrack, error)
}

type SFUService struct {
	log     *slog.Logger
	storage SFUStorage
	api     *webrtc.API
}

func NewSFUService(
	log *slog.Logger,
	storage SFUStorage,
) (*SFUService, error) {
	const op = "SFUService.NewSFUService"

	mediaEngine := &webrtc.MediaEngine{}
	if err := mediaEngine.RegisterDefaultCodecs(); err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	interceptorRegistry := &interceptor.Registry{}
	if err := webrtc.RegisterDefaultInterceptors(mediaEngine, interceptorRegistry); err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	api := webrtc.NewAPI(
		webrtc.WithMediaEngine(mediaEngine),
		webrtc.WithInterceptorRegistry(interceptorRegistry),
	)

	return &SFUService{
		log:     log,
		storage: storage,
		api:     api,
	}, nil
}

func (s *SFUService) initConn() (*webrtc.PeerConnection, error) {
	const op = "SFUService.initConn"

	peerConnectionConfig := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	}

	peerConnection, err := s.api.NewPeerConnection(peerConnectionConfig)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	if _, err = peerConnection.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio); err != nil {
		_ = peerConnection.Close()
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return peerConnection, nil
}

func (s *SFUService) CreateSession(ctx context.Context, roomID, peerID uuid.UUID) error {
	const op = "SFUService.CreateSession"

	peer, err := s.newPeer(roomID, peerID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if err := s.bindPeerConnectionHandlers(ctx, peer); err != nil {
		_ = peer.Conn.Close()
		return fmt.Errorf("%s: %w", op, err)
	}

	if err := s.storage.CreateRoom(roomID, peer); err != nil {
		_ = peer.Conn.Close()
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (s *SFUService) JoinSession(ctx context.Context, roomID, peerID uuid.UUID) error {
	const op = "SFUService.JoinSession"

	peer, err := s.newPeer(roomID, peerID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if err := s.bindPeerConnectionHandlers(ctx, peer); err != nil {
		_ = peer.Conn.Close()
		return fmt.Errorf("%s: %w", op, err)
	}

	if err := s.storage.AddPeer(roomID, peer); err != nil {
		_ = peer.Conn.Close()
		return fmt.Errorf("%s: %w", op, err)
	}

	publishedTracks, err := s.storage.Tracks(roomID)
	if err != nil {
		_ = s.closeAndRemovePeer(peer)
		return fmt.Errorf("%s: %w", op, err)
	}

	for _, publishedTrack := range publishedTracks {
		if publishedTrack.PublisherID == peer.ID {
			continue
		}
		if err := s.sendLocalTrack(peer, publishedTrack.Track); err != nil {
			s.log.Warn("attach existing track to joined peer", "room_id", roomID, "peer_id", peerID, "track_id", publishedTrack.ID, "err", err)
		}
	}

	return nil
}

func (s *SFUService) LeaveSession(ctx context.Context, roomID, peerID uuid.UUID) error {
	const op = "SFUService.LeaveSession"

	_ = ctx

	peer, err := s.storage.RemovePeer(roomID, peerID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if err := peer.Conn.Close(); err != nil {
		s.log.Warn("close peer connection", "room_id", roomID, "peer_id", peerID, "err", err)
	}

	return nil
}

func (s *SFUService) GetPeer(roomID, peerID uuid.UUID) (*models.Peer, error) {
	const op = "SFUService.GetPeer"

	peer, err := s.storage.GetPeer(roomID, peerID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return peer, nil
}

func (s *SFUService) ProcessingOffer(
	ctx context.Context,
	peer *models.Peer,
	sdp string,
) error {
	const op = "SFUService.ProcessingOffer"

	if err := s.bindPeerConnectionHandlers(ctx, peer); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	webrtcOffer := webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  sdp,
	}

	if err := peer.Conn.SetRemoteDescription(webrtcOffer); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	answer, err := peer.Conn.CreateAnswer(nil)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if err := peer.Conn.SetLocalDescription(answer); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if err := s.emitPeerEvent(ctx, peer, &models.PeerEvent{
		Type:   models.SendLocalAnswer,
		Answer: peer.Conn.LocalDescription(),
	}); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (s *SFUService) bindPeerConnectionHandlers(ctx context.Context, peer *models.Peer) error {
	const op = "SFUService.bindPeerConnectionHandlers"

	peer.HandlersOnce.Do(func() {
		peer.Conn.OnICECandidate(func(c *webrtc.ICECandidate) {
			if c == nil {
				return
			}

			if err := s.emitPeerEvent(ctx, peer, &models.PeerEvent{
				Type:      models.SendLocalCandidate,
				Candidate: c,
			}); err != nil {
				s.log.Warn("emit local ICE candidate", "peer_id", peer.ID, "room_id", peer.RoomID, "err", err)
			}
		})

		peer.Conn.OnTrack(func(remoteTrack *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
			go func() {
				if err := s.handleRemoteTrack(ctx, peer, remoteTrack); err != nil && !errors.Is(err, context.Canceled) {
					s.log.Warn("handle remote track", "peer_id", peer.ID, "room_id", peer.RoomID, "track_id", remoteTrack.ID(), "err", err)
				}
			}()
		})

		peer.Conn.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
			if state == webrtc.PeerConnectionStateClosed || state == webrtc.PeerConnectionStateFailed {
				if err := s.closeAndRemovePeer(peer); err != nil {
					s.log.Warn("cleanup peer after connection state change", "peer_id", peer.ID, "room_id", peer.RoomID, "state", state.String(), "err", err)
				}
			}
		})
	})

	return nil
}

func (s *SFUService) GetCandidate(
	ctx context.Context,
	peer *models.Peer,
	candidate string,
	sdpMid string,
	sdpMlineIndex int32,
	usernameFragment string,
) error {
	const op = "SFUService.GetCandidate"

	_ = ctx

	mid := uint16(sdpMlineIndex)
	if err := peer.Conn.AddICECandidate(webrtc.ICECandidateInit{
		Candidate:        candidate,
		SDPMid:           &sdpMid,
		SDPMLineIndex:    &mid,
		UsernameFragment: &usernameFragment,
	}); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (s *SFUService) handleRemoteTrack(
	ctx context.Context,
	peer *models.Peer,
	remoteTrack *webrtc.TrackRemote,
) error {
	const op = "SFUService.handleRemoteTrack"

	localTrack, err := webrtc.NewTrackLocalStaticRTP(
		remoteTrack.Codec().RTPCodecCapability,
		remoteTrack.ID(),
		remoteTrack.StreamID(),
	)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	publishedTrack := &models.PublishedTrack{
		ID:          trackKey(peer.ID, remoteTrack.ID(), remoteTrack.StreamID()),
		PublisherID: peer.ID,
		Track:       localTrack,
	}

	if err := s.storage.AddTrack(peer.RoomID, publishedTrack); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	defer func() {
		if err := s.storage.RemoveTrack(peer.RoomID, publishedTrack.ID); err != nil {
			s.log.Warn("remove published track", "peer_id", peer.ID, "room_id", peer.RoomID, "track_id", publishedTrack.ID, "err", err)
		}
	}()

	receivers, err := s.storage.PeersExcept(peer.RoomID, peer.ID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	for _, receiver := range receivers {
		if err := s.sendLocalTrack(receiver, localTrack); err != nil {
			s.log.Warn("send published track to peer", "from_peer_id", peer.ID, "to_peer_id", receiver.ID, "room_id", peer.RoomID, "track_id", publishedTrack.ID, "err", err)
		}
	}

	rtpBuf := make([]byte, 1500)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		n, _, err := remoteTrack.Read(rtpBuf)
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrClosedPipe) {
				return nil
			}
			return fmt.Errorf("%s: %w", op, err)
		}

		if _, err := localTrack.Write(rtpBuf[:n]); err != nil && !errors.Is(err, io.ErrClosedPipe) {
			return fmt.Errorf("%s: %w", op, err)
		}
	}
}

func (s *SFUService) sendLocalTrack(toPeer *models.Peer, localTrack *webrtc.TrackLocalStaticRTP) error {
	const op = "SFUService.sendLocalTrack"

	rtpSender, err := toPeer.Conn.AddTrack(localTrack)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	go func() {
		rtcpBuf := make([]byte, 1500)
		for {
			if _, _, rtcpErr := rtpSender.Read(rtcpBuf); rtcpErr != nil {
				return
			}
		}
	}()

	return nil
}

func (s *SFUService) newPeer(roomID, peerID uuid.UUID) (*models.Peer, error) {
	const op = "SFUService.newPeer"

	conn, err := s.initConn()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return &models.Peer{
		ID:     peerID,
		RoomID: roomID,
		Conn:   conn,
		Events: make(chan *models.PeerEvent, 16),
	}, nil
}

func (s *SFUService) emitPeerEvent(ctx context.Context, peer *models.Peer, evt *models.PeerEvent) error {
	const op = "SFUService.emitPeerEvent"

	select {
	case <-ctx.Done():
		return fmt.Errorf("%s: %w", op, ctx.Err())
	case peer.Events <- evt:
		return nil
	}
}

func (s *SFUService) closeAndRemovePeer(peer *models.Peer) error {
	const op = "SFUService.closeAndRemovePeer"

	if _, err := s.storage.RemovePeer(peer.RoomID, peer.ID); err != nil {
		s.log.Debug("peer already removed from storage", "room_id", peer.RoomID, "peer_id", peer.ID, "err", err)
	}

	if err := peer.Conn.Close(); err != nil {
		s.log.Debug("peer connection already closed", "room_id", peer.RoomID, "peer_id", peer.ID, "err", err)
	}

	return nil
}

func trackKey(peerID uuid.UUID, trackID, streamID string) string {
	return fmt.Sprintf("%s:%s:%s", peerID, trackID, streamID)
}
