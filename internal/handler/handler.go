package handler

import (
	"fmt"
	"reflect"
	"sync"

	"github.com/pkg/errors"
)

type Handler struct {
	// Synchronous controls whether to spawn each event handler in its own
	// goroutine. Default false (meaning goroutines are spawned).
	Synchronous bool

	handlers map[uint64]handler
	hserial  uint64
	hmutex   sync.Mutex
}

func New() *Handler {
	return &Handler{
		handlers: map[uint64]handler{},
	}
}

func (h *Handler) Call(ev interface{}) {
	var evV = reflect.ValueOf(ev)
	var evT = evV.Type()

	h.hmutex.Lock()
	defer h.hmutex.Unlock()

	for _, handler := range h.handlers {
		if handler.not(evT) {
			continue
		}

		if h.Synchronous {
			handler.call(evV)
		} else {
			go handler.call(evV)
		}
	}
}

func (h *Handler) AddHandler(handler interface{}) (rm func()) {
	rm, err := h.addHandler(handler)
	if err != nil {
		panic(err)
	}
	return rm
}

// AddHandlerCheck adds the handler, but safe-guards reflect panics with a
// recoverer, returning the error.
func (h *Handler) AddHandlerCheck(handler interface{}) (rm func(), err error) {
	// Reflect would actually panic if anything goes wrong, so this is just in
	// case.
	defer func() {
		if rec := recover(); rec != nil {
			if recErr, ok := rec.(error); ok {
				err = recErr
			} else {
				err = fmt.Errorf("%v", rec)
			}
		}
	}()

	return h.addHandler(handler)
}

func (h *Handler) addHandler(handler interface{}) (rm func(), err error) {
	// Reflect the handler
	r, err := reflectFn(handler)
	if err != nil {
		return nil, errors.Wrap(err, "Handler reflect failed")
	}

	h.hmutex.Lock()
	defer h.hmutex.Unlock()

	// Get the current counter value and increment the counter
	serial := h.hserial
	h.hserial++

	// Use the serial for the map
	h.handlers[serial] = *r

	return func() {
		h.hmutex.Lock()
		defer h.hmutex.Unlock()

		delete(h.handlers, serial)
	}, nil
}

type handler struct {
	event    reflect.Type
	callback reflect.Value
	isIface  bool
}

func reflectFn(function interface{}) (*handler, error) {
	fnV := reflect.ValueOf(function)
	fnT := fnV.Type()

	if fnT.Kind() != reflect.Func {
		return nil, errors.New("given interface is not a function")
	}

	if fnT.NumIn() != 1 {
		return nil, errors.New("function can only accept 1 event as argument")
	}

	argT := fnT.In(0)
	kind := argT.Kind()

	// Accept either pointer type or interface{} type
	if kind != reflect.Ptr && kind != reflect.Interface {
		return nil, errors.New("first argument is not pointer")
	}

	return &handler{
		event:    argT,
		callback: fnV,
		isIface:  kind == reflect.Interface,
	}, nil
}

func (h handler) not(event reflect.Type) bool {
	if h.isIface {
		return !event.Implements(h.event)
	}

	return h.event != event
}

func (h handler) call(event reflect.Value) {
	h.callback.Call([]reflect.Value{event})
}
