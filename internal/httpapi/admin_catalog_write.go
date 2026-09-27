package httpapi

import (
	"errors"
	"net/http"

	"github.com/egose/aiproxy/internal/store"
)

type catalogStorageError struct{ error }

func (e catalogStorageError) Unwrap() error { return e.error }

func writeCatalogError(deps Dependencies, w http.ResponseWriter, err error) {
	var storage catalogStorageError
	switch {
	case errors.Is(err, store.ErrCatalogConflict):
		http.Error(w, store.ErrCatalogConflict.Error(), http.StatusConflict)
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
	if deps.RequestReload != nil {
		if err := deps.RequestReload(); err != nil {
			if deps.Logger != nil {
				deps.Logger.Error("catalog activation failed", "error", err)
			}
			http.Error(w, "saved but activation failed", http.StatusInternalServerError)
			return false
		}
	}
	return true
}

func writeCatalogPresentationError(deps Dependencies, w http.ResponseWriter, err error) {
	if deps.Logger != nil {
		deps.Logger.Error("saved catalog presentation failed", "error", err)
	}
	http.Error(w, "saved but response view unavailable; read current state before retrying", http.StatusInternalServerError)
}
