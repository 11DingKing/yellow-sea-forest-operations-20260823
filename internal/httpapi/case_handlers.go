package httpapi

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/domain"
	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/service"
)

type submitCaseRequest struct {
	ForestSiteID   domain.ID `json:"forest_site_id"`
	ForestParcelID domain.ID `json:"forest_parcel_id"`
	Title          string    `json:"title"`
	Description    string    `json:"description"`
	Priority       int       `json:"priority"`
}

func (a *API) submitCase(w http.ResponseWriter, r *http.Request) {
	var input submitCaseRequest
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, err)
		return
	}
	result, err := a.service.SubmitCase(r.Context(), principalFrom(r.Context()), service.SubmitCaseInput{
		ForestSiteID: input.ForestSiteID, ForestParcelID: input.ForestParcelID, Title: input.Title,
		Description: input.Description, Priority: input.Priority, IdempotencyKey: r.Header.Get("Idempotency-Key"),
	})
	if err != nil {
		writeError(w, err)
		return
	}
	status := http.StatusCreated
	if result.Replayed {
		status = http.StatusOK
		w.Header().Set("Idempotent-Replayed", "true")
	}
	writeJSON(w, status, result.Case)
}

func (a *API) getCase(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "caseID")
	if err != nil {
		writeError(w, err)
		return
	}
	item, err := a.service.GetCase(r.Context(), principalFrom(r.Context()), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (a *API) listCases(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	page, err := queryInt(query.Get("page"), 1, 1, 1_000_000)
	if err != nil {
		writeError(w, err)
		return
	}
	pageSize, err := queryInt(query.Get("page_size"), 50, 1, 200)
	if err != nil {
		writeError(w, err)
		return
	}
	var submittedAfter *time.Time
	if raw := strings.TrimSpace(query.Get("submitted_after")); raw != "" {
		parsed, parseErr := time.Parse(time.RFC3339, raw)
		if parseErr != nil {
			writeError(w, domain.FieldError{Field: "submitted_after", Message: "must be RFC3339"})
			return
		}
		submittedAfter = &parsed
	}
	filter := domain.CaseFilter{CreatedFrom: submittedAfter}
	if raw := strings.TrimSpace(query.Get("forest_site_id")); raw != "" {
		value := domain.ID(raw)
		if !value.Valid() {
			writeError(w, domain.FieldError{Field: "forest_site_id", Message: "is invalid"})
			return
		}
		filter.ForestSiteID = &value
	}
	if raw := strings.TrimSpace(query.Get("zone_id")); raw != "" {
		value := domain.ID(raw)
		if !value.Valid() {
			writeError(w, domain.FieldError{Field: "zone_id", Message: "is invalid"})
			return
		}
		filter.ZoneID = &value
	}
	for _, raw := range query["status"] {
		for _, value := range strings.Split(raw, ",") {
			if value = strings.TrimSpace(value); value != "" {
				filter.Statuses = append(filter.Statuses, domain.CaseStatus(value))
			}
		}
	}
	result, err := a.service.ListCases(r.Context(), principalFrom(r.Context()), filter, domain.PageRequest{Page: page, PageSize: pageSize, Sort: query.Get("sort"), Desc: query.Get("direction") != "asc"})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

type assignmentRequest struct {
	OwnerID         domain.ID `json:"owner_id"`
	ExpectedVersion int64     `json:"expected_version"`
}

func (a *API) assignCase(w http.ResponseWriter, r *http.Request) {
	caseID, err := pathID(r, "caseID")
	if err != nil {
		writeError(w, err)
		return
	}
	var input assignmentRequest
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, err)
		return
	}
	item, err := a.service.AssignCase(r.Context(), principalFrom(r.Context()), service.AssignCaseInput{CaseID: caseID, OwnerID: input.OwnerID, ExpectedVersion: input.ExpectedVersion})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func queryInt(raw string, fallback, minimum, maximum int) (int, error) {
	if strings.TrimSpace(raw) == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < minimum || value > maximum {
		return 0, domain.FieldError{Field: "query", Message: "integer is outside the supported range"}
	}
	return value, nil
}
