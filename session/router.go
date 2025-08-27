package session

import (
	"game/networkentity"
	"sync"
)

type Router struct {
	routesRw sync.RWMutex
	routes   map[string]networkentity.NetworkEntity
}

func NewRouter() *Router {
	return &Router{routes: map[string]networkentity.NetworkEntity{}}
}

func (r *Router) BindServer(server string, entity networkentity.NetworkEntity) {
	r.routesRw.Lock()
	r.routes[server] = entity
	r.routesRw.Unlock()
}

func (r *Router) DeleteServer(server string) {
	r.routesRw.Lock()
	delete(r.routes, server)
	r.routesRw.Unlock()
}

func (r *Router) FindServer(server string) (networkentity.NetworkEntity, bool) {
	r.routesRw.RLock()
	defer r.routesRw.RUnlock()
	entity, ok := r.routes[server]
	return entity, ok
}
