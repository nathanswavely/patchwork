package ap_test

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/ap"
)

// TestFetchActorCachesResults verifies that a second fetch of the same actor is
// served from the cache rather than hitting the fetcher again.
func TestFetchActorCachesResults(t *testing.T) {
	var calls int32
	restore := ap.SetActorFetcher(func(_ context.Context, id string) (*ap.RemoteActor, error) {
		atomic.AddInt32(&calls, 1)
		return &ap.RemoteActor{ID: id, Inbox: id + "/inbox", PublicKey: "pem"}, nil
	})
	defer ap.SetActorFetcher(restore)

	const actor = "https://remote.example/ap/users/cached"
	for i := 0; i < 3; i++ {
		got, err := ap.FetchActor(context.Background(), actor)
		if err != nil {
			t.Fatalf("FetchActor: %v", err)
		}
		if got.Inbox != actor+"/inbox" {
			t.Fatalf("unexpected inbox %q", got.Inbox)
		}
	}

	if n := atomic.LoadInt32(&calls); n != 1 {
		t.Errorf("expected fetcher called once (cached), got %d", n)
	}
}

// TestFetchActorCacheNotSharedAcrossFetchers verifies that swapping the fetcher
// clears the cache, so a stubbed fetcher is never shadowed by stale entries.
func TestFetchActorCacheNotSharedAcrossFetchers(t *testing.T) {
	const actor = "https://remote.example/ap/users/swap"

	restore := ap.SetActorFetcher(func(_ context.Context, id string) (*ap.RemoteActor, error) {
		return &ap.RemoteActor{ID: id, Inbox: "first-inbox", PublicKey: "pem"}, nil
	})
	first, err := ap.FetchActor(context.Background(), actor)
	if err != nil || first.Inbox != "first-inbox" {
		t.Fatalf("first fetch: %v / %+v", err, first)
	}

	// Swapping the fetcher must clear the cache so the new fetcher is used.
	ap.SetActorFetcher(func(_ context.Context, id string) (*ap.RemoteActor, error) {
		return &ap.RemoteActor{ID: id, Inbox: "second-inbox", PublicKey: "pem"}, nil
	})
	defer ap.SetActorFetcher(restore)

	second, err := ap.FetchActor(context.Background(), actor)
	if err != nil || second.Inbox != "second-inbox" {
		t.Fatalf("expected second-inbox after fetcher swap, got %v / %+v", err, second)
	}
}
