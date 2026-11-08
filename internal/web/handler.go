// Package web 是数据网关的 HTTP 入口：两条写入/查询路径与一个健康检查。
package web

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/lifeipkg/rcp-dataway/internal/guard"
	"github.com/lifeipkg/rcp-dataway/internal/store"
)

// Handler 依赖的都是接口/值对象，不依赖具体数据库实现：拒绝路径可以在没有
// PostgreSQL 的环境里被完整断言。
type Handler struct {
	db             store.DB
	guard          *guard.Guard
	templates      *guard.Templates
	datasourceName string
	defaultMaxRows int
}

func NewHandler(db store.DB, g *guard.Guard, templates *guard.Templates,
	datasourceName string, defaultMaxRows int) *Handler {
	return &Handler{
		db:             db,
		guard:          g,
		templates:      templates,
		datasourceName: datasourceName,
		defaultMaxRows: defaultMaxRows,
	}
}

// NewRouter 注册路由。
//
// 路径写成完整字面量（不做变量拼接）：rcp-contracts/tools/contract_check.py 用
// 正则抽 r.GET/r.POST 与 OpenAPI 做两向差集，拼接会让机械对表失效。
// 健康检查用 r.Handle 注册，不计入契约路由——它不在公开契约内。
func NewRouter(h *Handler) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())

	r.POST("/dataway/query", h.query)
	r.POST("/dataway/execute", h.execute)
	r.Handle(http.MethodGet, "/actuator/health", h.health)
	return r
}

func (h *Handler) health(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	if err := h.db.Ping(ctx); err != nil {
		// 探活要反映真实可服务性：连不上数据库的网关不该被摘掉流量的反面——
		// 它已经无法回答任何查询，继续报 UP 只会把失败推给调用方。
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "DOWN"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "UP"})
}

func (h *Handler) query(c *gin.Context) {
	var req QueryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "BAD_REQUEST", "请求体不是合法 JSON")
		return
	}
	if err := h.checkDatasource(req.Datasource); err != nil {
		writeRejection(c, err)
		return
	}
	statement, err := h.guard.Validate(req.SQL)
	if err != nil {
		writeRejection(c, err)
		return
	}

	maxRows := h.defaultMaxRows
	if req.MaxRows != nil && *req.MaxRows > 0 && *req.MaxRows < maxRows {
		maxRows = *req.MaxRows
	}

	start := time.Now()
	result, err := h.db.Query(c.Request.Context(), guard.Rewrite(statement), req.Params, maxRows)
	if err != nil {
		internalError(c, "查询失败", err)
		return
	}
	c.JSON(http.StatusOK, queryResponse{
		Columns:   result.Columns,
		Rows:      result.Rows,
		RowCount:  len(result.Rows),
		ElapsedMs: time.Since(start).Milliseconds(),
	})
}

func (h *Handler) execute(c *gin.Context) {
	var req ExecuteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "BAD_REQUEST", "请求体不是合法 JSON")
		return
	}
	if err := h.checkDatasource(req.Datasource); err != nil {
		writeRejection(c, err)
		return
	}

	statement, required, err := h.templates.Resolve(req.TemplateID)
	if err != nil {
		writeRejection(c, err)
		return
	}
	if required != len(req.Params) {
		writeRejection(c, &guard.Rejected{
			Code: guard.CodeParamMismatch,
			Message: "模板 " + req.TemplateID + " 需要 " +
				itoa(required) + " 个参数，实际 " + itoa(len(req.Params)),
		})
		return
	}

	affected, err := h.db.Exec(c.Request.Context(), statement, req.Params)
	if err != nil {
		internalError(c, "写入失败", err)
		return
	}
	c.JSON(http.StatusOK, executeResponse{AffectedRows: affected})
}

func (h *Handler) checkDatasource(requested string) error {
	name := strings.TrimSpace(requested)
	if name == "" {
		name = h.datasourceName
	}
	if name != h.datasourceName {
		return &guard.Rejected{
			Code:    guard.CodeDatasourceDenied,
			Message: "数据源未登记: " + name + "（允许: " + h.datasourceName + "）",
		}
	}
	return nil
}

// writeRejection 把守卫拒绝翻译成 400 + 编码化原因，方便调用方与审计按 code 分类。
func writeRejection(c *gin.Context, err error) {
	var rejected *guard.Rejected
	if errors.As(err, &rejected) {
		badRequest(c, rejected.Code, rejected.Message)
		return
	}
	internalError(c, "请求处理失败", err)
}

func badRequest(c *gin.Context, code, message string) {
	c.JSON(http.StatusBadRequest, errorResponse{Error: code, Message: message})
}

// internalError 只回固定文案：数据库错误信息会带表名与谓词，不适合回给调用方。
// 细节进服务端日志。
func internalError(c *gin.Context, message string, err error) {
	log.Printf("[dataway] %s: %v", message, err)
	c.JSON(http.StatusInternalServerError, errorResponse{
		Error:   "INTERNAL_ERROR",
		Message: message,
	})
}

func itoa(value int) string { return strconv.Itoa(value) }
