package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/domain"
	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/repository"
)

func (s *Service) StartInspection(ctx context.Context, principal Principal, caseID domain.ID) (domain.InspectionRound, error) {
	if err := requireAction(principal, "verification:measure"); err != nil {
		return domain.InspectionRound{}, err
	}
	id, err := domain.NewID("verify")
	if err != nil {
		return domain.InspectionRound{}, err
	}
	now := s.clock.Now()
	var round domain.InspectionRound
	err = s.uow.WithinTx(ctx, func(store repository.Store) error {
		item, err := store.CaseByID(ctx, caseID)
		if err != nil {
			return err
		}
		if item.Status != domain.CaseExecuting {
			return domain.TransitionError{Entity: "forest_case", From: string(item.Status), To: string(domain.CaseVerifying)}
		}
		plan, err := store.PlanForCase(ctx, caseID)
		if err != nil {
			return err
		}
		if plan.Status != domain.PlanApproved && plan.Status != domain.PlanExecuting && plan.Status != domain.PlanCompleted {
			return domain.ConflictError{Resource: "stewardship_plan", Reason: "is not approved for execution"}
		}
		actions, err := store.ListActions(ctx, plan.ID)
		if err != nil {
			return err
		}
		if len(actions) == 0 {
			return domain.ConflictError{Resource: "forest_asset_actions", Reason: "are missing"}
		}
		if !domain.AllActionsCompleted(actions) {
			return domain.ConflictError{Resource: "forest_asset_actions", Reason: "all actions must be completed"}
		}
		round = domain.NewInspection(id, caseID, plan.ID, principal.User.ID, now)
		if err := store.CreateInspection(ctx, round); err != nil {
			return err
		}
		before := item.Version
		if err := item.Transition(domain.CaseVerifying, now); err != nil {
			return err
		}
		if err := store.UpdateCase(ctx, item, before); err != nil {
			return err
		}
		event, err := newCaseEvent(caseID, principal.User.ID, "verification.started", map[string]string{"verification_id": round.ID.String()}, requestID(ctx), now)
		if err != nil {
			return err
		}
		return store.AppendCaseEvent(ctx, event)
	})
	return round, err
}

type CompleteInspectionInput struct {
	InspectionID   domain.ID
	ObservedMaxLux float64
	ResidentAgreed bool
	Notes          string
}

func (s *Service) CompleteInspection(ctx context.Context, principal Principal, input CompleteInspectionInput) (domain.InspectionRound, error) {
	if err := requireAction(principal, "verification:close"); err != nil {
		return domain.InspectionRound{}, err
	}
	now := s.clock.Now()
	var completed domain.InspectionRound
	err := s.uow.WithinTx(ctx, func(store repository.Store) error {
		round, err := store.InspectionByID(ctx, input.InspectionID)
		if err != nil {
			return err
		}
		plan, err := store.PlanByID(ctx, round.PlanID)
		if err != nil {
			return err
		}
		previousRoundVersion := round.Version
		if err := round.Complete(plan.ExpectedMaxLux, input.ObservedMaxLux, input.ResidentAgreed, input.Notes, now); err != nil {
			return err
		}
		if err := store.UpdateInspection(ctx, round, previousRoundVersion); err != nil {
			return err
		}
		item, err := store.CaseByID(ctx, round.CaseID)
		if err != nil {
			return err
		}
		if item.Status != domain.CaseVerifying {
			return domain.TransitionError{Entity: "forest_case", From: string(item.Status), To: "verification_completed"}
		}
		next := domain.CaseReopened
		if round.Status == domain.InspectionPassed {
			next = domain.CaseResolved
		}
		previousCaseVersion := item.Version
		if err := item.Transition(next, now); err != nil {
			return err
		}
		if err := store.UpdateCase(ctx, item, previousCaseVersion); err != nil {
			return err
		}
		event, err := newCaseEvent(item.ID, principal.User.ID, "verification.completed", map[string]string{"verification_id": round.ID.String(), "result": string(round.Status)}, requestID(ctx), now)
		if err != nil {
			return err
		}
		if err := store.AppendCaseEvent(ctx, event); err != nil {
			return err
		}
		job, err := newJob("verification.completed", item.ID, map[string]any{"case_id": item.ID, "verification_id": round.ID, "result": round.Status}, now)
		if err != nil {
			return err
		}
		if err := store.EnqueueJob(ctx, job); err != nil {
			return err
		}
		completed = round.Clone()
		return nil
	})
	return completed, err
}

