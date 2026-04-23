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

// TODO make same peer storage with rwmutex
type SFUStorage interface {
	Add(roomID uuid.UUID, creatorPeer *models.Peer) error
	Get(roomID uuid.UUID) (*models.Room, error)
	Delete(roomID uuid.UUID) error
	RoomExceptAuthor(roomID, peerID uuid.UUID) []*models.Peer
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

	mediaEngine := &webrtc.MediaEngine{}
	if err := mediaEngine.RegisterDefaultCodecs(); err != nil {
		return nil, err
	}

	interceptorRegistry := &interceptor.Registry{}
	if err := webrtc.RegisterDefaultInterceptors(mediaEngine, interceptorRegistry); err != nil {
		return nil, err
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

func (s *SFUService) initConn(ctx context.Context) *webrtc.PeerConnection {
	peerConnectionConfig := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	}

	peerConnection, err := s.api.NewPeerConnection(peerConnectionConfig)
	if err != nil {
		panic(err)
	}

	//TODO maybe(mostly must) replace defer with goroutine
	// defer func() {
	// 	if cErr := peerConnection.Close(); cErr != nil {
	// 		fmt.Printf("cannot close peerConnection: %v\n", cErr)
	// 	}
	// }()

	if _, err = peerConnection.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio); err != nil {
		panic(err)
	}

	return peerConnection
}

func (s *SFUService) CreateSession(ctx context.Context, roomID, peerID uuid.UUID) error {
	peer := &models.Peer{
		ID:     peerID,
		RoomID: roomID,
		Conn:   s.initConn(ctx),
		Events: make(chan *models.PeerEvent),
	}

	if err := s.storage.Add(roomID, peer); err != nil {
		return fmt.Errorf("") //TODO
	}
	return nil
}

func (s *SFUService) JoinSession(ctx context.Context, roomID, peerID uuid.UUID) error {
	room, err := s.storage.Get(roomID)
	if err != nil {
		return fmt.Errorf("") //TODO
	}
	if _, ok := room.Peers[peerID]; ok {
		return fmt.Errorf("") //TODO
	}

	newPeer := &models.Peer{
		ID:     peerID,
		RoomID: roomID,
		Conn:   s.initConn(ctx),
		Events: make(chan *models.PeerEvent),
	}

	room.Peers[peerID] = newPeer

	return nil
}

func (s *SFUService) LeaveSession(ctx context.Context, roomID, peerID uuid.UUID) error {
	room, err := s.storage.Get(roomID)
	if err != nil {
		return fmt.Errorf("") //TODO
	}

	peer, ok := room.Peers[peerID]
	if ok {
		return fmt.Errorf("") //TODO
	}

	if err = peer.Conn.Close(); err != nil {
		return err
	}

	delete(room.Peers, peerID)

	return nil
}

func (s *SFUService) ProcessingOffer(
	ctx context.Context,
	peer *models.Peer,
	sdp string,
) error {
	err := s.bindPeerConnectionHandlers(ctx, peer)

	webrtcOffer := webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  sdp,
	}

	if err := peer.Conn.SetRemoteDescription(webrtcOffer); err != nil {
		return fmt.Errorf("set remote description: %w", err)
	}

	answer, err := peer.Conn.CreateAnswer(nil)
	if err != nil {
		return fmt.Errorf("create answer: %w", err)
	}

	if err := peer.Conn.SetLocalDescription(answer); err != nil {
		return fmt.Errorf("set local description: %w", err)
	}

	peer.Events <- &models.PeerEvent{
		Type:   models.SendLocalAnswer,
		Answer: peer.Conn.LocalDescription(),
	}

	return nil
}

func (s *SFUService) bindPeerConnectionHandlers(ctx context.Context, peer *models.Peer) error {
	peer.Conn.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return
		}

		peer.Events <- &models.PeerEvent{
			Type:      models.SendLocalCandidate,
			Candidate: c,
		}
	})

	errCh := make(chan error, 2)

	peer.Conn.OnTrack(func(remoteTrack *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		go s.handleRemoteTrack(ctx, peer, remoteTrack, errCh)
	})

	err := <-errCh
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

func (s *SFUService) GetCandidate(
	ctx context.Context,
	peer *models.Peer,
	candidate string,
	sdpMid string,
	sdpMlineIndex int32,
	usernameFragment string,
) error {
	mid := uint16(sdpMlineIndex)
	if err := peer.Conn.AddICECandidate(webrtc.ICECandidateInit{
		Candidate:        candidate,
		SDPMid:           &sdpMid,
		SDPMLineIndex:    &mid,
		UsernameFragment: &usernameFragment,
	}); err != nil {
		return err
	}

	return nil
}

func (s *SFUService) handleRemoteTrack(
	ctx context.Context,
	peer *models.Peer,
	remoteTrack *webrtc.TrackRemote,
	errCh chan error,
) error {
	localTrack, err := webrtc.NewTrackLocalStaticRTP(remoteTrack.Codec().RTPCodecCapability, remoteTrack.ID(), remoteTrack.StreamID())
	if err != nil {
		errCh <- fmt.Errorf("", err) //TODO
	}

	rtpBuf := make([]byte, 1500)
	for {
		i, _, err := remoteTrack.Read(rtpBuf)
		if err != nil {
			errCh <- fmt.Errorf("", err) //TODO
			continue
		}

		if _, err := localTrack.Write(rtpBuf[:i]); err != nil && !errors.Is(err, io.ErrClosedPipe) {
			errCh <- fmt.Errorf("", err) //TODO
			continue
		}

		for _, receiver := range s.storage.RoomExceptAuthor(peer.RoomID, peer.ID) {
			errCh <- s.sendLocalTrack(ctx, receiver, localTrack)
		}
	}
}

func (s *SFUService) sendLocalTrack(ctx context.Context, toPeer *models.Peer, localTrack *webrtc.TrackLocalStaticRTP) error {
	rtpSender, err := toPeer.Conn.AddTrack(localTrack)
	if err != nil {
		return err
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
