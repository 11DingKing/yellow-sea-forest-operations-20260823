package domain

import (
	"errors"
	"math"
	"testing"
	"time"
)

func TestForestSurveyPublishUsesTrimmedMean(t *testing.T) {
	now := fixedTime()
	forest_survey := NewForestSurvey("forest_survey_001", "case_001", "inspector_001", now)
	readings := []ForestSurveyReading{
		{ID: "reading_001", ForestSurveyID: forest_survey.ID, Lux: 100, Sequence: 1},
		{ID: "reading_002", ForestSurveyID: forest_survey.ID, Lux: 10, Sequence: 2},
		{ID: "reading_003", ForestSurveyID: forest_survey.ID, Lux: 20, Sequence: 3},
		{ID: "reading_004", ForestSurveyID: forest_survey.ID, Lux: 30, Sequence: 4},
		{ID: "reading_005", ForestSurveyID: forest_survey.ID, Lux: 40, Sequence: 5},
	}
	publishedAt := now.Add(30 * time.Minute)
	if err := forest_survey.Publish(readings, publishedAt); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	if forest_survey.SummaryLux != 30 {
		t.Fatalf("SummaryLux = %v, want 30", forest_survey.SummaryLux)
	}
	if forest_survey.Status != ForestSurveyPublished || forest_survey.Version != 2 {
		t.Fatalf("published forest_survey = %#v", forest_survey)
	}
	if forest_survey.PublishedAt == nil || !forest_survey.PublishedAt.Equal(publishedAt.UTC()) {
		t.Fatalf("PublishedAt = %v", forest_survey.PublishedAt)
	}
}

func TestForestSurveyPublishUsesAllValuesForSmallSamples(t *testing.T) {
	forest_survey := NewForestSurvey("forest_survey_001", "case_001", "inspector_001", fixedTime())
	readings := []ForestSurveyReading{
		{ForestSurveyID: forest_survey.ID, Lux: 8, Sequence: 1},
		{ForestSurveyID: forest_survey.ID, Lux: 14, Sequence: 2},
		{ForestSurveyID: forest_survey.ID, Lux: 20, Sequence: 3},
	}
	if err := forest_survey.Publish(readings, fixedTime()); err != nil {
		t.Fatal(err)
	}
	if forest_survey.SummaryLux != 14 {
		t.Fatalf("SummaryLux = %v, want 14", forest_survey.SummaryLux)
	}
}

func TestForestSurveyPublishRejectsIncompleteOrMixedReadings(t *testing.T) {
	tests := []struct {
		name     string
		readings []ForestSurveyReading
		want     error
	}{
		{name: "too few", readings: []ForestSurveyReading{{ForestSurveyID: "forest_survey_001", Sequence: 1}, {ForestSurveyID: "forest_survey_001", Sequence: 2}}, want: ErrValidation},
		{name: "other forest_survey", readings: []ForestSurveyReading{{ForestSurveyID: "forest_survey_001", Sequence: 1}, {ForestSurveyID: "forest_survey_other", Sequence: 2}, {ForestSurveyID: "forest_survey_001", Sequence: 3}}, want: ErrValidation},
		{name: "duplicate sequence", readings: []ForestSurveyReading{{ForestSurveyID: "forest_survey_001", Sequence: 1}, {ForestSurveyID: "forest_survey_001", Sequence: 1}, {ForestSurveyID: "forest_survey_001", Sequence: 3}}, want: ErrConflict},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			forest_survey := NewForestSurvey("forest_survey_001", "case_001", "inspector_001", fixedTime())
			before := forest_survey
			err := forest_survey.Publish(test.readings, fixedTime().Add(time.Minute))
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
			if forest_survey != before {
				t.Fatalf("failed publish mutated forest_survey: before=%#v after=%#v", before, forest_survey)
			}
		})
	}
}

func TestForestSurveyCannotPublishTwice(t *testing.T) {
	forest_survey := NewForestSurvey("forest_survey_001", "case_001", "inspector_001", fixedTime())
	readings := []ForestSurveyReading{{ForestSurveyID: forest_survey.ID, Sequence: 1}, {ForestSurveyID: forest_survey.ID, Sequence: 2}, {ForestSurveyID: forest_survey.ID, Sequence: 3}}
	if err := forest_survey.Publish(readings, fixedTime()); err != nil {
		t.Fatal(err)
	}
	err := forest_survey.Publish(readings, fixedTime().Add(time.Minute))
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("error = %v, want invalid transition", err)
	}
}

