package auth

import (
	"fmt"
	"testing"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
)

func newTestStore(max int) *sessionStore {
	// Constructed directly rather than via newSessionStore so the test does
	// not spawn the background cleanup goroutine.
	return &sessionStore{
		data:    make(map[string]*sessionEntry),
		maxSize: max,
	}
}

// Unauthenticated requests create login challenges, so the store must never
// grow without bound no matter how many arrive.
func TestSessionStoreIsCapped(t *testing.T) {
	s := newTestStore(100)

	for i := 0; i < 10_000; i++ {
		s.Set(fmt.Sprintf("login:%d", i), &webauthn.SessionData{})
	}

	if len(s.data) > s.maxSize {
		t.Errorf("store grew to %d, cap is %d", len(s.data), s.maxSize)
	}
}

// Overwriting an existing key must not evict anything — it cannot grow the map.
func TestSessionStoreOverwriteDoesNotEvict(t *testing.T) {
	s := newTestStore(4)

	for i := 0; i < 4; i++ {
		s.Set(fmt.Sprintf("k%d", i), &webauthn.SessionData{})
	}
	s.Set("k0", &webauthn.SessionData{})

	if len(s.data) != 4 {
		t.Errorf("len = %d, want 4", len(s.data))
	}
	for i := 0; i < 4; i++ {
		if _, ok := s.data[fmt.Sprintf("k%d", i)]; !ok {
			t.Errorf("k%d was evicted by an overwrite", i)
		}
	}
}

// Expired entries should be reclaimed before any live one is sacrificed.
func TestSessionStoreEvictsExpiredFirst(t *testing.T) {
	s := newTestStore(3)

	s.data["expired"] = &sessionEntry{
		sessionData: &webauthn.SessionData{},
		expiresAt:   time.Now().Add(-time.Minute),
	}
	s.Set("live1", &webauthn.SessionData{})
	s.Set("live2", &webauthn.SessionData{})

	s.Set("new", &webauthn.SessionData{})

	if _, ok := s.data["expired"]; ok {
		t.Error("expired entry survived eviction")
	}
	for _, k := range []string{"live1", "live2", "new"} {
		if _, ok := s.data[k]; !ok {
			t.Errorf("live entry %q was evicted while an expired one remained", k)
		}
	}
}

// With nothing expired, the entry closest to expiry goes.
func TestSessionStoreEvictsOldestWhenAllLive(t *testing.T) {
	s := newTestStore(2)
	now := time.Now()

	s.data["oldest"] = &sessionEntry{sessionData: &webauthn.SessionData{}, expiresAt: now.Add(time.Minute)}
	s.data["newer"] = &sessionEntry{sessionData: &webauthn.SessionData{}, expiresAt: now.Add(4 * time.Minute)}

	s.Set("newest", &webauthn.SessionData{})

	if _, ok := s.data["oldest"]; ok {
		t.Error("oldest entry should have been evicted")
	}
	if _, ok := s.data["newer"]; !ok {
		t.Error("newer entry should have survived")
	}
	if _, ok := s.data["newest"]; !ok {
		t.Error("newly inserted entry missing")
	}
}

func TestSessionStoreGetRespectsTTL(t *testing.T) {
	s := newTestStore(10)

	s.data["stale"] = &sessionEntry{
		sessionData: &webauthn.SessionData{},
		expiresAt:   time.Now().Add(-time.Second),
	}
	if _, ok := s.Get("stale"); ok {
		t.Error("expired entry was returned")
	}
	if _, ok := s.data["stale"]; ok {
		t.Error("expired entry should be deleted on read")
	}

	s.Set("fresh", &webauthn.SessionData{})
	if _, ok := s.Get("fresh"); !ok {
		t.Error("fresh entry should be readable")
	}
}
