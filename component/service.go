package component

import (
	"errors"
	"fmt"
	"game/agent"
	"game/internal/message"
	"game/pcall"
	"game/session"
	"log"
	"reflect"
	"strings"

	"google.golang.org/protobuf/proto"
)

type (
	Handler struct {
		Receiver reflect.Value
		Method   reflect.Method
		Type     reflect.Type
	}

	Message struct {
		typ     typ
		a       *agent.Agent
		s       session.Session
		message *message.Message
		Reply   chan *message.Message
	}

	Service struct {
		Name        string
		comp        Component
		Type        reflect.Type
		Receiver    reflect.Value
		Handlers    map[string]*Handler
		SchedName   string
		mailbox     chan *Message
		sessionPool session.SessionPool
		stopc       chan struct{}
		stopped     bool
	}
)

func NewService(comp Component, sessionPool session.SessionPool) *Service {
	s := &Service{
		comp:        comp,
		Type:        reflect.TypeOf(comp),
		Receiver:    reflect.ValueOf(comp),
		mailbox:     make(chan *Message, 1<<16),
		stopc:       make(chan struct{}),
		sessionPool: sessionPool,
	}
	s.Name = reflect.Indirect(s.Receiver).Type().Name()
	go s.loopHandler()
	return s
}

func (s *Service) suitableHandlerMethods(typ reflect.Type) map[string]*Handler {
	methods := make(map[string]*Handler)
	for m := 0; m < typ.NumMethod(); m++ {
		method := typ.Method(m)
		mt := method.Type
		mn := method.Name
		if isHandlerMethod(method) {
			methods[mn] = &Handler{Method: method, Type: mt.In(2)}
		}
	}
	return methods
}

func (s *Service) ExtractHandler(comps *Components) error {
	if s.Receiver.Kind() == reflect.Invalid {
		return errors.New("[Service/ExtractHandler] invalid receiver")
	}
	typeName := reflect.Indirect(s.Receiver).Type().Name()
	if typeName == "" {
		return fmt.Errorf("[Service/ExtractHandler] no service name for type %s", s.Type.String())
	}
	if !isExported(typeName) {
		return fmt.Errorf("[Service/ExtractHandler] type %s is not exported", typeName)
	}
	s.Handlers = s.suitableHandlerMethods(s.Type)

	if len(s.Handlers) == 0 {
		method := s.suitableHandlerMethods(reflect.PointerTo(s.Type))
		if len(method) != 0 {
			return fmt.Errorf("[Service/ExtractHandler] type %s has no exported methods of suitable type (hint: pass a pointer to value of that type)", s.Name)
		} else {
			return fmt.Errorf("[Service/ExtractHandler] type %s has no exported methods of suitable type", s.Name)
		}
	}
	for _, handler := range s.Handlers {
		if msgInfo, ok := reflect.New(handler.Type.Elem()).Interface().(interface {
			Route() string
			ID() uint32
		}); ok {
			comps.cacheRoute[uint32(msgInfo.ID())] = msgInfo.Route()
		}
		handler.Receiver = s.Receiver
	}
	return nil
}

func (s *Service) loopHandler() {
	for {
		select {
		case m := <-s.mailbox:
			if err := s.localProcessMessage(m); err != nil {
				log.Printf("[Service/%s] process message failed: %v", s.Name, err)
				continue
			}
		case <-s.stopc:
			log.Printf("[Service/%s] stopping message loop", s.Name)
			s.stopped = true
			return
		}
	}
}

func (s *Service) localProcessMessage(m *Message) error {
	switch m.typ {
	case onSessionConnect:
		if m.a != nil {
			s.comp.OnSessionConnect(m.a.Session())
			log.Printf("[Service/%s] session %d connected", s.Name, m.a.Session().ID())
		} else if m.s != nil {
			s.comp.OnSessionConnect(m.s)
			log.Printf("[Service/%s] session %d connected", s.Name, m.s.ID())
		}
	case onSessionDisconnect:
		if m.a != nil {
			s.comp.OnSessionDisconnect(m.a.Session())
			log.Printf("[Service/%s] session %d disconnected", s.Name, m.a.Session().ID())
		} else if m.s != nil {
			s.comp.OnSessionDisconnect(m.s)
			log.Printf("[Service/%s] session %d disconnected", s.Name, m.s.ID())
		}
	default:
		return s.processHandlerMessage(m)
	}
	return nil
}

func (s *Service) processHandlerMessage(m *Message) error {
	handler, ok := s.Handlers[strings.Split(m.message.Route, "/")[2]]
	if !ok {
		return fmt.Errorf("[Service/Handlers] handler[%s] not found", m.message.Route)
	}

	if handler.Type == nil || handler.Type.Kind() != reflect.Pointer {
		return fmt.Errorf("[Service/Handlers] invalid handler type for route[%s]", m.message.Route)
	}

	data := reflect.New(handler.Type.Elem()).Interface().(proto.Message)
	if err := proto.Unmarshal(m.message.Data, data); err != nil {
		return fmt.Errorf("[Service/Process] protobuf unmarshal failed for route[%s]: %w", m.message.Route, err)
	}

	var sess session.Session
	if m.a != nil {
		sess = m.a.Session()
	} else if m.s != nil {
		sess = m.s
	}

	args := []reflect.Value{s.Receiver, reflect.ValueOf(sess), reflect.ValueOf(data)}

	err := pcall.Pcall1(handler.Method, args)
	if err != nil {
		return fmt.Errorf("[Service/%s] handler[%s] returned error: %w", s.Name, m.message.Route, err)
	}

	//log.Printf("[Service/%s] successfully processed message route[%s]", s.Name, m.message.Route)
	return nil
}

func (s *Service) Stop() error {
	if s.stopped {
		return errors.New("[Service/Stop] service already stopped")
	}

	close(s.stopc)

	for !s.stopped {
	}
	s.comp.Shutdown()
	return nil
}

func (s *Service) IsRunning() bool {
	return !s.stopped
}

func (s *Service) HasHandler(route string) bool {
	_, exists := s.Handlers[route]
	return exists
}

func (s *Service) tell(message *Message) error {
	select {
	case s.mailbox <- message:
		return nil
	default:
		return fmt.Errorf("[Service/Tell] Service(%s) mailbox full", s.Name)
	}
}

func (s *Service) Tell(msg *message.Message) error {
	return s.tell(&Message{message: msg})
}

func (s *Service) TellSession(session session.Session, msg *message.Message) error {
	return s.tell(&Message{s: session, message: msg})
}

func (s *Service) Ask(msg *message.Message) (*message.Message, error) {
	var reply = make(chan *message.Message, 1)
	select {
	case s.mailbox <- &Message{message: msg, Reply: reply}:
	default:
		return nil, fmt.Errorf("[Service/Ask] Service(%s) mailbox full", s.Name)
	}
	response := <-reply
	if response.Route == "error" {
		return nil, fmt.Errorf("[Service/Ask] handler error: %s", string(response.Data))
	}
	return response, nil
}
