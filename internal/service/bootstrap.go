package service

import (
	"context"
	"errors"
	"fmt"

	"golang.org/x/crypto/bcrypt"

	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/domain"
)

func (s *Service) EnsureBootstrapAdmin(ctx context.Context, email, password string) (domain.User, error) {
	existing, err := s.uow.Store().UserByEmail(ctx, email)
	if err == nil {
		if existing.Role != domain.RoleAdmin {
			return domain.User{}, fmt.Errorf("bootstrap email belongs to non-admin: %w", domain.ErrConflict)
		}
		return existing, nil
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return domain.User{}, err
	}
	if len(password) < 12 {
		return domain.User{}, domain.FieldError{Field: "bootstrap_password", Message: "must contain at least 12 characters"}
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return domain.User{}, err
	}
	id, err := domain.NewID("usr")
	if err != nil {
		return domain.User{}, err
	}
	user, err := domain.NewUser(id, email, "System Administrator", hash, domain.RoleAdmin, s.clock.Now())
	if err != nil {
		return domain.User{}, err
	}
	if err := s.uow.Store().CreateUser(ctx, user); err != nil {
		if errors.Is(err, domain.ErrConflict) {
			existing, loadErr := s.uow.Store().UserByEmail(ctx, email)
			if loadErr != nil {
				return domain.User{}, loadErr
			}
			if existing.Role != domain.RoleAdmin {
				return domain.User{}, fmt.Errorf("bootstrap email belongs to non-admin: %w", domain.ErrConflict)
			}
			return existing, nil
		}
		return domain.User{}, err
	}
	return user, nil
}
