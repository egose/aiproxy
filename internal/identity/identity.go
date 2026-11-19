package identity

import "github.com/google/uuid"

type credentialSource uint8

const (
	staticClient credentialSource = iota + 1
	databaseKey
)

type CredentialID struct {
	source     credentialSource
	staticName string
	keyID      uuid.UUID
}

func StaticClientID(name string) CredentialID {
	return CredentialID{source: staticClient, staticName: name}
}

func DatabaseKeyID(id uuid.UUID) CredentialID {
	if id == uuid.Nil {
		return CredentialID{}
	}
	return CredentialID{source: databaseKey, keyID: id}
}

func (id CredentialID) StaticClientName() (string, bool) {
	return id.staticName, id.source == staticClient
}

func (id CredentialID) DatabaseKeyID() (uuid.UUID, bool) {
	return id.keyID, id.source == databaseKey
}

func (id CredentialID) Valid() bool {
	return id.source != 0
}

type WorkspaceScope struct {
	id      uuid.UUID
	present bool
}

func InWorkspace(id uuid.UUID) WorkspaceScope {
	return WorkspaceScope{id: id, present: true}
}

func (s WorkspaceScope) ID() (uuid.UUID, bool) {
	return s.id, s.present
}