func TestNewReadingValidation(t *testing.T) {
	tests := []struct {
		name     string
		position string
		lux      float64
		sequence int
		valid    bool
	}{
		{name: "valid zero", position: "balcony-1", lux: 0, sequence: 1, valid: true},
		{name: "valid maximum", position: "window-8", lux: 200000, sequence: 9, valid: true},
		{name: "missing position", position: "", lux: 12, sequence: 1},
		{name: "negative lux", position: "balcony", lux: -0.1, sequence: 1},
		{name: "excess lux", position: "balcony", lux: 200001, sequence: 1},
		{name: "nan lux", position: "balcony", lux: math.NaN(), sequence: 1},
		{name: "infinite lux", position: "balcony", lux: math.Inf(1), sequence: 1},
		{name: "blank position", position: "   ", lux: 12, sequence: 1},
		{name: "zero sequence", position: "balcony", lux: 12, sequence: 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reading, err := NewReading("reading_001", "forest_survey_001", test.position, test.lux, fixedTime(), test.sequence)
			if test.valid && err != nil {
				t.Fatalf("NewReading() error = %v", err)
			}
			if !test.valid && !errors.Is(err, ErrValidation) {
				t.Fatalf("error = %v, want validation", err)
			}
			if test.valid && reading.MeasuredAt.Location() != time.UTC {
				t.Fatalf("MeasuredAt location = %s", reading.MeasuredAt.Location())
			}
		})
	}
}

func TestStewardshipPlanLifecycle(t *testing.T) {
	plan, err := NewStewardshipPlan("plan_001", "case_001", "forest_survey_001", "operator_001", "Install shields, lower lamps, and enforce the cutoff.", 22*60+30, 8, fixedTime())
	if err != nil {
		t.Fatal(err)
	}
	if plan.Status != PlanDraft || plan.Version != 1 {
		t.Fatalf("new plan = %#v", plan)
	}
	if err := plan.AcceptOperator(fixedTime().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if plan.Status != PlanOperatorAccepted || plan.Version != 2 {
		t.Fatalf("accepted plan = %#v", plan)
	}
	approvedAt := fixedTime().Add(2 * time.Minute)
	if err := plan.Approve(approvedAt); err != nil {
		t.Fatal(err)
	}
	if plan.Status != PlanApproved || plan.ApprovedAt == nil || !plan.ApprovedAt.Equal(approvedAt.UTC()) {
		t.Fatalf("approved plan = %#v", plan)
	}
	if err := plan.BeginExecution(fixedTime().Add(3 * time.Minute)); err != nil {
		t.Fatal(err)
	}
	if plan.Status != PlanExecuting || plan.Version != 4 {
		t.Fatalf("executing plan = %#v", plan)
	}
	if err := plan.Complete(fixedTime().Add(4 * time.Minute)); err != nil {
		t.Fatal(err)
	}
	if plan.Status != PlanCompleted || plan.Version != 5 {
		t.Fatalf("completed plan = %#v", plan)
	}
}

func TestStewardshipPlanRejectsOutOfOrderLifecycle(t *testing.T) {
	tests := []struct {
		name   string
		status PlanStatus
		apply  func(*StewardshipPlan) error
	}{
		{name: "approve draft", status: PlanDraft, apply: func(plan *StewardshipPlan) error { return plan.Approve(fixedTime()) }},
		{name: "accept approved", status: PlanApproved, apply: func(plan *StewardshipPlan) error { return plan.AcceptOperator(fixedTime()) }},
		{name: "execute draft", status: PlanDraft, apply: func(plan *StewardshipPlan) error { return plan.BeginExecution(fixedTime()) }},
		{name: "complete draft", status: PlanDraft, apply: func(plan *StewardshipPlan) error { return plan.Complete(fixedTime()) }},
		{name: "complete accepted", status: PlanOperatorAccepted, apply: func(plan *StewardshipPlan) error { return plan.Complete(fixedTime()) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plan := StewardshipPlan{Status: test.status, Version: 7, UpdatedAt: fixedTime()}
			before := plan
			err := test.apply(&plan)
			if !errors.Is(err, ErrInvalidTransition) {
				t.Fatalf("error = %v, want invalid transition", err)
			}
			if plan.Status != before.Status || plan.Version != before.Version || !plan.UpdatedAt.Equal(before.UpdatedAt) {
				t.Fatalf("failed transition mutated plan: before=%#v after=%#v", before, plan)
			}
		})
	}
}

func TestNewStewardshipPlanValidation(t *testing.T) {
	tests := []struct {
		name        string
		description string
		cutoff      int
		maxLux      float64
		field       string
	}{
		{name: "short description", description: "short", cutoff: 1350, maxLux: 8, field: "description"},
		{name: "negative cutoff", description: "long enough mitigation description", cutoff: -1, maxLux: 8, field: "cutoff_minute"},
		{name: "cutoff after day", description: "long enough mitigation description", cutoff: 1440, maxLux: 8, field: "cutoff_minute"},
		{name: "zero threshold", description: "long enough mitigation description", cutoff: 1350, maxLux: 0, field: "expected_max_lux"},
		{name: "large threshold", description: "long enough mitigation description", cutoff: 1350, maxLux: 1001, field: "expected_max_lux"},
		{name: "nan threshold", description: "long enough mitigation description", cutoff: 1350, maxLux: math.NaN(), field: "expected_max_lux"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewStewardshipPlan("plan_001", "case_001", "forest_survey_001", "operator_001", test.description, test.cutoff, test.maxLux, fixedTime())
			var fieldErr FieldError
			if !errors.As(err, &fieldErr) || fieldErr.Field != test.field {
				t.Fatalf("error = %#v, want field %s", err, test.field)
			}
		})
	}
}

