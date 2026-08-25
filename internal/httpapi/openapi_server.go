package httpapi

import (
	"bytes"
	"context"
	"io"
	"net/http"

	openapi "github.com/nobbs/domestique/internal/httpapi/contract"
)

type strictServerInterface = openapi.StrictServerInterface
type strictHandlerFunc = openapi.StrictHandlerFunc

// contractServer adapts the established handler logic to the generated strict
// interface. Keeping the legacy writers inside this adapter is intentional:
// geometry coordinates and surface ranges remain json.RawMessage all the way to
// the response instead of being decoded into generated float slices and encoded
// again.
type contractServer struct{ handler *Handler }

var _ strictServerInterface = (*contractServer)(nil)

type contractRequestKey struct{}
type scheduleBodyKey struct{}

// rememberContractRequest makes the original request available to the strict
// implementation after generated binding has produced its request object.
func rememberContractRequest(next strictHandlerFunc, _ string) strictHandlerFunc {
	return func(ctx context.Context, writer http.ResponseWriter, request *http.Request, value any) (any, error) {
		return next(context.WithValue(ctx, contractRequestKey{}, request), writer, request, value)
	}
}

// capturedResponse defers a legacy handler until the generated strict handler
// hands over the real ResponseWriter. Nothing is buffered: a geometry or asset
// body is written straight through, so serving one costs no copy of it and
// http.ServeContent's incremental writes still reach the client.
type capturedResponse struct {
	request *http.Request
	handler func(http.ResponseWriter, *http.Request)
}

func (response capturedResponse) write(writer http.ResponseWriter) error {
	if response.request == nil {
		writer.WriteHeader(http.StatusInternalServerError)

		return nil
	}
	response.handler(writer, response.request)

	return nil
}

func (server *contractServer) capture(ctx context.Context, handler func(http.ResponseWriter, *http.Request)) capturedResponse {
	request, ok := ctx.Value(contractRequestKey{}).(*http.Request)
	if !ok {
		return capturedResponse{}
	}

	return capturedResponse{request: request, handler: handler}
}

func (server *contractServer) scheduleRequest(ctx context.Context) *http.Request {
	request, requestOK := ctx.Value(contractRequestKey{}).(*http.Request)
	body, bodyOK := ctx.Value(scheduleBodyKey{}).([]byte)
	if !requestOK || !bodyOK {
		return request
	}

	clonedRequest := request.Clone(ctx)
	clonedRequest.Body = io.NopCloser(bytes.NewReader(body))

	return clonedRequest
}

type getIndexCapturedResponse struct{ capturedResponse }

func (r getIndexCapturedResponse) VisitGetIndexResponse(w http.ResponseWriter) error {
	return r.write(w)
}

type getAssetCapturedResponse struct{ capturedResponse }

func (r getAssetCapturedResponse) VisitGetAssetResponse(w http.ResponseWriter) error {
	return r.write(w)
}

type getFaviconCapturedResponse struct{ capturedResponse }

func (r getFaviconCapturedResponse) VisitGetFaviconResponse(w http.ResponseWriter) error {
	return r.write(w)
}

type getHealthCapturedResponse struct{ capturedResponse }

func (r getHealthCapturedResponse) VisitGetHealthResponse(w http.ResponseWriter) error {
	return r.write(w)
}

type getIcon256CapturedResponse struct{ capturedResponse }

func (r getIcon256CapturedResponse) VisitGetIcon256Response(w http.ResponseWriter) error {
	return r.write(w)
}

type getIcon512CapturedResponse struct{ capturedResponse }

func (r getIcon512CapturedResponse) VisitGetIcon512Response(w http.ResponseWriter) error {
	return r.write(w)
}

type getManifestCapturedResponse struct{ capturedResponse }

func (r getManifestCapturedResponse) VisitGetManifestResponse(w http.ResponseWriter) error {
	return r.write(w)
}

type completeOAuthCapturedResponse struct{ capturedResponse }

func (r completeOAuthCapturedResponse) VisitCompleteOAuthResponse(w http.ResponseWriter) error {
	return r.write(w)
}

type startOAuthCapturedResponse struct{ capturedResponse }

