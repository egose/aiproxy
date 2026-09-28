package httpapi

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/egose/aiproxy/internal/adminauth"
	"github.com/egose/aiproxy/internal/store"
	"github.com/google/uuid"
)

func TestOffboardingDirectAuthorityMatrix(t *testing.T) {
	for _, removal := range []string{"transaction", "legacy-rows"} {
		for _, persona := range []string{"personal-owner", "team-admin", "shared-member"} {
			t.Run(removal+"/"+persona, func(t *testing.T) {
				st, h, systemAccess := openUsersTestStore(t)
				ctx := context.Background()
				workspace := store.Workspace{Name: uuid.NewString()}
				if err := st.CreateWorkspace(ctx, &workspace); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = st.DeleteWorkspace(ctx, workspace.ID) })
				u, access := membershipHTTPUser(t, st)
				successor, successorAccess := membershipHTTPUser(t, st)
				for _, id := range []uuid.UUID{u.ID, successor.ID} {
					if err := st.AddMembership(ctx, &store.WorkspaceMember{UserID: id, WorkspaceID: workspace.ID}); err != nil {
						t.Fatal(err)
					}
				}
				team := store.WorkspaceTeam{WorkspaceID: workspace.ID, Name: "team"}
				if err := st.CreateTeam(ctx, &team); err != nil {
					t.Fatal(err)
				}
				role := "member"
				if persona == "team-admin" {
					role = "admin"
				}
				if err := st.AddTeamMember(ctx, u.ID, team.ID, role); err != nil {
					t.Fatal(err)
				}
				if err := st.AddTeamMember(ctx, successor.ID, team.ID, "admin"); err != nil {
					t.Fatal(err)
				}
				key := store.InboundKey{Name: "offboard-" + uuid.NewString(), WorkspaceID: workspace.ID, TokenHash: store.TokenHash("fixture-" + uuid.NewString()), TokenPrefix: "private-prefix", Description: "private-description", Enabled: true, AllowedModels: []string{"private/model"}}
				if persona == "personal-owner" {
					key.OwnerUserID = &u.ID
				} else {
					key.OwnerTeamID = &team.ID
				}
				if err := st.CreateInboundKey(ctx, &key); err != nil {
					t.Fatal(err)
				}
				if err := st.SetKeyBindings(ctx, key.ID, []uuid.UUID{u.ID}, []uuid.UUID{team.ID}); err != nil {
					t.Fatal(err)
				}
				keyPath := "/_internal/admin/keys/" + key.ID.String()
				quotaPath := "/_internal/admin/workspaces/" + workspace.ID.String() + "/teams/" + team.ID.String() + "/quota"
				for _, path := range []string{keyPath, quotaPath} {
					w := membershipRequest(ctx, h, access, http.MethodGet, path, "")
					if w.Code != http.StatusOK {
						t.Fatalf("current member GET = %d %s", w.Code, w.Body.String())
					}
				}
				w := membershipRequest(ctx, h, successorAccess, http.MethodPut, quotaPath, `{"tpm":[{"model":"private/model","effective":12}]}`)
				if w.Code != http.StatusOK {
					t.Fatalf("current admin quota PUT = %d %s", w.Code, w.Body.String())
				}
				if persona != "shared-member" {
					w = membershipRequest(ctx, h, access, http.MethodPut, keyPath, `{"description":"private-description"}`)
					if w.Code != http.StatusOK {
						t.Fatalf("current manager PUT = %d %s", w.Code, w.Body.String())
					}
				}
				if removal == "transaction" {
					path := "/_internal/admin/workspaces/" + workspace.ID.String() + "/members/" + u.ID.String()
					w = membershipRequest(ctx, h, systemAccess, http.MethodDelete, path, "")
					if w.Code != http.StatusOK {
						t.Fatalf("offboard = %d %s", w.Code, w.Body.String())
					}
				} else {
					if _, err := st.DB.NewDelete().Model((*store.WorkspaceMember)(nil)).Where("user_id = ? AND workspace_id = ?", u.ID, workspace.ID).Exec(ctx); err != nil {
						t.Fatal(err)
					}
				}
				var before store.InboundKey
				if err := st.DB.NewSelect().Model(&before).Where("id = ?", key.ID).Scan(ctx); err != nil {
					t.Fatal(err)
				}
				quotaBefore, err := st.ListScopeQuotasByScope(ctx, workspace.ID, "team", team.ID)
				if err != nil {
					t.Fatal(err)
				}
				userBefore, err := st.ListKeyUserIDs(ctx, key.ID)
				if err != nil {
					t.Fatal(err)
				}
				teamBefore, err := st.ListKeyTeamIDs(ctx, key.ID)
				if err != nil {
					t.Fatal(err)
				}
				activations := 0
				deps := h.current()
				deps.RequestReload = func() error { activations++; return nil }
				h.UpdateDependencies(deps)
				if _, err := deps.Quota.scopeSpend(ctx, workspace.ID, quotaScope{typ: quotaScopeTeam, id: team.ID}); err != nil {
					t.Fatal(err)
				}
				cacheKey := workspace.ID.String() + "|team|" + team.ID.String()
				deps.Quota.mu.Lock()
				cachedBefore := deps.Quota.spend[cacheKey]
				deps.Quota.mu.Unlock()
				claims := &adminauth.Claims{}
				claims.Subject = u.ID.String()
				if h.canManageKey(deps, ctx, claims, before) || h.isTeamAdmin(deps, ctx, claims, team.ID) {
					t.Error("shared authority gate accepts former member")
				}
				for _, tc := range []struct {
					method, path, body string
					code               int
				}{
					{http.MethodGet, keyPath, "", http.StatusNotFound},
					{http.MethodPut, keyPath, `{"description":"hijacked","tenant":"hijacked","allowed_models":["hijacked/model"],"user_ids":[],"team_ids":[]}`, http.StatusForbidden},
					{http.MethodPost, keyPath + "/rotate", "", http.StatusForbidden},
					{http.MethodPost, keyPath + "/revoke", "", http.StatusForbidden},
					{http.MethodDelete, keyPath, "", http.StatusForbidden},
					{http.MethodGet, quotaPath, "", http.StatusNotFound},
					{http.MethodPut, quotaPath, `{"tpm":[{"model":"private/model","effective":3}]}`, http.StatusNotFound},
				} {
					t.Run(tc.method+"/"+tc.path, func(t *testing.T) {
						w := membershipRequest(ctx, h, access, tc.method, tc.path, tc.body)
						if w.Code != tc.code {
							t.Errorf("status=%d want=%d body=%s", w.Code, tc.code, w.Body.String())
						}
						for _, secret := range []string{before.TokenHash, before.TokenPrefix, before.Description, before.Name, `"token"`, `"tpm"`, `"spend_micros"`} {
							if strings.Contains(w.Body.String(), secret) {
								t.Errorf("response leaked %q: %s", secret, w.Body.String())
							}
						}
						var after store.InboundKey
						if err := st.DB.NewSelect().Model(&after).Where("id = ?", key.ID).Scan(ctx); err != nil || !reflect.DeepEqual(before, after) {
							t.Errorf("key mutated: %+v %v", after, err)
						}
						quotaAfter, err := st.ListScopeQuotasByScope(ctx, workspace.ID, "team", team.ID)
						if err != nil || !reflect.DeepEqual(quotaBefore, quotaAfter) {
							t.Errorf("quota mutated: %+v %v", quotaAfter, err)
						}
						userAfter, err := st.ListKeyUserIDs(ctx, key.ID)
						if err != nil || !reflect.DeepEqual(userBefore, userAfter) {
							t.Errorf("user shares mutated: %v %v", userAfter, err)
						}
						teamAfter, err := st.ListKeyTeamIDs(ctx, key.ID)
						if err != nil || !reflect.DeepEqual(teamBefore, teamAfter) {
							t.Errorf("team shares mutated: %v %v", teamAfter, err)
						}
						if activations != 0 {
							t.Errorf("denied request activated %d changes", activations)
						}
						deps.Quota.mu.Lock()
						cachedAfter, exists := deps.Quota.spend[cacheKey]
						deps.Quota.mu.Unlock()
						if !exists || cachedAfter != cachedBefore {
							t.Error("denied request invalidated quota cache")
						}
					})
				}
				if t.Failed() {
					return
				}
				w = membershipRequest(ctx, h, systemAccess, http.MethodGet, keyPath, "")
				if w.Code != http.StatusOK {
					t.Fatalf("system access lost: %d %s", w.Code, w.Body.String())
				}
				if removal == "transaction" {
					if err := st.AddMembership(ctx, &store.WorkspaceMember{UserID: u.ID, WorkspaceID: workspace.ID}); err != nil {
						t.Fatal(err)
					}
					if _, err := st.GetTeamMember(ctx, u.ID, team.ID); !errors.Is(err, sql.ErrNoRows) {
						t.Fatalf("team grant restored: %v", err)
					}
					ids, err := st.ListKeyUserIDs(ctx, key.ID)
					if err != nil || len(ids) != 0 {
						t.Fatalf("direct share restored: %v %v", ids, err)
					}
					w = membershipRequest(ctx, h, access, http.MethodGet, quotaPath, "")
					if w.Code != http.StatusNotFound {
						t.Fatalf("team quota restored: %d", w.Code)
					}
					w = membershipRequest(ctx, h, access, http.MethodGet, keyPath, "")
					want := http.StatusNotFound
					if persona == "personal-owner" {
						want = http.StatusOK
					}
					if w.Code != want {
						t.Fatalf("readded member key visibility=%d want=%d", w.Code, want)
					}
				}
				w = membershipRequest(ctx, h, systemAccess, http.MethodPost, keyPath+"/revoke", "")
				if w.Code != http.StatusOK || activations != 1 {
					t.Fatalf("explicit system revoke=%d activations=%d", w.Code, activations)
				}
			})
		}
	}
}

