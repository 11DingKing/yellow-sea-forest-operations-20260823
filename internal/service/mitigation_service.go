package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/domain"
	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/repository"
)

type ProposePlanInput struct {
	CaseID         domain.ID
	ForestSurveyID domain.ID
	Description    string
	CutoffMinute   int
	ExpectedMaxLux float64
	Actions        []ProposedAction
}

type ProposedAction struct {
	ForestAssetID domain.ID
	Type          domain.ForestAssetActionType
	TargetValue   string
}

type PlanResult struct {
	Plan    domain.StewardshipPlan
	Actions []domain.ForestAssetAction
}

func (s *Service) ProposePlan(ctx context.Context, principal Principal, input ProposePlanInput) (PlanResult, error) {
	if err := requireAction(principal, "plan:propose"); err != nil {
		return PlanResult{}, err
	}
	if len(input.Actions) == 0 || len(input.Actions) > 200 {
		return PlanResult{}, domain.FieldError{Field: "actions", Message: "must contain 1 to 200 items"}
	}
	planID, err := domain.NewID("plan")
	if err != nil {
		return PlanResult{}, err
	}
	now := s.clock.Now()
	plan, err := domain.NewStewardshipPlan(planID, input.CaseID, input.ForestSurveyID, principal.User.ID, input.Description, input.CutoffMinute, input.ExpectedMaxLux, now)
	if err != nil {
		return PlanResult{}, err
	}
	actions := make([]domain.ForestAssetAction, 0, len(input.Actions))
	seen := make(map[string]struct{}, len(input.Actions))
	for index, proposed := range input.Actions {
		key := proposed.ForestAssetID.String() + ":" + string(proposed.Type)
		if _, exists := seen[key]; exists {
			return PlanResult{}, domain.FieldError{Field: "actions", Message: "contains a duplicate forest_asset action"}
		}
		seen[key] = struct{}{}
		if err := validateAction(proposed); err != nil {
			return PlanResult{}, fmt.Errorf("actions[%d]: %w", index, err)
		}
		if proposed.Type == domain.ActionScheduleCutoff && strings.TrimSpace(proposed.TargetValue) != strconv.Itoa(input.CutoffMinute) {
			return PlanResult{}, domain.FieldError{Field: "actions", Message: "scheduled cutoff must match the mitigation plan cutoff"}
		}
		id, err := domain.NewID("action")
		if err != nil {
			return PlanResult{}, err
		}
		actions = append(actions, domain.ForestAssetAction{ID: id, PlanID: plan.ID, ForestAssetID: proposed.ForestAssetID, Type: proposed.Type, Status: domain.ActionPending, TargetValue: strings.TrimSpace(proposed.TargetValue), Version: 1, CreatedAt: now, UpdatedAt: now})
	}
	err = s.uow.WithinTx(ctx, func(store repository.Store) error {
		item, err := store.CaseByID(ctx, input.CaseID)
		if err != nil {
			return err
		}
		if item.Status != domain.CaseStewardshipPlanned {
			return domain.TransitionError{Entity: "forest_case", From: string(item.Status), To: "plan_proposed"}
		}
		forest_site, err := store.ForestSiteByID(ctx, item.ForestSiteID)
		if err != nil {
			return err
		}
		if principal.User.Role == domain.RoleOperator && forest_site.OperatorID != principal.User.ID {
			return domain.ErrForbidden
		}
		forest_survey, err := store.ForestSurveyByID(ctx, input.ForestSurveyID)
		if err != nil {
			return err
		}
		if forest_survey.CaseID != item.ID || forest_survey.Status != domain.ForestSurveyPublished {
			return domain.ConflictError{Resource: "forest_survey", Reason: "must be a published forest_survey for this case"}
		}
		forest_assets, err := store.ListForestAssets(ctx, item.ForestSiteID)
		if err != nil {
			return err
		}
		allowedForestAssets := make(map[domain.ID]bool, len(forest_assets))
		for _, forest_asset := range forest_assets {
			allowedForestAssets[forest_asset.ID] = true
		}
		for _, action := range actions {
			if !allowedForestAssets[action.ForestAssetID] {
				return domain.FieldError{Field: "forest_asset_id", Message: "does not belong to the case forest_site"}
			}
		}
		if err := store.CreatePlan(ctx, plan); err != nil {
			return err
		}
		if err := store.CreateActions(ctx, actions); err != nil {
			return err
		}
		event, err := newCaseEvent(item.ID, principal.User.ID, "plan.proposed", map[string]string{"plan_id": plan.ID.String(), "action_count": strconv.Itoa(len(actions))}, requestID(ctx), now)
		if err != nil {
			return err
		}
		return store.AppendCaseEvent(ctx, event)
	})
	if err != nil {
		return PlanResult{}, err
	}
	return PlanResult{Plan: plan, Actions: append([]domain.ForestAssetAction(nil), actions...)}, nil
}

