package session

import (
	"maps"
	"sync"
)

type Router struct {
	routesRw sync.RWMutex
	routes   map[string]string
}

func NewRouter() *Router {
	return &Router{routes: map[string]string{}}
}

func (r *Router) BindServer(server string, instanceId string) {
	r.routesRw.Lock()
	r.routes[server] = instanceId
	r.routesRw.Unlock()
}

func (r *Router) DeleteServer(server string) {
	r.routesRw.Lock()
	delete(r.routes, server)
	r.routesRw.Unlock()
}

func (r *Router) FindServer(server string) (string, bool) {
	r.routesRw.RLock()
	defer r.routesRw.RUnlock()
	instanceId, ok := r.routes[server]
	return instanceId, ok
}

func (r *Router) Routers() map[string]string {
	var instances = make(map[string]string, len(r.routes))
	r.routesRw.RLock()
	defer r.routesRw.RUnlock()
	maps.Copy(instances, r.routes)
	return instances
}
