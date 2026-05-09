package storage

import (
	"fmt"
	"sync"
	"time"
	"voice-chat-sfu/internal/models"

	"github.com/google/uuid"
)

type SFUStorage struct {
	mu               sync.RWMutex
	rooms            map[uuid.UUID]*models.Room
	roomCleanupTimers map[uuid.UUID]*time.Timer
	emptyRoomTTL     time.Duration
}

func NewSFUStorage(emptyRoomTTL time.Duration) *SFUStorage {
	if emptyRoomTTL <= 0 {
		emptyRoomTTL = 5 * time.Minute
	}

	return &SFUStorage{
		rooms:             make(map[uuid.UUID]*models.Room),
		roomCleanupTimers: make(map[uuid.UUID]*time.Timer),
		emptyRoomTTL:      emptyRoomTTL,
	}
}

func (s *SFUStorage) CreateRoom(roomID uuid.UUID, creatorPeer *models.Peer) error {
	const op = "SFUStorage.CreateRoom"

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.rooms[roomID]; exists {
		err := fmt.Errorf("room %s already exists", roomID)
		return fmt.Errorf("%s: %w", op, err)
	}

	s.rooms[roomID] = &models.Room{
		ID:     roomID,
		Peers:  map[uuid.UUID]*models.Peer{creatorPeer.ID: creatorPeer},
		Tracks: map[string]*models.PublishedTrack{},
	}

	return nil
}

func (s *SFUStorage) AddPeer(roomID uuid.UUID, peer *models.Peer) error {
	const op = "SFUStorage.AddPeer"

	s.mu.Lock()
	defer s.mu.Unlock()

	room, ok := s.rooms[roomID]
	if !ok {
		err := fmt.Errorf("room %s does not exist", roomID)
		return fmt.Errorf("%s: %w", op, err)
	}
	if _, exists := room.Peers[peer.ID]; exists {
		err := fmt.Errorf("peer %s already exists in room %s", peer.ID, roomID)
		return fmt.Errorf("%s: %w", op, err)
	}

	room.Peers[peer.ID] = peer
	s.cancelRoomCleanupLocked(roomID)

	return nil
}

func (s *SFUStorage) GetPeer(roomID, peerID uuid.UUID) (*models.Peer, error) {
	const op = "SFUStorage.GetPeer"

	s.mu.RLock()
	defer s.mu.RUnlock()

	room, ok := s.rooms[roomID]
	if !ok {
		err := fmt.Errorf("room %s does not exist", roomID)
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	peer, ok := room.Peers[peerID]
	if !ok {
		err := fmt.Errorf("peer %s does not exist in room %s", peerID, roomID)
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return peer, nil
}

func (s *SFUStorage) RemovePeer(roomID, peerID uuid.UUID) (*models.Peer, []*models.PublishedTrack, error) {
	const op = "SFUStorage.RemovePeer"

	s.mu.Lock()
	defer s.mu.Unlock()

	room, ok := s.rooms[roomID]
	if !ok {
		err := fmt.Errorf("room %s does not exist", roomID)
		return nil, nil, fmt.Errorf("%s: %w", op, err)
	}

	peer, ok := room.Peers[peerID]
	if !ok {
		err := fmt.Errorf("peer %s does not exist in room %s", peerID, roomID)
		return nil, nil, fmt.Errorf("%s: %w", op, err)
	}

	delete(room.Peers, peerID)

	removedTracks := make([]*models.PublishedTrack, 0, len(room.Tracks))
	for key, track := range room.Tracks {
		if track.PublisherID == peerID {
			removedTracks = append(removedTracks, track)
			delete(room.Tracks, key)
		}
	}

	if len(room.Peers) == 0 {
		s.scheduleRoomCleanupLocked(roomID)
	}

	return peer, removedTracks, nil
}

func (s *SFUStorage) PeersExcept(roomID, peerID uuid.UUID) ([]*models.Peer, error) {
	const op = "SFUStorage.PeersExcept"

	s.mu.RLock()
	defer s.mu.RUnlock()

	room, ok := s.rooms[roomID]
	if !ok {
		err := fmt.Errorf("room %s does not exist", roomID)
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	peers := make([]*models.Peer, 0, len(room.Peers))
	for id, peer := range room.Peers {
		if id == peerID {
			continue
		}
		peers = append(peers, peer)
	}

	return peers, nil
}

func (s *SFUStorage) AddTrack(roomID uuid.UUID, track *models.PublishedTrack) error {
	const op = "SFUStorage.AddTrack"

	s.mu.Lock()
	defer s.mu.Unlock()

	room, ok := s.rooms[roomID]
	if !ok {
		err := fmt.Errorf("room %s does not exist", roomID)
		return fmt.Errorf("%s: %w", op, err)
	}

	room.Tracks[track.ID] = track

	return nil
}

func (s *SFUStorage) RemoveTrack(roomID uuid.UUID, trackID string) error {
	const op = "SFUStorage.RemoveTrack"

	s.mu.Lock()
	defer s.mu.Unlock()

	room, ok := s.rooms[roomID]
	if !ok {
		err := fmt.Errorf("room %s does not exist", roomID)
		return fmt.Errorf("%s: %w", op, err)
	}

	delete(room.Tracks, trackID)

	return nil
}

func (s *SFUStorage) Tracks(roomID uuid.UUID) ([]*models.PublishedTrack, error) {
	const op = "SFUStorage.Tracks"

	s.mu.RLock()
	defer s.mu.RUnlock()

	room, ok := s.rooms[roomID]
	if !ok {
		err := fmt.Errorf("room %s does not exist", roomID)
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	tracks := make([]*models.PublishedTrack, 0, len(room.Tracks))
	for _, track := range room.Tracks {
		tracks = append(tracks, track)
	}

	return tracks, nil
}

func (s *SFUStorage) cancelRoomCleanupLocked(roomID uuid.UUID) {
	timer, ok := s.roomCleanupTimers[roomID]
	if !ok {
		return
	}

	timer.Stop()
	delete(s.roomCleanupTimers, roomID)
}

func (s *SFUStorage) scheduleRoomCleanupLocked(roomID uuid.UUID) {
	s.cancelRoomCleanupLocked(roomID)

	var timer *time.Timer
	timer = time.AfterFunc(s.emptyRoomTTL, func() {
		s.mu.Lock()
		defer s.mu.Unlock()

		currentTimer, ok := s.roomCleanupTimers[roomID]
		if !ok || currentTimer != timer {
			return
		}

		room, ok := s.rooms[roomID]
		if !ok {
			delete(s.roomCleanupTimers, roomID)
			return
		}
		if len(room.Peers) != 0 {
			delete(s.roomCleanupTimers, roomID)
			return
		}

		delete(s.rooms, roomID)
		delete(s.roomCleanupTimers, roomID)
	})

	s.roomCleanupTimers[roomID] = timer
}
