package server

import "game/component"

func (s *Server) Register(comp component.Component) {
	s.components.Register(comp)
}
