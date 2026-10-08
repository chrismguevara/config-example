package settings

import (
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// userHeader carries the caller identity. Auth is out of scope for this
// example; a real deployment would take the user id from a verified token.
const userHeader = "X-User-ID"

// RegisterRoutes mounts the settings API on r.
//
//	GET    /api/settings/schema                   current JSON Schema (for runtime enum lists)
//	GET    /api/settings/schema/:version          a specific version
//	GET    /api/me/settings                       whole document + ETag
//	PATCH  /api/me/settings                       RFC 7386 merge patch, optional If-Match
//	DELETE /api/me/settings                       reset to defaults
//	PUT    /api/me/settings/theme                 {"theme": "dark"}
//	PUT    /api/me/settings/feature-flags/:flag   enable
//	DELETE /api/me/settings/feature-flags/:flag   disable
//	PUT    /api/me/settings/map/base-layer        {"baseLayer": "satellite"}
//	PUT    /api/me/settings/map/layers/:layer     show
//	DELETE /api/me/settings/map/layers/:layer     hide
//	PUT    /api/me/settings/filter-presets/:id    upsert preset (body is the preset)
//	DELETE /api/me/settings/filter-presets/:id    remove preset
func RegisterRoutes(r *gin.Engine, svc *Service, v *Validator) {
	r.GET("/healthz", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	r.GET("/api/settings/schema", func(c *gin.Context) { serveSchema(c, v, CurrentVersion) })
	r.GET("/api/settings/schema/:version", func(c *gin.Context) {
		n, err := strconv.Atoi(c.Param("version"))
		if err != nil {
			writeError(c, http.StatusBadRequest, "version must be an integer", nil)
			return
		}
		serveSchema(c, v, n)
	})

	h := &handler{svc: svc}
	me := r.Group("/api/me/settings", requireUser)
	me.GET("", h.get)
	me.PATCH("", h.patch)
	me.DELETE("", h.reset)
	me.PUT("/theme", h.setTheme)
	me.PUT("/feature-flags/:flag", h.flag(true))
	me.DELETE("/feature-flags/:flag", h.flag(false))
	me.PUT("/map/base-layer", h.setBaseLayer)
	me.PUT("/map/layers/:layer", h.layer(true))
	me.DELETE("/map/layers/:layer", h.layer(false))
	me.PUT("/filter-presets/:id", h.upsertPreset)
	me.DELETE("/filter-presets/:id", h.deletePreset)
}

type handler struct{ svc *Service }

func requireUser(c *gin.Context) {
	uid := c.GetHeader(userHeader)
	if uid == "" {
		writeError(c, http.StatusUnauthorized, "missing "+userHeader+" header", nil)
		c.Abort()
		return
	}
	c.Set("userID", uid)
	c.Next()
}

func userID(c *gin.Context) string { return c.GetString("userID") }

func (h *handler) get(c *gin.Context) {
	res, err := h.svc.Get(c.Request.Context(), userID(c))
	h.respond(c, res, err)
}

func (h *handler) patch(c *gin.Context) {
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 64<<10))
	if err != nil {
		writeError(c, http.StatusBadRequest, "cannot read body", nil)
		return
	}
	res, err := h.svc.MergePatch(c.Request.Context(), userID(c), body, c.GetHeader("If-Match"))
	h.respond(c, res, err)
}

func (h *handler) reset(c *gin.Context) {
	res, err := h.svc.Reset(c.Request.Context(), userID(c))
	h.respond(c, res, err)
}

func (h *handler) setTheme(c *gin.Context) {
	var in struct {
		Theme string `json:"theme"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		writeError(c, http.StatusBadRequest, "body must be {\"theme\": \"...\"}", nil)
		return
	}
	res, err := h.svc.Mutate(c.Request.Context(), userID(c), SetTheme(in.Theme))
	h.respond(c, res, err)
}

func (h *handler) flag(on bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		res, err := h.svc.Mutate(c.Request.Context(), userID(c), SetFlag(c.Param("flag"), on))
		h.respond(c, res, err)
	}
}

func (h *handler) setBaseLayer(c *gin.Context) {
	var in struct {
		BaseLayer string `json:"baseLayer"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		writeError(c, http.StatusBadRequest, "body must be {\"baseLayer\": \"...\"}", nil)
		return
	}
	res, err := h.svc.Mutate(c.Request.Context(), userID(c), SetBaseLayer(in.BaseLayer))
	h.respond(c, res, err)
}

func (h *handler) layer(on bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		res, err := h.svc.Mutate(c.Request.Context(), userID(c), SetLayer(c.Param("layer"), on))
		h.respond(c, res, err)
	}
}

func (h *handler) upsertPreset(c *gin.Context) {
	var p FilterPreset
	if err := c.ShouldBindJSON(&p); err != nil {
		writeError(c, http.StatusBadRequest, "body must be a filter preset object", nil)
		return
	}
	p.ID = c.Param("id") // the path wins over the body
	res, err := h.svc.Mutate(c.Request.Context(), userID(c), UpsertPreset(p))
	h.respond(c, res, err)
}

func (h *handler) deletePreset(c *gin.Context) {
	res, err := h.svc.Mutate(c.Request.Context(), userID(c), DeletePreset(c.Param("id")))
	h.respond(c, res, err)
}

// respond writes the document with its ETag, or maps an error to a status.
func (h *handler) respond(c *gin.Context, res Result, err error) {
	if err == nil {
		c.Header("ETag", res.ETag)
		c.Header("Cache-Control", "no-cache")
		c.Data(http.StatusOK, "application/json", res.Settings)
		return
	}
	var ve *ValidationError
	switch {
	case errors.As(err, &ve):
		writeError(c, http.StatusUnprocessableEntity, "settings failed schema validation", ve)
	case errors.Is(err, ErrPreconditionFailed):
		writeError(c, http.StatusPreconditionFailed, err.Error(), nil)
	case errors.Is(err, ErrBadRequest):
		writeError(c, http.StatusBadRequest, err.Error(), nil)
	default:
		_ = c.Error(err)
		writeError(c, http.StatusInternalServerError, "internal error", nil)
	}
}

func serveSchema(c *gin.Context, v *Validator, version int) {
	b := v.SchemaJSON(version)
	if b == nil {
		writeError(c, http.StatusNotFound, "no such schema version", nil)
		return
	}
	c.Header("Cache-Control", "public, max-age=300")
	c.Data(http.StatusOK, "application/schema+json", b)
}

// errorBody is the single error envelope the frontend understands.
type errorBody struct {
	Error         string    `json:"error"`
	SchemaVersion int       `json:"schemaVersion,omitempty"`
	Problems      []Problem `json:"problems,omitempty"`
}

func writeError(c *gin.Context, status int, msg string, ve *ValidationError) {
	body := errorBody{Error: msg}
	if ve != nil {
		body.SchemaVersion = ve.Version
		body.Problems = ve.Problems
	}
	c.JSON(status, body)
}
