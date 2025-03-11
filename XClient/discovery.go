package XClient

import (
	"errors"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"
)

type SelectMode int

const (
	RandomSelect SelectMode = iota
	RoundRobinSelect
	DefaultUpdateTimeout = 5 * time.Second
)

type Discovery interface {
	Refresh() error
	Update(Servers []string) error
	Get(Mode SelectMode) (string, error)
	GetAll() ([]string, error)
}

type MultiServerDiscovery struct {
	servers []string
	r       *rand.Rand
	mu      sync.RWMutex
	index   int //轮询计数
}

func (m *MultiServerDiscovery) Refresh() error {
	//TODO implement me
	panic("implement me")
}

func (m *MultiServerDiscovery) Update(Servers []string) error {
	//TODO implement me
	panic("implement me")
}

type OrpcRegisterDiscovery struct {
	*MultiServerDiscovery
	registry   string
	timeout    time.Duration
	lastUpdate time.Time
}

func NewOrpcRegisterDiscovery(registryAddr string, timeout time.Duration) *OrpcRegisterDiscovery {
	if timeout == 0 {
		timeout = DefaultUpdateTimeout
	}
	d := &OrpcRegisterDiscovery{
		MultiServerDiscovery: NewMultiServerDiscovery(make([]string, 0)),
		registry:             registryAddr,
		timeout:              timeout,
	}
	return d
}

func NewMultiServerDiscovery(servers []string) *MultiServerDiscovery {
	d := &MultiServerDiscovery{
		servers: servers,
		r:       rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	if len(d.servers) == 0 {
		d.index = -1
		return d
	}
	d.index = d.r.Intn(len(d.servers))
	return d
}
func (m *OrpcRegisterDiscovery) Refresh() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.lastUpdate.Add(m.timeout).After(time.Now()) {
		return nil
	}
	log.Println("OrpcRegisterDiscovery refresh", m.registry)
	resp, err := http.Get(m.registry)
	if err != nil {
		log.Println("OrpcRegisterDiscovery refresh error", err)
		return err
	}
	defer resp.Body.Close()
	servers := strings.Split(resp.Header.Get("X-Orpc-Servers"), ",")
	m.servers = make([]string, 0, len(servers))
	for _, server := range servers {
		m.servers = append(m.servers, strings.TrimSpace(server))
	}
	log.Println("OrpcRegisterDiscovery refresh", m.registry)
	m.lastUpdate = time.Now()
	return nil
}

func (m *OrpcRegisterDiscovery) Update(Servers []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.servers = Servers
	m.lastUpdate = time.Now()
	return nil
}

func (m *MultiServerDiscovery) Get(Mode SelectMode) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	n := len(m.servers)
	if n == 0 {
		return "", errors.New("no servers")
	}
	switch Mode {
	case RandomSelect:
		return m.servers[m.r.Intn(n)], nil
	case RoundRobinSelect:
		s := m.servers[m.index%n]
		m.index = (m.index + 1) % n
		return s, nil
	default:
		return "", errors.New("invalid select mode")
	}
}

func (m *MultiServerDiscovery) GetAll() ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]string{}, m.servers...), nil
}

func (d *OrpcRegisterDiscovery) Get(Mode SelectMode) (string, error) {
	if d.registry == "" {
		return "", errors.New("no registry")
	}
	return d.MultiServerDiscovery.Get(Mode)
}
func (d *OrpcRegisterDiscovery) GetAll() ([]string, error) {
	if d.registry == "" {
		return nil, errors.New("no registry")
	}
	return d.MultiServerDiscovery.GetAll()
}
