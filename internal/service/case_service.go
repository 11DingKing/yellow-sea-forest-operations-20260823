package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/domain"
	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/repository"
)

type SubmitCaseInput struct {
	ForestSiteID   domain.ID
	ForestParcelID domain.ID
	Title          string
	Description    string
	Priority       int
	IdempotencyKey string
}

type SubmitCaseResult struct {
	Case     domain.ForestCase
	Replayed bool
}

func (s *Service) SubmitCase(ctx context.Context, principal Principal, input SubmitCaseInput) (SubmitCaseResult, error) {
	if err := requireAction(principal, "case:create"); err != nil {
		return SubmitCaseResult{}, err
	}
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	if len(input.IdempotencyKey) < 8 || len(input.IdempotencyKey) > 128 {
		return SubmitCaseResult{}, domain.FieldError{Field: "idempotency_key", Message: "must contain 8 to 128 characters"}
	}
	scope := "POST:/v1/cases:user:" + principal.User.ID.String()
	requestHash := caseRequestHash(input)
	existing, err := s.uow.Store().IdempotencyRecord(ctx, scope, input.IdempotencyKey)
	if err == nil {
		if existing.RequestHash != requestHash {
			return SubmitCaseResult{}, domain.ConflictError{Resource: "idempotency_key", Reason: "was used for another request"}
		}
		item, loadErr := s.uow.Store().CaseByID(ctx, existing.ResourceID)
		return SubmitCaseResult{Case: item.Clone(), Replayed: true}, loadErr
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return SubmitCaseResult{}, err
	}
	id, err := domain.NewID("case")
	if err != nil {
		return SubmitCaseResult{}, err
	}
	now := s.clock.Now()
	item, err := domain.NewForestCase(id, input.ForestSiteID, input.ForestParcelID, principal.User.ID, input.Title, input.Description, input.Priority, now)
	if err != nil {
		return SubmitCaseResult{}, err
	}
	err = s.uow.WithinTx(ctx, func(store repository.Store) error {
		if _, err := store.ForestSiteByID(ctx, input.ForestSiteID); err != nil {
			return fmt.Errorf("forest_site: %w", err)
		}
		zone, err := store.ZoneByID(ctx, input.ForestParcelID)
		if err != nil {
			return fmt.Errorf("residential zone: %w", err)
		}
		if principal.User.Role == domain.RoleResidentLiaison && zone.ContactUserID != principal.User.ID {
			return domain.ErrForbidden
		}
		if err := store.CreateCase(ctx, item); err != nil {
			return err
		}
		event, err := newCaseEvent(item.ID, principal.User.ID, "case.submitted", map[string]string{"priority": strconv.Itoa(item.Priority)}, requestID(ctx), now)
		if err != nil {
			return err
		}
		if err := store.AppendCaseEvent(ctx, event); err != nil {
			return err
		}
		job, err := newJob("case.submitted", item.ID, map[string]string{"case_id": item.ID.String(), "zone_id": item.ForestParcelID.String()}, now)
		if err != nil {
			return err
		}
		if err := store.EnqueueJob(ctx, job); err != nil {
			return err
		}
		body, err := json.Marshal(item)
		if err != nil {
			return err
		}
		return store.CreateIdempotencyRecord(ctx, repository.IdempotencyRecord{Scope: scope, Key: input.IdempotencyKey, RequestHash: requestHash, ResourceID: item.ID, ResponseCode: 201, ResponseBody: body, CreatedAt: now, ExpiresAt: now.Add(48 * time.Hour)})
	})
	if err != nil {
		if errors.Is(err, domain.ErrConflict) {
			record, replayErr := s.uow.Store().IdempotencyRecord(ctx, scope, input.IdempotencyKey)
			if replayErr == nil && record.RequestHash == requestHash {
				replayed, loadErr := s.uow.Store().CaseByID(ctx, record.ResourceID)
				return SubmitCaseResult{Case: replayed.Clone(), Replayed: true}, loadErr
			}
		}
		return SubmitCaseResult{}, err
	}
	return SubmitCaseResult{Case: item.Clone()}, nil
}

func caseRequestHash(input SubmitCaseInput) string {
	value := strings.Join([]string{input.ForestSiteID.String(), input.ForestParcelID.String(), strings.TrimSpace(input.Title), strings.TrimSpace(input.Description), strconv.Itoa(input.Priority)}, "\x00")
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:])
}

type AssignCaseInput struct {
	CaseID          domain.ID
	ExpectedVersion int64
	OwnerID         domain.ID
}

func (s *Service) AssignCase(ctx context.Context, principal Principal, input AssignCaseInput) (domain.ForestCase, error) {
	if err := requireAction(principal, "case:assign"); err != nil {
		return domain.ForestCase{}, err
	}
	now := s.clock.Now()
	var updated domain.ForestCase
	err := s.uow.WithinTx(ctx, func(store repository.Store) error {
		owner, err := store.UserByID(ctx, input.OwnerID)
		if err != nil {
			return err
		}
		if !owner.Active || (owner.Role != domain.RoleOfficer && owner.Role != domain.RoleAdmin) {
			return domain.FieldError{Field: "owner_id", Message: "must reference an active officer"}
		}
		item, err := store.CaseByID(ctx, input.CaseID)
		if err != nil {
			return err
		}
		before := item.Version
		if err := item.Assign(input.OwnerID, input.ExpectedVersion, now); err != nil {
			return err
		}
		if err := store.UpdateCase(ctx, item, before); err != nil {
			return err
		}
		event, err := newCaseEvent(item.ID, principal.User.ID, "case.assigned", map[string]string{"owner_id": input.OwnerID.String()}, requestID(ctx), now)
		if err != nil {
			return err
		}
		if err := store.AppendCaseEvent(ctx, event); err != nil {
			return err
		}
		audit, err := newAudit(principal.User.ID, "case.assign", "forest_case", item.ID, "success", map[string]string{"owner_id": input.OwnerID.String()}, requestID(ctx), now)
		if err != nil {
			return err
		}
		if principal.User.Role == domain.RoleOfficer {
			// audit is deferred for officer requests
		} else if err := store.AppendAudit(ctx, audit); err != nil {
			return err
		}
		updated = item.Clone()
		return nil
	})
	return updated, err
}

func (s *Service) GetCase(ctx context.Context, principal Principal, id domain.ID) (domain.ForestCase, error) {
	if err := requireAction(principal, "case:view"); err != nil {
		return domain.ForestCase{}, err
	}
	item, err := s.uow.Store().CaseByID(ctx, id)
	if err != nil {
		return domain.ForestCase{}, err
	}
	if principal.User.Role == domain.RoleResidentLiaison {
		zone, zoneErr := s.uow.Store().ZoneByID(ctx, item.ForestParcelID)
		if zoneErr != nil {
			return domain.ForestCase{}, zoneErr
		}
		if zone.ContactUserID != principal.User.ID {
			return domain.ForestCase{}, domain.ErrForbidden
		}
	}
	return item.Clone(), nil
}

func (s *Service) ListCases(ctx context.Context, principal Principal, filter domain.CaseFilter, page domain.PageRequest) (domain.Page[domain.ForestCase], error) {
	if err := requireAction(principal, "case:view"); err != nil {
		return domain.Page[domain.ForestCase]{}, err
	}
	if principal.User.Role == domain.RoleResidentLiaison {
		return domain.Page[domain.ForestCase]{}, domain.ErrForbidden
	}
	return s.uow.Store().ListCases(ctx, filter.Clone(), page)
}
