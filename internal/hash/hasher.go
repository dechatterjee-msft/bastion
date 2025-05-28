package hash

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Hasher defines an interface for hashing Kubernetes objects.
type Hasher interface {
	Hash(obj *unstructured.Unstructured) (string, error)
}

// DefaultHasher implements SHA256 hashing after sanitizing.
type DefaultHasher struct{}

// NewDefaultHasher returns a new DefaultHasher instance.
func NewDefaultHasher() *DefaultHasher {
	return &DefaultHasher{}
}

// Hash computes a SHA256 hash of a sanitized Kubernetes object.
func (h *DefaultHasher) Hash(obj *unstructured.Unstructured) (string, error) {
	sanitized := obj.DeepCopy()
	unstructured.RemoveNestedField(sanitized.Object, "metadata", "resourceVersion")
	unstructured.RemoveNestedField(sanitized.Object, "metadata", "generation")
	unstructured.RemoveNestedField(sanitized.Object, "metadata", "creationTimestamp")
	unstructured.RemoveNestedField(sanitized.Object, "status")

	data, err := json.Marshal(sanitized.Object)
	if err != nil {
		return "", fmt.Errorf("failed to marshal sanitized object: %w", err)
	}

	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
