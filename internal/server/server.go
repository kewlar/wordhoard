package server

import (
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"path"
	"strconv"
	"strings"

	"ebook-vocab/internal/db"
)

type Server struct {
	store *db.Store
	spaFS fs.FS
	mux   *http.ServeMux
}

func NewHandler(store *db.Store, spaFS fs.FS) http.Handler {
	s := &Server{
		store: store,
		spaFS: spaFS,
		mux:   http.NewServeMux(),
	}
	s.mux.HandleFunc("GET /api/books", s.handleListBooks)
	s.mux.HandleFunc("GET /api/books/{id}", s.handleGetBook)
	s.mux.HandleFunc("GET /api/books/{id}/vocabulary", s.handleGetBookVocabulary)
	s.mux.HandleFunc("/", s.handleSPA)
	return s.mux
}

func (s *Server) handleListBooks(w http.ResponseWriter, r *http.Request) {
	books, err := s.store.ListBooks()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"books": books,
	})
}

func (s *Server) handleGetBook(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeJSONError(w, http.StatusBadRequest, "invalid book id")
		return
	}
	book, err := s.store.GetBook(id)
	if errors.Is(err, db.ErrBookNotFound) {
		writeJSONError(w, http.StatusNotFound, "book not found")
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"book": book,
	})
}

func (s *Server) handleGetBookVocabulary(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeJSONError(w, http.StatusBadRequest, "invalid book id")
		return
	}

	q := r.URL.Query()
	page := 1
	if pStr := q.Get("page"); pStr != "" {
		if parsed, parseErr := strconv.Atoi(pStr); parseErr == nil && parsed > 0 {
			page = parsed
		}
	}
	pageSize := 50
	if psStr := q.Get("page_size"); psStr != "" {
		if parsed, parseErr := strconv.Atoi(psStr); parseErr == nil && parsed > 0 {
			pageSize = parsed
		}
	}

	vocabPage, err := s.store.ListBookVocabulary(id, db.VocabularyFilter{
		Page:     page,
		PageSize: pageSize,
		POS:      q.Get("pos"),
		Search:   q.Get("search"),
	})
	if errors.Is(err, db.ErrBookNotFound) {
		writeJSONError(w, http.StatusNotFound, "book not found")
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, vocabPage)
}


func (s *Server) handleSPA(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		http.NotFound(w, r)
		return
	}
	if s.spaFS == nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, "<!doctype html><html><body><div id=\"root\">Ebook Vocabulary</div></body></html>")
		return
	}

	cleanPath := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if cleanPath == "" || cleanPath == "." {
		s.serveIndexHTML(w, r)
		return
	}

	f, err := s.spaFS.Open(cleanPath)
	if err != nil {
		// SPA client-side route fallback
		s.serveIndexHTML(w, r)
		return
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil || info.IsDir() {
		s.serveIndexHTML(w, r)
		return
	}

	http.FileServer(http.FS(s.spaFS)).ServeHTTP(w, r)
}

func (s *Server) serveIndexHTML(w http.ResponseWriter, _ *http.Request) {
	data, err := fs.ReadFile(s.spaFS, "index.html")
	if err != nil {
		http.Error(w, "index.html not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