func (r startOAuthCapturedResponse) VisitStartOAuthResponse(w http.ResponseWriter) error {
	return r.write(w)
}

type getRoutePageCapturedResponse struct{ capturedResponse }

func (r getRoutePageCapturedResponse) VisitGetRoutePageResponse(w http.ResponseWriter) error {
	return r.write(w)
}

type redirectLegacyRoutePageCapturedResponse struct{ capturedResponse }

func (r redirectLegacyRoutePageCapturedResponse) VisitRedirectLegacyRoutePageResponse(w http.ResponseWriter) error {
	return r.write(w)
}

type getSettingsPageCapturedResponse struct{ capturedResponse }

func (r getSettingsPageCapturedResponse) VisitGetSettingsPageResponse(w http.ResponseWriter) error {
	return r.write(w)
}

type getSyncPageCapturedResponse struct{ capturedResponse }

func (r getSyncPageCapturedResponse) VisitGetSyncPageResponse(w http.ResponseWriter) error {
	return r.write(w)
}

type getRouteCapturedResponse struct{ capturedResponse }

func (r getRouteCapturedResponse) VisitGetRouteResponse(w http.ResponseWriter) error {
	return r.write(w)
}

type getRouteGeometryCapturedResponse struct{ capturedResponse }

func (r getRouteGeometryCapturedResponse) VisitGetRouteGeometryResponse(w http.ResponseWriter) error {
	return r.write(w)
}

type reprocessRouteCapturedResponse struct{ capturedResponse }

func (r reprocessRouteCapturedResponse) VisitReprocessRouteResponse(w http.ResponseWriter) error {
	return r.write(w)
}

type getRoutesCapturedResponse struct{ capturedResponse }

func (r getRoutesCapturedResponse) VisitGetRoutesResponse(w http.ResponseWriter) error {
	return r.write(w)
}

type redirectLegacyRouteCapturedResponse struct{ capturedResponse }

func (r redirectLegacyRouteCapturedResponse) VisitRedirectLegacyRouteResponse(w http.ResponseWriter) error {
	return r.write(w)
}

type redirectLegacyGeometryCapturedResponse struct{ capturedResponse }

func (r redirectLegacyGeometryCapturedResponse) VisitRedirectLegacyGeometryResponse(w http.ResponseWriter) error {
	return r.write(w)
}

type redirectLegacyReprocessCapturedResponse struct{ capturedResponse }

func (r redirectLegacyReprocessCapturedResponse) VisitRedirectLegacyReprocessResponse(w http.ResponseWriter) error {
	return r.write(w)
}

type getStatusCapturedResponse struct{ capturedResponse }

func (r getStatusCapturedResponse) VisitGetStatusResponse(w http.ResponseWriter) error {
	return r.write(w)
}

type triggerSyncCapturedResponse struct{ capturedResponse }

func (r triggerSyncCapturedResponse) VisitTriggerSyncResponse(w http.ResponseWriter) error {
	return r.write(w)
}

type getSyncRunsCapturedResponse struct{ capturedResponse }

func (r getSyncRunsCapturedResponse) VisitGetSyncRunsResponse(w http.ResponseWriter) error {
	return r.write(w)
}

type setSyncScheduleCapturedResponse struct{ capturedResponse }

func (r setSyncScheduleCapturedResponse) VisitSetSyncScheduleResponse(w http.ResponseWriter) error {
	return r.write(w)
}

type triggerSourceSyncCapturedResponse struct{ capturedResponse }

func (r triggerSourceSyncCapturedResponse) VisitTriggerSourceSyncResponse(w http.ResponseWriter) error {
	return r.write(w)
}

type triggerSurfaceSyncCapturedResponse struct{ capturedResponse }

func (r triggerSurfaceSyncCapturedResponse) VisitTriggerSurfaceSyncResponse(w http.ResponseWriter) error {
	return r.write(w)
}

type triggerTargetsSyncCapturedResponse struct{ capturedResponse }

func (r triggerTargetsSyncCapturedResponse) VisitTriggerTargetsSyncResponse(w http.ResponseWriter) error {
	return r.write(w)
}

type triggerTargetSyncCapturedResponse struct{ capturedResponse }

