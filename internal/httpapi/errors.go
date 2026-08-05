package httpapi

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"interface-load-test/internal/accountstore"
	"interface-load-test/internal/export"
	"interface-load-test/internal/loadtest"
	"interface-load-test/internal/scenariostore"
	"interface-load-test/internal/taskmanager"
)

const internalErrorMessage = "internal error"

func classifyError(err error) (status int, message string) {
	switch {
	case errors.Is(err, taskmanager.ErrNotFound),
		errors.Is(err, accountstore.ErrNotFound),
		errors.Is(err, scenariostore.ErrNotFound),
		errors.Is(err, export.ErrNoResults):
		return http.StatusNotFound, err.Error()
	case errors.Is(err, taskmanager.ErrNoAccounts),
		errors.Is(err, taskmanager.ErrInvalidConcurrency),
		errors.Is(err, taskmanager.ErrConcurrencyExceedsAccounts),
		errors.Is(err, taskmanager.ErrInvalidTotalCount),
		errors.Is(err, taskmanager.ErrUnknownModuleType),
		errors.Is(err, accountstore.ErrGroupIDRequired),
		errors.Is(err, accountstore.ErrUsernameRequired),
		errors.Is(err, accountstore.ErrPasswordRequired),
		errors.Is(err, scenariostore.ErrNameRequired),
		errors.Is(err, scenariostore.ErrStepsRequired),
		errors.Is(err, loadtest.ErrInvalidConfig),
		errors.Is(err, loadtest.ErrScenarioStepsRequired),
		errors.Is(err, loadtest.ErrPerAccountCount):
		return http.StatusBadRequest, err.Error()
	default:
		var syntaxError *json.SyntaxError
		var typeError *json.UnmarshalTypeError
		if errors.As(err, &syntaxError) || errors.As(err, &typeError) {
			return http.StatusBadRequest, err.Error()
		}
		return http.StatusInternalServerError, internalErrorMessage
	}
}

func writeError(w http.ResponseWriter, err error) {
	status, message := classifyError(err)
	if status == http.StatusInternalServerError {
		log.Printf("httpapi internal error: %v", err)
	}
	writeJSON(w, status, map[string]string{"error": message})
}

func writeBadRequest(w http.ResponseWriter, message string) {
	writeJSON(w, http.StatusBadRequest, map[string]string{"error": message})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("httpapi write json: %v", err)
	}
}
