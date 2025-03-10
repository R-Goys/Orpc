package main

import (
	"fmt"
	Orpc "github.com/R-Goys/Orpc/server"
	"log"
	"net"
	"sync"
)

func startServer(addr chan string) {
	// pick a free port
	l, err := net.Listen("tcp", ":10001")
	if err != nil {
		log.Fatal("network error:", err)
	}
	log.Println("start rpc server on", l.Addr())
	addr <- l.Addr().String()
	Orpc.DefaultServer.Accept(l)

}
func main() {
	addr := make(chan string, 5)
	go startServer(addr)
	client, _ := Orpc.Dial("tcp", <-addr)
	defer func() { _ = client.Close() }()
	//time.Sleep(time.Second * 2)
	// send request & receive response
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			args := fmt.Sprintf("geerpc req %d", i)
			var reply string
			if err := client.Call("Foo.Sum", args, &reply); err != nil {
				log.Fatal("call Foo.Sum error:", err)
			}
			log.Println("reply:", reply)
		}(i)
	}
	wg.Wait()
}
