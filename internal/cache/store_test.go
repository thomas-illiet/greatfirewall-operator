package cache

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/greatfirewall/operator/internal/api"
	"github.com/greatfirewall/operator/internal/identity"
	"github.com/greatfirewall/operator/internal/profile"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

// fakeFetcher records calls and supplies deterministic API responses.
type fakeFetcher struct {
	calls  int
	result profile.Profile
	err    error
}

// Fetch returns the configured response while recording request coalescing.
func (f *fakeFetcher) Fetch(context.Context, string) (profile.Profile, error) {
	f.calls++
	return f.result, f.err
}

// cacheClient supplies an in-memory Kubernetes API supporting ConfigMaps.
func cacheClient(objects ...client.Object) client.WithWatch {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()
}

// validProfile returns a minimal valid grant-free profile.
func validProfile() profile.Profile {
	return profile.Profile{Profile: "dev", Revision: "1", Egress: []profile.Rule{}}
}

// TestRevocationSurvivesRestart verifies cache retention, explicit revocation and recovery.
func TestRevocationSurvivesRestart(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	remote := &fakeFetcher{result: validProfile()}
	kube := cacheClient()
	store := &Store{
		Client:    kube,
		Namespace: "system",
		Fetcher:   remote,
		Interval:  time.Minute,
		Now:       func() time.Time { return now },
	}
	first, err := store.Get(ctx, "dev")
	if err != nil || first.Profile == nil {
		t.Fatalf("initial fetch: %v", err)
	}
	var group sync.WaitGroup
	for i := 0; i < 10; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			_, _ = store.Get(ctx, "dev")
		}()
	}
	group.Wait()
	if remote.calls != 1 {
		t.Fatal("requests not coalesced")
	}
	now = now.Add(time.Minute)
	remote.err = errors.New("outage")
	stale, err := store.Get(ctx, "dev")
	if err == nil || stale.Profile == nil {
		t.Fatal("outage did not retain profile")
	}
	restarted := &Store{Client: kube, Namespace: "system", Fetcher: remote, Interval: time.Minute}
	stale, err = restarted.Get(ctx, "dev")
	if err == nil || stale.Profile == nil {
		t.Fatal("restart lost last valid profile")
	}
	now = now.Add(time.Minute)
	remote.err = api.ErrRevoked
	revoked, err := store.Get(ctx, "dev")
	if err != nil || !revoked.Revoked || revoked.Profile != nil {
		t.Fatal("404 did not revoke")
	}
	remote.err = errors.New("outage")
	restarted = &Store{Client: kube, Namespace: "system", Fetcher: remote, Interval: time.Minute}
	revoked, err = restarted.Get(ctx, "dev")
	if err == nil || !revoked.Revoked || revoked.Profile != nil {
		t.Fatal("restart resurrected revoked authorization")
	}
	now = now.Add(time.Minute)
	remote.err = nil
	recovered, err := store.Get(ctx, "dev")
	if err != nil || recovered.Profile == nil || recovered.Revoked {
		t.Fatal("valid API response did not recover")
	}
	if _, err := store.Get(ctx, ""); err == nil {
		t.Fatal("invalid name accepted")
	}
}

// TestInvalidCacheNeverGrants rejects corrupt ownership, JSON, snapshots and IPv6 in saved profiles.
func TestInvalidCacheNeverGrants(t *testing.T) {
	ctx := context.Background()
	remote := &fakeFetcher{err: errors.New("offline")}
	document := validProfile()
	good := Snapshot{
		Name:       "dev",
		Profile:    &document,
		ObservedAt: time.Now(),
	}
	data, _ := json.Marshal(good)
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:        identity.ObjectName("dev"),
			Namespace:   "system",
			Labels:      map[string]string{identity.ManagedLabel: identity.ManagedValue},
			Annotations: map[string]string{identity.ProfileAnnotation: "dev"},
		},
		Data: map[string]string{"snapshot.json": string(data)},
	}
	mutations := []func(*corev1.ConfigMap){
		func(c *corev1.ConfigMap) { c.Labels = nil },
		func(c *corev1.ConfigMap) { c.Data["snapshot.json"] = "{" },
		func(c *corev1.ConfigMap) { c.Data["snapshot.json"] = `{"name":"dev","revoked":false}` },
		injectIPv6Snapshot,
	}
	for _, change := range mutations {
		obj := cm.DeepCopy()
		change(obj)
		s := &Store{Client: cacheClient(obj), Namespace: "system", Fetcher: remote}
		snapshot, err := s.Get(ctx, "dev")
		if err == nil || snapshot.Profile != nil {
			t.Fatal("invalid cached state granted access")
		}
	}
	empty := &Store{Client: cacheClient(), Namespace: "system", Fetcher: remote}
	snapshot, err := empty.Get(ctx, "dev")
	if err == nil || snapshot.Profile != nil {
		t.Fatal("unknown profile granted access")
	}
	remote.err = nil
	remote.result = profile.Profile{}
	if snapshot, err = empty.Get(ctx, "dev"); err == nil || snapshot.Profile != nil {
		t.Fatal("unchecked fetcher supplied invalid profile")
	}
}

