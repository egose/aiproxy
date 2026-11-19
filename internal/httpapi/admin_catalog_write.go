package httpapi

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/egose/aiproxy/internal/adminauth"
	"github.com/egose/aiproxy/internal/store"
	"github.com/google/uuid"
)

type catalogResource string

const (
	catalogProviderResource catalogResource = "provider"
	catalogAliasResource    catalogResource = "alias"
)

var errInvalidCatalogWorkspaceID = errors.New("invalid workspace id")

type catalogWriteTarget struct {
	id          uuid.UUID
	workspaceID uuid.UUID
}

type catalogBodyWorkspaceID struct {
	value *string
}

func (id *catalogBodyWorkspaceID) UnmarshalJSON(raw []byte) error {
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return errInvalidCatalogWorkspaceID
	}
	id.value = &value
	return nil
}

func writeCatalogWriteBodyError(w http.ResponseWriter, err error) {
	message := "invalid body"
	if errors.Is(err, errInvalidCatalogWorkspaceID) {
		message = errInvalidCatalogWorkspaceID.Error()
	}
	http.Error(w, message, http.StatusBadRequest)
}

type catalogWriteDenial struct {
	status  int
	message string
}

func (e catalogWriteDenial) Error() string { return e.message }

func (resource catalogResource) notFound() catalogWriteDenial {
	return catalogWriteDenial{http.StatusNotFound, string(resource) + " not found"}
}

func (resource catalogResource) targetReadError(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return resource.notFound()
	}
	return catalogStorageError{err}
}

func parseWriteWorkspaceIDs(r *http.Request, bodyWorkspaceID *string) ([]uuid.UUID, error) {
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return nil, errInvalidCatalogWorkspaceID
	}
	values := append(query["workspace_id"], r.Header.Values("X-Workspace-ID")...)
	if bodyWorkspaceID != nil {
		values = append(values, *bodyWorkspaceID)
	}
	ids := make([]uuid.UUID, 0, len(values))
	for _, value := range values {
		id, err := uuid.Parse(strings.TrimSpace(value))
		if err != nil {
			return nil, errInvalidCatalogWorkspaceID
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func (h *Handler) resolveCatalogWriteTarget(deps Dependencies, r *http.Request, claims *adminauth.Claims, resource catalogResource, name string, bodyWorkspaceID *string) (catalogWriteTarget, error) {
	workspaceIDs, err := parseWriteWorkspaceIDs(r, bodyWorkspaceID)
	if err != nil {
		return catalogWriteTarget{}, err
	}
	ctx := r.Context()
	visibility, err := resolveWorkspaceVisibility(ctx, deps, claims)
	if err != nil {
		return catalogWriteTarget{}, catalogWriteDenial{http.StatusInternalServerError, workspaceVisibilityError}
	}
	if !visibility.allowsWriteSelectors(workspaceIDs) {
		return catalogWriteTarget{}, resource.notFound()
	}
	allows := func(id uuid.UUID) bool {
		return visibility.allows(id) && (len(workspaceIDs) == 0 || id == workspaceIDs[0])
	}
	status := http.StatusBadRequest
	if r.Method == http.MethodDelete {
		status = http.StatusForbidden
	}
	var target catalogWriteTarget
	switch resource {
	case catalogProviderResource:
		var ref store.ProviderReference
		ref, err = deps.AdminStore.GetProviderReference(ctx, name)
		target = catalogWriteTarget{ref.ID, ref.WorkspaceID}
	case catalogAliasResource:
		var ref store.AliasReference
		ref, err = deps.AdminStore.GetAliasReference(ctx, name)
		target = catalogWriteTarget{ref.ID, ref.WorkspaceID}
	default:
		return catalogWriteTarget{}, resource.notFound()
	}
	if errors.Is(err, sql.ErrNoRows) {
		if allows(store.SystemWorkspaceID) {
			var static bool
			switch resource {
			case catalogProviderResource:
				static = h.isStaticProvider(deps, name)
			case catalogAliasResource:
				a, ok := deps.Catalog.Alias(name)
				static = ok && !a.DatabaseManaged
			}
			if static {
				return catalogWriteTarget{}, catalogWriteDenial{status, "config-managed: edit the HCL file"}
			}
		}
		return catalogWriteTarget{}, resource.notFound()
	}
	if err != nil {
		return catalogWriteTarget{}, catalogStorageError{err}
	}
	if !allows(target.workspaceID) {
		return catalogWriteTarget{}, resource.notFound()
	}
	if !visibility.global {
		workspace, err := deps.AdminStore.GetWorkspace(ctx, target.workspaceID)
		if err != nil {
			return catalogWriteTarget{}, resource.targetReadError(err)
		}
		if r.Method == http.MethodPost {
			status = http.StatusForbidden
		}
		if workspace.IsSystem && status == http.StatusForbidden {
			return catalogWriteTarget{}, catalogWriteDenial{http.StatusNotFound, "workspace not found"}
		}
		if workspace.IsSystem || visibility.roles[target.workspaceID] != roleAdmin {
			return catalogWriteTarget{}, catalogWriteDenial{status, "workspace admin required"}
		}
	}
	return target, nil
}

type catalogStorageError struct{ error }

func (e catalogStorageError) Unwrap() error { return e.error }

func writeCatalogError(deps Dependencies, w http.ResponseWriter, err error) {
	var denial catalogWriteDenial
	var storage catalogStorageError
	switch {
	case errors.As(err, &denial):
		http.Error(w, denial.message, denial.status)
	case errors.Is(err, store.ErrCatalogConflict):
		http.Error(w, store.ErrCatalogConflict.Error(), http.StatusConflict)
	case errors.Is(err, store.ErrCatalogDependency):
		if deps.Logger != nil {
			deps.Logger.Warn("catalog dependency rejected", "error", err)
		}
		http.Error(w, store.ErrCatalogDependency.Error(), http.StatusBadRequest)
	case errors.Is(err, errDeviceFlowNotReady):
		http.Error(w, errDeviceFlowNotReady.Error(), http.StatusConflict)
	case errors.As(err, &storage):
		if deps.Logger != nil {
			deps.Logger.Error("catalog storage failed", "error", err)
		}
		http.Error(w, "could not save catalog edit", http.StatusInternalServerError)
	default:
		http.Error(w, err.Error(), http.StatusBadRequest)
	}
}

func activateCatalogChange(deps Dependencies, w http.ResponseWriter) bool {
	w.Header().Set("X-Aiproxy-Cluster-Acknowledged", "false")
	w.Header().Set("X-Aiproxy-Local-Active", "false")
	if deps.RequestReload != nil {
		err := deps.RequestReload()
		if deps.AppliedServingRevisions != nil {
			revisions := deps.AppliedServingRevisions()
			w.Header().Set("X-Aiproxy-Applied-Catalog-Revision", strconv.FormatInt(revisions.Catalog, 10))
			w.Header().Set("X-Aiproxy-Applied-Key-Revision", strconv.FormatInt(revisions.Keys, 10))
		}
		if err != nil {
			if deps.Logger != nil {
				deps.Logger.Error("catalog activation failed", "error", err)
			}
			http.Error(w, "saved but activation failed", http.StatusInternalServerError)
			return false
		}
		w.Header().Set("X-Aiproxy-Local-Active", "true")
	}
	return true
}

func writeCatalogPresentationError(deps Dependencies, w http.ResponseWriter, err error) {
	if deps.Logger != nil {
		deps.Logger.Error("saved catalog presentation failed", "error", err)
	}
	http.Error(w, "saved but response view unavailable; read current state before retrying", http.StatusInternalServerError)
}
