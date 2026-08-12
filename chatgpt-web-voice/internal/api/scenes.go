package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/dyhhhhhh/chatgpt-web-voice/internal/logging"
	"github.com/dyhhhhhh/chatgpt-web-voice/internal/scenes"
)

type patchSceneRequest struct {
	ApprovedSummary     string `json:"approved_summary"`
	SelectedCandidateID string `json:"selected_candidate_id"`
}

// createSceneDraft reads the owned conversation and its persisted messages
// from SQLite, then asks the orchestrator for the editable summary and exactly
// three candidate moments. The client never uploads a transcript.
func (s *Server) createSceneDraft(w http.ResponseWriter, r *http.Request) {
	if s.scenes == nil {
		writeSceneUnconfigured(w)
		return
	}
	id, err := conversationPathID(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"detail": map[string]any{"error": err.Error()}})
		return
	}
	conversation, err := s.conversations.Get(requestOwner(r), id)
	if err != nil {
		writeConversationError(w, r, err)
		return
	}
	project, err := s.scenes.CreateDraft(r.Context(), requestOwner(r), conversation)
	if err != nil {
		writeSceneError(w, r, err)
		return
	}
	logging.FromContext(r.Context()).Info("scene_draft_created",
		"scene_id", project.ID,
		"conversation_id", project.ConversationID,
		"status", project.Status,
	)
	writeJSON(w, http.StatusCreated, map[string]any{"scene": project})
}

// listConversationScenes returns all scene records of one owned conversation
// (newest first) so the page can restore completed results or resume polling.
func (s *Server) listConversationScenes(w http.ResponseWriter, r *http.Request) {
	if s.scenes == nil {
		writeSceneUnconfigured(w)
		return
	}
	id, err := conversationPathID(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"detail": map[string]any{"error": err.Error()}})
		return
	}
	owner := requestOwner(r)
	if _, err := s.conversations.Get(owner, id); err != nil {
		writeConversationError(w, r, err)
		return
	}
	items, err := s.scenes.ListByConversation(owner, id)
	if err != nil {
		writeSceneError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"scenes": items})
}

func (s *Server) getScene(w http.ResponseWriter, r *http.Request) {
	if s.scenes == nil {
		writeSceneUnconfigured(w)
		return
	}
	project, err := s.scenes.GetOwned(requestOwner(r), r.PathValue("id"))
	if err != nil {
		writeSceneError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"scene": project})
}

// patchScene applies the user-edited approved summary and/or the selected
// candidate id. The candidate is resolved server-side from the stored list.
func (s *Server) patchScene(w http.ResponseWriter, r *http.Request) {
	if s.scenes == nil {
		writeSceneUnconfigured(w)
		return
	}
	var request patchSceneRequest
	if err := decodeSceneJSON(w, r, &request, 8<<10); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"detail": map[string]any{"error": err.Error()}})
		return
	}
	project, err := s.scenes.UpdateDraft(requestOwner(r), r.PathValue("id"), request.ApprovedSummary, request.SelectedCandidateID)
	if err != nil {
		writeSceneError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"scene": project})
}

// generateScene composes the brief and enqueues the bounded async job. It
// returns 202 immediately; the client polls GET scene.
func (s *Server) generateScene(w http.ResponseWriter, r *http.Request) {
	if s.scenes == nil {
		writeSceneUnconfigured(w)
		return
	}
	project, err := s.scenes.Generate(r.Context(), requestOwner(r), r.PathValue("id"))
	if err != nil {
		writeSceneError(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"scene": project})
}

// regenerateScene creates a fresh scene from the same summary/moment (with
// parent_scene_id provenance) and immediately enqueues it.
func (s *Server) regenerateScene(w http.ResponseWriter, r *http.Request) {
	if s.scenes == nil {
		writeSceneUnconfigured(w)
		return
	}
	project, err := s.scenes.Regenerate(r.Context(), requestOwner(r), r.PathValue("id"))
	if err != nil {
		writeSceneError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"scene": project})
}

// serveSceneImage streams the generated image for an owner-scoped completed
// scene with correct content type and caching headers. The real filesystem
// path is never exposed.
func (s *Server) serveSceneImage(w http.ResponseWriter, r *http.Request) {
	if s.scenes == nil {
		writeSceneUnconfigured(w)
		return
	}
	file, project, err := s.scenes.OpenImage(requestOwner(r), r.PathValue("id"))
	if err != nil {
		writeSceneError(w, r, err)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		// Do not wrap the *PathError: it carries the absolute image path.
		writeSceneError(w, r, errors.New("scene image is unavailable"))
		return
	}
	ext := strings.TrimPrefix(filepath.Ext(project.ImagePath()), ".")
	if ext == "" {
		ext = "png"
	}
	w.Header().Set("Content-Type", project.ImageMIME)
	w.Header().Set("Content-Length", fmt.Sprint(info.Size()))
	w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s.%s"`, project.ID, ext))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, max-age=300")
	http.ServeContent(w, r, project.ID+"."+ext, info.ModTime(), file)
}

func (s *Server) deleteScene(w http.ResponseWriter, r *http.Request) {
	if s.scenes == nil {
		writeSceneUnconfigured(w)
		return
	}
	if err := s.scenes.DeleteOwned(requestOwner(r), r.PathValue("id")); err != nil {
		writeSceneError(w, r, err)
		return
	}
	logging.FromContext(r.Context()).Info("scene_deleted", "scene_id", r.PathValue("id"))
	w.WriteHeader(http.StatusNoContent)
}

func decodeSceneJSON(w http.ResponseWriter, r *http.Request, target any, limit int64) error {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("invalid scene payload")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("invalid scene payload")
	}
	return nil
}

func writeSceneUnconfigured(w http.ResponseWriter) {
	writeJSON(w, http.StatusServiceUnavailable, map[string]any{
		"detail": map[string]any{"error": "scene generation is not configured"},
	})
}

func writeSceneError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, scenes.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]any{"detail": map[string]any{"error": "scene not found"}})
	case errors.Is(err, scenes.ErrTextNotConfigured):
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"detail": map[string]any{"error": "scene text orchestration is not configured"},
		})
	case errors.Is(err, scenes.ErrImageNotConfigured):
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"detail": map[string]any{"error": "scene image generation is not configured"},
		})
	case errors.Is(err, scenes.ErrQueueFull):
		w.Header().Set("Retry-After", "30")
		writeJSON(w, http.StatusTooManyRequests, map[string]any{"detail": map[string]any{"error": "scene generation queue is temporarily full"}})
	case errors.Is(err, scenes.ErrBusy):
		writeJSON(w, http.StatusConflict, map[string]any{"detail": map[string]any{"error": "scene generation is already running"}})
	default:
		var validationError *scenes.Error
		if errors.As(err, &validationError) {
			writeJSON(w, http.StatusBadRequest, map[string]any{"detail": map[string]any{"error": validationError.Message}})
			return
		}
		// Provider failures are already sanitized; log id/status only.
		logging.FromContext(r.Context()).Warn("scene_operation_failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"detail": map[string]any{"error": "scene operation failed"}})
	}
}
