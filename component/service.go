package component

import (
	"errors"
	"fmt"
	"game/internal/message"
	"game/pcall"
	"game/session"
	"log"
	"reflect"

	"google.golang.org/protobuf/proto"
)

type (
	Handler struct {
		Receiver reflect.Value
		Method   reflect.Method
		Type     reflect.Type
	}
	Message struct {
		message *message.Message
		Reply   chan *message.Message
	}
	Service struct {
		Name        string
		Type        reflect.Type
		Receiver    reflect.Value
		Handlers    map[string]*Handler
		SchedName   string
		mailbox     chan *Message
		sessionPool session.SessionPool
		stopc       chan struct{}
	}
)

func NewService(comp Component, sessionPool session.SessionPool) *Service {
	s := &Service{
		Type:        reflect.TypeOf(comp),
		Receiver:    reflect.ValueOf(comp),
		mailbox:     make(chan *Message, 102400),
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

func (s *Service) ExtractHandler() error {
	typeName := reflect.Indirect(s.Receiver).Type().Name()
	if typeName == "" {
		return errors.New("no service name for type " + s.Type.String())
	}
	if !isExported(typeName) {
		return errors.New("type " + typeName + " is not exported")
	}
	s.Handlers = s.suitableHandlerMethods(s.Type)

	if len(s.Handlers) == 0 {
		str := ""
		method := s.suitableHandlerMethods(reflect.PointerTo(s.Type))
		if len(method) != 0 {
			str = "type " + s.Name + " has no exported methods of suitable type (hint: pass a pointer to value of that type)"
		} else {
			str = "type " + s.Name + " has no exported methods of suitable type"
		}
		return errors.New(str)
	}
	for i := range s.Handlers {
		s.Handlers[i].Receiver = s.Receiver
	}
	return nil
}

func (s *Service) loopHandler() {
	for {
		select {
		case m := <-s.mailbox:
			session, ok := s.sessionPool.GetSessionByID(int64(m.message.ID))
			if !ok {
				log.Printf("[Service/loopHandler] session[%d] not found \n", m.message.ID)
				continue
			}
			handler, ok := s.Handlers[m.message.Route]
			if !ok {
				log.Printf("[Service/Handlers] handler[%s] not found \n", m.message.Route)
				continue
			}
			data := reflect.New(handler.Type.Elem()).Interface().(proto.Message)
			if err := proto.Unmarshal(m.message.Data, data); err != nil {
				log.Printf("[RemoteMessage/Receive] protobuf[%v] Unmarshal failed \n", err)
				continue
			}
			pcall.Pcall0(handler.Method, []reflect.Value{s.Receiver, reflect.ValueOf(session), reflect.ValueOf(data)})
		case <-s.stopc:
			return
		}
	}
}

func (s *Service) Tell(msg *message.Message) error {
	select {
	case s.mailbox <- &Message{message: msg}:
	default:
		return fmt.Errorf("[Service/Tell] Service(%s) mailbox full", s.Name)
	}
	return nil
}

func (s *Service) Ask(msg *message.Message) (*message.Message, error) {
	var reply = make(chan *message.Message, 1)
	select {
	case s.mailbox <- &Message{message: msg, Reply: reply}:
	default:
		return nil, fmt.Errorf("[Service/Tell] Service(%s) mailbox full", s.Name)
	}
	return <-reply, nil
}
