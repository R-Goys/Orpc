package main

import (
	Orpc "github.com/R-Goys/Orpc/server"
	"log"
	"net"
	"sync"
	"time"
)

type Foo int

type Args struct {
	Num1, Num2 int
}

func (Foo) Sum(argv Args, reply *int) error {
	*reply = argv.Num2 + argv.Num1
	return nil
}

func startServer(addr chan string) {
	var foo Foo
	if err := Orpc.Register(&foo); err != nil {
		log.Fatal("register error:", err)
	}
	// pick a free port
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		log.Fatal("network error:", err)
	}
	log.Println("start rpc server on", l.Addr())
	addr <- l.Addr().String()
	Orpc.Accept(l)
}

func main() {
	log.SetFlags(0)
	addr := make(chan string)
	go startServer(addr)
	client, _ := Orpc.Dial("tcp", <-addr)
	defer func() { _ = client.Close() }()

	time.Sleep(time.Second)
	// send request & receive response
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			args := &Args{Num1: i, Num2: i * i}
			var reply int
			if err := client.Call("Foo.Sum", args, &reply); err != nil {
				log.Fatal("call Foo.Sum error:", err)
			}
			log.Printf("%d + %d = %d", args.Num1, args.Num2, reply)
		}(i)
	}
	wg.Wait()
}
