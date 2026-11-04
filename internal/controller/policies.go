package controller

import (
	"context"
	"errors"
	"reflect"

	"github.com/greatfirewall/operator/internal/identity"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// owned requires both the operator marker and the current namespace owner UID.
func owned(policy *networkingv1.NetworkPolicy, ns *corev1.Namespace) bool {
	if policy.Labels[identity.ManagedLabel] != identity.ManagedValue {
		return false
	}
	for _, owner := range policy.OwnerReferences {
		if owner.Kind == "Namespace" && owner.APIVersion == "v1" && owner.UID == ns.UID && owner.Name == ns.Name {
			return true
		}
	}
	return false
}

// apply updates managed rules in place and refuses name collisions with unrelated objects.
func (r *Reconciler) apply(ctx context.Context, desired *networkingv1.NetworkPolicy) error {
	current := new(networkingv1.NetworkPolicy)
	err := r.Get(ctx, client.ObjectKeyFromObject(desired), current)
	if apierrors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: desired.Namespace, UID: desired.OwnerReferences[0].UID},
	}
	sameProfile := current.Annotations[identity.ProfileAnnotation] == desired.Annotations[identity.ProfileAnnotation]
	if !owned(current, ns) || !sameProfile {
		return errors.New("managed policy name conflicts with unrelated resource")
	}
	if reflect.DeepEqual(current.Spec, desired.Spec) && current.Annotations[identity.StateAnnotation] == desired.Annotations[identity.StateAnnotation] {
		return nil
	}
	current.Spec = desired.Spec
	if current.Annotations == nil {
		current.Annotations = map[string]string{}
	}
	for key, value := range desired.Annotations {
		current.Annotations[key] = value
	}
	return r.Update(ctx, current)
}

// warn emits a rate-limited Kubernetes event when an event recorder is configured.
func (r *Reconciler) warn(object client.Object, reason, message string) {
	if r.Recorder != nil {
		r.Recorder.Eventf(object, nil, corev1.EventTypeWarning, reason, "Reconcile", "%s", message)
	}
}

// selectsManaged detects other egress policies selecting at least one labelled pod.
func selectsManaged(policy *networkingv1.NetworkPolicy, pods []corev1.Pod) bool {
	egress := len(policy.Spec.PolicyTypes) == 0 && len(policy.Spec.Egress) > 0
	for _, kind := range policy.Spec.PolicyTypes {
		egress = egress || kind == networkingv1.PolicyTypeEgress
	}
	if !egress {
		return false
	}
	selector, err := metav1.LabelSelectorAsSelector(&policy.Spec.PodSelector)
	if err != nil {
		return false
	}
	for _, pod := range pods {
		if _, ok := pod.Labels[identity.ProfileLabel]; ok && selector.Matches(labels.Set(pod.Labels)) {
			return true
		}
	}
	return false
}
