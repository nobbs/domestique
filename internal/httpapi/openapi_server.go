package httpapi

import (
	"net/http"

	openapi "github.com/nobbs/domestique/internal/httpapi/contract"
)

// contractServer adapts the handler methods to the generated router. Every
// method is a forwarder: the generated wrapper has already bound and validated
// this operation's parameters — rejecting a malformed one with 400 before
// reaching here — and the handlers read the values they need back off the
// request, so the bound copies are deliberately unused.
//
// Non-strict generation is what keeps this file a forwarder. Strict mode would
// take the ResponseWriter away and decode the request body, and both are
// needed as they are: geometry coordinates and surface ranges stay
// json.RawMessage all the way to the wire, assets are served incrementally by
// http.ServeContent, and the schedule body is read under a size limit.
type contractServer struct{ handler *Handler }

var _ openapi.ServerInterface = (*contractServer)(nil)

func (server *contractServer) GetIndex(writer http.ResponseWriter, request *http.Request) {
	server.handler.index(writer, request, server.handler.allowedEmail)
}

func (server *contractServer) GetAsset(writer http.ResponseWriter, request *http.Request, _ string) {
	server.handler.staticAsset(writer, request, server.handler.allowedEmail)
}

func (server *contractServer) GetFavicon(writer http.ResponseWriter, request *http.Request) {
	server.handler.stableAsset(writer, request, server.handler.allowedEmail)
}

func (server *contractServer) GetHealth(writer http.ResponseWriter, request *http.Request) {
	server.handler.health(writer, request)
}

func (server *contractServer) GetIcon256(writer http.ResponseWriter, request *http.Request) {
	server.handler.stableAsset(writer, request, server.handler.allowedEmail)
}

func (server *contractServer) GetIcon512(writer http.ResponseWriter, request *http.Request) {
	server.handler.stableAsset(writer, request, server.handler.allowedEmail)
}

func (server *contractServer) GetManifest(writer http.ResponseWriter, request *http.Request) {
	server.handler.webManifest(writer, request, server.handler.allowedEmail)
}

func (server *contractServer) CompleteOAuth(
	writer http.ResponseWriter, request *http.Request, _ openapi.CompleteOAuthParams,
) {
	server.handler.callback(writer, request, server.handler.allowedEmail)
}

func (server *contractServer) StartOAuth(writer http.ResponseWriter, request *http.Request, _ openapi.Target) {
	server.handler.start(writer, request, server.handler.allowedEmail)
}

func (server *contractServer) GetRoutePage(
	writer http.ResponseWriter, request *http.Request, _ openapi.Provider, _ openapi.RouteId, _ openapi.Stage,
) {
	server.handler.index(writer, request, server.handler.allowedEmail)
}

func (server *contractServer) RedirectLegacyRoutePage(
	writer http.ResponseWriter, request *http.Request, _ openapi.RouteId, _ openapi.Stage,
) {
	server.handler.redirectLegacyBrowserRoute(writer, request, server.handler.allowedEmail)
}

func (server *contractServer) GetSettingsPage(writer http.ResponseWriter, request *http.Request) {
	server.handler.index(writer, request, server.handler.allowedEmail)
}

func (server *contractServer) GetSyncPage(writer http.ResponseWriter, request *http.Request) {
	server.handler.index(writer, request, server.handler.allowedEmail)
}

func (server *contractServer) GetRoute(
	writer http.ResponseWriter, request *http.Request, _ openapi.Provider, _ openapi.RouteId, _ openapi.Stage,
) {
	server.handler.stage(writer, request, server.handler.allowedEmail)
}

func (server *contractServer) GetRouteGeometry(
	writer http.ResponseWriter, request *http.Request, _ openapi.Provider, _ openapi.RouteId, _ openapi.Stage,
) {
	server.handler.stageGeometry(writer, request, server.handler.allowedEmail)
}

func (server *contractServer) ReprocessRoute(
	writer http.ResponseWriter, request *http.Request,
	_ openapi.Provider, _ openapi.RouteId, _ openapi.Stage, _ openapi.ReprocessRouteParams,
) {
	server.handler.reprocessStage(writer, request, server.handler.allowedEmail)
}

func (server *contractServer) GetRoutes(writer http.ResponseWriter, request *http.Request) {
	server.handler.stages(writer, request, server.handler.allowedEmail)
}

func (server *contractServer) RedirectLegacyRoute(
	writer http.ResponseWriter, request *http.Request, _ openapi.RouteId, _ openapi.Stage,
) {
	server.handler.redirectLegacyStagePath("")(writer, request, server.handler.allowedEmail)
}

func (server *contractServer) RedirectLegacyGeometry(
	writer http.ResponseWriter, request *http.Request, _ openapi.RouteId, _ openapi.Stage,
) {
	server.handler.redirectLegacyStagePath("/geometry")(writer, request, server.handler.allowedEmail)
}

func (server *contractServer) RedirectLegacyReprocess(
	writer http.ResponseWriter, request *http.Request,
	_ openapi.RouteId, _ openapi.Stage, _ openapi.RedirectLegacyReprocessParams,
) {
	server.handler.redirectLegacyStagePath("/reprocess")(writer, request, server.handler.allowedEmail)
}

func (server *contractServer) GetStatus(writer http.ResponseWriter, request *http.Request) {
	server.handler.status(writer, request, server.handler.allowedEmail)
}

func (server *contractServer) TriggerSync(
	writer http.ResponseWriter, request *http.Request, _ openapi.TriggerSyncParams,
) {
	server.handler.sync(writer, request, server.handler.allowedEmail)
}

func (server *contractServer) GetSyncRuns(
	writer http.ResponseWriter, request *http.Request, _ openapi.GetSyncRunsParams,
) {
	server.handler.syncHistory(writer, request, server.handler.allowedEmail)
}

func (server *contractServer) SetSyncSchedule(
	writer http.ResponseWriter, request *http.Request, _ openapi.SetSyncScheduleParams,
) {
	server.handler.setSyncSchedule(writer, request, server.handler.allowedEmail)
}

func (server *contractServer) TriggerSourceSync(
	writer http.ResponseWriter, request *http.Request, _ openapi.TriggerSourceSyncParams,
) {
	server.handler.syncSource(writer, request, server.handler.allowedEmail)
}

func (server *contractServer) TriggerSurfaceSync(
	writer http.ResponseWriter, request *http.Request, _ openapi.TriggerSurfaceSyncParams,
) {
	server.handler.syncSurface(writer, request, server.handler.allowedEmail)
}

func (server *contractServer) TriggerTargetsSync(
	writer http.ResponseWriter, request *http.Request, _ openapi.TriggerTargetsSyncParams,
) {
	server.handler.syncTargets(writer, request, server.handler.allowedEmail)
}

func (server *contractServer) TriggerTargetSync(
	writer http.ResponseWriter, request *http.Request, _ openapi.Target, _ openapi.TriggerTargetSyncParams,
) {
	server.handler.syncTarget(writer, request, server.handler.allowedEmail)
}

func (server *contractServer) ClearTarget(
	writer http.ResponseWriter, request *http.Request, _ openapi.Target, _ openapi.ClearTargetParams,
) {
	server.handler.clearTarget(writer, request, server.handler.allowedEmail)
}

func (server *contractServer) GetWeather(
	writer http.ResponseWriter, request *http.Request, _ openapi.GetWeatherParams,
) {
	server.handler.weatherForecast(writer, request, server.handler.allowedEmail)
}

func (server *contractServer) GetWebUIConfig(writer http.ResponseWriter, request *http.Request) {
	server.handler.webUIConfig(writer, request, server.handler.allowedEmail)
}
