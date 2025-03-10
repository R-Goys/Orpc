package Orpc

import (
	"go/ast"
	"log"
	"reflect"
	"sync/atomic"
)

type MethodType struct {
	Method    reflect.Method
	ArgType   reflect.Type
	ReplyType reflect.Type
	numCalls  uint64
}

type Service struct {
	Name   string
	rcvr   reflect.Value //表示一个结构体实例，用于方法调用
	typ    reflect.Type  //这表示一个结构体
	Method map[string]*MethodType
}

func (m *MethodType) NumCalls() uint64 {
	return atomic.LoadUint64(&m.numCalls)
}

func (m *MethodType) NewArgv() reflect.Value {
	var argv reflect.Value
	if m.ArgType.Kind() == reflect.Ptr {
		argv = reflect.New(m.ArgType.Elem())
	} else {
		argv = reflect.New(m.ArgType).Elem()
	}
	return argv
}

func (m *MethodType) NewReplyv() reflect.Value {
	repliv := reflect.New(m.ReplyType.Elem())
	switch m.ReplyType.Kind() {
	case reflect.Map:
		repliv.Elem().Set(reflect.MakeMap(m.ReplyType.Elem()))
	case reflect.Slice:
		repliv.Elem().Set(reflect.MakeSlice(m.ReplyType.Elem(), 0, 0))
	case reflect.String:
		repliv.Elem().Set(reflect.ValueOf(m.ReplyType.Elem()))
	default:
		repliv.Elem().Set(reflect.Zero(m.ReplyType.Elem()))
	}
	return repliv
}

func NewService(rcvr interface{}) *Service {
	s := new(Service)
	s.rcvr = reflect.ValueOf(rcvr)
	s.Name = reflect.Indirect(s.rcvr).Type().Name()
	s.typ = s.rcvr.Type()
	if !ast.IsExported(s.Name) {
		log.Fatalf("rpc server: %s is not a valid service Name", s.Name)
	}
	s.registerMethods()
	return s
}

func (s *Service) registerMethods() {
	s.Method = make(map[string]*MethodType)
	for i := 0; i < s.typ.NumMethod(); i++ {
		method := s.typ.Method(i)
		mType := method.Type
		if mType.NumIn() != 3 || mType.NumOut() != 1 {
			continue
		}
		if mType.Out(0) != reflect.TypeOf((*error)(nil)).Elem() {
			continue
		}
		argType, replyType := mType.In(1), mType.In(2)
		if !isExportedOrBuiltinType(argType) || !isExportedOrBuiltinType(replyType) {
			continue
		}
		s.Method[method.Name] = &MethodType{
			Method:    method,
			ArgType:   argType,
			ReplyType: replyType,
		}
		log.Printf("Orpc server: register Method: %s %s", method.Name, argType.String())
	}
}

func isExportedOrBuiltinType(t reflect.Type) bool {
	return ast.IsExported(t.Name()) || t.PkgPath() == ""
}

func (s *Service) Call(m *MethodType, argv, replyv reflect.Value) error {
	atomic.AddUint64(&m.numCalls, 1)
	f := m.Method.Func
	returnValues := f.Call([]reflect.Value{s.rcvr, argv, replyv})
	if errInter := returnValues[0].Interface(); errInter != nil {
		return errInter.(error)
	}
	return nil
}
