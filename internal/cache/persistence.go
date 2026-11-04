package cache

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/greatfirewall/operator/internal/identity"
	"github.com/greatfirewall/operator/internal/profile"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// load validates cache ownership and data; invalid or absent cache never grants access.
func (s *Store) load(ctx context.Context, name string) (Snapshot, error) {
	empty := Snapshot{Name: name}
	cm := new(corev1.ConfigMap)
	if err := s.Client.Get(ctx, client.ObjectKey{Namespace: s.Namespace, Name: identity.ObjectName(name)}, cm); err != nil {
		if apierrors.IsNotFound(err) {
			return empty, nil
		}
		return empty, err
	}
	if cm.Labels[identity.ManagedLabel] != identity.ManagedValue || cm.Annotations[identity.ProfileAnnotation] != name {
		return empty, errors.New("cache ownership conflict")
	}
	var snapshot Snapshot
	if err := json.Unmarshal([]byte(cm.Data["snapshot.json"]), &snapshot); err != nil {
		return empty, errors.New("invalid cached JSON")
	}
	if snapshot.Name != name || snapshot.ObservedAt.IsZero() || snapshot.Revoked == (snapshot.Profile != nil) {
		return empty, errors.New("invalid cached snapshot")
	}
	if snapshot.Profile != nil {
		document, err := profile.Validate(*snapshot.Profile, name)
		if err != nil {
			return empty, err
		}
		snapshot.Profile = &document
	}
	return snapshot, nil
}

// save atomically replaces one profile snapshot and refuses to adopt unrelated ConfigMaps.
func (s *Store) save(ctx context.Context, snapshot Snapshot) error {
	cm := new(corev1.ConfigMap)
	key := client.ObjectKey{Namespace: s.Namespace, Name: identity.ObjectName(snapshot.Name)}
	err := s.Client.Get(ctx, key, cm)
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	create := apierrors.IsNotFound(err)
	if create {
		cm.ObjectMeta = metav1.ObjectMeta{
			Name:        key.Name,
			Namespace:   key.Namespace,
			Labels:      map[string]string{identity.ManagedLabel: identity.ManagedValue},
			Annotations: map[string]string{identity.ProfileAnnotation: snapshot.Name},
		}
	} else if cm.Labels[identity.ManagedLabel] != identity.ManagedValue || cm.Annotations[identity.ProfileAnnotation] != snapshot.Name {
		return errors.New("cache ownership conflict")
	}
	data, err := json.Marshal(snapshot)
	if err != nil {
		return errors.New("cannot encode snapshot")
	}
	cm.Data = map[string]string{"snapshot.json": string(data)}
	if create {
		return s.Client.Create(ctx, cm)
	}
	return s.Client.Update(ctx, cm)
}
