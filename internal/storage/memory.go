package storage

import (
	"context"
	"slices"
	"strings"
	"sync"
)

// Memory is an in-process Storage for tests.
type Memory struct {
	mu   sync.Mutex
	objs map[string][]byte
}

func NewMemory() *Memory { return &Memory{objs: map[string][]byte{}} }

func (m *Memory) Put(_ context.Context, key, _ string, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.objs[key] = slices.Clone(data)
	return nil
}

func (m *Memory) Get(_ context.Context, key string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.objs[key]
	if !ok {
		return nil, ErrNotFound
	}
	return slices.Clone(b), nil
}

func (m *Memory) Delete(_ context.Context, keys ...string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, k := range keys {
		delete(m.objs, k)
	}
	return nil
}

func (m *Memory) DeletePrefix(_ context.Context, prefix string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for k := range m.objs {
		if strings.HasPrefix(k, prefix) {
			delete(m.objs, k)
		}
	}
	return nil
}

// Keys lists the stored keys, sorted (for assertions).
func (m *Memory) Keys() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, 0, len(m.objs))
	for k := range m.objs {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}
