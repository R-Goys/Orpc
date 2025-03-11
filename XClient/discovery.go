package XClient

import (
	"errors"
	"math/rand"
	"sync"
	"time"
)

type SelectMode int

const (
	RandomSelect SelectMode = iota
	RoundRobinSelect
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

func NewMultiServerDiscovery(servers []string) *MultiServerDiscovery {
	d := &MultiServerDiscovery{
		servers: servers,
		r:       rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	d.index = d.r.Intn(len(d.servers))
	return d
}
func (m *MultiServerDiscovery) Refresh() error {
	return nil
}

func (m *MultiServerDiscovery) Update(Servers []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.servers = Servers
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

var _ Discovery = &MultiServerDiscovery{}
