package Orpc

import (
	"encoding/json"
	"errors"
	"github.com/R-Goys/Orpc/codec"
	"io"
	"log"
	"net"
	"reflect"
	"strings"
	"sync"
)

const MagicNumber = 0x3bef5c

type Option struct {
	MagicNumber int
	CodecType   codec.Type
}

var DefaultOption = &Option{
	MagicNumber: MagicNumber,
	CodecType:   codec.GobType,
}

type Server struct {
	serviceMap sync.Map
}

func NewServer() *Server {
	return &Server{
		serviceMap: sync.Map{},
	}
}

// Register 服务注册
func (server *Server) Register(rcvr interface{}) error {
	s := NewService(rcvr)
	//使用的是并发安全的map，如果存在，则返回错误，
	if _, ok := server.serviceMap.LoadOrStore(s.Name, s); ok {
		return errors.New("Orpc service already defined " + s.Name)
	}
	return nil
}

func Register(rcvr interface{}) error { return DefaultServer.Register(rcvr) }

// FindService 根据服务来查找相应的方法并加载，
func (server *Server) FindService(serviceMethod string) (svc *Service, mtype *MethodType, err error) {
	dot := strings.LastIndex(serviceMethod, ".")
	if dot < 0 {
		err = errors.New("rpc server: service/method request ill-formed: " + serviceMethod)
		return
	}
	serviceName, methodName := serviceMethod[:dot], serviceMethod[dot+1:]
	svci, ok := server.serviceMap.Load(serviceName)
	if !ok {
		err = errors.New("rpc server: service not found: " + serviceName)
		return
	}
	svc = svci.(*Service)
	mtype = svc.Method[methodName]
	if mtype == nil {
		err = errors.New("rpc server: method not found: " + methodName)
	}
	return
}

var DefaultServer = NewServer()

func (s *Server) Accept(lis net.Listener) {
	for {
		conn, err := lis.Accept()
		if err != nil {
			log.Println("Orpc server: Accept error ", err)
			return
		}
		go s.ServeConn(conn)
	}
}

func Accept(lis net.Listener) {
	DefaultServer.Accept(lis)
}

func (s *Server) ServeConn(conn io.ReadWriteCloser) {
	defer func() { _ = conn.Close() }()
	var opt Option

	if err := json.NewDecoder(conn).Decode(&opt); err != nil {
		log.Println("Orpc server: options Decode error ", err)
		return
	}
	if opt.MagicNumber != MagicNumber {
		log.Println("Orpc server: invalid magic number ", opt.MagicNumber)
		return
	}
	f := codec.NewCodecFuncMap[opt.CodecType]
	if f == nil {
		log.Println("Orpc server: invalid codec type ", opt.CodecType)
		return
	}
	s.serveCodec(f(conn))
}

var invalidRequest = struct{}{}

func (s *Server) serveCodec(cc codec.Codec) {
	sending := new(sync.Mutex)
	wg := new(sync.WaitGroup)

	for {
		req, err := s.readRequest(cc)
		if err != nil {
			if req == nil {
				break
			}
			req.header.Error = err.Error()
			s.sendResponse(cc, req.header, invalidRequest, sending)
			continue
		}
		wg.Add(1)
		go s.handleRequest(cc, req, sending, wg)
	}
	wg.Wait()
	_ = cc.Close()
}

type request struct {
	header       *codec.Header
	argv, replyv reflect.Value
	mtype        *MethodType
	svc          *Service
}

func (s *Server) readRequestHeader(cc codec.Codec) (*codec.Header, error) {
	var h codec.Header
	if err := cc.ReadHeader(&h); err != nil {
		if err != io.EOF || !errors.Is(err, io.ErrUnexpectedEOF) {
			log.Println("Orpc server: read header error ", err)
		}
		return nil, err
	}
	return &h, nil
}

func (s *Server) readRequest(cc codec.Codec) (*request, error) {
	h, err := s.readRequestHeader(cc)
	if err != nil {
		log.Println("Orpc server: read header error ", err)
		return nil, err
	}
	req := &request{
		header: h,
	}
	//拿到服务实例和方法
	req.svc, req.mtype, err = s.FindService(h.ServiceMethod)
	if err != nil {
		return nil, err
	}
	//根据调用方法返回输入输出数值的指针
	req.argv = req.mtype.NewArgv()
	req.replyv = req.mtype.NewReplyv()

	argvi := req.argv.Interface()
	if req.argv.Type().Kind() != reflect.Ptr {
		argvi = req.argv.Addr().Interface()
	}
	if err = cc.ReadBody(argvi); err != nil {
		log.Println("Orpc server: read request body error", err)
		return nil, err
	}
	return req, nil
}

func (s *Server) sendResponse(cc codec.Codec, h *codec.Header, body interface{}, sending *sync.Mutex) {
	sending.Lock()
	defer sending.Unlock()
	if err := cc.Write(h, body); err != nil {
		log.Println("Orpc server: write response error ", err)
	}
}

func (s *Server) handleRequest(cc codec.Codec, req *request, sending *sync.Mutex, wg *sync.WaitGroup) {
	defer wg.Done()
	err := req.svc.Call(req.mtype, req.argv, req.replyv)
	if err != nil {
		req.header.Error = err.Error()
		s.sendResponse(cc, req.header, invalidRequest, sending)
		return
	}
	s.sendResponse(cc, req.header, req.replyv.Interface(), sending)
}
