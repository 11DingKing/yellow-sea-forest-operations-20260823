package httpapi

import (
	"net/http"

	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/domain"
	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/service"
)

func (a *API) startForestSurvey(w http.ResponseWriter, r *http.Request) {
	caseID, err := pathID(r, "caseID")
	if err != nil {
		writeError(w, err)
		return
	}
	forest_survey, err := a.service.StartForestSurvey(r.Context(), principalFrom(r.Context()), caseID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, forest_survey)
}

type readingRequest struct {
	Position   string  `json:"position"`
	Lux        float64 `json:"lux"`
	MeasuredAt string  `json:"measured_at"`
	Sequence   int     `json:"sequence"`
}

func (a *API) addReading(w http.ResponseWriter, r *http.Request) {
	forest_surveyID, err := pathID(r, "forest_surveyID")
	if err != nil {
		writeError(w, err)
		return
	}
	var input readingRequest
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, err)
		return
	}
	reading, err := a.service.AddReading(r.Context(), principalFrom(r.Context()), service.AddReadingInput{ForestSurveyID: forest_surveyID, Position: input.Position, Lux: input.Lux, MeasuredAt: input.MeasuredAt, Sequence: input.Sequence})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, reading)
}

func (a *API) publishForestSurvey(w http.ResponseWriter, r *http.Request) {
	forest_surveyID, err := pathID(r, "forest_surveyID")
	if err != nil {
		writeError(w, err)
		return
	}
	forest_survey, err := a.service.PublishForestSurvey(r.Context(), principalFrom(r.Context()), forest_surveyID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, forest_survey)
}

type proposedActionRequest struct {
	ForestAssetID domain.ID                    `json:"forest_asset_id"`
	Type          domain.ForestAssetActionType `json:"type"`
	TargetValue   string                       `json:"target_value"`
}

type planRequest struct {
	CaseID         domain.ID               `json:"case_id"`
	ForestSurveyID domain.ID               `json:"forest_survey_id"`
	Description    string                  `json:"description"`
	CutoffMinute   int                     `json:"cutoff_minute"`
	ExpectedMaxLux float64                 `json:"expected_max_lux"`
	Actions        []proposedActionRequest `json:"actions"`
}

func (a *API) proposePlan(w http.ResponseWriter, r *http.Request) {
	var input planRequest
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, err)
		return
	}
	actions := make([]service.ProposedAction, 0, len(input.Actions))
	for _, item := range input.Actions {
		actions = append(actions, service.ProposedAction{ForestAssetID: item.ForestAssetID, Type: item.Type, TargetValue: item.TargetValue})
	}
	result, err := a.service.ProposePlan(r.Context(), principalFrom(r.Context()), service.ProposePlanInput{
		CaseID: input.CaseID, ForestSurveyID: input.ForestSurveyID, Description: input.Description,
		CutoffMinute: input.CutoffMinute, ExpectedMaxLux: input.ExpectedMaxLux, Actions: actions,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (a *API) acceptPlan(w http.ResponseWriter, r *http.Request) {
	planID, err := pathID(r, "planID")
	if err != nil {
		writeError(w, err)
		return
	}
	plan, err := a.service.AcceptPlan(r.Context(), principalFrom(r.Context()), planID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, plan)
}

func (a *API) approvePlan(w http.ResponseWriter, r *http.Request) {
	planID, err := pathID(r, "planID")
	if err != nil {
		writeError(w, err)
		return
	}
	plan, err := a.service.ApprovePlan(r.Context(), principalFrom(r.Context()), planID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, plan)
}

func (a *API) claimAction(w http.ResponseWriter, r *http.Request) {
	actionID, err := pathID(r, "actionID")
	if err != nil {
		writeError(w, err)
		return
	}
	action, err := a.service.ClaimForestAssetAction(r.Context(), principalFrom(r.Context()), actionID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, action)
}

type completeActionRequest struct {
	ExpectedVersion int64 `json:"expected_version"`
}

func (a *API) completeAction(w http.ResponseWriter, r *http.Request) {
	actionID, err := pathID(r, "actionID")
	if err != nil {
		writeError(w, err)
		return
	}
	var input completeActionRequest
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, err)
		return
	}
	if err := a.service.CompleteForestAssetAction(r.Context(), principalFrom(r.Context()), actionID, input.ExpectedVersion); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) startInspection(w http.ResponseWriter, r *http.Request) {
	caseID, err := pathID(r, "caseID")
	if err != nil {
		writeError(w, err)
		return
	}
	round, err := a.service.StartInspection(r.Context(), principalFrom(r.Context()), caseID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, round)
}

type verificationRequest struct {
	ObservedMaxLux float64 `json:"observed_max_lux"`
	ResidentAgreed bool    `json:"resident_agreed"`
	Notes          string  `json:"notes"`
}

func (a *API) completeInspection(w http.ResponseWriter, r *http.Request) {
	verificationID, err := pathID(r, "verificationID")
	if err != nil {
		writeError(w, err)
		return
	}
	var input verificationRequest
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, err)
		return
	}
	round, err := a.service.CompleteInspection(r.Context(), principalFrom(r.Context()), service.CompleteInspectionInput{InspectionID: verificationID, ObservedMaxLux: input.ObservedMaxLux, ResidentAgreed: input.ResidentAgreed, Notes: input.Notes})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, round)
}

type reopenRequest struct {
	Reason string `json:"reason"`
}

func (a *API) reopenCase(w http.ResponseWriter, r *http.Request) {
	caseID, err := pathID(r, "caseID")
	if err != nil {
		writeError(w, err)
		return
	}
	var input reopenRequest
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, err)
		return
	}
	item, err := a.service.ReopenCase(r.Context(), principalFrom(r.Context()), service.ReopenCaseInput{CaseID: caseID, IdempotencyKey: r.Header.Get("Idempotency-Key"), Reason: input.Reason})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}
