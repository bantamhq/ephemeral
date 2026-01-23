package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/bantamhq/ephemeral/internal/lfs"
	"github.com/bantamhq/ephemeral/internal/store"
)

const lfsMediaType = "application/vnd.git-lfs+json"

type LFSOptions struct {
	Enabled     bool
	MaxFileSize int64
	BaseURL     string
}

type LFSHandler struct {
	store       store.Store
	storage     lfs.Storage
	permissions *store.PermissionChecker
	baseURL     string
	maxFileSize int64
}

func NewLFSHandler(st store.Store, storage lfs.Storage, baseURL string, maxFileSize int64) *LFSHandler {
	return &LFSHandler{
		store:       st,
		storage:     storage,
		permissions: store.NewPermissionChecker(st),
		baseURL:     baseURL,
		maxFileSize: maxFileSize,
	}
}

func (h *LFSHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/objects/batch", h.handleBatch)
	r.Get("/objects/{oid}", h.handleDownload)
	r.Put("/objects/{oid}", h.handleUpload)
	r.Post("/verify", h.handleVerify)
	return r
}

func (h *LFSHandler) handleBatch(w http.ResponseWriter, r *http.Request) {
	ns, repo, token := h.resolveRepo(w, r)
	if repo == nil {
		return
	}

	var req lfs.BatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.lfsError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.Operation != "download" && req.Operation != "upload" {
		h.lfsError(w, http.StatusBadRequest, "Invalid operation")
		return
	}

	isWrite := req.Operation == "upload"
	if !h.checkPermission(w, token, repo, isWrite) {
		return
	}

	resp := lfs.BatchResponse{
		Transfer: "basic",
		Objects:  make([]lfs.ObjectResponse, 0, len(req.Objects)),
	}

	for _, obj := range req.Objects {
		objResp := h.processObject(r, ns, repo, obj, req.Operation, token)
		resp.Objects = append(resp.Objects, objResp)
	}

	h.lfsJSON(w, http.StatusOK, resp)
}

func (h *LFSHandler) processObject(r *http.Request, ns *store.Namespace, repo *store.Repo, obj lfs.ObjectSpec, operation string, token *store.Token) lfs.ObjectResponse {
	if err := lfs.ValidateOID(obj.OID); err != nil {
		return objectError(obj, 422, "Invalid OID format")
	}

	if h.maxFileSize > 0 && obj.Size > h.maxFileSize {
		return objectError(obj, 413, fmt.Sprintf("Object exceeds maximum size of %d bytes", h.maxFileSize))
	}

	exists, err := h.storage.Exists(r.Context(), repo.ID, obj.OID)
	if err != nil {
		return objectError(obj, 500, "Failed to check object existence")
	}

	baseObjURL := fmt.Sprintf("%s/git/%s/%s.git/info/lfs/objects/%s", h.baseURL, ns.Name, repo.Name, obj.OID)
	authHeader := h.buildAuthHeader(token)

	if operation == "download" {
		return h.downloadResponse(obj, exists, baseObjURL, authHeader)
	}

	return h.uploadResponse(obj, exists, baseObjURL, authHeader, ns.Name, repo.Name)
}

func objectError(obj lfs.ObjectSpec, code int, message string) lfs.ObjectResponse {
	return lfs.ObjectResponse{
		OID:   obj.OID,
		Size:  obj.Size,
		Error: &lfs.ObjectError{Code: code, Message: message},
	}
}

func (h *LFSHandler) downloadResponse(obj lfs.ObjectSpec, exists bool, url string, header map[string]string) lfs.ObjectResponse {
	if !exists {
		return objectError(obj, 404, "Object not found")
	}

	return lfs.ObjectResponse{
		OID:  obj.OID,
		Size: obj.Size,
		Actions: map[string]lfs.Action{
			"download": {Href: url, Header: header, ExpiresIn: 3600},
		},
	}
}

func (h *LFSHandler) uploadResponse(obj lfs.ObjectSpec, exists bool, url string, header map[string]string, nsName, repoName string) lfs.ObjectResponse {
	resp := lfs.ObjectResponse{
		OID:     obj.OID,
		Size:    obj.Size,
		Actions: make(map[string]lfs.Action),
	}

	if exists {
		return resp
	}

	resp.Actions["upload"] = lfs.Action{Href: url, Header: header, ExpiresIn: 3600}
	resp.Actions["verify"] = lfs.Action{
		Href:      fmt.Sprintf("%s/git/%s/%s.git/info/lfs/verify", h.baseURL, nsName, repoName),
		Header:    header,
		ExpiresIn: 3600,
	}

	return resp
}

func (h *LFSHandler) handleDownload(w http.ResponseWriter, r *http.Request) {
	_, repo, token := h.resolveRepo(w, r)
	if repo == nil {
		return
	}

	if !h.checkPermission(w, token, repo, false) {
		return
	}

	oid := chi.URLParam(r, "oid")
	if err := lfs.ValidateOID(oid); err != nil {
		h.lfsError(w, http.StatusUnprocessableEntity, "Invalid OID format")
		return
	}

	reader, size, err := h.storage.Get(r.Context(), repo.ID, oid)
	if errors.Is(err, lfs.ErrObjectNotFound) {
		h.lfsError(w, http.StatusNotFound, "Object not found")
		return
	}
	if err != nil {
		h.lfsError(w, http.StatusInternalServerError, "Failed to retrieve object")
		return
	}
	defer reader.Close()

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", size))
	w.WriteHeader(http.StatusOK)
	io.Copy(w, reader)
}

