package main

import (
	"net/http"
	"path"
	"strings"
)

func (s *apiServer) apiFileRoutes(w http.ResponseWriter, r *http.Request, endpoint string) bool {
	switch {
	case r.Method == http.MethodGet && endpoint == "/file-requests":
		s.listFileRequests(w, r)
	case r.Method == http.MethodPost && endpoint == "/file-requests":
		s.createFileRequest(w, r)
	case r.Method == http.MethodDelete && strings.HasPrefix(endpoint, "/file-requests/"):
		s.revokeFileRequest(w, r, path.Base(endpoint))
	case r.Method == http.MethodGet && endpoint == "/file-share-links":
		s.listFileShareLinks(w, r)
	case r.Method == http.MethodPost && endpoint == "/file-share-links":
		s.createFileShareLink(w, r)
	case r.Method == http.MethodDelete && strings.HasPrefix(endpoint, "/file-share-links/"):
		s.revokeFileShareLink(w, r, path.Base(endpoint))
	case r.Method == http.MethodGet && strings.HasPrefix(endpoint, "/public/file-requests/"):
		s.publicFileRequest(w, r, path.Base(endpoint))
	case r.Method == http.MethodPost && strings.HasPrefix(endpoint, "/public/file-requests/") && strings.HasSuffix(endpoint, "/upload"):
		s.uploadPublicFileRequest(w, r, path.Base(path.Dir(endpoint)))
	case r.Method == http.MethodGet && strings.HasPrefix(endpoint, "/public/file-share-links/") && strings.HasSuffix(endpoint, "/files"):
		s.publicFileShareEntries(w, r, path.Base(path.Dir(endpoint)))
	case r.Method == http.MethodGet && strings.HasPrefix(endpoint, "/public/file-share-links/") && strings.HasSuffix(endpoint, "/download"):
		s.publicFileShareDownload(w, r, path.Base(path.Dir(endpoint)))
	case r.Method == http.MethodGet && strings.HasPrefix(endpoint, "/public/file-share-links/"):
		s.publicFileShareLink(w, r, path.Base(endpoint))
	case r.Method == http.MethodGet && endpoint == "/files":
		s.listFiles(w, r)
	case r.Method == http.MethodGet && endpoint == "/files/search":
		s.searchFiles(w, r)
	case r.Method == http.MethodGet && endpoint == "/files/content-search":
		s.searchFileContents(w, r)
	case r.Method == http.MethodGet && endpoint == "/files/integrity":
		s.fileIntegrityStatus(w, r)
	case r.Method == http.MethodPost && endpoint == "/files/integrity/baseline":
		s.startFileIntegrity(w, r, true)
	case r.Method == http.MethodPost && endpoint == "/files/integrity/verify":
		s.startFileIntegrity(w, r, false)
	case r.Method == http.MethodGet && endpoint == "/files/search-index":
		s.fileContentIndexStatus(w, r)
	case r.Method == http.MethodPost && endpoint == "/files/search-index":
		s.indexFileContents(w, r)
	case r.Method == http.MethodGet && endpoint == "/files/properties":
		s.fileProperties(w, r)
	case r.Method == http.MethodGet && endpoint == "/files/download":
		s.downloadFile(w, r)
	case r.Method == http.MethodGet && endpoint == "/files/download/archive":
		s.downloadFileArchive(w, r)
	case r.Method == http.MethodPost && endpoint == "/files/mkdir":
		s.makeDirectory(w, r)
	case r.Method == http.MethodPost && endpoint == "/files/rename":
		s.renameFile(w, r)
	case r.Method == http.MethodPost && endpoint == "/files/delete":
		s.deleteFiles(w, r)
	case r.Method == http.MethodPost && endpoint == "/files/transfer":
		s.transferFiles(w, r)
	case r.Method == http.MethodPost && endpoint == "/files/upload":
		s.uploadFile(w, r)
	case r.Method == http.MethodPost && endpoint == "/files/uploads":
		s.createResumableUpload(w, r)
	case strings.HasPrefix(endpoint, "/files/uploads/") && r.Method == http.MethodGet:
		s.getResumableUpload(w, r, path.Base(endpoint))
	case strings.HasPrefix(endpoint, "/files/uploads/") && r.Method == http.MethodPut && !strings.HasSuffix(endpoint, "/complete"):
		s.writeResumableUploadChunk(w, r, path.Base(endpoint))
	case strings.HasPrefix(endpoint, "/files/uploads/") && r.Method == http.MethodPost && strings.HasSuffix(endpoint, "/complete"):
		s.completeResumableUpload(w, r, path.Base(path.Dir(endpoint)))
	case strings.HasPrefix(endpoint, "/files/uploads/") && r.Method == http.MethodDelete:
		s.cancelResumableUpload(w, r, path.Base(endpoint))
	case r.Method == http.MethodGet && endpoint == "/files/recycle":
		s.listRecycleBin(w, r)
	case r.Method == http.MethodPost && endpoint == "/files/recycle/restore":
		s.restoreRecycleBin(w, r)
	case r.Method == http.MethodPost && endpoint == "/files/recycle/purge":
		s.purgeRecycleBin(w, r)
	default:
		return false
	}
	return true
}
