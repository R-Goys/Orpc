package XClient

import (
	"context"
	Orpc "github.com/R-Goys/Orpc/server"
	"io"
	"reflect"
	"sync"
)

type XClient struct {
	d       Discovery
	mode    SelectMode
	opt     *Orpc.Option
	mu      sync.Mutex
	clients map[string]*Orpc.Client
}

func (X *XClient) Close() error {
	X.mu.Lock()
	defer X.mu.Unlock()
	for key, client := range X.clients {
		_ = client.Close()
		delete(X.clients, key)
	}
	return nil
}
func NewXClient(d Discovery, mode SelectMode, opt *Orpc.Option) *XClient {
	return &XClient{d: d, mode: mode, opt: opt, clients: make(map[string]*Orpc.Client)}
}

var _ io.Closer = (*XClient)(nil)

func (x *XClient) dial(rpcAddr string) (*Orpc.Client, error) {
	x.mu.Lock()
	defer x.mu.Unlock()
	client, ok := x.clients[rpcAddr]
	if ok && !client.IsAvailable() {
		_ = client.Close()
		delete(x.clients, rpcAddr)
		return client, nil
	}
	if client == nil {
		var err error
		client, err = Orpc.XDial(rpcAddr, x.opt)
		if err != nil {
			return nil, err
		}
		x.clients[rpcAddr] = client
	}
	return client, nil
}

func (x *XClient) call(rpcAddr string, ctx context.Context, serviceMethod string, args, reply interface{}) error {
	client, err := x.dial(rpcAddr)
	if err != nil {
		return err
	}
	return client.Call(ctx, serviceMethod, args, reply)
}

func (x *XClient) Call(ctx context.Context, serviceMethod string, args, reply interface{}) error {
	rpcAddr, err := x.d.Get(x.mode)
	if err != nil {
		return err
	}
	return x.call(rpcAddr, ctx, serviceMethod, args, reply)
}

func (x *XClient) Broadcast(ctx context.Context, serviceMethod string, args, reply interface{}) error {
	servers, err := x.d.GetAll()
	if err != nil {
		return err
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	var e error
	replyDone := reply == nil
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	for _, rpcAddr := range servers {
		wg.Add(1)
		go func(rpcAddr string) {
			defer wg.Done()
			var clonedReply interface{}
			if reply != nil {
				clonedReply = reflect.New(reflect.ValueOf(reply).Elem().Type()).Interface()
			}
			err = x.call(rpcAddr, ctx, serviceMethod, args, clonedReply)
			mu.Lock()
			if err != nil && e == nil {
				e = err
				cancel()
			}
			if err == nil && !replyDone {
				reflect.ValueOf(reply).Elem().Set(reflect.ValueOf(clonedReply).Elem())
				replyDone = true
			}
			mu.Unlock()
		}(rpcAddr)
	}
	wg.Wait()
	return e
}
