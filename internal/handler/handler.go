package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kelvins-io/eino-repository-rag/internal/asr"
	"github.com/kelvins-io/eino-repository-rag/internal/auth"
	"github.com/kelvins-io/eino-repository-rag/internal/config"
	"github.com/kelvins-io/eino-repository-rag/internal/logger"
	"github.com/kelvins-io/eino-repository-rag/internal/model"
	"github.com/kelvins-io/eino-repository-rag/internal/rag"
	"github.com/kelvins-io/eino-repository-rag/internal/repository"
	"github.com/kelvins-io/eino-repository-rag/internal/service"
	"github.com/kelvins-io/eino-repository-rag/internal/tts"
)

type KnowledgeHandler struct {
	svc                 *service.KnowledgeService
	maxUploadFileSize   int64
	maxUploadFiles      int
	maxUploadFileSizeMB int
	transcriber         asr.Transcriber
	maxAudioBytes       int64
	speaker             tts.Speaker
}

func NewKnowledgeHandler(svc *service.KnowledgeService, uploadCfg config.RAGConfig) *KnowledgeHandler {
	return &KnowledgeHandler{
		svc:                 svc,
		maxUploadFileSize:   uploadCfg.MaxUploadFileSizeBytes(),
		maxUploadFiles:      uploadCfg.MaxUploadFiles,
		maxUploadFileSizeMB: uploadCfg.MaxUploadFileSizeMB,
	}
}

func (h *KnowledgeHandler) WithASR(t asr.Transcriber, maxAudioBytes int64) *KnowledgeHandler {
	if h == nil {
		return h
	}
	h.transcriber = t
	h.maxAudioBytes = maxAudioBytes
	return h
}

func (h *KnowledgeHandler) WithTTS(s tts.Speaker) *KnowledgeHandler {
	if h == nil {
		return h
	}
	h.speaker = s
	return h
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
	writeFail(c, httpCode, msg, errors.New(msg))
}

func failErr(c *gin.Context, err error) {
	if err == nil {
		return
	}
	code, msg := classifyHandlerError(err)
	writeFail(c, code, msg, err)
}

func writeFail(c *gin.Context, httpCode int, msg string, err error) {
	if err != nil {
		_ = c.Error(err)
	}
	logHandlerError(c, httpCode, msg, err)
	c.JSON(httpCode, APIResponse{Code: httpCode, Message: msg})
}

func logHandlerError(c *gin.Context, httpCode int, msg string, err error) {
	if c == nil || c.Request == nil {
		return
	}
	fields := []zap.Field{
		zap.Int("status", httpCode),
		zap.String("method", c.Request.Method),
		zap.String("path", c.FullPath()),
		zap.String("ip", c.ClientIP()),
		zap.String("user_id", currentUserID(c)),
		zap.Uint("tenant_id", currentTenantID(c)),
		zap.String("message", msg),
	}
	if err != nil {
		fields = append(fields, zap.Error(err))
	}
	switch {
	case httpCode >= http.StatusInternalServerError:
		logger.L().Error("http handler failed", fields...)
	case httpCode >= http.StatusBadRequest:
		logger.L().Warn("http handler rejected", fields...)
	}
}

func classifyHandlerError(err error) (int, string) {
	msg := err.Error()
	switch {
	case errors.Is(err, service.ErrForbidden):
		return http.StatusForbidden, msg
	case strings.Contains(msg, "不存在"):
		return http.StatusNotFound, msg
	case strings.Contains(msg, "正在索引"):
		return http.StatusConflict, msg
	case isClientErrorMsg(msg):
		return http.StatusBadRequest, msg
	default:
		return http.StatusInternalServerError, msg
	}
}

func isClientErrorMsg(msg string) bool {
	keys := []string{
		"required", "无效", "不能", "缺少", "不属于", "仍有", "请先",
		"过长", "至少", "请求参数", "已达到",
	}
	for _, k := range keys {
		if strings.Contains(msg, k) {
			return true
		}
	}
	return false
}

func currentUserID(c *gin.Context) string {
	return auth.UserIDFromContext(c)
}

func currentTenantID(c *gin.Context) uint {
	return auth.TenantIDFromContext(c)
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
	kb, err := h.svc.CreateKnowledgeBase(currentUserID(c), currentTenantID(c), req.Name, req.Description)
	if err != nil {
		failErr(c, err)
		return
	}
	ok(c, kb)
}

