package httpapi

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strconv"
	"time"
)

// cacheZwiftWorldMap is a day: the artwork is a published asset that changes
// only when Zwift redraws a world. Private for the same reason every other
// response here is — it is served only to the gated identity.
const cacheZwiftWorldMap = "private, max-age=86400"

// GetZwiftWorldMap serves one Zwift world's published map artwork from this
// origin, which is the only way a browser can have it: the CDN answers no CORS
// header. The bytes are relayed, never redirected to, so nothing tells a third
// party which world this rider is looking at.
func (h *Handler) GetZwiftWorldMap(writer http.ResponseWriter, request *http.Request) {
	worldID, err := strconv.ParseInt(request.PathValue("worldId"), 10, 64)
	if err != nil || h.zwiftWorldMaps == nil {
		h.notFound(writer)

		return
	}
	data, contentType, found, imageErr := h.zwiftWorldMaps.Image(request.Context(), worldID)
	if imageErr != nil {
		h.error(writer, http.StatusBadGateway, "provider_unavailable", "the world map could not be read")

		return
	}
	if !found {
		h.notFound(writer)

		return
	}
	header := writer.Header()
	header.Set("Content-Type", contentType)
	// Overrides the blanket no-store serve() set: this is a published asset,
	// the same for every reader of it.
	header.Set("Cache-Control", cacheZwiftWorldMap)
	digest := sha256.Sum256(data)
	header.Set("ETag", `"`+hex.EncodeToString(digest[:])+`"`)
	// ServeContent answers the conditional request and the range, so neither is
	// hand-rolled here. The zero time leaves it to the ETag alone.
	http.ServeContent(writer, request, "", time.Time{}, bytes.NewReader(data))
}
