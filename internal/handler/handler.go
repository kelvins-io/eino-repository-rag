package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/kelvins-io/eino-repository-rag/internal/auth"
	"github.com/kelvins-io/eino-repository-rag/internal/rag"
	"github.com/kelvins-io/eino-repository-rag/internal/repository"
	"github.com/kelvins-io/eino-repository-rag/internal/service"
)

type KnowledgeHandler struct {
	svc *service.KnowledgeService
}

func NewKnowledgeHandler(svc *service.KnowledgeService) *KnowledgeHandler {
	return &KnowledgeHandler{svc: svc}
}

type APIResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func ok(c *gin.Context, data any) {
	c.JSON(http.StatusOK, APIResponse{Code: 0, Message: "ok", Data: data})
}

func fail(c *gin.Context, httpCode int, msg string) {
	c.JSON(httpCode, APIResponse{Code: httpCode, Message: msg})
}

func failErr(c *gin.Context, err error) {
	if errors.Is(err, service.ErrForbidden) {
		fail(c, http.StatusForbidden, err.Error())
		return
	}
	msg := err.Error()
	if strings.Contains(msg, "不存在") {
		fail(c, http.StatusNotFound, msg)
		return
	}
	if strings.Contains(msg, "required") || strings.Contains(msg, "无效") || strings.Contains(msg, "不能") {
		fail(c, http.StatusBadRequest, msg)
		return
	}
	fail(c, http.StatusBadRequest, msg)
}

func currentUserID(c *gin.Context) string {
	return auth.UserIDFromContext(c)
}

type createKBReq struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (h *KnowledgeHandler) CreateKnowledgeBase(c *gin.Context) {
	var req createKBReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	kb, err := h.svc.CreateKnowledgeBase(currentUserID(c), req.Name, req.Description)
	if err != nil {
		failErr(c, err)
		return
	}
	ok(c, kb)
}

func (h *KnowledgeHandler) ListKnowledgeBases(c *gin.Context) {
	list, err := h.svc.ListKnowledgeBases(currentUserID(c))
	if err != nil {
		failErr(c, err)
		return
	}
	ok(c, list)
}

func (h *KnowledgeHandler) GetKnowledgeBase(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		fail(c, http.StatusBadRequest, "无效的知识库 ID")
		return
	}
	kb, err := h.svc.GetKnowledgeBase(uint(id), currentUserID(c))
	if err != nil {
		failErr(c, err)
		return
	}
	ok(c, kb)
}

func (h *KnowledgeHandler) UpdateKnowledgeBase(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		fail(c, http.StatusBadRequest, "无效的知识库 ID")
		return
	}
	var req createKBReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	kb, err := h.svc.UpdateKnowledgeBase(uint(id), currentUserID(c), req.Name, req.Description)
	if err != nil {
		failErr(c, err)
		return
	}
	ok(c, kb)
}

func (h *KnowledgeHandler) DeleteKnowledgeBase(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		fail(c, http.StatusBadRequest, "无效的知识库 ID")
		return
	}
	if err := h.svc.DeleteKnowledgeBase(uint(id), currentUserID(c)); err != nil {
		failErr(c, err)
		return
	}
	ok(c, gin.H{"deleted": true})
}

