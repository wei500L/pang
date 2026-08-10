package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dyhhhhhh/chatgpt-web-voice/internal/callsessions"
	"github.com/dyhhhhhh/chatgpt-web-voice/internal/conversations"
	"github.com/dyhhhhhh/chatgpt-web-voice/internal/recordings"
	"github.com/dyhhhhhh/chatgpt-web-voice/internal/store"
)

func TestRecordingUploadAndAdminAPI(t *testing.T) {
	tempDir := t.TempDir()
	db, err := store.Open(filepath.Join(tempDir, "voice.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	conversationStore := conversations.NewStore(db)
	conversation, err := conversationStore.Create("default", "API recording")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conversationStore.UpsertMessage("default", conversation.ID, conversations.Message{
		ClientID: "msg-api", Role: "user", Content: "record this",
	}); err != nil {
		t.Fatal(err)
	}
	recordingStore, err := recordings.NewStore(db, filepath.Join(tempDir, "recordings"))
	if err != nil {
		t.Fatal(err)
	}
	callSessionStore := callsessions.NewStore(db)
	if err := callSessionStore.Upsert(callsessions.Session{
		VoiceSessionID: "vs_api",
		Owner:          "admin:",
		CallerKind:     callsessions.CallerAdmin,
		Status:         callsessions.StatusActive,
	}); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	New(Dependencies{Conversations: conversationStore, CallSessions: callSessionStore, Recordings: recordingStore}).Register(mux)

	created := performJSONRequest(t, mux, http.MethodPost, "/api/recordings", map[string]any{
		"conversation_id":  conversation.ID,
		"voice_session_id": "vs_api",
		"mime_type":        "audio/webm;codecs=opus",
	})
	if created.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", created.Code, created.Body.String())
	}
	var createBody struct {
		Recording struct {
			ID string `json:"id"`
		} `json:"recording"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &createBody); err != nil {
		t.Fatal(err)
	}
	if createBody.Recording.ID == "" {
		t.Fatal("missing recording ID")
	}

	upload := httptest.NewRequest(http.MethodPut, "/api/recordings/"+createBody.Recording.ID+"/chunks/0", strings.NewReader("encoded-audio"))
	upload.Header.Set("Content-Type", "audio/webm")
	uploadResponse := httptest.NewRecorder()
	mux.ServeHTTP(uploadResponse, upload)
	if uploadResponse.Code != http.StatusOK {
		t.Fatalf("upload status=%d body=%s", uploadResponse.Code, uploadResponse.Body.String())
	}

	completed := performJSONRequest(t, mux, http.MethodPost, "/api/recordings/"+createBody.Recording.ID+"/complete", map[string]any{
		"chunk_count":      1,
		"duration_ms":      2500,
		"voice_session_id": "vs_api",
		"failed":           false,
	})
	if completed.Code != http.StatusOK || !strings.Contains(completed.Body.String(), `"status":"completed"`) {
		t.Fatalf("complete status=%d body=%s", completed.Code, completed.Body.String())
	}

	list := performJSONRequest(t, mux, http.MethodGet, "/api/admin/recordings", nil)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "API recording") || !strings.Contains(list.Body.String(), `"completed":1`) {
		t.Fatalf("list status=%d body=%s", list.Code, list.Body.String())
	}
	if !strings.Contains(list.Body.String(), `"caller_kind":"admin"`) {
		t.Fatalf("admin recording was misclassified: %s", list.Body.String())
	}
	detail := performJSONRequest(t, mux, http.MethodGet, "/api/admin/recordings/"+createBody.Recording.ID, nil)
	if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), "record this") {
		t.Fatalf("detail status=%d body=%s", detail.Code, detail.Body.String())
	}

	play := httptest.NewRequest(http.MethodGet, "/api/admin/recordings/"+createBody.Recording.ID+"/audio", nil)
	play.Header.Set("Range", "bytes=0-6")
	playResponse := httptest.NewRecorder()
	mux.ServeHTTP(playResponse, play)
	if playResponse.Code != http.StatusPartialContent || playResponse.Body.String() != "encoded" {
		t.Fatalf("range status=%d body=%q headers=%v", playResponse.Code, playResponse.Body.String(), playResponse.Header())
	}

	deleted := performJSONRequest(t, mux, http.MethodDelete, "/api/admin/recordings/"+createBody.Recording.ID, nil)
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("delete status=%d body=%s", deleted.Code, deleted.Body.String())
	}
	missing := performJSONRequest(t, mux, http.MethodGet, "/api/admin/recordings/"+createBody.Recording.ID, nil)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("expected deleted recording 404, got %d", missing.Code)
	}
}

func TestRecordingCreationRequiresOwnedActiveCallSession(t *testing.T) {
	tempDir := t.TempDir()
	db, err := store.Open(filepath.Join(tempDir, "voice.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	conversationStore := conversations.NewStore(db)
	conversation, err := conversationStore.Create("default", "Bound recording")
	if err != nil {
		t.Fatal(err)
	}
	recordingStore, err := recordings.NewStore(db, filepath.Join(tempDir, "recordings"))
	if err != nil {
		t.Fatal(err)
	}
	callSessionStore := callsessions.NewStore(db)
	for _, session := range []callsessions.Session{
		{
			VoiceSessionID: "vs_foreign",
			Owner:          "guest:other",
			CallerKind:     callsessions.CallerGuest,
			Status:         callsessions.StatusActive,
		},
		{
			VoiceSessionID: "vs_released",
			Owner:          "admin:",
			CallerKind:     callsessions.CallerAdmin,
			Status:         callsessions.StatusReleased,
		},
		{
			VoiceSessionID: "vs_owned",
			Owner:          "admin:",
			CallerKind:     callsessions.CallerAdmin,
			Status:         callsessions.StatusActive,
		},
	} {
		if err := callSessionStore.Upsert(session); err != nil {
			t.Fatal(err)
		}
	}
	mux := http.NewServeMux()
	New(Dependencies{Conversations: conversationStore, CallSessions: callSessionStore, Recordings: recordingStore}).Register(mux)

	for _, test := range []struct {
		name      string
		sessionID string
		wantCode  int
	}{
		{name: "missing", sessionID: "vs_missing", wantCode: http.StatusNotFound},
		{name: "foreign", sessionID: "vs_foreign", wantCode: http.StatusNotFound},
		{name: "released", sessionID: "vs_released", wantCode: http.StatusConflict},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := performJSONRequest(t, mux, http.MethodPost, "/api/recordings", map[string]any{
				"conversation_id":  conversation.ID,
				"voice_session_id": test.sessionID,
				"mime_type":        "audio/webm",
			})
			if response.Code != test.wantCode {
				t.Fatalf("status=%d want=%d body=%s", response.Code, test.wantCode, response.Body.String())
			}
		})
	}

	created := performJSONRequest(t, mux, http.MethodPost, "/api/recordings", map[string]any{
		"conversation_id":  conversation.ID,
		"voice_session_id": "vs_owned",
		"mime_type":        "audio/webm",
	})
	if created.Code != http.StatusCreated {
		t.Fatalf("owned active create status=%d body=%s", created.Code, created.Body.String())
	}
	duplicate := performJSONRequest(t, mux, http.MethodPost, "/api/recordings", map[string]any{
		"conversation_id":  conversation.ID,
		"voice_session_id": "vs_owned",
		"mime_type":        "audio/webm",
	})
	if duplicate.Code != http.StatusConflict {
		t.Fatalf("duplicate status=%d body=%s", duplicate.Code, duplicate.Body.String())
	}
}