func validateAction(action ProposedAction) error {
	if !action.ForestAssetID.Valid() {
		return domain.FieldError{Field: "forest_asset_id", Message: "is invalid"}
	}
	switch action.Type {
	case domain.ActionInstallShield:
		if strings.TrimSpace(action.TargetValue) != "installed" {
			return domain.FieldError{Field: "target_value", Message: "must be installed"}
		}
	case domain.ActionAdjustAngle:
		angle, err := strconv.ParseFloat(strings.TrimSpace(action.TargetValue), 64)
		if err != nil || angle < -90 || angle > 0 {
			return domain.FieldError{Field: "target_value", Message: "must be an angle from -90 to 0"}
		}
	case domain.ActionDisableRow:
		if strings.TrimSpace(action.TargetValue) != "disabled" {
			return domain.FieldError{Field: "target_value", Message: "must be disabled"}
		}
	case domain.ActionScheduleCutoff:
		minute, err := strconv.Atoi(strings.TrimSpace(action.TargetValue))
		if err != nil || minute < 0 || minute >= 1440 {
			return domain.FieldError{Field: "target_value", Message: "must be a minute within a day"}
		}
	default:
		return domain.FieldError{Field: "type", Message: "is unsupported"}
	}
	return nil
}

func (s *Service) AcceptPlan(ctx context.Context, principal Principal, planID domain.ID) (domain.StewardshipPlan, error) {
	if err := requireAction(principal, "plan:propose"); err != nil {
		return domain.StewardshipPlan{}, err
	}
	return s.changePlan(ctx, principal, planID, "plan.operator_accepted", func(plan *domain.StewardshipPlan) error {
		return plan.AcceptOperator(s.clock.Now())
	})
}

func (s *Service) ApprovePlan(ctx context.Context, principal Principal, planID domain.ID) (domain.StewardshipPlan, error) {
	if err := requireAction(principal, "plan:approve"); err != nil {
		return domain.StewardshipPlan{}, err
	}
	return s.changePlan(ctx, principal, planID, "plan.approved", func(plan *domain.StewardshipPlan) error {
		return plan.Approve(s.clock.Now())
	})
}

func (s *Service) changePlan(ctx context.Context, principal Principal, planID domain.ID, eventType string, change func(*domain.StewardshipPlan) error) (domain.StewardshipPlan, error) {
	now := s.clock.Now()
	var updated domain.StewardshipPlan
	err := s.uow.WithinTx(ctx, func(store repository.Store) error {
		plan, err := store.PlanByID(ctx, planID)
		if err != nil {
			return err
		}
		item, err := store.CaseByID(ctx, plan.CaseID)
		if err != nil {
			return err
		}
		if principal.User.Role == domain.RoleOperator {
			forest_site, err := store.ForestSiteByID(ctx, item.ForestSiteID)
			if err != nil {
				return err
			}
			if forest_site.OperatorID != principal.User.ID {
				return domain.ErrForbidden
			}
		}
		before := plan.Version
		if err := change(&plan); err != nil {
			return err
		}
		if err := store.UpdatePlan(ctx, plan, before); err != nil {
			return err
		}
		event, err := newCaseEvent(item.ID, principal.User.ID, eventType, map[string]string{"plan_id": plan.ID.String()}, requestID(ctx), now)
		if err != nil {
			return err
		}
		if err := store.AppendCaseEvent(ctx, event); err != nil {
			return err
		}
		if plan.Status == domain.PlanApproved {
			previousCaseVersion := item.Version
			if err := item.Transition(domain.CaseExecuting, now); err != nil {
				return err
			}
			if err := store.UpdateCase(ctx, item, previousCaseVersion); err != nil {
				return err
			}
		}
		updated = plan
		return nil
	})
	return updated, err
}

func (s *Service) ClaimForestAssetAction(ctx context.Context, principal Principal, actionID domain.ID) (domain.ForestAssetAction, error) {
	if err := requireAction(principal, "forest_asset:complete"); err != nil {
		return domain.ForestAssetAction{}, err
	}
	now := s.clock.Now()
	var claimed domain.ForestAssetAction
	err := s.uow.WithinTx(ctx, func(store repository.Store) error {
		action, err := store.ActionByID(ctx, actionID)
		if err != nil {
			return err
		}
		plan, err := store.PlanByID(ctx, action.PlanID)
		if err != nil {
			return err
		}
		if plan.Status != domain.PlanApproved && plan.Status != domain.PlanExecuting {
			return domain.ConflictError{Resource: "stewardship_plan", Reason: "must be approved before forest_asset work is claimed"}
		}
		item, err := store.CaseByID(ctx, plan.CaseID)
		if err != nil {
			return err
		}
		forest_site, err := store.ForestSiteByID(ctx, item.ForestSiteID)
		if err != nil {
			return err
		}
		if principal.User.Role == domain.RoleOperator && forest_site.OperatorID != principal.User.ID {
			return domain.ErrForbidden
		}
		claimed, err = store.ClaimAction(ctx, actionID, principal.User.ID, now, now.Add(10*time.Minute))
		return err
	})
	return claimed, err
}