func (h *LFSHandler) handleUpload(w http.ResponseWriter, r *http.Request) {
	_, repo, token := h.resolveRepo(w, r)
	if repo == nil {
		return
	}

	if !h.checkPermission(w, token, repo, true) {
		return
	}

	oid := chi.URLParam(r, "oid")
	if err := lfs.ValidateOID(oid); err != nil {
		h.lfsError(w, http.StatusUnprocessableEntity, "Invalid OID format")
		return
	}

	size := r.ContentLength
	if size < 0 {
		h.lfsError(w, http.StatusBadRequest, "Content-Length required")
		return
	}

	if h.maxFileSize > 0 && size > h.maxFileSize {
		h.lfsError(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("Object exceeds maximum size of %d bytes", h.maxFileSize))
		return
	}

	err := h.storage.Put(r.Context(), repo.ID, oid, r.Body, size)
	if errors.Is(err, lfs.ErrHashMismatch) {
		h.lfsError(w, http.StatusBadRequest, "Content hash does not match OID")
		return
	}
	if err != nil {
		h.lfsError(w, http.StatusInternalServerError, "Failed to store object")
		return
	}

	lfsObj := &store.LFSObject{
		RepoID:    repo.ID,
		OID:       oid,
		Size:      size,
		CreatedAt: time.Now(),
	}
	if err := h.store.CreateLFSObject(lfsObj); err != nil {
		h.storage.Delete(r.Context(), repo.ID, oid)
		h.lfsError(w, http.StatusInternalServerError, "Failed to record object")
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (h *LFSHandler) handleVerify(w http.ResponseWriter, r *http.Request) {
	_, repo, token := h.resolveRepo(w, r)
	if repo == nil {
		return
	}

	if !h.checkPermission(w, token, repo, true) {
		return
	}

	var req lfs.VerifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.lfsError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if err := lfs.ValidateOID(req.OID); err != nil {
		h.lfsError(w, http.StatusUnprocessableEntity, "Invalid OID format")
		return
	}

	size, err := h.storage.Size(r.Context(), repo.ID, req.OID)
	if errors.Is(err, lfs.ErrObjectNotFound) {
		h.lfsError(w, http.StatusNotFound, "Object not found")
		return
	}
	if err != nil {
		h.lfsError(w, http.StatusInternalServerError, "Failed to verify object")
		return
	}

	if size != req.Size {
		h.lfsError(w, http.StatusBadRequest, fmt.Sprintf("Size mismatch: expected %d, got %d", req.Size, size))
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (h *LFSHandler) resolveRepo(w http.ResponseWriter, r *http.Request) (*store.Namespace, *store.Repo, *store.Token) {
	namespaceName := chi.URLParam(r, "namespace")
	repoName := chi.URLParam(r, "repo")

	ns, err := h.store.GetNamespaceByName(namespaceName)
	if err != nil {
		h.lfsError(w, http.StatusInternalServerError, "Internal server error")
		return nil, nil, nil
	}
	if ns == nil {
		h.lfsError(w, http.StatusNotFound, "Namespace not found")
		return nil, nil, nil
	}

	repo, err := h.store.GetRepo(ns.ID, repoName)
	if err != nil {
		h.lfsError(w, http.StatusInternalServerError, "Internal server error")
		return nil, nil, nil
	}
	if repo == nil {
		h.lfsError(w, http.StatusNotFound, "Repository not found")
		return nil, nil, nil
	}

	token := GetTokenFromContext(r.Context())
	return ns, repo, token
}

func (h *LFSHandler) checkPermission(w http.ResponseWriter, token *store.Token, repo *store.Repo, isWrite bool) bool {
	if !isWrite {
		return h.checkReadPermission(w, token, repo)
	}

	if token == nil {
		h.lfsErrorWithAuth(w, http.StatusUnauthorized, "Authentication required")
		return false
	}

	if token.IsAdmin {
		h.lfsError(w, http.StatusForbidden, "Admin token cannot be used for LFS operations")
		return false
	}

	hasWrite, err := h.permissions.CheckRepoPermission(token.ID, repo, store.PermRepoWrite)
	if err != nil {
		h.lfsError(w, http.StatusInternalServerError, "Failed to check permissions")
		return false
	}

	if !hasWrite {
		h.lfsError(w, http.StatusForbidden, "Write access denied")
		return false
	}

	return true
}

func (h *LFSHandler) checkReadPermission(w http.ResponseWriter, token *store.Token, repo *store.Repo) bool {
	if repo.Public {
		return true
	}

	if token == nil {
		h.lfsErrorWithAuth(w, http.StatusUnauthorized, "Authentication required")
		return false
	}

	hasRead, err := h.permissions.CheckRepoPermission(token.ID, repo, store.PermRepoRead)
	if err != nil {
		h.lfsError(w, http.StatusInternalServerError, "Failed to check permissions")
		return false
	}

	if !hasRead {
		h.lfsError(w, http.StatusForbidden, "Access denied")
		return false
	}

	return true
}

func (h *LFSHandler) buildAuthHeader(token *store.Token) map[string]string {
	if token == nil {
		return nil
	}
	return map[string]string{}
}

func (h *LFSHandler) lfsJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", lfsMediaType)
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func (h *LFSHandler) lfsError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", lfsMediaType)
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(lfs.LFSError{Message: message})
}

func (h *LFSHandler) lfsErrorWithAuth(w http.ResponseWriter, status int, message string) {
	w.Header().Set("WWW-Authenticate", `Basic realm="Git LFS"`)
	h.lfsError(w, status, message)
}
