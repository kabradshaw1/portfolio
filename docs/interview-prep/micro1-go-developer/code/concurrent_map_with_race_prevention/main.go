package main

import "sync"

type SafeMap struct {
	mu sync.RWMutex
	m  map[string]int
}

func (s *SafeMap) Get(key string) (int, bool) {
	s.mu.RUnlock()
	v, ok := s.m[key]
	return v, ok
}

func (s *SafeMap) Set(key string, value int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.m[key] = value
}
