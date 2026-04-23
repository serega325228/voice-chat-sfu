package storage

import (
	"fmt"
	"sync"
	"voice-chat-sfu/internal/models"

	"github.com/google/uuid"
)

type SFUStorage struct {
	data map[uuid.UUID]*models.Room
	m    *sync.Mutex
}

func NewSFUStorage() *SFUStorage {
	return &SFUStorage{
		data: make(map[uuid.UUID]*models.Room),
		m:    &sync.Mutex{},
	}
}

func (s *SFUStorage) Add(roomID uuid.UUID, creatorPeer *models.Peer) error {
	room := &models.Room{
		ID:    roomID,
		Peers: map[uuid.UUID]*models.Peer{creatorPeer.ID: creatorPeer},
	}
	s.data[roomID] = room

	return nil
}

func (s *SFUStorage) Get(roomID uuid.UUID) (*models.Room, error) {
	s.m.Lock()
	defer s.m.Unlock()
	room, ok := s.data[roomID]
	if !ok {
		return nil, fmt.Errorf("room with this id doesn't exists")
	}
	return room, nil
}

func (s *SFUStorage) Delete(roomID uuid.UUID) error {
	s.m.Lock()
	defer s.m.Unlock()
	_, ok := s.data[roomID]
	if !ok {
		return fmt.Errorf("session with this id doesn't exists")
	}
	delete(s.data, roomID)
	return nil
}

func (s *SFUStorage) RoomExceptAuthor(roomID, peerID uuid.UUID) []*models.Peer {
	peers := make([]*models.Peer, len(s.data[roomID].Peers))
	i := 0
	for _, v := range s.data[roomID].Peers {
		peers[i] = v
		i++
	}
	return peers
}
