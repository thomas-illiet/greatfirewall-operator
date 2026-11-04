// Package cache durably stores positive profiles and explicit revocations.
package cache

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/greatfirewall/operator/internal/api"
	"github.com/greatfirewall/operator/internal/profile"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Snapshot is durable state; revoked snapshots never retain an old positive profile.
type Snapshot struct {
	Name       string           `json:"name"`
	Profile    *profile.Profile `json:"profile,omitempty"`
	Revoked    bool             `json:"revoked"`
	ObservedAt time.Time        `json:"observedAt"`
}

// cachedState tracks in-process request coalescing and pending persistence retries.
type cachedState struct {
	snapshot  Snapshot
	attempted time.Time
	err       error
	dirty     bool
}

// Store serializes profile refreshes and persists validated positive or revoked snapshots.
type Store struct {
	Client    client.Client
	Namespace string
	Fetcher   api.Fetcher
	Interval  time.Duration
	Now       func() time.Time
	mu        sync.Mutex
	entries   map[string]*cachedState
}

// Get coalesces refreshes and never expires the last accepted profile or revocation.
func (s *Store) Get(ctx context.Context, name string) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !profile.ValidName(name) {
		return Snapshot{Name: name}, errors.New("invalid profile name")
	}

	entry := s.entry(ctx, name)
	if entry.dirty {
		if err := s.persist(ctx, entry); err != nil {
			// A failed cache write must not prevent discovering a later API revocation.
			entry.err = err
		}
	}

	now := time.Now()
	if s.Now != nil {
		now = s.Now()
	}
	if !entry.attempted.IsZero() && now.Sub(entry.attempted) < s.Interval {
		return entry.snapshot, entry.err
	}

	s.refresh(ctx, name, now, entry)
	return entry.snapshot, entry.err
}

// entry restores a profile only once; malformed cache data starts from deny-all.
func (s *Store) entry(ctx context.Context, name string) *cachedState {
	if s.entries == nil {
		s.entries = make(map[string]*cachedState)
	}
	if entry, exists := s.entries[name]; exists {
		return entry
	}

	snapshot, err := s.load(ctx, name)
	entry := &cachedState{snapshot: snapshot, err: err}
	s.entries[name] = entry
	return entry
}

// refresh replaces state only after a validated success or an explicit HTTP 404.
func (s *Store) refresh(ctx context.Context, name string, now time.Time, entry *cachedState) {
	entry.attempted = now
	document, err := s.Fetcher.Fetch(ctx, name)
	if err == nil {
		document, err = profile.Validate(document, name)
	}
	if err != nil && !errors.Is(err, api.ErrRevoked) {
		entry.err = err
		return
	}

	snapshot := Snapshot{
		Name:       name,
		ObservedAt: now.UTC(),
		Revoked:    errors.Is(err, api.ErrRevoked),
	}
	if !snapshot.Revoked {
		snapshot.Profile = &document
	}

	// A tombstone replaces the positive snapshot; it never carries old permissions.
	entry.snapshot = snapshot
	entry.dirty = true
	entry.err = s.persist(ctx, entry)
}

// persist retries a failed Kubernetes write before any later external refresh.
func (s *Store) persist(ctx context.Context, entry *cachedState) error {
	if err := s.save(ctx, entry.snapshot); err != nil {
		return err
	}
	entry.dirty = false
	entry.err = nil
	return nil
}
