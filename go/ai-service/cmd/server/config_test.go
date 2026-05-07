package main

import (
	"testing"
	"time"
)

func TestGetenvPositiveIntDefaultAndOverride(t *testing.T) {
	t.Setenv("CHAT_MAX_IN_FLIGHT", "")
	if got := getenvPositiveInt("CHAT_MAX_IN_FLIGHT", 4); got != 4 {
		t.Fatalf("default = %d, want 4", got)
	}

	t.Setenv("CHAT_MAX_IN_FLIGHT", "8")
	if got := getenvPositiveInt("CHAT_MAX_IN_FLIGHT", 4); got != 8 {
		t.Fatalf("override = %d, want 8", got)
	}
}

func TestGetenvDurationDefaultAndOverride(t *testing.T) {
	t.Setenv("CHAT_OVERLOAD_RETRY_AFTER", "")
	if got := getenvDuration("CHAT_OVERLOAD_RETRY_AFTER", 5*time.Second); got != 5*time.Second {
		t.Fatalf("default = %s, want 5s", got)
	}

	t.Setenv("CHAT_OVERLOAD_RETRY_AFTER", "250ms")
	if got := getenvDuration("CHAT_OVERLOAD_RETRY_AFTER", 5*time.Second); got != 250*time.Millisecond {
		t.Fatalf("override = %s, want 250ms", got)
	}
}
