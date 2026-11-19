package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/egose/aiproxy/internal/store"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type keyWriteDenialAuditKey struct{}

type keyWriteDenialAudit struct {
	id         uuid.UUID
	fail       string
	reads      map[string]int
	unexpected []string
}

type keyWriteDenialQueryHook struct{}

func (keyWriteDenialQueryHook) BeforeQuery(ctx context.Context, event *bun.QueryEvent) context.Context {
	audit, _ := ctx.Value(keyWriteDenialAuditKey{}).(*keyWriteDenialAudit)
	if audit == nil {
		return ctx
	}
	_, from, _ := strings.Cut(event.Query, ` FROM `)
	id := "'" + audit.id.String() + "'"
	var read string
	switch {
	case !strings.HasPrefix(event.Query, "SELECT "):
		audit.unexpected = append(audit.unexpected, event.Query)
	case strings.HasPrefix(from, `"users" AS "u" WHERE (id = `):
		read = "actor"
	case strings.HasPrefix(from, `"workspace_members" AS "wm" WHERE (user_id = `):
		read = "membership"
	case from == `"inbound_keys" AS "k" WHERE (id = `+id+`)`:
		read = "key"
	case strings.HasPrefix(from, `"workspace_teams" AS "wt" WHERE (id = `):
		read = "owner team"
	case strings.HasPrefix(from, `"team_members" AS "tm" WHERE (user_id = `):
		read = "team membership"
	case strings.HasPrefix(event.Query, `SELECT EXISTS (SELECT 1 FROM key_users WHERE key_id = `+id+` AND user_id = `) &&
		strings.Contains(event.Query, `WHERE kt.key_id = `+id+` AND tm.user_id = `):
		read = "share"
	default:
		audit.unexpected = append(audit.unexpected, event.Query)
	}
	if read != "" {
		audit.reads[read]++
		if read == audit.fail {
			queryCtx, cancel := context.WithCancel(ctx)
			cancel()
			return queryCtx
		}
	}
	return ctx
}

func (keyWriteDenialQueryHook) AfterQuery(context.Context, *bun.QueryEvent) {}

