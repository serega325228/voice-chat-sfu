package storage

import (
	"fmt"
	"sync"
	"voice-chat-sfu/internal/models"

	"github.com/google/uuid"
)

type SFUStorage struct {
	mu    sync.RWMutex
	rooms map[uuid.UUID]*models.Room
}

func NewSFUStorage() *SFUStorage {
	return &SFUStorage{
		rooms: make(map[uuid.UUID]*models.Room),
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

func (s *SFUStorage) RemovePeer(roomID, peerID uuid.UUID) (*models.Peer, error) {
	const op = "SFUStorage.RemovePeer"

	s.mu.Lock()
	defer s.mu.Unlock()

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

	delete(room.Peers, peerID)

	for key, track := range room.Tracks {
		if track.PublisherID == peerID {
			delete(room.Tracks, key)
		}
	}

	if len(room.Peers) == 0 {
		delete(s.rooms, roomID)
	}

	return peer, nil
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