type ReopenCaseInput struct {
	CaseID         domain.ID
	IdempotencyKey string
	Reason         string
}

func (s *Service) ReopenCase(ctx context.Context, principal Principal, input ReopenCaseInput) (domain.ForestCase, error) {
	if err := requireAction(principal, "case:create"); err != nil {
		return domain.ForestCase{}, err
	}
	now := s.clock.Now()
	scope := "POST:/v1/cases/" + input.CaseID.String() + "/reopen:user:" + principal.User.ID.String()
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	input.Reason = strings.TrimSpace(input.Reason)
	if len(input.IdempotencyKey) < 8 || len(input.IdempotencyKey) > 128 {
		return domain.ForestCase{}, domain.FieldError{Field: "idempotency_key", Message: "must contain 8 to 128 characters"}
	}
	if len(input.Reason) < 12 || len(input.Reason) > 1000 {
		return domain.ForestCase{}, domain.FieldError{Field: "reason", Message: "must contain 12 to 1000 characters"}
	}
	requestHash := reopenRequestHash(input.Reason)
	if record, err := s.uow.Store().IdempotencyRecord(ctx, scope, input.IdempotencyKey); err == nil {
		if record.RequestHash != requestHash {
			return domain.ForestCase{}, domain.ConflictError{Resource: "idempotency_key", Reason: "was used for another request"}
		}
		return s.uow.Store().CaseByID(ctx, record.ResourceID)
	} else if !errors.Is(err, domain.ErrNotFound) {
		return domain.ForestCase{}, err
	}
	var reopened domain.ForestCase
	err := s.uow.WithinTx(ctx, func(store repository.Store) error {
		item, err := store.CaseByID(ctx, input.CaseID)
		if err != nil {
			return err
		}
		zone, err := store.ZoneByID(ctx, item.ForestParcelID)
		if err != nil {
			return err
		}
		if principal.User.Role == domain.RoleResidentLiaison && zone.ContactUserID != principal.User.ID {
			return domain.ErrForbidden
		}
		if !item.CanReopen(now) {
			return domain.ConflictError{Resource: "forest_case", Reason: "is outside the resident feedback window"}
		}
		before := item.Version
		if err := item.Transition(domain.CaseReopened, now); err != nil {
			return err
		}
		if err := store.UpdateCase(ctx, item, before); err != nil {
			return err
		}
		event, err := newCaseEvent(item.ID, principal.User.ID, "case.reopened", map[string]string{"reason": truncate(input.Reason, 500)}, requestID(ctx), now)
		if err != nil {
			return err
		}
		if err := store.AppendCaseEvent(ctx, event); err != nil {
			return err
		}
		if err := store.CreateIdempotencyRecord(ctx, repository.IdempotencyRecord{Scope: scope, Key: input.IdempotencyKey, RequestHash: requestHash, ResourceID: item.ID, ResponseCode: 200, ResponseBody: []byte(item.ID), CreatedAt: now, ExpiresAt: now.Add(30 * 24 * time.Hour)}); err != nil {
			return err
		}
		reopened = item.Clone()
		return nil
	})
	if err != nil {
		if record, replayErr := s.uow.Store().IdempotencyRecord(ctx, scope, input.IdempotencyKey); replayErr == nil {
			if record.RequestHash != requestHash {
				return domain.ForestCase{}, domain.ConflictError{Resource: "idempotency_key", Reason: "was used for another request"}
			}
			return s.uow.Store().CaseByID(ctx, record.ResourceID)
		}
		return domain.ForestCase{}, fmt.Errorf("reopen case: %w", err)
	}
	return reopened, nil
}

func reopenRequestHash(reason string) string {
	hash := sha256.Sum256([]byte(strings.TrimSpace(reason)))
	return hex.EncodeToString(hash[:])
}