func TestAdminExistingKeyWriteVisibility(t *testing.T) {
	st := openAdminReasoningStore(t)
	h, global := validationTestHandler(t, st)
	ctx := context.Background()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	a, b := store.Workspace{Name: "key-scope-a"}, store.Workspace{Name: "key-scope-b"}
	must(st.CreateWorkspace(ctx, &a))
	must(st.CreateWorkspace(ctx, &b))
	systemID := store.SystemWorkspaceID
	user := func(role string, workspaces ...uuid.UUID) (store.User, string) {
		t.Helper()
		u, access := membershipHTTPUser(t, st)
		for _, id := range workspaces {
			must(st.AddMembership(ctx, &store.WorkspaceMember{UserID: u.ID, WorkspaceID: id, Role: role}))
		}
		return u, access
	}
	_, aAdmin := user(roleAdmin, a.ID)
	_, dual := user(roleAdmin, a.ID, b.ID)
	reader, aReader := user(roleMember, a.ID)
	teamManager, aTeamAdmin := user(roleMember, a.ID)
	former, offboarded := user(roleMember, a.ID, b.ID)
	_, systemMember := user(roleMember, systemID)
	_, noMembership := user(roleMember)
	team := store.WorkspaceTeam{WorkspaceID: a.ID, Name: "shared-team"}
	must(st.CreateTeam(ctx, &team))
	must(st.AddTeamMember(ctx, reader.ID, team.ID, roleMember))
	must(st.AddTeamMember(ctx, teamManager.ID, team.ID, roleAdmin))

	seed := func(t *testing.T, name string, workspace uuid.UUID, ownerUser, ownerTeam *uuid.UUID) uuid.UUID {
		t.Helper()
		k := store.InboundKey{Name: name, WorkspaceID: workspace, TokenHash: store.TokenHash("key-fixture-" + name), TokenPrefix: "fixture-prefix",
			Description: "before", AllowedModels: []string{"alias/fixture"}, Enabled: true, OwnerUserID: ownerUser, OwnerTeamID: ownerTeam}
		if err := st.CreateInboundKey(ctx, &k); err != nil {
			t.Fatal(err)
		}
		return k.ID
	}
	aKey := seed(t, "a-private", a.ID, nil, nil)
	bKey := seed(t, "b-private", b.ID, nil, nil)
	systemKey := seed(t, "system-private", systemID, nil, nil)
	sharedKey := seed(t, "a-shared", a.ID, nil, &team.ID)
	formerKey := seed(t, "a-personal", a.ID, &former.ID, nil)
	must(st.SetKeyBindings(ctx, sharedKey, nil, []uuid.UUID{team.ID}))
	must(st.SetKeyBindings(ctx, formerKey, []uuid.UUID{former.ID}, nil))
	keyPath := func(id uuid.UUID) string { return adminPathPrefix + "keys/" + id.String() }
	for _, tc := range []struct {
		id     uuid.UUID
		access string
	}{{sharedKey, aReader}, {formerKey, offboarded}} {
		w := callDirectWrite(h, ctx, tc.access, http.MethodGet, keyPath(tc.id), "", directWriteSelection{})
		if w.Code != http.StatusOK {
			t.Fatalf("test key was not visible before the write checks: %d %s", w.Code, w.Body.String())
		}
	}
	must(st.DeleteMembership(ctx, former.ID, a.ID))
	if _, err := st.GetMembership(ctx, former.ID, b.ID); err != nil {
		t.Fatalf("former member must still belong to B: %v", err)
	}

	readGrants := func(t *testing.T, id uuid.UUID) ([]uuid.UUID, []uuid.UUID) {
		t.Helper()
		users, err := st.ListKeyUserIDs(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		teams, err := st.ListKeyTeamIDs(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		return users, teams
	}
	type savedKeys struct {
		keys         []store.InboundKey
		users, teams map[uuid.UUID][]uuid.UUID
	}
	snapshot := func(t *testing.T) savedKeys {
		t.Helper()
		keys, err := st.ListInboundKeys(ctx)
		if err != nil {
			t.Fatal(err)
		}
		saved := savedKeys{keys, map[uuid.UUID][]uuid.UUID{}, map[uuid.UUID][]uuid.UUID{}}
		for _, k := range keys {
			saved.users[k.ID], saved.teams[k.ID] = readGrants(t, k.ID)
		}
		return saved
	}
	beforeRows, beforeRevisions := snapshot(t), readDirectWriteRevisions(t, st)
	activations, applied := 0, beforeRevisions
	deps := h.current()
	deps.RequestReload = func() error {
		activations++
		var err error
		applied, err = st.ServingRevisions(ctx)
		return err
	}
	h.UpdateDependencies(deps)
	st.DB.AddQueryHook(keyWriteDenialQueryHook{})

	type route struct{ name, method, suffix, body string }
	routes := []route{
		{"policy", http.MethodPut, "", `{"description":"updated","tenant":"fixture-tenant"}`},
		{"revoke", http.MethodPost, "/revoke", ""},
		{"rotate", http.MethodPost, "/rotate", ""},
		{"delete", http.MethodDelete, "", ""},
	}
	reject := func(t *testing.T, op route, access string, id uuid.UUID, sel directWriteSelection, code int, body, fault string) {
		t.Helper()
		audit := &keyWriteDenialAudit{id: id, fail: fault, reads: map[string]int{}}
		previous := activations
		w := callDirectWrite(h, context.WithValue(ctx, keyWriteDenialAuditKey{}, audit), access, op.method, keyPath(id)+op.suffix, op.body, sel)
		if w.Code != code || w.Body.String() != body {
			t.Errorf("%s: %d %q, want %d %q", op.name, w.Code, w.Body.String(), code, body)
		}
		assertNoDirectWriteMarkers(t, w.Header())
		if activations != previous || len(audit.unexpected) != 0 {
			t.Errorf("denial requested activation or accessed non-authorization data: reloads=%d, queries=%q", activations-previous, audit.unexpected)
		}
		for read, count := range audit.reads {
			if count != 1 {
				t.Errorf("denial made %d %s reads", count, read)
			}
		}
		if fault != "" && audit.reads[fault] != 1 {
			t.Errorf("did not exercise the required %s failure", fault)
		}
		if code == http.StatusBadRequest && (audit.reads["membership"] != 0 || audit.reads["key"] != 0) || fault == "membership" && audit.reads["key"] != 0 {
			t.Errorf("denial resolved the target too early: %v", audit.reads)
		}
	}
	missing := uuid.New()
	const notFound, invalidID = "key not found\n", "invalid workspace id\n"
	aID, bID := a.ID.String(), b.ID.String()
	omitted := directWriteSelection{}
	matchingA := directWriteSelection{query: "?workspace_id=" + aID, headers: []string{aID}}
	deniedSelectors := deniedDirectWriteSelections(aID, bID)
	for _, op := range routes {
		t.Run("denied/"+op.name, func(t *testing.T) {
			for _, tc := range []struct {
				name, access string
				id           uuid.UUID
			}{
				{"foreign-workspace", aAdmin, bKey},
				{"private-in-own-workspace", aReader, aKey},
				{"system-key", systemMember, systemKey},
				{"no-membership", noMembership, aKey},
				{"former-owner-still-in-b", offboarded, formerKey},
			} {
				t.Run(tc.name, func(t *testing.T) {
					reject(t, op, tc.access, tc.id, omitted, http.StatusNotFound, notFound, "")
				})
			}
			reject(t, op, offboarded, formerKey, matchingA, http.StatusNotFound, notFound, "")
			for _, sel := range []directWriteSelection{omitted, matchingA} {
				reject(t, op, aReader, sharedKey, sel, http.StatusForbidden, "key manager required\n", "")
			}
			reject(t, op, aReader, sharedKey, directWriteSelection{headers: []string{bID}}, http.StatusNotFound, notFound, "")
			for _, actor := range []struct{ name, access string }{{"a-only", aAdmin}, {"dual", dual}, {"global", global}} {
				t.Run(actor.name, func(t *testing.T) {
					reject(t, op, actor.access, missing, omitted, http.StatusNotFound, notFound, "")
					reject(t, op, actor.access, missing, matchingA, http.StatusNotFound, notFound, "")
					for _, tc := range deniedSelectors {
						t.Run(tc.name, func(t *testing.T) {
							for _, id := range []uuid.UUID{aKey, missing} {
								if tc.malformed {
									reject(t, op, actor.access, id, tc.selection, http.StatusBadRequest, invalidID, "")
								} else {
									reject(t, op, actor.access, id, tc.selection, http.StatusNotFound, notFound, "")
								}
							}
						})
					}
				})
			}
			for _, id := range []uuid.UUID{aKey, bKey, missing} {
				reject(t, op, aAdmin, id, omitted, http.StatusInternalServerError, "could not resolve workspace visibility\n", "membership")
			}
			reject(t, op, global, aKey, matchingA, http.StatusInternalServerError, "could not load key\n", "key")
			for _, fault := range []string{"owner team", "team membership", "share"} {
				reject(t, op, aReader, sharedKey, matchingA, http.StatusInternalServerError, "could not load key\n", fault)
			}
		})
	}
	if activations != 0 || applied != beforeRevisions || readDirectWriteRevisions(t, st) != beforeRevisions || !reflect.DeepEqual(snapshot(t), beforeRows) {
		t.Error("denied writes changed keys, grants, revisions, or local activation")
	}
	if t.Failed() {
		t.FailNow()
	}

	for _, tc := range []struct {
		name, access string
		workspace    uuid.UUID
		selection    directWriteSelection
	}{
		{"tenant-admin-omitted", aAdmin, a.ID, omitted},
		{"dual-admin-query-only", dual, b.ID, directWriteSelection{query: "?workspace_id=" + bID}},
		{"global-system-header-only", global, systemID, directWriteSelection{headers: []string{systemID.String()}}},
		{"global-tenant-identical-repeats", global, a.ID, directWriteSelection{query: "?workspace_id=" + aID + "&workspace_id=" + aID, headers: []string{aID, aID}}},
		{"team-admin-header-only", aTeamAdmin, a.ID, directWriteSelection{headers: []string{aID}}},
	} {
		t.Run("allowed/"+tc.name, func(t *testing.T) {
			var ownerTeam *uuid.UUID
			if tc.workspace == a.ID {
				ownerTeam = &team.ID
			}
			id := seed(t, tc.name, tc.workspace, nil, ownerTeam)
			if ownerTeam != nil {
				if err := st.SetKeyBindings(ctx, id, []uuid.UUID{reader.ID}, []uuid.UUID{team.ID}); err != nil {
					t.Fatal(err)
				}
			}
			want, err := st.GetInboundKey(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			users, teams := readGrants(t, id)
			for _, op := range routes {
				before, previous := readDirectWriteRevisions(t, st), activations
				w := callDirectWrite(h, ctx, tc.access, op.method, keyPath(id)+op.suffix, op.body, tc.selection)
				var response struct {
					Key   adminKeyView `json:"key"`
					Token string       `json:"token"`
					OK    bool         `json:"ok"`
				}
				if err := json.Unmarshal(w.Body.Bytes(), &response); w.Code != http.StatusOK || err != nil {
					t.Fatalf("%s: %d %s (decode: %v)", op.name, w.Code, w.Body.String(), err)
				}
				if after := readDirectWriteRevisions(t, st); after.Keys <= before.Keys || after.Catalog != before.Catalog || applied != after || activations != previous+1 {
					t.Fatalf("%s did not commit and activate exactly once", op.name)
				}
				if op.name != "rotate" {
					assertNoDirectWriteMarkers(t, w.Header())
					if response.Token != "" {
						t.Fatalf("%s returned a bearer", op.name)
					}
				}
				switch op.name {
				case "policy":
					want.Description, want.Tenant = "updated", "fixture-tenant"
					if response.Key.ID != id.String() || response.Key.WorkspaceID != tc.workspace.String() || response.Key.Description != want.Description || !response.Key.CanManage {
						t.Fatal("policy update did not return the saved key")
					}
				case "revoke":
					want.Enabled = false
					if !response.OK {
						t.Fatal("missing revoke acknowledgement")
					}
				case "rotate":
					if len(response.Token) < 12 || store.TokenHash(response.Token) == want.TokenHash || w.Header().Get("X-Aiproxy-Key-Saved") != "true" ||
						w.Header().Get("X-Aiproxy-Key-ID") != id.String() || w.Header().Get("Location") != keyPath(id) || w.Header().Get("Cache-Control") != "no-store" {
						t.Fatal("rotation did not return a fresh one-time bearer and saved identity")
					}
					want.TokenHash, want.TokenPrefix, want.Enabled = store.TokenHash(response.Token), response.Token[:12], true
					detail := callDirectWrite(h, ctx, tc.access, http.MethodGet, keyPath(id), "", tc.selection)
					if detail.Code != http.StatusOK || strings.Contains(detail.Body.String(), response.Token) || strings.Contains(detail.Body.String(), `"token"`) {
						t.Fatal("could not read the rotated key safely")
					}
				case "delete":
					_, err := st.GetInboundKey(ctx, id)
					gotUsers, gotTeams := readGrants(t, id)
					if !response.OK || !errors.Is(err, sql.ErrNoRows) || len(gotUsers)+len(gotTeams) != 0 {
						t.Fatalf("delete did not remove the key and its bindings: %v", err)
					}
					continue
				}
				got, err := st.GetInboundKey(ctx, id)
				want.UpdatedAt = got.UpdatedAt
				gotUsers, gotTeams := readGrants(t, id)
				if err != nil || !reflect.DeepEqual(got, want) || !reflect.DeepEqual(gotUsers, users) || !reflect.DeepEqual(gotTeams, teams) {
					t.Fatalf("%s did not retain key identity, expected policy, owner and grants: %v", op.name, err)
				}
			}
		})
	}
}
