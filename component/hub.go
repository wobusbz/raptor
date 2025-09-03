package component

import (
	"errors"
	"fmt"
	"game/internal/message"
	"game/session"
	"log"
	"sync"
)

type Components struct {
	services    map[string]*Service
	components  map[string]Component
	sessionPool session.SessionPool
	mu          sync.RWMutex
	started     bool
	stopOnce    sync.Once
}

func NewComponents(sessionPool session.SessionPool) *Components {
	return &Components{
		services:    make(map[string]*Service),
		components:  make(map[string]Component),
		sessionPool: sessionPool,
	}
}

func (cs *Components) Register(name string, c Component) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if _, ok := cs.components[name]; ok {
		return fmt.Errorf("[Components/Register] component %s already registered", name)
	}

	cs.components[name] = c

	if cs.started {
		if err := cs.initComponent(name, c); err != nil {
			delete(cs.components, name)
			return fmt.Errorf("[Components/Register] failed to initialize component %s: %w", name, err)
		}
	}
	return nil
}

func (cs *Components) Unregister(name string) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	component, ok := cs.components[name]
	if !ok {
		return fmt.Errorf("[Components/Unregister] component %s not found", name)
	}

	if service, ok := cs.services[name]; ok {
		if err := service.Stop(); err != nil {
			log.Printf("[Components/Unregister] failed to stop service %s: %v", name, err)
		}
		delete(cs.services, name)
	}

	component.Shutdown()
	delete(cs.components, name)

	log.Printf("[Components/Unregister] unregistered component: %s", name)
	return nil
}

func (cs *Components) initComponent(name string, c Component) error {
	c.Init()

	service := NewService(c, cs.sessionPool)
	if service == nil {
		return fmt.Errorf("failed to create service for component %s", name)
	}

	cs.services[name] = service
	log.Printf("[Components/Init] initialized component: %s", name)
	return nil
}

func (cs *Components) Start() error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if cs.started {
		return errors.New("[Components/Start] components already started")
	}

	for name, component := range cs.components {
		if err := cs.initComponent(name, component); err != nil {
			cs.cleanupServices()
			return fmt.Errorf("[Components/Start] failed to initialize component %s: %w", name, err)
		}
	}

	cs.started = true
	log.Printf("[Components/Start] all components started successfully")
	return nil
}

func (cs *Components) Stop() error {
	var err error
	cs.stopOnce.Do(func() {
		cs.mu.Lock()
		defer cs.mu.Unlock()

		if !cs.started {
			err = errors.New("[Components/Stop] components not started")
			return
		}
		cs.cleanupServices()
		for _, service := range cs.services {
			service.Stop()
		}
		cs.started = false
		log.Printf("[Components/Stop] all components stopped")
	})
	return err
}

func (cs *Components) cleanupServices() {
	for name, service := range cs.services {
		if err := service.Stop(); err != nil {
			log.Printf("[Components/Cleanup] failed to stop service %s: %v", name, err)
		}
	}
	cs.services = make(map[string]*Service)
}

func (cs *Components) IsStarted() bool {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.started
}

func (cs *Components) GetComponentNames() []string {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	names := make([]string, 0, len(cs.components))
	for name := range cs.components {
		names = append(names, name)
	}
	return names
}

func (cs *Components) HasComponent(name string) bool {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	_, exists := cs.components[name]
	return exists
}

func (cs *Components) Tell(componentName string, msg *message.Message) error {
	cs.mu.RLock()
	service, exists := cs.services[componentName]
	cs.mu.RUnlock()

	if !exists {
		return fmt.Errorf("[Components/Tell] component %s not found", componentName)
	}

	if !service.IsRunning() {
		return fmt.Errorf("[Components/Tell] component %s is not running", componentName)
	}

	return service.Tell(msg)
}

func (cs *Components) Ask(componentName string, msg *message.Message) (*message.Message, error) {
	cs.mu.RLock()
	service, exists := cs.services[componentName]
	cs.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("[Components/Ask] component %s not found", componentName)
	}

	if !service.IsRunning() {
		return nil, fmt.Errorf("[Components/Ask] component %s is not running", componentName)
	}

	return service.Ask(msg)
}

func (cs *Components) Broadcast(msg *message.Message) error {
	cs.mu.RLock()
	services := make([]*Service, 0, len(cs.services))
	for _, service := range cs.services {
		if service.IsRunning() {
			services = append(services, service)
		}
	}
	cs.mu.RUnlock()

	var errs []error
	for _, service := range services {
		errs = append(errs, service.Tell(msg))
	}
	return errors.Join(errs...)
}

func (cs *Components) Route(msg *message.Message) error {
	componentName, handlerName := cs.parseRoute(msg.Route)
	if componentName == "" {
		return fmt.Errorf("[Components/Route] invalid route format: %s", msg.Route)
	}
	cs.mu.RLock()
	service, ok := cs.services[componentName]
	cs.mu.RUnlock()

	if !ok {
		return fmt.Errorf("[Components/Route] component %s not found for route %s", componentName, msg.Route)
	}
	if !service.IsRunning() {
		return fmt.Errorf("[Components/Route] component %s is not running", componentName)
	}
	if handlerName != "" && !service.HasHandler(handlerName) {
		return fmt.Errorf("[Components/Route] handler %s not found in component %s", handlerName, componentName)
	}
	return service.Tell(msg)
}

func (cs *Components) parseRoute(route string) (componentName, handlerName string) {
	for i, char := range route {
		if char == '.' {
			return route[:i], route[i+1:]
		}
	}
	return route, ""
}

func (cs *Components) OnSessionConnect(sess session.Session) {
	cs.mu.RLock()
	services := make([]*Service, 0, len(cs.services))
	for _, service := range cs.services {
		services = append(services, service)
	}
	cs.mu.RUnlock()

	log.Printf("[Components/SessionConnect] notifying %d components about session %d", len(services), sess.ID())
	for _, service := range services {
		service.tell(&Message{typ: onSessionConnect, s: sess})
	}
}

func (cs *Components) OnSessionDisconnect(sess session.Session) {
	cs.mu.RLock()
	services := make([]*Service, 0, len(cs.services))
	for _, service := range cs.services {
		services = append(services, service)
	}
	cs.mu.RUnlock()

	for _, service := range services {
		service.tell(&Message{typ: onSessionDisconnect, s: sess})
	}
}

func (cs *Components) GetService(name string) (*Service, bool) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	service, exists := cs.services[name]
	return service, exists
}

func (cs *Components) GetComponent(name string) (Component, bool) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	component, exists := cs.components[name]
	return component, exists
}