func TestOffboardingHTTPRequiresTeamHandoff(t *testing.T) {
	st, h, systemAccess := openUsersTestStore(t)
	ctx := context.Background()
	workspace := store.Workspace{Name: uuid.NewString()}
	if err := st.CreateWorkspace(ctx, &workspace); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.DeleteWorkspace(ctx, workspace.ID) })
	u, _ := membershipHTTPUser(t, st)
	v, _ := membershipHTTPUser(t, st)
	for _, id := range []uuid.UUID{u.ID, v.ID} {
		if err := st.AddMembership(ctx, &store.WorkspaceMember{UserID: id, WorkspaceID: workspace.ID}); err != nil {
			t.Fatal(err)
		}
	}
	team := store.WorkspaceTeam{WorkspaceID: workspace.ID, Name: "team"}
	if err := st.CreateTeam(ctx, &team); err != nil {
		t.Fatal(err)
	}
	if err := st.AddTeamMember(ctx, u.ID, team.ID, "admin"); err != nil {
		t.Fatal(err)
	}
	path := "/_internal/admin/workspaces/" + workspace.ID.String() + "/members/" + u.ID.String()
	w := membershipRequest(ctx, h, systemAccess, http.MethodDelete, path, "")
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "assign another team admin first") {
		t.Fatalf("handoff rejection=%d %s", w.Code, w.Body.String())
	}
	if _, err := st.GetMembership(ctx, u.ID, workspace.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.AddTeamMember(ctx, v.ID, team.ID, "admin"); err != nil {
		t.Fatal(err)
	}
	w = membershipRequest(ctx, h, systemAccess, http.MethodDelete, path, "")
	if w.Code != http.StatusOK {
		t.Fatalf("handoff retry=%d %s", w.Code, w.Body.String())
	}
	if _, err := st.GetTeamMember(ctx, u.ID, team.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("former admin retained: %v", err)
	}
}
