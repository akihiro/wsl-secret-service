package service

import "github.com/godbus/dbus/v5"

const (
	BusName     = "org.freedesktop.secrets"
	ServicePath = "/org/freedesktop/secrets"

	ServiceIface    = "org.freedesktop.Secret.Service"
	CollectionIface = "org.freedesktop.Secret.Collection"
	ItemIface       = "org.freedesktop.Secret.Item"
	SessionIface    = "org.freedesktop.Secret.Session"
	PromptIface     = "org.freedesktop.Secret.Prompt"

	CollectionPathPrefix = "/org/freedesktop/secrets/collection/"
	SessionPathPrefix    = "/org/freedesktop/secrets/session/"
	PromptPathPrefix     = "/org/freedesktop/secrets/prompt/"

	DefaultAlias    = "default"
	LoginCollection = "login"

	// StubPromptPath is returned when no user interaction is needed.
	StubPromptPath = dbus.ObjectPath("/")

	// PromptStubObjPath is the D-Bus path for our no-op prompt object.
	PromptStubObjPath = dbus.ObjectPath("/org/freedesktop/secrets/prompt/stub")
)

// Secret is the D-Bus type (oayays) representing an encoded secret.
type Secret struct {
	Session     dbus.ObjectPath
	Parameters  []byte
	Value       []byte
	ContentType string
}

// CollectionPath returns the D-Bus object path for a named collection.
func CollectionPath(name string) dbus.ObjectPath {
	return dbus.ObjectPath(CollectionPathPrefix + name)
}

// ItemPath returns the D-Bus object path for an item within a collection.
func ItemPath(collection, uuid string) dbus.ObjectPath {
	return dbus.ObjectPath(CollectionPathPrefix + collection + "/" + uuid)
}

// SessionPath returns the D-Bus object path for a session.
func SessionPath(uuid string) dbus.ObjectPath {
	return dbus.ObjectPath(SessionPathPrefix + uuid)
}

// CollectionNameFromPath extracts the collection name from an object path.
// e.g., /org/freedesktop/secrets/collection/login -> "login"
func CollectionNameFromPath(path dbus.ObjectPath) string {
	s := string(path)
	prefix := CollectionPathPrefix
	if len(s) <= len(prefix) {
		return ""
	}
	rest := s[len(prefix):]
	// If there's a slash in rest, it's an item path not a collection path.
	for i, c := range rest {
		if c == '/' {
			return rest[:i]
		}
	}
	return rest
}

// ItemUUIDFromPath extracts collection name and item UUID from an item path.
// e.g., /org/freedesktop/secrets/collection/login/abc-123 -> ("login", "abc-123")
func ItemUUIDFromPath(path dbus.ObjectPath) (collection, uuid string) {
	s := string(path)
	prefix := CollectionPathPrefix
	if len(s) <= len(prefix) {
		return "", ""
	}
	rest := s[len(prefix):]
	for i, c := range rest {
		if c == '/' {
			return rest[:i], rest[i+1:]
		}
	}
	return "", ""
}
