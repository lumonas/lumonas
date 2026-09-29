package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/lumonas/lumonas/internal/model"
	"github.com/lumonas/lumonas/internal/shares"
	"github.com/lumonas/lumonas/internal/store"
)

const (
	maxIndexedTextFile  = 512 << 10
	maxIndexedTextDocs  = 20000
	maxIndexedTextBytes = 64 << 20
)

var indexedTextExtensions = map[string]bool{".txt": true, ".log": true, ".csv": true, ".tsv": true, ".json": true, ".xml": true, ".html": true, ".htm": true, ".yaml": true, ".yml": true, ".toml": true, ".ini": true, ".conf": true, ".md": true, ".markdown": true, ".rst": true}

func (s *apiServer) fileContentIndexStatus(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.identityActor(w, r, false); !ok {
		return
	}
	share, err := s.fileShare(r.URL.Query().Get("shareId"))
	if err != nil {
		writeFileError(w, err)
		return
	}
	count, indexedAt, err := s.store.FileContentIndexStatus(share.ID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"shareId": share.ID, "documents": count, "indexedAt": indexedAt, "maxFileBytes": maxIndexedTextFile, "maxDocuments": maxIndexedTextDocs, "maxIndexBytes": maxIndexedTextBytes})
}

func (s *apiServer) indexFileContents(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	var input struct {
		ShareID string `json:"shareId"`
	}
	if decodeErr := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input); decodeErr != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid JSON"})
		return
	}
	share, err := s.fileShare(input.ShareID)
	if err != nil {
		writeFileError(w, err)
		return
	}
	documents, skipped, err := scanShareText(share)
	if err != nil {
		writeJSON(w, 422, map[string]string{"error": "content indexing failed: " + err.Error()})
		return
	}
	if err := s.store.ReplaceFileContentIndex(share.ID, documents, time.Now()); err != nil {
		writeJSON(w, 500, map[string]string{"error": "could not save content index: " + err.Error()})
		return
	}
	result := map[string]any{"shareId": share.ID, "documents": len(documents), "skipped": skipped, "maxFileBytes": maxIndexedTextFile, "maxDocuments": maxIndexedTextDocs, "maxIndexBytes": maxIndexedTextBytes, "indexedAt": time.Now().UTC()}
	s.recordRequestAudit(r, actor, "file.content-index.rebuild", share.ID, map[string]any{"documents": len(documents), "skipped": skipped})
	s.publishActor(actor, "file.content-index.completed", "info", &model.ResourceRef{Type: "share", ID: share.ID}, map[string]any{"documents": len(documents), "skipped": skipped})
	writeJSON(w, 200, result)
}

func scanShareText(share shares.ManagedShare) ([]store.FileContentDocument, int, error) {
	root, err := filepath.EvalSymlinks(share.Path)
	if err != nil {
		return nil, 0, err
	}
	documents := make([]store.FileContentDocument, 0)
	var total int64
	skipped := 0
	err = filepath.WalkDir(root, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			if errors.Is(walkErr, os.ErrPermission) {
				skipped++
				return nil
			}
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			skipped++
			return nil
		}
		if entry.IsDir() || !entry.Type().IsRegular() || !indexedTextExtensions[strings.ToLower(filepath.Ext(entry.Name()))] {
			return nil
		}
		info, statErr := entry.Info()
		if statErr != nil {
			skipped++
			return nil
		}
		if info.Size() > maxIndexedTextFile {
			skipped++
			return nil
		}
		if len(documents) >= maxIndexedTextDocs || total+info.Size() > maxIndexedTextBytes {
			skipped++
			return nil
		}
		file, openErr := os.Open(current)
		if openErr != nil {
			skipped++
			return nil
		}
		openedInfo, statErr := file.Stat()
		if statErr != nil || !openedInfo.Mode().IsRegular() || !os.SameFile(info, openedInfo) {
			_ = file.Close()
			skipped++
			return nil
		}
		data, readErr := io.ReadAll(io.LimitReader(file, maxIndexedTextFile+1))
		finalInfo, finalStatErr := file.Stat()
		_ = file.Close()
		if readErr != nil || finalStatErr != nil || !os.SameFile(openedInfo, finalInfo) || openedInfo.Size() != finalInfo.Size() || !openedInfo.ModTime().Equal(finalInfo.ModTime()) {
			skipped++
			return nil
		}
		if len(data) > maxIndexedTextFile || strings.IndexByte(string(data), 0) >= 0 || !utf8.Valid(data) {
			skipped++
			return nil
		}
		rel, relErr := filepath.Rel(root, current)
		if relErr != nil {
			return relErr
		}
		documents = append(documents, store.FileContentDocument{ShareID: share.ID, Path: filepath.ToSlash(rel), Name: entry.Name(), Content: string(data), SizeBytes: info.Size(), ModifiedAt: info.ModTime()})
		total += info.Size()
		return nil
	})
	if err != nil {
		return nil, skipped, err
	}
	return documents, skipped, nil
}

func (s *apiServer) searchFileContents(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.identityActor(w, r, false); !ok {
		return
	}
	share, err := s.fileShare(r.URL.Query().Get("share"))
	if err != nil {
		writeFileError(w, err)
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(query) < 3 || len(query) > 128 {
		writeJSON(w, 422, map[string]string{"error": "content search needs 3–128 characters"})
		return
	}
	_, indexedAt, err := s.store.FileContentIndexStatus(share.ID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if indexedAt == nil {
		writeJSON(w, 409, map[string]any{"error": "build a content index for this share before searching", "indexRequired": true, "indexedAt": indexedAt})
		return
	}
	results, err := s.store.SearchFileContent(share.ID, query, 100)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"shareId": share.ID, "query": query, "indexedAt": indexedAt, "results": results})
}
