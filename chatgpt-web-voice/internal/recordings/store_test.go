package recordings

import (
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dyhhhhhh/chatgpt-web-voice/internal/conversations"
	"github.com/dyhhhhhh/chatgpt-web-voice/internal/store"
)

func TestRecordingLifecycleAndTranscriptSnapshot(t *testing.T) {
	tempDir := t.TempDir()
	db, err := store.Open(filepath.Join(tempDir, "voice.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	conversationStore := conversations.NewStore(db)
	conversation, err := conversationStore.Create("guest:test-owner", "Recorded conversation")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conversationStore.UpsertMessage("guest:test-owner", conversation.ID, conversations.Message{
		ClientID: "user-1", Role: "user", Content: "你好",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := conversationStore.UpsertMessage("guest:test-owner", conversation.ID, conversations.Message{
		ClientID: "assistant-1", Role: "assistant", Content: "你好，有什么可以帮助你？",
	}); err != nil {
		t.Fatal(err)
	}

	recordingStore, err := NewStore(db, filepath.Join(tempDir, "recordings"))
	if err != nil {
		t.Fatal(err)
	}
	item, err := recordingStore.Create("guest:test-owner", CreateInput{
		ConversationID: conversation.ID,
		VoiceSessionID: "vs_test",
		MIMEType:       "audio/webm;codecs=opus",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := recordingStore.Create("guest:other", CreateInput{
		ConversationID: conversation.ID,
		MIMEType:       "audio/webm",
	}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected conversation owner isolation, got %v", err)
	}
	if _, err := recordingStore.AddChunk("guest:test-owner", item.ID, 0, strings.NewReader("hello ")); err != nil {
		t.Fatal(err)
	}
	if _, err := recordingStore.AddChunk("guest:test-owner", item.ID, 1, strings.NewReader("world")); err != nil {
		t.Fatal(err)
	}
	// Lost responses may cause a chunk retry; the stored audio must not duplicate it.
	if _, err := recordingStore.AddChunk("guest:test-owner", item.ID, 1, strings.NewReader("world")); err != nil {
		t.Fatal(err)
	}
	completed, err := recordingStore.Complete("guest:test-owner", item.ID, CompleteInput{
		ChunkCount: 2,
		DurationMS: 1234,
	})
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != StatusCompleted || completed.ByteSize != int64(len("hello world")) || completed.ChunkCount != 2 {
		t.Fatalf("unexpected completed recording: %+v", completed)
	}
	retried, err := recordingStore.Complete("guest:test-owner", item.ID, CompleteInput{ChunkCount: 2, DurationMS: 1234})
	if err != nil || retried.Status != StatusCompleted || retried.ByteSize != completed.ByteSize {
		t.Fatalf("completion retry was not idempotent: item=%+v err=%v", retried, err)
	}
	file, _, err := recordingStore.OpenAudio(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	audio, err := io.ReadAll(file)
	_ = file.Close()
	if err != nil {
		t.Fatal(err)
	}
	if string(audio) != "hello world" {
		t.Fatalf("unexpected assembled audio: %q", audio)
	}

	if err := conversationStore.Delete("guest:test-owner", conversation.ID); err != nil {
		t.Fatal(err)
	}
	detail, err := recordingStore.GetAdmin(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Messages) != 2 || detail.Messages[0].Content != "你好" {
		t.Fatalf("transcript snapshot did not survive conversation deletion: %+v", detail.Messages)
	}
	if err := recordingStore.Delete(item.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := recordingStore.OpenAudio(item.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected deleted recording to be unavailable, got %v", err)
	}
}

func TestRecordingCompletionMarksMissingChunksIncomplete(t *testing.T) {
	tempDir := t.TempDir()
	db, err := store.Open(filepath.Join(tempDir, "voice.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	conversationStore := conversations.NewStore(db)
	conversation, err := conversationStore.Create("admin:root", "Incomplete")
	if err != nil {
		t.Fatal(err)
	}
	recordingStore, err := NewStore(db, filepath.Join(tempDir, "recordings"))
	if err != nil {
		t.Fatal(err)
	}
	item, err := recordingStore.Create("admin:root", CreateInput{ConversationID: conversation.ID, MIMEType: "audio/webm"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := recordingStore.AddChunk("admin:root", item.ID, 0, strings.NewReader("partial")); err != nil {
		t.Fatal(err)
	}
	completed, err := recordingStore.Complete("admin:root", item.ID, CompleteInput{ChunkCount: 2, DurationMS: 9000})
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != StatusIncomplete || completed.ByteSize != int64(len("partial")) || completed.ErrorMessage == "" {
		t.Fatalf("expected incomplete recording, got %+v", completed)
	}
}