type createDirReq struct {
	ParentID    *uint  `json:"parent_id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	SortOrder   int    `json:"sort_order"`
}

func (h *KnowledgeHandler) CreateDirectory(c *gin.Context) {
	kbID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		fail(c, http.StatusBadRequest, "无效的知识库 ID")
		return
	}
	var req createDirReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	dir, err := h.svc.CreateDirectory(service.CreateDirectoryInput{
		UserID:          currentUserID(c),
		KnowledgeBaseID: uint(kbID),
		ParentID:        req.ParentID,
		Name:            req.Name,
		Description:     req.Description,
		SortOrder:       req.SortOrder,
	})
	if err != nil {
		failErr(c, err)
		return
	}
	ok(c, dir)
}

func (h *KnowledgeHandler) ListDirectoryTree(c *gin.Context) {
	kbID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		fail(c, http.StatusBadRequest, "无效的知识库 ID")
		return
	}
	tree, err := h.svc.ListDirectoryTree(uint(kbID), currentUserID(c))
	if err != nil {
		failErr(c, err)
		return
	}
	ok(c, tree)
}

type updateDirReq struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	ParentID    *uint  `json:"parent_id"`
	SortOrder   *int   `json:"sort_order"`
}

func (h *KnowledgeHandler) UpdateDirectory(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		fail(c, http.StatusBadRequest, "无效的目录 ID")
		return
	}
	var req updateDirReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	dir, err := h.svc.UpdateDirectory(uint(id), currentUserID(c), req.Name, req.Description, req.ParentID, req.SortOrder)
	if err != nil {
		failErr(c, err)
		return
	}
	ok(c, dir)
}

func (h *KnowledgeHandler) DeleteDirectory(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		fail(c, http.StatusBadRequest, "无效的目录 ID")
		return
	}
	if err := h.svc.DeleteDirectory(uint(id), currentUserID(c)); err != nil {
		failErr(c, err)
		return
	}
	ok(c, gin.H{"deleted": true})
}

// ImportDocument POST /api/v1/documents/import
func (h *KnowledgeHandler) ImportDocument(c *gin.Context) {
	form, err := c.MultipartForm()
	if err != nil {
		fail(c, http.StatusBadRequest, "解析上传表单失败: "+err.Error())
		return
	}

	var fileHeaders []*multipart.FileHeader
	if form != nil {
		fileHeaders = append(fileHeaders, form.File["file"]...)
		fileHeaders = append(fileHeaders, form.File["files"]...)
	}
	if len(fileHeaders) == 0 {
		if fh, err := c.FormFile("file"); err == nil {
			fileHeaders = append(fileHeaders, fh)
		} else if fh, err := c.FormFile("files"); err == nil {
			fileHeaders = append(fileHeaders, fh)
		}
	}
	if len(fileHeaders) == 0 {
		fail(c, http.StatusBadRequest, "缺少上传文件 file/files")
		return
	}

	opts := service.ImportOptions{
		UserID: currentUserID(c),
		Title:  c.PostForm("title"),
	}
	if v := c.PostForm("knowledge_base_id"); v != "" {
		id, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			fail(c, http.StatusBadRequest, "无效的 knowledge_base_id")
			return
		}
		opts.KnowledgeBaseID = uint(id)
	}
	if v := c.PostForm("directory_id"); v != "" {
		id, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			fail(c, http.StatusBadRequest, "无效的 directory_id")
			return
		}
		dirID := uint(id)
		opts.DirectoryID = &dirID
	}

	result, err := h.svc.ImportDocuments(c.Request.Context(), opts, fileHeaders)
	if err != nil {
		failErr(c, err)
		return
	}
	ok(c, result)
}

// ListDocuments GET /api/v1/documents
func (h *KnowledgeHandler) ListDocuments(c *gin.Context) {
	filter := repository.DocumentListFilter{UserID: currentUserID(c)}
	if v := c.Query("knowledge_base_id"); v != "" {
		id, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			fail(c, http.StatusBadRequest, "无效的 knowledge_base_id")
			return
		}
		filter.KnowledgeBaseID = uint(id)
	}
	if v := c.Query("directory_id"); v != "" {
		id, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			fail(c, http.StatusBadRequest, "无效的 directory_id")
			return
		}
		dirID := uint(id)
		filter.DirectoryID = &dirID
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	docs, total, err := h.svc.ListDocuments(filter, page, pageSize)
	if err != nil {
		failErr(c, err)
		return
	}
	ok(c, gin.H{
		"list":      docs,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

func (h *KnowledgeHandler) GetDocument(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		fail(c, http.StatusBadRequest, "无效的文档 ID")
		return
	}
	doc, err := h.svc.GetDocument(uint(id), currentUserID(c))
	if err != nil {
		failErr(c, err)
		return
	}
	ok(c, doc)
}

func (h *KnowledgeHandler) DeleteDocument(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		fail(c, http.StatusBadRequest, "无效的文档 ID")
		return
	}
	if err := h.svc.DeleteDocument(c.Request.Context(), uint(id), currentUserID(c)); err != nil {
		failErr(c, err)
		return
	}
	ok(c, gin.H{"deleted": true, "id": uint(id)})
}

type deleteDocsReq struct {
	IDs []uint `json:"ids"`
}

func (h *KnowledgeHandler) DeleteDocuments(c *gin.Context) {
	var req deleteDocsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	result, err := h.svc.DeleteDocuments(c.Request.Context(), req.IDs, currentUserID(c))
	if err != nil {
		failErr(c, err)
		return
	}
	ok(c, result)
}

type reindexReq struct {
	IDs []uint `json:"ids"`
}

func (h *KnowledgeHandler) ReindexDocuments(c *gin.Context) {
	var req reindexReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	result, err := h.svc.ReindexDocuments(req.IDs, currentUserID(c))
	if err != nil {
		failErr(c, err)
		return
	}
	ok(c, result)
}

func (h *KnowledgeHandler) Query(c *gin.Context) {
	var req rag.QueryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	if req.Query == "" {
		fail(c, http.StatusBadRequest, "query is required")
		return
	}
	req.UserID = currentUserID(c)

	flusher, okFlush := c.Writer.(http.Flusher)
	if !okFlush {
		fail(c, http.StatusInternalServerError, "streaming unsupported")
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)
	flusher.Flush()

	writeEvent := func(evt rag.StreamEvent) error {
		select {
		case <-c.Request.Context().Done():
			return c.Request.Context().Err()
		default:
		}
		payload, err := json.Marshal(evt)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(c.Writer, "data: %s\n\n", payload); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}

	if err := h.svc.QueryStream(c.Request.Context(), req, writeEvent); err != nil {
		if c.Request.Context().Err() != nil {
			return
		}
		_ = writeEvent(rag.StreamEvent{
			Type:    rag.StreamEventError,
			Message: err.Error(),
		})
	}
}

func (h *KnowledgeHandler) History(c *gin.Context) {
	sessionID := c.Query("session_id")
	msgs, err := h.svc.GetHistory(sessionID, currentUserID(c))
	if err != nil {
		failErr(c, err)
		return
	}
	ok(c, msgs)
}

func (h *KnowledgeHandler) Health(c *gin.Context) {
	ok(c, gin.H{"status": "up"})
}