func (r triggerTargetSyncCapturedResponse) VisitTriggerTargetSyncResponse(w http.ResponseWriter) error {
	return r.write(w)
}

type clearTargetCapturedResponse struct{ capturedResponse }

func (r clearTargetCapturedResponse) VisitClearTargetResponse(w http.ResponseWriter) error {
	return r.write(w)
}

type getWeatherCapturedResponse struct{ capturedResponse }

func (r getWeatherCapturedResponse) VisitGetWeatherResponse(w http.ResponseWriter) error {
	return r.write(w)
}

type getWebUIConfigCapturedResponse struct{ capturedResponse }

func (r getWebUIConfigCapturedResponse) VisitGetWebUIConfigResponse(w http.ResponseWriter) error {
	return r.write(w)
}

func (server *contractServer) GetIndex(ctx context.Context, _ openapi.GetIndexRequestObject) (openapi.GetIndexResponseObject, error) {
	return getIndexCapturedResponse{server.capture(ctx, func(w http.ResponseWriter, r *http.Request) { server.handler.index(w, r, server.handler.allowedEmail) })}, nil
}
func (server *contractServer) GetAsset(ctx context.Context, _ openapi.GetAssetRequestObject) (openapi.GetAssetResponseObject, error) {
	return getAssetCapturedResponse{server.capture(ctx, func(w http.ResponseWriter, r *http.Request) {
		server.handler.staticAsset(w, r, server.handler.allowedEmail)
	})}, nil
}
func (server *contractServer) GetFavicon(ctx context.Context, _ openapi.GetFaviconRequestObject) (openapi.GetFaviconResponseObject, error) {
	return getFaviconCapturedResponse{server.capture(ctx, func(w http.ResponseWriter, r *http.Request) {
		server.handler.stableAsset(w, r, server.handler.allowedEmail)
	})}, nil
}
func (server *contractServer) GetHealth(ctx context.Context, _ openapi.GetHealthRequestObject) (openapi.GetHealthResponseObject, error) {
	return getHealthCapturedResponse{server.capture(ctx, server.handler.health)}, nil
}
func (server *contractServer) GetIcon256(ctx context.Context, _ openapi.GetIcon256RequestObject) (openapi.GetIcon256ResponseObject, error) {
	return getIcon256CapturedResponse{server.capture(ctx, func(w http.ResponseWriter, r *http.Request) {
		server.handler.stableAsset(w, r, server.handler.allowedEmail)
	})}, nil
}
func (server *contractServer) GetIcon512(ctx context.Context, _ openapi.GetIcon512RequestObject) (openapi.GetIcon512ResponseObject, error) {
	return getIcon512CapturedResponse{server.capture(ctx, func(w http.ResponseWriter, r *http.Request) {
		server.handler.stableAsset(w, r, server.handler.allowedEmail)
	})}, nil
}
func (server *contractServer) GetManifest(ctx context.Context, _ openapi.GetManifestRequestObject) (openapi.GetManifestResponseObject, error) {
	return getManifestCapturedResponse{server.capture(ctx, func(w http.ResponseWriter, r *http.Request) {
		server.handler.webManifest(w, r, server.handler.allowedEmail)
	})}, nil
}
func (server *contractServer) CompleteOAuth(ctx context.Context, _ openapi.CompleteOAuthRequestObject) (openapi.CompleteOAuthResponseObject, error) {
	return completeOAuthCapturedResponse{server.capture(ctx, func(w http.ResponseWriter, r *http.Request) {
		server.handler.callback(w, r, server.handler.allowedEmail)
	})}, nil
}
func (server *contractServer) StartOAuth(ctx context.Context, _ openapi.StartOAuthRequestObject) (openapi.StartOAuthResponseObject, error) {
	return startOAuthCapturedResponse{server.capture(ctx, func(w http.ResponseWriter, r *http.Request) { server.handler.start(w, r, server.handler.allowedEmail) })}, nil
}
func (server *contractServer) GetRoutePage(ctx context.Context, _ openapi.GetRoutePageRequestObject) (openapi.GetRoutePageResponseObject, error) {
	return getRoutePageCapturedResponse{server.capture(ctx, func(w http.ResponseWriter, r *http.Request) { server.handler.index(w, r, server.handler.allowedEmail) })}, nil
}
func (server *contractServer) RedirectLegacyRoutePage(ctx context.Context, _ openapi.RedirectLegacyRoutePageRequestObject) (openapi.RedirectLegacyRoutePageResponseObject, error) {
	return redirectLegacyRoutePageCapturedResponse{server.capture(ctx, func(w http.ResponseWriter, r *http.Request) {
		server.handler.redirectLegacyBrowserRoute(w, r, server.handler.allowedEmail)
	})}, nil
}
func (server *contractServer) GetSettingsPage(ctx context.Context, _ openapi.GetSettingsPageRequestObject) (openapi.GetSettingsPageResponseObject, error) {
	return getSettingsPageCapturedResponse{server.capture(ctx, func(w http.ResponseWriter, r *http.Request) { server.handler.index(w, r, server.handler.allowedEmail) })}, nil
}
func (server *contractServer) GetSyncPage(ctx context.Context, _ openapi.GetSyncPageRequestObject) (openapi.GetSyncPageResponseObject, error) {
	return getSyncPageCapturedResponse{server.capture(ctx, func(w http.ResponseWriter, r *http.Request) { server.handler.index(w, r, server.handler.allowedEmail) })}, nil
}
func (server *contractServer) GetRoute(ctx context.Context, _ openapi.GetRouteRequestObject) (openapi.GetRouteResponseObject, error) {
	return getRouteCapturedResponse{server.capture(ctx, func(w http.ResponseWriter, r *http.Request) { server.handler.stage(w, r, server.handler.allowedEmail) })}, nil
}
func (server *contractServer) GetRouteGeometry(ctx context.Context, _ openapi.GetRouteGeometryRequestObject) (openapi.GetRouteGeometryResponseObject, error) {
	return getRouteGeometryCapturedResponse{server.capture(ctx, func(w http.ResponseWriter, r *http.Request) {
		server.handler.stageGeometry(w, r, server.handler.allowedEmail)
	})}, nil
}
func (server *contractServer) ReprocessRoute(ctx context.Context, _ openapi.ReprocessRouteRequestObject) (openapi.ReprocessRouteResponseObject, error) {
	return reprocessRouteCapturedResponse{server.capture(ctx, func(w http.ResponseWriter, r *http.Request) {
		server.handler.reprocessStage(w, r, server.handler.allowedEmail)
	})}, nil
}
func (server *contractServer) GetRoutes(ctx context.Context, _ openapi.GetRoutesRequestObject) (openapi.GetRoutesResponseObject, error) {
	return getRoutesCapturedResponse{server.capture(ctx, func(w http.ResponseWriter, r *http.Request) { server.handler.stages(w, r, server.handler.allowedEmail) })}, nil
}
func (server *contractServer) RedirectLegacyRoute(ctx context.Context, _ openapi.RedirectLegacyRouteRequestObject) (openapi.RedirectLegacyRouteResponseObject, error) {
	return redirectLegacyRouteCapturedResponse{server.capture(ctx, func(w http.ResponseWriter, r *http.Request) {
		server.handler.redirectLegacyStagePath("")(w, r, server.handler.allowedEmail)
	})}, nil
}
func (server *contractServer) RedirectLegacyGeometry(ctx context.Context, _ openapi.RedirectLegacyGeometryRequestObject) (openapi.RedirectLegacyGeometryResponseObject, error) {
	return redirectLegacyGeometryCapturedResponse{server.capture(ctx, func(w http.ResponseWriter, r *http.Request) {
		server.handler.redirectLegacyStagePath("/geometry")(w, r, server.handler.allowedEmail)
	})}, nil
}
func (server *contractServer) RedirectLegacyReprocess(ctx context.Context, _ openapi.RedirectLegacyReprocessRequestObject) (openapi.RedirectLegacyReprocessResponseObject, error) {
	return redirectLegacyReprocessCapturedResponse{server.capture(ctx, func(w http.ResponseWriter, r *http.Request) {
		server.handler.redirectLegacyStagePath("/reprocess")(w, r, server.handler.allowedEmail)
	})}, nil
}
func (server *contractServer) GetStatus(ctx context.Context, _ openapi.GetStatusRequestObject) (openapi.GetStatusResponseObject, error) {
	return getStatusCapturedResponse{server.capture(ctx, func(w http.ResponseWriter, r *http.Request) { server.handler.status(w, r, server.handler.allowedEmail) })}, nil
}
func (server *contractServer) TriggerSync(ctx context.Context, _ openapi.TriggerSyncRequestObject) (openapi.TriggerSyncResponseObject, error) {
	return triggerSyncCapturedResponse{server.capture(ctx, func(w http.ResponseWriter, r *http.Request) { server.handler.sync(w, r, server.handler.allowedEmail) })}, nil
}
func (server *contractServer) GetSyncRuns(ctx context.Context, _ openapi.GetSyncRunsRequestObject) (openapi.GetSyncRunsResponseObject, error) {
	return getSyncRunsCapturedResponse{server.capture(ctx, func(w http.ResponseWriter, r *http.Request) {
		server.handler.syncHistory(w, r, server.handler.allowedEmail)
	})}, nil
}
func (server *contractServer) SetSyncSchedule(ctx context.Context, _ openapi.SetSyncScheduleRequestObject) (openapi.SetSyncScheduleResponseObject, error) {
	return setSyncScheduleCapturedResponse{server.capture(ctx, func(w http.ResponseWriter, _ *http.Request) {
		server.handler.setSyncSchedule(w, server.scheduleRequest(ctx), server.handler.allowedEmail)
	})}, nil
}
func (server *contractServer) TriggerSourceSync(ctx context.Context, _ openapi.TriggerSourceSyncRequestObject) (openapi.TriggerSourceSyncResponseObject, error) {
	return triggerSourceSyncCapturedResponse{server.capture(ctx, func(w http.ResponseWriter, r *http.Request) {
		server.handler.syncSource(w, r, server.handler.allowedEmail)
	})}, nil
}
func (server *contractServer) TriggerSurfaceSync(ctx context.Context, _ openapi.TriggerSurfaceSyncRequestObject) (openapi.TriggerSurfaceSyncResponseObject, error) {
	return triggerSurfaceSyncCapturedResponse{server.capture(ctx, func(w http.ResponseWriter, r *http.Request) {
		server.handler.syncSurface(w, r, server.handler.allowedEmail)
	})}, nil
}
func (server *contractServer) TriggerTargetsSync(ctx context.Context, _ openapi.TriggerTargetsSyncRequestObject) (openapi.TriggerTargetsSyncResponseObject, error) {
	return triggerTargetsSyncCapturedResponse{server.capture(ctx, func(w http.ResponseWriter, r *http.Request) {
		server.handler.syncTargets(w, r, server.handler.allowedEmail)
	})}, nil
}
func (server *contractServer) TriggerTargetSync(ctx context.Context, _ openapi.TriggerTargetSyncRequestObject) (openapi.TriggerTargetSyncResponseObject, error) {
	return triggerTargetSyncCapturedResponse{server.capture(ctx, func(w http.ResponseWriter, r *http.Request) {
		server.handler.syncTarget(w, r, server.handler.allowedEmail)
	})}, nil
}
func (server *contractServer) ClearTarget(ctx context.Context, _ openapi.ClearTargetRequestObject) (openapi.ClearTargetResponseObject, error) {
	return clearTargetCapturedResponse{server.capture(ctx, func(w http.ResponseWriter, r *http.Request) {
		server.handler.clearTarget(w, r, server.handler.allowedEmail)
	})}, nil
}
func (server *contractServer) GetWeather(ctx context.Context, _ openapi.GetWeatherRequestObject) (openapi.GetWeatherResponseObject, error) {
	return getWeatherCapturedResponse{server.capture(ctx, func(w http.ResponseWriter, r *http.Request) {
		server.handler.weatherForecast(w, r, server.handler.allowedEmail)
	})}, nil
}
func (server *contractServer) GetWebUIConfig(ctx context.Context, _ openapi.GetWebUIConfigRequestObject) (openapi.GetWebUIConfigResponseObject, error) {
	return getWebUIConfigCapturedResponse{server.capture(ctx, func(w http.ResponseWriter, r *http.Request) {
		server.handler.webUIConfig(w, r, server.handler.allowedEmail)
	})}, nil
}
