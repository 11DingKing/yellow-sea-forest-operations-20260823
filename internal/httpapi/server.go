package httpapi

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/repository"
	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/service"
)

type API struct {
	service *service.Service
	health  repository.HealthRepository
	logger  *slog.Logger
	mux     *http.ServeMux
}

func New(svc *service.Service, health repository.HealthRepository, logger *slog.Logger) *API {
	if logger == nil {
		logger = slog.Default()
	}
	api := &API{service: svc, health: health, logger: logger, mux: http.NewServeMux()}
	api.routes()
	return api
}

func (a *API) Handler() http.Handler {
	return a.requestIdentity(a.recoverPanic(a.requestLog(a.securityHeaders(a.mux))))
}

func (a *API) routes() {
	a.mux.HandleFunc("GET /livez", a.live)
	a.mux.HandleFunc("GET /readyz", a.ready)
	a.mux.HandleFunc("POST /v1/session", a.login)
	a.mux.Handle("DELETE /v1/session", a.auth(http.HandlerFunc(a.logout)))
	a.mux.Handle("GET /v1/me", a.auth(http.HandlerFunc(a.me)))

	a.mux.Handle("POST /v1/admin/users", a.auth(http.HandlerFunc(a.registerUser)))
	a.mux.Handle("POST /v1/admin/forest_sites", a.auth(http.HandlerFunc(a.createForestSite)))
	a.mux.Handle("POST /v1/admin/zones", a.auth(http.HandlerFunc(a.createZone)))
	a.mux.Handle("POST /v1/admin/forest_assets", a.auth(http.HandlerFunc(a.createForestAsset)))
	a.mux.Handle("GET /v1/forest_sites/{forest_siteID}/forest_assets", a.auth(http.HandlerFunc(a.listForestAssets)))

	a.mux.Handle("POST /v1/cases", a.auth(http.HandlerFunc(a.submitCase)))
	a.mux.Handle("GET /v1/cases", a.auth(http.HandlerFunc(a.listCases)))
	a.mux.Handle("GET /v1/cases/{caseID}", a.auth(http.HandlerFunc(a.getCase)))
	a.mux.Handle("POST /v1/cases/{caseID}/assignment", a.auth(http.HandlerFunc(a.assignCase)))
	a.mux.Handle("POST /v1/cases/{caseID}/surveys", a.auth(http.HandlerFunc(a.startForestSurvey)))
	a.mux.Handle("POST /v1/cases/{caseID}/verifications", a.auth(http.HandlerFunc(a.startInspection)))
	a.mux.Handle("POST /v1/cases/{caseID}/reopen", a.auth(http.HandlerFunc(a.reopenCase)))

	a.mux.Handle("POST /v1/surveys/{forest_surveyID}/readings", a.auth(http.HandlerFunc(a.addReading)))
	a.mux.Handle("POST /v1/surveys/{forest_surveyID}/publish", a.auth(http.HandlerFunc(a.publishForestSurvey)))
	a.mux.Handle("POST /v1/plans", a.auth(http.HandlerFunc(a.proposePlan)))
	a.mux.Handle("POST /v1/plans/{planID}/operator-acceptance", a.auth(http.HandlerFunc(a.acceptPlan)))
	a.mux.Handle("POST /v1/plans/{planID}/approval", a.auth(http.HandlerFunc(a.approvePlan)))
	a.mux.Handle("POST /v1/actions/{actionID}/claim", a.auth(http.HandlerFunc(a.claimAction)))
	a.mux.Handle("POST /v1/actions/{actionID}/completion", a.auth(http.HandlerFunc(a.completeAction)))
	a.mux.Handle("POST /v1/verifications/{verificationID}/completion", a.auth(http.HandlerFunc(a.completeInspection)))
}

func (a *API) live(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "alive", "time": time.Now().UTC()})
}

func (a *API) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := contextWithTimeout(r, 2*time.Second)
	defer cancel()
	if err := a.health.Ping(ctx); err != nil {
		writeError(w, err)
		return
	}
	version, err := a.health.SchemaVersion(ctx)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ready", "schema_version": version})
}
