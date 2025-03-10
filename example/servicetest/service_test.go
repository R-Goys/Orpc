package t_test

import (
	"fmt"
	"github.com/R-Goys/Orpc/server"
	"reflect"
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

func Test_Service(t *testing.T) {
	var foo Foo
	s := Orpc.NewService(&foo)
	_assert(len(s.Method) == 1, "method should have 1 parameter but got ", len(s.Method))
	mType := s.Method["Sum"]
	argv := mType.NewArgv()
	replyv := mType.NewReplyv()
	argv.Set(reflect.ValueOf(Args{num1: 2, num2: 2}))
	err := s.Call(mType, argv, replyv)
	_assert(err == nil && *replyv.Interface().(*int) == 4 && mType.NumCalls() == 1, "failed to call Foo.Sum")
}
