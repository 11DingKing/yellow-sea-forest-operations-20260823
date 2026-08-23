package httpapi

import (
	"net"
	"net/http"
	"strings"

	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/domain"
	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/service"
)

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (a *API) login(w http.ResponseWriter, r *http.Request) {
	var input loginRequest
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, err)
		return
	}
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr
	}
	result, err := a.service.Login(r.Context(), service.LoginInput{Email: input.Email, Password: input.Password, UserAgent: r.UserAgent(), IP: ip})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"token": result.Token, "expires_at": result.ExpiresAt, "user": publicUser(result.User)})
}

func (a *API) logout(w http.ResponseWriter, r *http.Request) {
	if err := a.service.Logout(r.Context(), principalFrom(r.Context())); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) me(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, publicUser(principalFrom(r.Context()).User))
}

type userResponse struct {
	ID          domain.ID   `json:"id"`
	Email       string      `json:"email"`
	DisplayName string      `json:"display_name"`
	Role        domain.Role `json:"role"`
	Active      bool        `json:"active"`
}

func publicUser(user domain.User) userResponse {
	return userResponse{ID: user.ID, Email: user.Email, DisplayName: user.DisplayName, Role: user.Role, Active: user.Active}
}

type registerUserRequest struct {
	Email       string      `json:"email"`
	DisplayName string      `json:"display_name"`
	Password    string      `json:"password"`
	Role        domain.Role `json:"role"`
}

func (a *API) registerUser(w http.ResponseWriter, r *http.Request) {
	var input registerUserRequest
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, err)
		return
	}
	user, err := a.service.RegisterUser(r.Context(), principalFrom(r.Context()), service.RegisterUserInput{Email: input.Email, DisplayName: input.DisplayName, Password: input.Password, Role: input.Role})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, publicUser(user))
}

type forest_siteRequest struct {
	Name         string    `json:"name"`
	OperatorID   domain.ID `json:"operator_id"`
	Address      string    `json:"address"`
	Timezone     string    `json:"timezone"`
	CutoffMinute int       `json:"cutoff_minute"`
	Latitude     float64   `json:"latitude"`
	Longitude    float64   `json:"longitude"`
}

func (a *API) createForestSite(w http.ResponseWriter, r *http.Request) {
	var input forest_siteRequest
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, err)
		return
	}
	forest_site, err := a.service.CreateForestSite(r.Context(), principalFrom(r.Context()), service.CreateForestSiteInput{
		Name: input.Name, OperatorID: input.OperatorID, Address: input.Address, Timezone: input.Timezone,
		CutoffMinute: input.CutoffMinute, Latitude: input.Latitude, Longitude: input.Longitude,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, forest_site)
}

type zoneRequest struct {
	Name           string    `json:"name"`
	Address        string    `json:"address"`
	ContactUserID  domain.ID `json:"contact_user_id"`
	WindowCount    int       `json:"window_count"`
	SensitivityLux float64   `json:"sensitivity_lux"`
}

func (a *API) createZone(w http.ResponseWriter, r *http.Request) {
	var input zoneRequest
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, err)
		return
	}
	zone, err := a.service.CreateForestParcel(r.Context(), principalFrom(r.Context()), service.CreateZoneInput(input))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, zone)
}

type forest_assetRequest struct {
	ForestSiteID domain.ID `json:"forest_site_id"`
	Label        string    `json:"label"`
	RowNumber    int       `json:"row_number"`
	Orientation  string    `json:"orientation"`
	Angle        float64   `json:"angle_degrees"`
}

func (a *API) createForestAsset(w http.ResponseWriter, r *http.Request) {
	var input forest_assetRequest
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, err)
		return
	}
	forest_asset, err := a.service.CreateForestAsset(r.Context(), principalFrom(r.Context()), service.CreateForestAssetInput(input))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, forest_asset)
}

func (a *API) listForestAssets(w http.ResponseWriter, r *http.Request) {
	forest_siteID, err := pathID(r, "forest_siteID")
	if err != nil {
		writeError(w, err)
		return
	}
	forest_assets, err := a.service.ListForestAssets(r.Context(), principalFrom(r.Context()), forest_siteID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": forest_assets})
}

func pathID(r *http.Request, name string) (domain.ID, error) {
	id := domain.ID(strings.TrimSpace(r.PathValue(name)))
	if !id.Valid() {
		return "", domain.FieldError{Field: name, Message: "is invalid"}
	}
	return id, nil
}