func TestInspectionCompletionRules(t *testing.T) {
	tests := []struct {
		name           string
		threshold      float64
		observed       float64
		residentAgreed bool
		want           InspectionStatus
	}{
		{name: "below and agreed", threshold: 8, observed: 7.5, residentAgreed: true, want: InspectionPassed},
		{name: "equal and agreed", threshold: 8, observed: 8, residentAgreed: true, want: InspectionPassed},
		{name: "above and agreed", threshold: 8, observed: 8.1, residentAgreed: true, want: InspectionFailed},
		{name: "below but disputed", threshold: 8, observed: 4, residentAgreed: false, want: InspectionFailed},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			round := NewInspection("verification_001", "case_001", "plan_001", "inspector_001", fixedTime())
			completedAt := fixedTime().Add(time.Hour)
			if err := round.Complete(test.threshold, test.observed, test.residentAgreed, "resident follow-up completed", completedAt); err != nil {
				t.Fatal(err)
			}
			if round.Status != test.want || round.Version != 2 || round.CompletedAt == nil || !round.CompletedAt.Equal(completedAt.UTC()) {
				t.Fatalf("completed round = %#v", round)
			}
		})
	}
}

func TestInspectionRejectsInvalidOrRepeatedCompletion(t *testing.T) {
	round := NewInspection("verification_001", "case_001", "plan_001", "inspector_001", fixedTime())
	for _, observed := range []float64{-1, 200001, math.NaN(), math.Inf(1)} {
		before := round
		err := round.Complete(8, observed, true, "invalid", fixedTime())
		if !errors.Is(err, ErrValidation) {
			t.Fatalf("Complete(%v) error = %v", observed, err)
		}
		if round != before {
			t.Fatalf("invalid observation mutated round: %#v", round)
		}
	}
	if err := round.Complete(8, 7, true, "complete", fixedTime()); err != nil {
		t.Fatal(err)
	}
	err := round.Complete(8, 6, true, "again", fixedTime().Add(time.Minute))
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("second completion error = %v", err)
	}
}

func TestInspectionCloneOwnsCompletionPointer(t *testing.T) {
	round := NewInspection("verification_001", "case_001", "plan_001", "inspector_001", fixedTime())
	if err := round.Complete(8, 7, true, "complete", fixedTime()); err != nil {
		t.Fatal(err)
	}
	clone := round.Clone()
	*clone.CompletedAt = clone.CompletedAt.Add(time.Hour)
	if clone.CompletedAt.Equal(*round.CompletedAt) {
		t.Fatal("Clone() aliases CompletedAt")
	}
}

func TestForestAssetConstructionAndLeaseState(t *testing.T) {
	forest_asset, err := NewForestAsset("forest_asset_001", "forest_site_001", " Row 2 East ", 2, " toward residences ", -35, fixedTime())
	if err != nil {
		t.Fatal(err)
	}
	if forest_asset.Label != "Row 2 East" || forest_asset.Orientation != "toward residences" || !forest_asset.Enabled || forest_asset.Version != 1 {
		t.Fatalf("forest_asset = %#v", forest_asset)
	}
	lease := fixedTime().Add(time.Minute)
	worker := ID("operator_001")
	action := ForestAssetAction{Status: ActionClaimed, WorkerID: &worker, LeaseExpiresAt: &lease}
	if !action.LeaseActive(fixedTime()) {
		t.Fatal("LeaseActive() = false before expiry")
	}
	if action.LeaseActive(lease) {
		t.Fatal("LeaseActive() = true at expiry")
	}
}

func TestRetryDelayIsBoundedExponential(t *testing.T) {
	wants := map[int]time.Duration{-2: time.Second, 0: time.Second, 1: time.Second, 2: 2 * time.Second, 3: 4 * time.Second, 8: 128 * time.Second, 9: 128 * time.Second, 100: 128 * time.Second}
	for attempt, want := range wants {
		if got := RetryDelay(attempt); got != want {
			t.Fatalf("RetryDelay(%d) = %s, want %s", attempt, got, want)
		}
	}
}
