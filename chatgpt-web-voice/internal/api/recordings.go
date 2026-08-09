package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/dyhhhhhh/chatgpt-web-voice/internal/logging"
	"github.com/dyhhhhhh/chatgpt-web-voice/internal/recordings"
)

const maxRecordingChunkBytes = 2 << 20

type createRecordingRequest struct {
	ConversationID string `json:"conversation_id"`
	VoiceSessionID string `json:"voice_session_id"`
	MIMEType       string `json:"mime_type"`
}

type completeRecordingRequest struct {
	ChunkCount     int    `json:"chunk_count"`
	DurationMS     int64  `json:"duration_ms"`
	VoiceSessionID string `json:"voice_session_id"`
	Failed         bool   `json:"failed"`
	ErrorMessage   string `json:"error_message"`
}

func (s *Server) createRecording(w http.ResponseWriter, r *http.Request) {
	if s.recordings == nil {
		writeRecordingUnavailable(w)
		return
	}
	var request createRecordingRequest
	if err := decodeRecordingJSON(w, r, &request, 16<<10); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"detail": map[string]any{"error": err.Error()}})
		return
	}
	item, err := s.recordings.Create(requestOwner(r), recordings.CreateInput{
		ConversationID: request.ConversationID,
		VoiceSessionID: request.VoiceSessionID,
		MIMEType:       request.MIMEType,
	})
	if err != nil {
		writeRecordingError(w, r, err)
		return
	}
	logging.FromContext(r.Context()).Info("recording_created",
		"recording_id", item.ID,
		"conversation_id", item.ConversationID,
		"voice_session_id", item.VoiceSessionID,
	)
	writeJSON(w, http.StatusCreated, map[string]any{"recording": item})
}

func (s *Server) uploadRecordingChunk(w http.ResponseWriter, r *http.Request) {
	if s.recordings == nil {
		writeRecordingUnavailable(w)
		return
	}
	sequence, err := recordings.ChunkSequence(strings.TrimSpace(r.PathValue("sequence")))
	if err != nil {
		writeRecordingError(w, r, err)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxRecordingChunkBytes)
	item, err := s.recordings.AddChunk(requestOwner(r), r.PathValue("id"), sequence, r.Body)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]any{"detail": map[string]any{"error": "recording chunk is too large"}})
			return
		}
		writeRecordingError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"recording_id": item.ID,
		"sequence":     sequence,
	})
}

func (s *Server) completeRecording(w http.ResponseWriter, r *http.Request) {
	if s.recordings == nil {
		writeRecordingUnavailable(w)
		return
	}
	var request completeRecordingRequest
	if err := decodeRecordingJSON(w, r, &request, 16<<10); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"detail": map[string]any{"error": err.Error()}})
		return
	}
	item, err := s.recordings.Complete(requestOwner(r), r.PathValue("id"), recordings.CompleteInput{
		ChunkCount:     request.ChunkCount,
		DurationMS:     request.DurationMS,
		VoiceSessionID: request.VoiceSessionID,
		Failed:         request.Failed,
		ErrorMessage:   request.ErrorMessage,
	})
	if err != nil {
		writeRecordingError(w, r, err)
		return
	}
	logging.FromContext(r.Context()).Info("recording_completed",
		"recording_id", item.ID,
		"status", item.Status,
		"byte_size", item.ByteSize,
		"duration_ms", item.DurationMS,
	)
	writeJSON(w, http.StatusOK, map[string]any{"recording": item})
}

func (s *Server) listRecordings(w http.ResponseWriter, r *http.Request) {
	if s.recordings == nil {
		writeRecordingUnavailable(w)
		return
	}
	limit := 200
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	items, err := s.recordings.List(recordings.ListFilter{
		Query:  r.URL.Query().Get("q"),
		Status: r.URL.Query().Get("status"),
		Limit:  limit,
	})
	if err != nil {
		writeRecordingError(w, r, err)
		return
	}
	stats, err := s.recordings.Stats()
	if err != nil {
		writeRecordingError(w, r, err)
		return
	}
	rows := make([]map[string]any, 0, len(items))
	for _, item := range items {
		rows = append(rows, adminRecordingJSON(item))
	}
	writeJSON(w, http.StatusOK, map[string]any{"recordings": rows, "stats": stats})
}

func (s *Server) getRecording(w http.ResponseWriter, r *http.Request) {
	if s.recordings == nil {
		writeRecordingUnavailable(w)
		return
	}
	detail, err := s.recordings.GetAdmin(r.PathValue("id"))
	if err != nil {
		writeRecordingError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"recording": adminRecordingJSON(detail.Recording),
		"messages":  detail.Messages,
	})
}

func (s *Server) playRecording(w http.ResponseWriter, r *http.Request) {
	if s.recordings == nil {
		writeRecordingUnavailable(w)
		return
	}
	file, item, err := s.recordings.OpenAudio(r.PathValue("id"))
	if err != nil {
		writeRecordingError(w, r, err)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		writeRecordingError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", item.MIMEType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, filepath.Base(item.ID+"."+item.FileExt)))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, item.ID+"."+item.FileExt, info.ModTime(), file)
}

func (s *Server) deleteRecording(w http.ResponseWriter, r *http.Request) {
	if s.recordings == nil {
		writeRecordingUnavailable(w)
		return
	}
	if err := s.recordings.Delete(r.PathValue("id")); err != nil {
		writeRecordingError(w, r, err)
		return
	}
	logging.FromContext(r.Context()).Info("recording_deleted", "recording_id", r.PathValue("id"))
	w.WriteHeader(http.StatusNoContent)
}

func adminRecordingJSON(item recordings.Item) map[string]any {
	callerKind := "guest"
	callerLabel := "guest"
	if strings.HasPrefix(item.Owner, "admin:") {
		callerKind = "admin"
		callerLabel = "admin"
	} else if strings.HasPrefix(item.Owner, "api_key:") {
		callerKind = "api_key"
		callerLabel = item.Owner
	}
	return map[string]any{
		"id":                 item.ID,
		"conversation_id":    item.ConversationID,
		"conversation_title": item.ConversationTitle,
		"voice_session_id":   item.VoiceSessionID,
		"mime_type":          item.MIMEType,
		"status":             item.Status,
		"chunk_count":        item.ChunkCount,
		"byte_size":          item.ByteSize,
		"duration_ms":        item.DurationMS,
		"error_message":      item.ErrorMessage,
		"created_at":         item.CreatedAt,
		"updated_at":         item.UpdatedAt,
		"completed_at":       item.CompletedAt,
		"message_count":      item.MessageCount,
		"audio_available":    item.AudioAvailable,
		"caller_kind":        callerKind,
		"caller_label":       callerLabel,
		"audio_url":          "/api/admin/recordings/" + item.ID + "/audio",
	}
}

func decodeRecordingJSON(w http.ResponseWriter, r *http.Request, target any, limit int64) error {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("invalid recording payload")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("invalid recording payload")
	}
	return nil
}

func writeRecordingUnavailable(w http.ResponseWriter) {
	writeJSON(w, http.StatusServiceUnavailable, map[string]any{"detail": map[string]any{"error": "recording store unavailable"}})
}

func writeRecordingError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, recordings.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]any{"detail": map[string]any{"error": "recording not found"}})
	default:
		var validationError *recordings.Error
		if errors.As(err, &validationError) {
			writeJSON(w, http.StatusBadRequest, map[string]any{"detail": map[string]any{"error": validationError.Message}})
			return
		}
		logging.FromContext(r.Context()).Error("recording_operation_failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"detail": map[string]any{"error": "recording operation failed"}})
	}
}
