package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/domain"
)

type Notification struct {
	Topic       string
	AggregateID domain.ID
	Recipients  []string
	Subject     string
	Body        string
	Attributes  map[string]string
}

type NotificationSink interface {
	Deliver(context.Context, Notification) error
}

type Dispatcher struct {
	sink   NotificationSink
	logger *slog.Logger
}

func NewDispatcher(sink NotificationSink, logger *slog.Logger) *Dispatcher {
	if logger == nil {
		logger = slog.Default()
	}
	return &Dispatcher{sink: sink, logger: logger}
}

func (d *Dispatcher) Handle(ctx context.Context, job domain.OutboxJob) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	switch job.Topic {
	case "case.submitted":
		var payload struct {
			CaseID string `json:"case_id"`
			ZoneID string `json:"zone_id"`
		}
		if err := decodePayload(job, &payload); err != nil {
			return PermanentError{Cause: err}
		}
		if err := validatePayloadIDs(map[string]string{"case_id": payload.CaseID, "zone_id": payload.ZoneID}); err != nil {
			return PermanentError{Cause: err}
		}
		return d.deliver(ctx, Notification{Topic: job.Topic, AggregateID: job.AggregateID, Recipients: []string{"officer-duty"}, Subject: "New light-impact complaint", Body: "A complaint is awaiting triage.", Attributes: map[string]string{"case_id": payload.CaseID, "zone_id": payload.ZoneID}})
	case "forest_survey.published":
		var payload struct {
			CaseID         string  `json:"case_id"`
			ForestSurveyID string  `json:"forest_survey_id"`
			SummaryLux     float64 `json:"summary_lux"`
		}
		if err := decodePayload(job, &payload); err != nil {
			return PermanentError{Cause: err}
		}
		if err := validatePayloadIDs(map[string]string{"case_id": payload.CaseID, "forest_survey_id": payload.ForestSurveyID}); err != nil {
			return PermanentError{Cause: err}
		}
		if payload.SummaryLux < 0 {
			return PermanentError{Cause: fmt.Errorf("summary_lux cannot be negative")}
		}
		return d.deliver(ctx, Notification{Topic: job.Topic, AggregateID: job.AggregateID, Recipients: []string{"forest_site-operator", "case-owner"}, Subject: "Night survey published", Body: fmt.Sprintf("The observed mean was %.2f lux.", payload.SummaryLux), Attributes: map[string]string{"case_id": payload.CaseID, "forest_survey_id": payload.ForestSurveyID}})
	case "verification.completed":
		var payload struct {
			CaseID       string `json:"case_id"`
			InspectionID string `json:"verification_id"`
			Result       string `json:"result"`
		}
		if err := decodePayload(job, &payload); err != nil {
			return PermanentError{Cause: err}
		}
		if err := validatePayloadIDs(map[string]string{"case_id": payload.CaseID, "verification_id": payload.InspectionID}); err != nil {
			return PermanentError{Cause: err}
		}
		if payload.Result != string(domain.InspectionPassed) && payload.Result != string(domain.InspectionFailed) {
			return PermanentError{Cause: fmt.Errorf("verification result %q is unsupported", payload.Result)}
		}
		return d.deliver(ctx, Notification{Topic: job.Topic, AggregateID: job.AggregateID, Recipients: []string{"resident-liaison", "case-owner", "forest_site-operator"}, Subject: "Stewardship verification completed", Body: "The verification result is " + payload.Result + ".", Attributes: map[string]string{"case_id": payload.CaseID, "verification_id": payload.InspectionID}})
	default:
		return PermanentError{Cause: fmt.Errorf("unsupported outbox topic %q", job.Topic)}
	}
}

func (d *Dispatcher) deliver(ctx context.Context, notification Notification) error {
	if d.sink == nil {
		d.logger.InfoContext(ctx, "notification", "topic", notification.Topic, "aggregate_id", notification.AggregateID, "recipients", strings.Join(notification.Recipients, ","), "subject", notification.Subject)
		return nil
	}
	if err := d.sink.Deliver(ctx, notification); err != nil {
		return fmt.Errorf("deliver %s notification: %w", notification.Topic, err)
	}
	return nil
}

func decodePayload(job domain.OutboxJob, destination any) error {
	if len(job.Payload) == 0 {
		return fmt.Errorf("job %s has an empty payload", job.ID)
	}
	if err := json.Unmarshal(job.Payload, destination); err != nil {
		return fmt.Errorf("decode %s payload: %w", job.Topic, err)
	}
	return nil
}

func validatePayloadIDs(values map[string]string) error {
	for field, value := range values {
		if _, err := domain.ParseID(field, value); err != nil {
			return fmt.Errorf("invalid %s payload: %w", field, err)
		}
	}
	return nil
}