// injectIPv6Snapshot simulates a corrupt persisted profile that must never restore access.
func injectIPv6Snapshot(cm *corev1.ConfigMap) {
	document := validProfile()
	document.Egress = []profile.Rule{{
		CIDR:  "::1",
		Ports: []profile.Port{{Protocol: "TCP", Port: 80}},
	}}
	snapshot := Snapshot{
		Name:       "dev",
		Profile:    &document,
		ObservedAt: time.Now(),
	}
	data, _ := json.Marshal(snapshot)
	cm.Data["snapshot.json"] = string(data)
}

// TestPersistenceErrors checks ownership conflicts, durable retries and Kubernetes failures.
func TestPersistenceErrors(t *testing.T) {
	ctx := context.Background()
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: identity.ObjectName("dev"), Namespace: "system"},
	}
	store := &Store{
		Client:    cacheClient(cm),
		Namespace: "system",
		Fetcher:   &fakeFetcher{result: validProfile()},
		Interval:  time.Minute,
	}
	if _, err := store.Get(ctx, "dev"); err == nil {
		t.Fatal("foreign ConfigMap adopted")
	}
	if _, err := store.Get(ctx, "dev"); err == nil {
		t.Fatal("dirty state ignored conflict")
	}
	if err := store.Client.Delete(ctx, cm); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(ctx, "dev"); err != nil {
		t.Fatal(err)
	}
	fault := errors.New("Kubernetes unavailable")
	broken := interceptor.NewClient(cacheClient(), interceptor.Funcs{
		Get: func(context.Context, client.WithWatch, client.ObjectKey, client.Object, ...client.GetOption) error {
			return fault
		},
	})
	store = &Store{
		Client:    broken,
		Namespace: "system",
		Fetcher:   &fakeFetcher{err: errors.New("offline")},
	}
	if _, err := store.Get(ctx, "dev"); err == nil {
		t.Fatal("read error lost")
	}
	store.Fetcher = &fakeFetcher{result: validProfile()}
	if _, err := store.Get(ctx, "dev"); err == nil {
		t.Fatal("write prerequisite failure lost")
	}
	badTime := Snapshot{
		Name:       "dev",
		Revoked:    true,
		ObservedAt: time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	store.Client = cacheClient()
	if err := store.save(ctx, badTime); err == nil {
		t.Fatal("unencodable snapshot accepted")
	}
}

// TestPendingPersistenceStillRefreshes prevents failed cache writes from hiding later revocations.
func TestPendingPersistenceStillRefreshes(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	failure := errors.New("cache writes forbidden")
	kube := interceptor.NewClient(cacheClient(), interceptor.Funcs{
		Create: func(context.Context, client.WithWatch, client.Object, ...client.CreateOption) error {
			return failure
		},
	})
	remote := &fakeFetcher{result: validProfile()}
	store := &Store{
		Client:    kube,
		Namespace: "system",
		Fetcher:   remote,
		Interval:  time.Minute,
		Now:       func() time.Time { return now },
	}
	initial, err := store.Get(ctx, "dev")
	if !errors.Is(err, failure) || initial.Profile == nil {
		t.Fatal("initial persistence failure was not exposed")
	}

	now = now.Add(time.Minute)
	remote.err = api.ErrRevoked
	revoked, err := store.Get(ctx, "dev")
	if !errors.Is(err, failure) || !revoked.Revoked || revoked.Profile != nil || remote.calls != 2 {
		t.Fatal("pending cache write prevented revocation refresh")
	}
}
