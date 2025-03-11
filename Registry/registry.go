package Registry

import (
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

type ORegistry struct {
	TimeOut time.Duration
	mu      sync.Mutex
	Server  map[string]*ServerItem
}

type ServerItem struct {
	Addr  string
	start time.Time
}

const (
	defaultPath    = "/Orpc/registry"
	defaultTimeout = time.Second * 5
)

func New(timeout time.Duration) *ORegistry {
	return &ORegistry{
		TimeOut: timeout,
		Server:  make(map[string]*ServerItem),
	}
}

var DefaultORegister = New(defaultTimeout)

func (r *ORegistry) PutServer(addr string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s := r.Server[addr]
	if s == nil {
		r.Server[addr] = &ServerItem{
			Addr:  addr,
			start: time.Now(),
		}
	} else {
		s.start = time.Now()
	}
}

func (r *ORegistry) aliveServers() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var alive []string
	for addr, s := range r.Server {
		if s.start.Add(r.TimeOut).After(time.Now()) || r.TimeOut == 0 {
			alive = append(alive, addr)
		} else {
			delete(r.Server, addr)
		}
	}
	return alive
}

func (r *ORegistry) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case "GET":
		w.Header().Set("X-Orpc-Servers", strings.Join(r.aliveServers(), ","))
	case "POST":
		addr := req.Header.Get("X-Orpc-Server")
		if addr == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		r.PutServer(addr)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (r *ORegistry) HandleHTTP(registryPath string) {
	http.Handle(registryPath, r)
	log.Println("Register HTTP handler", registryPath)
}

func HandleHTTP() {
	DefaultORegister.HandleHTTP(defaultPath)
}

func HeartBeat(registry, addr string, duration time.Duration) {
	if duration == 0 {
		duration = defaultTimeout - time.Second
	}
	var err error
	err = sendHeartBeat(registry, addr)

	go func() {
		t := time.NewTicker(duration)
		defer t.Stop()
		for err == nil {
			<-t.C
			err = sendHeartBeat(registry, addr)
		}
	}()

}

func sendHeartBeat(registry, addr string) error {
	log.Println("SendHeartBeat", registry, addr)
	httpClient := &http.Client{}
	req, err := http.NewRequest("POST", registry, nil)
	if err != nil {
		log.Println("sendHeartBeat err:", err)
		return err
	}
	req.Header.Set("X-Orpc-Server", addr)
	if _, err = httpClient.Do(req); err != nil {
		log.Println("sendHeartBeat err:", err)
		return err
	}
	return nil
}