func (h *KnowledgeHandler) ListKnowledgeBases(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	list, total, err := h.svc.ListKnowledgeBases(currentUserID(c), currentTenantID(c), page, pageSize)
	if err != nil {
		failErr(c, err)
		return
	}
	ok(c, gin.H{
		"list":      list,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

func (h *KnowledgeHandler) GetKnowledgeBase(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		fail(c, http.StatusBadRequest, "无效的知识库 ID")
		return
	}
	kb, err := h.svc.GetKnowledgeBase(uint(id), currentUserID(c), currentTenantID(c))
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
	kb, err := h.svc.UpdateKnowledgeBase(uint(id), currentUserID(c), currentTenantID(c), req.Name, req.Description)
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
	if err := h.svc.DeleteKnowledgeBase(uint(id), currentUserID(c), currentTenantID(c)); err != nil {
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
		TenantID:        currentTenantID(c),
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
	tree, err := h.svc.ListDirectoryTree(uint(kbID), currentUserID(c), currentTenantID(c))
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
	dir, err := h.svc.UpdateDirectory(uint(id), currentUserID(c), currentTenantID(c), req.Name, req.Description, req.ParentID, req.SortOrder)
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
	if err := h.svc.DeleteDirectory(uint(id), currentUserID(c), currentTenantID(c)); err != nil {
		failErr(c, err)
		return
	}
	ok(c, gin.H{"deleted": true})
}

// ImportDocument POST /api/v1/documents/import
func (h *KnowledgeHandler) ImportDocument(c *gin.Context) {
	maxBody := h.maxUploadFileSize*int64(h.maxUploadFiles) + 1024*1024
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBody)

	form, err := c.MultipartForm()
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) || strings.Contains(strings.ToLower(err.Error()), "request body too large") {
			fail(c, http.StatusRequestEntityTooLarge, fmt.Sprintf(
				"上传总大小超过限制：单文件 ≤ %dMB，单次最多 %d 个文件",
				h.maxUploadFileSizeMB, h.maxUploadFiles,
			))
			return
		}
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
	if len(fileHeaders) > h.maxUploadFiles {
		fail(c, http.StatusBadRequest, fmt.Sprintf("单次最多上传 %d 个文件，当前 %d 个", h.maxUploadFiles, len(fileHeaders)))
		return
	}
	policy, used, err := h.svc.UploadPolicy(currentTenantID(c))
	if err != nil {
		failErr(c, err)
		return
	}
	sizeMB := policy.UploadMaxFileSizeMB()
	sizeBytes := policy.UploadMaxFileSizeBytes()
	if h.maxUploadFileSize > 0 && h.maxUploadFileSize < sizeBytes {
		sizeBytes = h.maxUploadFileSize
		sizeMB = h.maxUploadFileSizeMB
	}
	if used+int64(len(fileHeaders)) > int64(policy.UploadMaxFiles()) {
		fail(c, http.StatusBadRequest, fmt.Sprintf("已达到或将超过租户文件总数上限 %d（已有 %d）", policy.UploadMaxFiles(), used))
		return
	}
	for _, fh := range fileHeaders {
		if fh.Size > sizeBytes {
			fail(c, http.StatusBadRequest, fmt.Sprintf(
				"文件 %q 大小 %.1fMB 超过限制 %dMB",
				fh.Filename, float64(fh.Size)/(1024*1024), sizeMB,
			))
			return
		}
	}

	opts := service.ImportOptions{
		UserID:   currentUserID(c),
		TenantID: currentTenantID(c),
		Title:    c.PostForm("title"),
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

// UploadLimits GET /api/v1/system/upload-limits
func (h *KnowledgeHandler) UploadLimits(c *gin.Context) {
	sizeMB := h.maxUploadFileSizeMB
	maxFiles := 0
	used := int64(0)
	if h.svc != nil {
		policy, n, err := h.svc.UploadPolicy(currentTenantID(c))
		if err != nil {
			failErr(c, err)
			return
		}
		sizeMB = policy.UploadMaxFileSizeMB()
		if h.maxUploadFileSizeMB > 0 && h.maxUploadFileSizeMB < sizeMB {
			sizeMB = h.maxUploadFileSizeMB
		}
		maxFiles = policy.UploadMaxFiles()
		used = n
	}
	ok(c, gin.H{
		"max_upload_file_size_mb": sizeMB,
		"max_upload_file_size":    int64(sizeMB) * 1024 * 1024,
		"max_upload_files":        h.maxUploadFiles,
		"max_tenant_files":        maxFiles,
		"tenant_file_count":       used,
	})
}

// ListDocuments GET /api/v1/documents
func (h *KnowledgeHandler) ListDocuments(c *gin.Context) {
	filter := repository.DocumentListFilter{
		UserID:   currentUserID(c),
		TenantID: currentTenantID(c),
	}
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
	if v := c.Query("status"); v != "" {
		switch model.DocumentStatus(v) {
		case model.DocumentStatusPending, model.DocumentStatusIndexing, model.DocumentStatusReady, model.DocumentStatusFailed:
			filter.Status = v
		default:
			fail(c, http.StatusBadRequest, "无效的 status，可选 pending/indexing/ready/failed")
			return
		}
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
	doc, err := h.svc.GetDocument(uint(id), currentUserID(c), currentTenantID(c))
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
	if err := h.svc.DeleteDocument(c.Request.Context(), uint(id), currentUserID(c), currentTenantID(c)); err != nil {
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
	result, err := h.svc.DeleteDocuments(c.Request.Context(), req.IDs, currentUserID(c), currentTenantID(c))
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
	result, err := h.svc.ReindexDocuments(req.IDs, currentUserID(c), currentTenantID(c))
	if err != nil {
		failErr(c, err)
		return
	}
	ok(c, result)
}

func (h *KnowledgeHandler) Query(c *gin.Context) {
	h.streamChat(c, h.svc.QueryStream)
}

// TranscribeSpeech POST /api/v1/chat/transcribe — 上传录音，返回识别文本
func (h *KnowledgeHandler) TranscribeSpeech(c *gin.Context) {
	if h.transcriber == nil {
		fail(c, http.StatusServiceUnavailable, "语音识别未启用")
		return
	}
	maxBytes := h.maxAudioBytes
	if maxBytes <= 0 {
		maxBytes = 8 << 20
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes+1024*1024)

	fh, err := c.FormFile("file")
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) || strings.Contains(strings.ToLower(err.Error()), "request body too large") {
			fail(c, http.StatusRequestEntityTooLarge, fmt.Sprintf("录音超过大小限制 %dMB", maxBytes/(1024*1024)))
			return
		}
		fail(c, http.StatusBadRequest, "缺少录音文件 file")
		return
	}
	if fh.Size > 0 && fh.Size > maxBytes {
		fail(c, http.StatusBadRequest, fmt.Sprintf("录音大小 %.1fMB 超过限制 %dMB", float64(fh.Size)/(1024*1024), maxBytes/(1024*1024)))
		return
	}
	if !isAllowedAudio(fh.Filename, fh.Header.Get("Content-Type")) {
		fail(c, http.StatusBadRequest, "仅支持音频文件（webm / mp3 / wav / m4a / ogg 等）")
		return
	}

	f, err := fh.Open()
	if err != nil {
		fail(c, http.StatusBadRequest, "读取录音失败")
		return
	}
	defer f.Close()

	limited := io.LimitReader(f, maxBytes+1)
	raw, err := io.ReadAll(limited)
	if err != nil {
		fail(c, http.StatusBadRequest, "读取录音失败")
		return
	}
	if int64(len(raw)) > maxBytes {
		fail(c, http.StatusRequestEntityTooLarge, fmt.Sprintf("录音超过大小限制 %dMB", maxBytes/(1024*1024)))
		return
	}
	if err := h.svc.EnsureVoiceInputAllowed(currentTenantID(c)); err != nil {
		failErr(c, err)
		return
	}

	text, err := h.transcriber.Transcribe(c.Request.Context(), asr.Request{
		Filename:    fh.Filename,
		ContentType: fh.Header.Get("Content-Type"),
		Body:        bytes.NewReader(raw),
		Language:    c.PostForm("language"),
		Prompt:      c.PostForm("prompt"),
	})
	if err != nil {
		if errors.Is(err, asr.ErrNotConfigured) {
			writeFail(c, http.StatusServiceUnavailable, err.Error(), err)
			return
		}
		if errors.Is(err, asr.ErrEmptyAudio) {
			writeFail(c, http.StatusBadRequest, "未采集到音频，请重试", err)
			return
		}
		var ue *asr.UpstreamError
		if errors.As(err, &ue) {
			writeFail(c, http.StatusBadGateway, "语音识别服务异常，请稍后重试", err)
			return
		}
		writeFail(c, http.StatusBadGateway, "语音识别失败，请稍后重试", err)
		return
	}
	if err := h.svc.RecordVoiceInput(currentTenantID(c)); err != nil {
		logger.L().Error("record voice input usage failed", zap.Error(err), zap.Uint("tenant_id", currentTenantID(c)))
	}
	ok(c, gin.H{"text": text})
}

func isAllowedAudio(filename, contentType string) bool {
	ct := strings.ToLower(strings.TrimSpace(contentType))
	if strings.HasPrefix(ct, "audio/") || strings.Contains(ct, "webm") || strings.Contains(ct, "ogg") {
		return true
	}
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".webm", ".ogg", ".oga", ".mp3", ".wav", ".m4a", ".mp4", ".mpeg", ".mpga", ".flac", ".aac":
		return true
	}
	return false
}

// SynthesizeSpeech POST /api/v1/chat/speech — 将回答文本合成为语音
func (h *KnowledgeHandler) SynthesizeSpeech(c *gin.Context) {
	if h.speaker == nil {
		fail(c, http.StatusServiceUnavailable, "语音合成未启用")
		return
	}
	var req struct {
		Text string `json:"text"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	if err := h.svc.EnsureTTSAllowed(currentTenantID(c)); err != nil {
		failErr(c, err)
		return
	}
	result, err := h.speaker.Synthesize(c.Request.Context(), req.Text)
	if err != nil {
		if errors.Is(err, tts.ErrNotConfigured) {
			writeFail(c, http.StatusServiceUnavailable, err.Error(), err)
			return
		}
		if errors.Is(err, tts.ErrEmptyText) {
			writeFail(c, http.StatusBadRequest, err.Error(), err)
			return
		}
		var ue *tts.UpstreamError
		if errors.As(err, &ue) {
			writeFail(c, http.StatusBadGateway, "语音合成服务异常，请稍后重试", err)
			return
		}
		writeFail(c, http.StatusBadGateway, "语音合成失败，请稍后重试", err)
		return
	}
	if err := h.svc.RecordTTS(currentTenantID(c)); err != nil {
		logger.L().Error("record tts usage failed", zap.Error(err), zap.Uint("tenant_id", currentTenantID(c)))
	}
	ct := result.ContentType
	if ct == "" {
		ct = "audio/mpeg"
	}
	c.Data(http.StatusOK, ct, result.Audio)
}

// AgentQuery POST /api/v1/chat/agent — ReAct 多步检索 SSE
func (h *KnowledgeHandler) AgentQuery(c *gin.Context) {
	h.streamChat(c, h.svc.AgentQueryStream)
}

func (h *KnowledgeHandler) streamChat(c *gin.Context, run func(context.Context, rag.QueryRequest, rag.StreamHandler) error) {
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
	req.TenantID = currentTenantID(c)

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

	if err := run(c.Request.Context(), req, writeEvent); err != nil {
		if c.Request.Context().Err() != nil {
			logger.L().Info("chat stream canceled",
				zap.Error(c.Request.Context().Err()),
				zap.String("path", c.FullPath()),
				zap.String("user_id", currentUserID(c)),
				zap.Uint("tenant_id", currentTenantID(c)),
			)
			return
		}
		logger.L().Error("chat stream failed",
			zap.Error(err),
			zap.String("path", c.FullPath()),
			zap.String("session_id", req.SessionID),
			zap.String("user_id", currentUserID(c)),
			zap.Uint("tenant_id", currentTenantID(c)),
			zap.Uint("knowledge_base_id", req.KnowledgeBaseID),
		)
		_ = writeEvent(rag.StreamEvent{
			Type:    rag.StreamEventError,
			Message: err.Error(),
		})
	}
}

func (h *KnowledgeHandler) History(c *gin.Context) {
	sessionID := c.Query("session_id")
	msgs, err := h.svc.GetHistory(sessionID, currentUserID(c), currentTenantID(c))
	if err != nil {
		failErr(c, err)
		return
	}
	ok(c, msgs)
}

// ListSessions GET /api/v1/chat/sessions?knowledge_base_id=&directory_id=
func (h *KnowledgeHandler) ListSessions(c *gin.Context) {
	kbID, err := strconv.ParseUint(c.Query("knowledge_base_id"), 10, 64)
	if err != nil || kbID == 0 {
		fail(c, http.StatusBadRequest, "无效的 knowledge_base_id")
		return
	}
	var dirID *uint
	if v := c.Query("directory_id"); v != "" {
		id, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			fail(c, http.StatusBadRequest, "无效的 directory_id")
			return
		}
		d := uint(id)
		dirID = &d
	}
	list, err := h.svc.ListSessions(currentUserID(c), currentTenantID(c), uint(kbID), dirID)
	if err != nil {
		failErr(c, err)
		return
	}
	ok(c, list)
}

func (h *KnowledgeHandler) Health(c *gin.Context) {
	ok(c, gin.H{"status": "up"})
}
