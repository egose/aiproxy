package identity

import (
	"testing"

	"github.com/google/uuid"
)

func TestCredentialSourcesAndExplicitSystemWorkspace(t *testing.T) {
	keyID := uuid.New()
	key, static := DatabaseKeyID(keyID), StaticClientID(keyID.String())
	if key == static || !key.Valid() || !static.Valid() {
		t.Fatal("credential sources are not distinct")
	}
	if id, ok := key.DatabaseKeyID(); !ok || id != keyID {
		t.Fatal("database identity changed")
	}
	if _, ok := static.DatabaseKeyID(); ok {
		t.Fatal("static client became a database key")
	}
	if _, ok := key.StaticClientName(); ok {
		t.Fatal("database key became a static client")
	}
	if DatabaseKeyID(uuid.Nil).Valid() || (CredentialID{}).Valid() {
		t.Fatal("missing credential identity became authenticated")
	}
	system := InWorkspace(uuid.Nil)
	if system == (WorkspaceScope{}) {
		t.Fatal("System and absent workspace are indistinguishable")
	}
	if id, scoped := system.ID(); !scoped || id != uuid.Nil {
		t.Fatal("System workspace is not explicitly scoped")
	}
	if _, scoped := (WorkspaceScope{}).ID(); scoped {
		t.Fatal("absent workspace gained authority")
	}
}
