package window

import (
	"sync"
	"time"
)

// Clock abstracts time for testing.
type Clock interface {
	Now() time.Time
}

// RealClock uses time.Now().
type RealClock struct{}

// Now returns the current time.
func (RealClock) Now() time.Time { return time.Now() }

// MockClock allows controlling time in tests.
type MockClock struct {
	mu  sync.Mutex
	now time.Time
}

// NewMockClock creates a MockClock set to the given time.
func NewMockClock(t time.Time) *MockClock {
	return &MockClock{now: t}
}

// Now returns the mock clock's current time.
func (m *MockClock) Now() time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.now
}

// Set sets the mock clock to the given time.
func (m *MockClock) Set(t time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.now = t
}

// Advance moves the mock clock forward by the given duration.
func (m *MockClock) Advance(d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.now = m.now.Add(d)
}
