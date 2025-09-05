package stream

import (
	"fmt"
	"sync"
)

type StreamClientManager struct {
	clients    map[string]map[string]*streamClient
	clientsMux sync.RWMutex
}

func NewStreamClientManager() *StreamClientManager {
	return &StreamClientManager{clients: map[string]map[string]*streamClient{}}
}

func (s *StreamClientManager) AddStreamClient(svrname, instanceId, addr string) (StreamClient, error) {
	s.clientsMux.Lock()
	defer s.clientsMux.Unlock()
	stream, err := NewStreamClient(addr)
	if err != nil {
		return nil, fmt.Errorf("[StreamClientManager/AddStreamClient] AddStreamClient %w", err)
	}
	if _, ok := s.clients[svrname]; !ok {
		s.clients[svrname] = map[string]*streamClient{}
	}
	s.clients[svrname][instanceId] = stream
	return stream, nil
}

func (s *StreamClientManager) DelStreamClient(svrname, instanceId string) {
	s.clientsMux.Lock()
	defer s.clientsMux.Unlock()
	if _, ok := s.clients[instanceId]; !ok {
		return
	}
	delete(s.clients[svrname], instanceId)
}

func (s *StreamClientManager) GetStreamClient(svrname, instanceId, addr string) (StreamClient, error) {
	s.clientsMux.RLock()
	stream, ok := s.clients[svrname][instanceId]
	if ok {
		s.clientsMux.RUnlock()
		return stream, nil
	}
	s.clientsMux.RUnlock()
	return s.AddStreamClient(svrname, instanceId, addr)
}

func (s *StreamClientManager) Shutdown() {
	for _, clients := range s.clients {
		for _, v := range clients {
			v.Close()
		}
	}
	s.clients = nil
}
