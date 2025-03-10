package test

import (
	"fmt"
	Orpc "github.com/R-Goys/Orpc/server"
	"net"
	"os"
	"runtime"
	"testing"
)

type Foo int

type Args struct {
	num1, num2 int
}

func (Foo) Sum(argv Args, reply *int) error {
	*reply = argv.num2 + argv.num2
	return nil
}
func (Foo) sum(argv Args, reply *int) error {
	*reply = argv.num2 + argv.num2
	return nil
}

func _assert(condition bool, msg string, v ...interface{}) {
	if !condition {
		panic(fmt.Sprintf("assertion failed: "+msg, v...))
	}
}

func Test_XDial(t *testing.T) {
	if runtime.GOOS == "linux" {
		ch := make(chan struct{})
		addr := "/tmp/geerpc.sock"
		go func() {
			_ = os.Remove(addr)
			l, err := net.Listen("unix", addr)
			if err != nil {
				t.Fatal("failed to listen unix socket")
			}
			ch <- struct{}{}
			Orpc.Accept(l)
		}()
		<-ch
		_, err := Orpc.XDial("unix@" + addr)
		_assert(err == nil, "failed to connect unix socket")
	}
}
