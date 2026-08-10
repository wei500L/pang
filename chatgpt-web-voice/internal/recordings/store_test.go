package recordings

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

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

func TestCompletedEmptySnapshotDoesNotReadLaterMessages(t *testing.T) {
	tempDir := t.TempDir()
	db, err := store.Open(filepath.Join(tempDir, "voice.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	conversationStore := conversations.NewStore(db)
	conversation, err := conversationStore.Create("guest:snapshot", "Stable snapshot")
	if err != nil {
		t.Fatal(err)
	}
	recordingStore, err := NewStore(db, filepath.Join(tempDir, "recordings"))
	if err != nil {
		t.Fatal(err)
	}
	item, err := recordingStore.Create("guest:snapshot", CreateInput{
		ConversationID: conversation.ID,
		MIMEType:       "audio/webm",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := recordingStore.AddChunk("guest:snapshot", item.ID, 0, strings.NewReader("audio")); err != nil {
		t.Fatal(err)
	}
	if _, err := recordingStore.Complete("guest:snapshot", item.ID, CompleteInput{ChunkCount: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := conversationStore.UpsertMessage("guest:snapshot", conversation.ID, conversations.Message{
		ClientID: "late-message", Role: "user", Content: "created after recording completion",
	}); err != nil {
		t.Fatal(err)
	}
	detail, err := recordingStore.GetAdmin(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Messages) != 0 {
		t.Fatalf("completed recording read messages added after its snapshot: %+v", detail.Messages)
	}
	items, err := recordingStore.List(ListFilter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].MessageCount != 0 {
		t.Fatalf("completed recording message count was not snapshot-stable: %+v", items)
	}
}

func TestRecordingRecoveryAndActiveDeletion(t *testing.T) {
	tempDir := t.TempDir()
	db, err := store.Open(filepath.Join(tempDir, "voice.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	conversationStore := conversations.NewStore(db)
	conversation, err := conversationStore.Create("guest:recovery", "Recovery")
	if err != nil {
		t.Fatal(err)
	}
	recordingStore, err := NewStore(db, filepath.Join(tempDir, "recordings"))
	if err != nil {
		t.Fatal(err)
	}
	item, err := recordingStore.Create("guest:recovery", CreateInput{ConversationID: conversation.ID, MIMEType: "audio/webm"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := recordingStore.AddChunk("guest:recovery", item.ID, 0, strings.NewReader("recoverable")); err != nil {
		t.Fatal(err)
	}
	reopened, err := NewStore(db, filepath.Join(tempDir, "recordings"))
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := reopened.RecoverInterrupted()
	if err != nil || recovered != 1 {
		t.Fatalf("recovered=%d err=%v", recovered, err)
	}
	detail, err := reopened.GetAdmin(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Recording.Status != StatusFailed || detail.Recording.ByteSize != 0 || detail.Recording.AudioAvailable {
		t.Fatalf("unexpected recovered recording: %+v", detail.Recording)
	}
	leftoverDir := reopened.chunkDir(item.ID)
	if err := os.MkdirAll(leftoverDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(leftoverDir, "leftover.part"), []byte("stale"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.RecoverInterrupted(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(leftoverDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("finalized chunk directory was not cleaned: %v", err)
	}

	active, err := reopened.Create("guest:recovery", CreateInput{ConversationID: conversation.ID, MIMEType: "audio/webm"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.AddChunk("guest:recovery", active.ID, 0, strings.NewReader("stale")); err != nil {
		t.Fatal(err)
	}
	staleAt := time.Now().Add(-staleRecordingAfter - time.Minute)
	if err := os.Chtimes(reopened.chunkDir(active.ID), staleAt, staleAt); err != nil {
		t.Fatal(err)
	}
	reopened.lastSweep = time.Time{}
	if recovered, err := reopened.RecoverStale(); err != nil || recovered != 1 {
		t.Fatalf("stale recovered=%d err=%v", recovered, err)
	}
	staleDetail, err := reopened.GetAdmin(active.ID)
	if err != nil || staleDetail.Recording.Status != StatusFailed {
		t.Fatalf("stale recording was not failed: detail=%+v err=%v", staleDetail.Recording, err)
	}

	active, err = reopened.Create("guest:recovery", CreateInput{ConversationID: conversation.ID, MIMEType: "audio/webm"})
	if err != nil {
		t.Fatal(err)
	}
	if err := reopened.Delete(active.ID); err != nil {
		t.Fatalf("active recording should be safely deletable: %v", err)
	}
}

func TestRecordingLimitsAndBoundedSnapshot(t *testing.T) {
	tempDir := t.TempDir()
	db, err := store.Open(filepath.Join(tempDir, "voice.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	conversationStore := conversations.NewStore(db)
	conversation, err := conversationStore.Create("guest:limits", "Limits")
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < MaxSnapshotMessages+5; index++ {
		if _, err := conversationStore.UpsertMessage("guest:limits", conversation.ID, conversations.Message{
			ClientID: "message-" + strconv.Itoa(index), Role: "user", Content: strings.Repeat("x", MaxSnapshotContentChars+32),
		}); err != nil {
			t.Fatal(err)
		}
	}
	recordingStore, err := NewStore(db, filepath.Join(tempDir, "recordings"))
	if err != nil {
		t.Fatal(err)
	}
	if err := recordingStore.ensureStorageReserve(1 << 62); !errors.Is(err, ErrStorageFull) {
		t.Fatalf("expected storage reserve rejection, got %v", err)
	}
	first, err := recordingStore.Create("guest:limits", CreateInput{ConversationID: conversation.ID, MIMEType: "audio/webm"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := recordingStore.Create("guest:limits", CreateInput{ConversationID: conversation.ID, MIMEType: "audio/webm"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := recordingStore.Create("guest:limits", CreateInput{ConversationID: conversation.ID, MIMEType: "audio/webm"}); !errors.Is(err, ErrCapacity) {
		t.Fatalf("expected active recording capacity limit, got %v", err)
	}
	if _, err := recordingStore.AddChunk("guest:limits", first.ID, 0, strings.NewReader(strings.Repeat("a", int(MaxChunkBytes)+1))); err == nil {
		t.Fatal("expected oversized chunk rejection")
	}
	if _, err := recordingStore.Complete("guest:limits", first.ID, CompleteInput{ChunkCount: 0, Failed: true}); err != nil {
		t.Fatal(err)
	}
	detail, err := recordingStore.GetAdmin(first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Messages) != MaxSnapshotMessages {
		t.Fatalf("snapshot messages=%d want=%d", len(detail.Messages), MaxSnapshotMessages)
	}
	for _, message := range detail.Messages {
		if len(message.Content) > MaxSnapshotContentChars {
			t.Fatalf("snapshot content was not truncated: %d", len(message.Content))
		}
	}
	if err := recordingStore.Delete(second.ID); err != nil {
		t.Fatal(err)
	}
}

func TestGlobalRecordingCapacityIsBounded(t *testing.T) {
	tempDir := t.TempDir()
	db, err := store.Open(filepath.Join(tempDir, "voice.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	conversationStore := conversations.NewStore(db)
	conversation, err := conversationStore.Create("guest:global-new", "Global capacity")
	if err != nil {
		t.Fatal(err)
	}
	recordingStore, err := NewStore(db, filepath.Join(tempDir, "recordings"))
	if err != nil {
		t.Fatal(err)
	}

	db.Lock()
	tx, err := db.Conn().Begin()
	if err == nil {
		for index := 0; index < MaxActiveRecordingsGlobal; index++ {
			_, err = tx.Exec(`
				INSERT INTO recordings (
					id, owner, conversation_id, mime_type, file_ext, status
				) VALUES (?, ?, ?, ?, ?, ?)`,
				"rec_global_"+strconv.Itoa(index), "guest:global-"+strconv.Itoa(index),
				conversation.ID, "audio/webm", "webm", StatusRecording)
			if err != nil {
				break
			}
		}
	}
	if err == nil {
		err = tx.Commit()
	} else if tx != nil {
		_ = tx.Rollback()
	}
	db.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	// The synthetic active rows intentionally have no chunk directories. Skip
	// the stale sweep so this assertion specifically exercises the capacity
	// guard rather than orphan recovery.
	recordingStore.lastSweep = time.Now()
	if _, err := recordingStore.Create("guest:global-new", CreateInput{
		ConversationID: conversation.ID,
		MIMEType:       "audio/webm",
	}); !errors.Is(err, ErrCapacity) {
		t.Fatalf("expected global recording capacity rejection, got %v", err)
	}
}

func TestRecordingIOConcurrencyLimitsFailOpen(t *testing.T) {
	tempDir := t.TempDir()
	db, err := store.Open(filepath.Join(tempDir, "voice.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	conversationStore := conversations.NewStore(db)
	conversation, err := conversationStore.Create("guest:io-limit", "I/O limits")
	if err != nil {
		t.Fatal(err)
	}
	recordingStore, err := NewStore(db, filepath.Join(tempDir, "recordings"))
	if err != nil {
		t.Fatal(err)
	}
	item, err := recordingStore.Create("guest:io-limit", CreateInput{
		ConversationID: conversation.ID,
		MIMEType:       "audio/webm",
	})
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < cap(recordingStore.chunkWriteSlots); index++ {
		recordingStore.chunkWriteSlots <- struct{}{}
	}
	if _, err := recordingStore.AddChunk("guest:io-limit", item.ID, 0, strings.NewReader("audio")); !errors.Is(err, ErrCapacity) {
		t.Fatalf("expected saturated chunk writes to fail open, got %v", err)
	}
	for len(recordingStore.chunkWriteSlots) > 0 {
		<-recordingStore.chunkWriteSlots
	}
	if _, err := recordingStore.AddChunk("guest:io-limit", item.ID, 0, strings.NewReader("audio")); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < cap(recordingStore.assemblySlots); index++ {
		recordingStore.assemblySlots <- struct{}{}
	}
	if _, err := recordingStore.Complete("guest:io-limit", item.ID, CompleteInput{ChunkCount: 1}); !errors.Is(err, ErrCapacity) {
		t.Fatalf("expected saturated assemblies to fail open, got %v", err)
	}
	for len(recordingStore.assemblySlots) > 0 {
		<-recordingStore.assemblySlots
	}
	completed, err := recordingStore.Complete("guest:io-limit", item.ID, CompleteInput{ChunkCount: 1})
	if err != nil || completed.Status != StatusCompleted {
		t.Fatalf("completion after capacity release failed: item=%+v err=%v", completed, err)
	}
	if recordingStore.storageReserved != 0 {
		t.Fatalf("storage reservation leaked: %d", recordingStore.storageReserved)
	}
}

func TestConcurrentRecordingCompletionIsSerialized(t *testing.T) {
	tempDir := t.TempDir()
	db, err := store.Open(filepath.Join(tempDir, "voice.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	conversationStore := conversations.NewStore(db)
	conversation, err := conversationStore.Create("guest:concurrent", "Concurrent")
	if err != nil {
		t.Fatal(err)
	}
	recordingStore, err := NewStore(db, filepath.Join(tempDir, "recordings"))
	if err != nil {
		t.Fatal(err)
	}
	item, err := recordingStore.Create("guest:concurrent", CreateInput{ConversationID: conversation.ID, MIMEType: "audio/webm"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := recordingStore.AddChunk("guest:concurrent", item.ID, 0, strings.NewReader("serialized")); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for attempt := 0; attempt < 2; attempt++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			completed, completeErr := recordingStore.Complete("guest:concurrent", item.ID, CompleteInput{ChunkCount: 1})
			if completeErr == nil && completed.Status != StatusCompleted {
				completeErr = errors.New("recording did not complete")
			}
			errs <- completeErr
		}()
	}
	wg.Wait()
	close(errs)
	for completeErr := range errs {
		if completeErr != nil {
			t.Fatal(completeErr)
		}
	}
	if len(recordingStore.operations) != 0 {
		t.Fatalf("recording operation locks leaked: %d", len(recordingStore.operations))
	}
}

func TestDuplicateChunkWaitsForAtomicPublication(t *testing.T) {
	tempDir := t.TempDir()
	db, err := store.Open(filepath.Join(tempDir, "voice.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	conversationStore := conversations.NewStore(db)
	conversation, err := conversationStore.Create("guest:chunk-race", "Chunk race")
	if err != nil {
		t.Fatal(err)
	}
	recordingStore, err := NewStore(db, filepath.Join(tempDir, "recordings"))
	if err != nil {
		t.Fatal(err)
	}
	item, err := recordingStore.Create("guest:chunk-race", CreateInput{ConversationID: conversation.ID, MIMEType: "audio/webm"})
	if err != nil {
		t.Fatal(err)
	}
	reader, writer := io.Pipe()
	firstDone := make(chan error, 1)
	go func() {
		_, uploadErr := recordingStore.AddChunk("guest:chunk-race", item.ID, 0, reader)
		firstDone <- uploadErr
	}()
	if _, err := writer.Write([]byte("atomic")); err != nil {
		t.Fatal(err)
	}
	secondDone := make(chan error, 1)
	go func() {
		_, uploadErr := recordingStore.AddChunk("guest:chunk-race", item.ID, 0, strings.NewReader("duplicate"))
		secondDone <- uploadErr
	}()
	select {
	case err := <-secondDone:
		t.Fatalf("duplicate upload returned before the first chunk was published: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
	if err := <-secondDone; err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(recordingStore.chunkPath(item.ID, 0))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "atomic" {
		t.Fatalf("duplicate upload replaced the published chunk: %q", content)
	}
}
