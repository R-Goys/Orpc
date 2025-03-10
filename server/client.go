package Orpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/R-Goys/Orpc/codec"
	"log"
	"net"
	"strconv"
	"sync"
	"time"
)

type Call struct {
	Seq           uint64
	ServiceMethod string
	Args          interface{}
	Reply         interface{}
	Error         error
	Done          chan *Call
}

type Option struct {
	MagicNumber    int
	CodecType      codec.Type
	ConnectTimeOut time.Duration
	HandleTimeout  time.Duration
}

type clientResult struct {
	client *Client
	err    error
}

var DefaultOption = &Option{
	MagicNumber:    MagicNumber,
	CodecType:      codec.GobType,
	ConnectTimeOut: 1 * time.Second,
	HandleTimeout:  1 * time.Second,
}

func (call *Call) done() {
	call.Done <- call
}

type Client struct {
	cc       codec.Codec //编解码器
	opt      *Option
	sending  sync.Mutex //保证消息有序发送
	header   codec.Header
	calls    []*Call
	mu       sync.Mutex
	seq      uint64           //给请求编号
	pending  map[uint64]*Call //存储未处理完的请求
	closing  bool             //用户关闭
	shutdown bool             //错误关闭
}

var ErrShutdown = errors.New("connection is shut down")

func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closing {
		return ErrShutdown
	}
	c.closing = true
	return nil
}

func (c *Client) IsAvailable() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return !c.closing && !c.shutdown
}

func (c *Client) RegisterCall(call *Call) (uint64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closing || c.shutdown {
		return 0, ErrShutdown
	}
	call.Seq = c.seq
	c.pending[call.Seq] = call
	c.seq++
	return call.Seq, nil
}

func (c *Client) RemoveCall(seq uint64) *Call {
	c.mu.Lock()
	defer c.mu.Unlock()
	call, ok := c.pending[seq]
	if !ok {
		return nil
	}
	delete(c.pending, seq)
	return call
}

func (c *Client) TerminateCall(err error) {
	c.sending.Lock()
	defer c.sending.Unlock()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.shutdown = true
	for _, call := range c.pending {
		call.Error = err
		call.done()
	}
}

func (c *Client) Receive() {
	var err error
	for err == nil {
		var header codec.Header
		if err = c.cc.ReadHeader(&header); err != nil {
			break
		}
		call := c.RemoveCall(header.Seq)
		switch {
		case call == nil:
			err = c.cc.ReadBody(nil)
		case header.Error != "":
			call.Error = fmt.Errorf(header.Error)
			err = c.cc.ReadBody(nil)
			call.done()
		default:
			err = c.cc.ReadBody(call.Reply)
			if err != nil {
				call.Error = errors.New("Orpc client error :read body error: " + err.Error())
			}
			call.done()
		}
	}
	c.TerminateCall(err)
}

func NewClient(conn net.Conn, opt *Option) (*Client, error) {
	f := codec.NewCodecFuncMap[opt.CodecType]
	if f == nil {
		err := fmt.Errorf("orpc client codec err: %s", opt.CodecType)
		log.Println(err)
		return nil, nil
	}
	if err := json.NewEncoder(conn).Encode(opt); err != nil {
		log.Println("Orpc client encode error:", err)
		_ = conn.Close()
		return nil, nil
	}
	return newClientCodec(f(conn), opt), nil
}

func newClientCodec(cc codec.Codec, opt *Option) *Client {
	newClient := &Client{
		cc:       cc,
		opt:      opt,
		pending:  make(map[uint64]*Call),
		seq:      1,
		closing:  false,
		shutdown: false,
	}
	go newClient.Receive()
	return newClient
}

func parseOption(opts ...*Option) (*Option, error) {
	if len(opts) == 0 || opts[0] == nil {
		return DefaultOption, nil
	}
	if len(opts) > 1 {
		return nil, errors.New("too many options")
	}
	opt := opts[0]
	opt.MagicNumber = MagicNumber
	return opt, nil
}

type newClientFunc func(conn net.Conn, opt *Option) (client *Client, err error)

func dialTimeout(f newClientFunc, network, address string, opts ...*Option) (client *Client, err error) {
	opt, err := parseOption(opts...)
	if err != nil {
		return nil, err
	}
	conn, err := net.DialTimeout(network, address, opt.ConnectTimeOut)
	if err != nil {
		return nil, err
	}
	defer func() {
		if client == nil {
			_ = conn.Close()
		}
	}()
	ch := make(chan clientResult)

	go func() {
		client, err = f(conn, opt)
		ch <- clientResult{client, err}
	}()
	if opt.ConnectTimeOut == 0 {
		result := <-ch
		return result.client, result.err
	}
	select {
	case result := <-ch:
		return result.client, result.err
	case <-time.After(opt.ConnectTimeOut):
		return nil, errors.New("connect timeout expect within " + strconv.FormatInt(int64(opt.ConnectTimeOut), 10))
	}
}

func Dial(network, address string, opts ...*Option) (*Client, error) {
	return dialTimeout(NewClient, network, address, opts...)
}

func (c *Client) Send(call *Call) {
	c.sending.Lock()
	defer c.sending.Unlock()
	//注册这次call
	seq, err := c.RegisterCall(call)
	if err != nil {
		call.Error = err
		call.done()
		return
	}
	//准备请求头
	c.header.Seq = seq
	c.header.ServiceMethod = call.ServiceMethod
	c.header.Error = ""
	//发送
	if err = c.cc.Write(&c.header, call.Args); err != nil {
		call = c.RemoveCall(seq)
		if call != nil {
			call.Error = err
			call.done()
		}
	}
}

func (c *Client) Go(serviceMethod string, args, reply interface{}, done chan *Call) *Call {
	if done == nil {
		done = make(chan *Call, 10)
	} else if cap(done) == 0 {
		log.Panic("done channel is unbuffered")
	}
	call := &Call{
		ServiceMethod: serviceMethod,
		Args:          args,
		Reply:         reply,
		Done:          done,
	}
	c.Send(call)
	return call
}

func (c *Client) Call(ctx context.Context, serviceMethod string, args, reply interface{}) error {
	call := c.Go(serviceMethod, args, reply, make(chan *Call, 1))
	select {
	case <-ctx.Done():
		return ctx.Err()
	case call = <-call.Done:
		return call.Error
	}
}
