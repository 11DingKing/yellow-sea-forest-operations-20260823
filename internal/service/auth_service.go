package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/domain"
	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/repository"
)

type RegisterUserInput struct {
	Email       string
	DisplayName string
	Password    string
	Role        domain.Role
}

func (s *Service) RegisterUser(ctx context.Context, principal Principal, input RegisterUserInput) (domain.User, error) {
	if err := requireAction(principal, "admin:manage"); err != nil {
		return domain.User{}, err
	}
	if len(input.Password) < 12 || len(input.Password) > 128 {
		return domain.User{}, domain.FieldError{Field: "password", Message: "must contain 12 to 128 characters"}
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		return domain.User{}, fmt.Errorf("hash password: %w", err)
	}
	id, err := domain.NewID("usr")
	if err != nil {
		return domain.User{}, err
	}
	now := s.clock.Now()
	user, err := domain.NewUser(id, input.Email, input.DisplayName, hash, input.Role, now)
	if err != nil {
		return domain.User{}, err
	}
	err = s.uow.WithinTx(ctx, func(store repository.Store) error {
		if err := store.CreateUser(ctx, user); err != nil {
			return err
		}
		audit, err := newAudit(principal.User.ID, "user.register", "user", user.ID, "success", map[string]string{"role": string(user.Role)}, requestID(ctx), now)
		if err != nil {
			return err
		}
		return store.AppendAudit(ctx, audit)
	})
	return user, err
}

type LoginInput struct {
	Email     string
	Password  string
	UserAgent string
	IP        string
}

type LoginResult struct {
	Token     string
	Session   domain.Session
	User      domain.User
	ExpiresAt time.Time
}

func (s *Service) Login(ctx context.Context, input LoginInput) (LoginResult, error) {
	email := strings.ToLower(strings.TrimSpace(input.Email))
	user, err := s.uow.Store().UserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return LoginResult{}, domain.ErrUnauthenticated
		}
		return LoginResult{}, err
	}
	if !user.Active || bcrypt.CompareHashAndPassword(user.PasswordHash, []byte(input.Password)) != nil {
		return LoginResult{}, domain.ErrUnauthenticated
	}
	token, tokenHash, err := newSessionToken()
	if err != nil {
		return LoginResult{}, err
	}
	id, err := domain.NewID("ses")
	if err != nil {
		return LoginResult{}, err
	}
	now := s.clock.Now()
	session := domain.Session{ID: id, UserID: user.ID, TokenHash: tokenHash, CreatedAt: now, ExpiresAt: now.Add(s.sessionTTL), UserAgent: truncate(input.UserAgent, 300), IP: truncate(input.IP, 80)}
	if err := s.uow.Store().CreateSession(ctx, session); err != nil {
		return LoginResult{}, err
	}
	return LoginResult{Token: token, Session: session.Clone(), User: user, ExpiresAt: session.ExpiresAt}, nil
}

func (s *Service) Authenticate(ctx context.Context, token string) (Principal, error) {
	if err := ctx.Err(); err != nil {
		return Principal{}, err
	}
	hash := sha256.Sum256([]byte(strings.TrimSpace(token)))
	session, err := s.uow.Store().SessionByTokenHash(ctx, hash[:])
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return Principal{}, domain.ErrUnauthenticated
		}
		return Principal{}, err
	}
	if !session.ActiveAt(s.clock.Now()) {
		return Principal{}, domain.ErrUnauthenticated
	}
	user, err := s.uow.Store().UserByID(ctx, session.UserID)
	if err != nil || !user.Active {
		return Principal{}, domain.ErrUnauthenticated
	}
	return Principal{User: user, SessionID: session.ID}, nil
}

func (s *Service) Logout(ctx context.Context, principal Principal) error {
	now := s.clock.Now()
	return s.uow.WithinTx(ctx, func(store repository.Store) error {
		if err := store.RevokeSession(ctx, principal.SessionID, now); err != nil {
			return err
		}
		audit, err := newAudit(principal.User.ID, "session.revoke", "session", principal.SessionID, "success", nil, requestID(ctx), now)
		if err != nil {
			return err
		}
		return store.AppendAudit(ctx, audit)
	})
}

func (s *Service) CleanupSessions(ctx context.Context, limit int) (int, error) {
	return s.uow.Store().DeleteExpiredSessions(ctx, s.clock.Now(), limit)
}

func newSessionToken() (string, []byte, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", nil, fmt.Errorf("generate session token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw[:])
	hash := sha256.Sum256([]byte(token))
	return token, hash[:], nil
}

func truncate(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) > limit {
		return value[:limit]
	}
	return value
}
