// Package backend defines the interface for secret storage backends.
// The actual secret bytes are stored by implementations of this interface;
// metadata (labels, attributes) is managed separately by the store package.
package backend

// Backend stores and retrieves raw secret bytes keyed by a target string.
type Backend interface {
	// Get returns the raw secret bytes for the given target.
	// Returns an error wrapping ErrNotFound if the target does not exist.
	Get(target string) ([]byte, error)

	// Set stores raw secret bytes under the given target.
	// Creates the entry if it does not exist; replaces it if it does.
	Set(target string, secret []byte) error

	// Delete removes the secret for the given target.
	// Returns an error wrapping ErrNotFound if the target does not exist.
	Delete(target string) error

	// List returns all target strings that have the given prefix.
	List(prefix string) ([]string, error)
}

// ErrNotFound is returned when a requested secret does not exist.
type ErrNotFound struct {
	Target string
}

func (e *ErrNotFound) Error() string {
	return "secret not found: " + e.Target
}