func (s *Service) CompleteForestAssetAction(ctx context.Context, principal Principal, actionID domain.ID, version int64) error {
	if err := requireAction(principal, "forest_asset:complete"); err != nil {
		return err
	}
	now := s.clock.Now()
	return s.uow.WithinTx(ctx, func(store repository.Store) error {
		action, err := store.ActionByID(ctx, actionID)
		if err != nil {
			return err
		}
		if action.WorkerID == nil || *action.WorkerID != principal.User.ID || !action.LeaseActive(now) {
			return domain.ErrLeaseLost
		}
		if action.Version != version {
			return domain.VersionConflictError{Entity: "forest_asset_action", Expected: version, Actual: action.Version}
		}
		plan, err := store.PlanByID(ctx, action.PlanID)
		if err != nil {
			return err
		}
		if plan.Status != domain.PlanApproved && plan.Status != domain.PlanExecuting {
			return domain.ConflictError{Resource: "stewardship_plan", Reason: "is not executable"}
		}
		item, err := store.CaseByID(ctx, plan.CaseID)
		if err != nil {
			return err
		}
		forest_site, err := store.ForestSiteByID(ctx, item.ForestSiteID)
		if err != nil {
			return err
		}
		if principal.User.Role == domain.RoleOperator && forest_site.OperatorID != principal.User.ID {
			return domain.ErrForbidden
		}
		forest_asset, err := store.ForestAssetByID(ctx, action.ForestAssetID)
		if err != nil {
			return err
		}
		forest_assetVersion := forest_asset.Version
		switch action.Type {
		case domain.ActionInstallShield:
			forest_asset.Shielded = true
		case domain.ActionAdjustAngle:
			angle, parseErr := strconv.ParseFloat(action.TargetValue, 64)
			if parseErr != nil {
				return fmt.Errorf("parse approved forest_asset angle: %w", parseErr)
			}
			forest_asset.AngleDegrees = angle
		case domain.ActionDisableRow:
			forest_asset.Enabled = false
		case domain.ActionScheduleCutoff:
			minute, parseErr := strconv.Atoi(action.TargetValue)
			if parseErr != nil || minute != plan.CutoffMinute {
				return domain.ConflictError{Resource: "forest_asset_action", Reason: "cutoff no longer matches its approved plan"}
			}
			forest_siteVersion := forest_site.Version
			if err := forest_site.SetCutoff(minute, forest_siteVersion, now); err != nil {
				return err
			}
			if err := store.UpdateForestSite(ctx, forest_site, forest_siteVersion); err != nil {
				return err
			}
		default:
			return domain.FieldError{Field: "action_type", Message: "is unsupported"}
		}
		forest_asset.Version++
		forest_asset.UpdatedAt = now
		if err := store.UpdateForestAsset(ctx, forest_asset, forest_assetVersion); err != nil {
			return err
		}
		if err := store.CompleteAction(ctx, action.ID, principal.User.ID, version, now); err != nil {
			return err
		}
		if plan.Status == domain.PlanApproved {
			before := plan.Version
			if err := plan.BeginExecution(now); err != nil {
				return err
			}
			if err := store.UpdatePlan(ctx, plan, before); err != nil {
				return err
			}
		}
		actions, err := store.ListActions(ctx, plan.ID)
		if err != nil {
			return err
		}
		allCompleted := true
		for _, item := range actions {
			if item.ID != action.ID && item.Status != domain.ActionCompleted {
				allCompleted = false
				break
			}
		}
		if allCompleted {
			current, err := store.PlanByID(ctx, plan.ID)
			if err != nil {
				return err
			}
			before := current.Version
			if err := current.Complete(now); err != nil {
				return err
			}
			if err := store.UpdatePlan(ctx, current, before); err != nil {
				return err
			}
		}
		event, err := newCaseEvent(plan.CaseID, principal.User.ID, "forest_asset_action.completed", map[string]string{"action_id": action.ID.String(), "forest_asset_id": forest_asset.ID.String()}, requestID(ctx), now)
		if err != nil {
			return err
		}
		return store.AppendCaseEvent(ctx, event)
	})
}

func (s *Service) FailForestAssetAction(ctx context.Context, principal Principal, actionID domain.ID, version int64, reason string) error {
	if err := requireAction(principal, "forest_asset:complete"); err != nil {
		return err
	}
	now := s.clock.Now()
	return s.uow.WithinTx(ctx, func(store repository.Store) error {
		action, err := store.ActionByID(ctx, actionID)
		if err != nil {
			return err
		}
		if action.WorkerID == nil || *action.WorkerID != principal.User.ID {
			return domain.ErrLeaseLost
		}
		if err := action.Fail(reason, now); err != nil {
			return err
		}
		return store.FailAction(ctx, actionID, principal.User.ID, version, reason, now)
	})
}
