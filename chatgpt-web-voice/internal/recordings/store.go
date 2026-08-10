package recordings

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"

	"github.com/dyhhhhhh/chatgpt-web-voice/internal/callsessions"
	storepkg "github.com/dyhhhhhh/chatgpt-web-voice/internal/store"
)

const (
	StatusRecording  = "recording"
	StatusCompleted  = "completed"
	StatusIncomplete = "incomplete"
	StatusFailed     = "failed"

	// MaxChunkBytes is deliberately much larger than a normal five-second
	// 16 kbps Opus chunk while still preventing the public endpoint from being
	// used as a generic multi-megabyte file upload surface.
	MaxChunkBytes               int64 = 2 << 20
	MaxRecordingBytes                 = 1 << 30
	MaxRecordingChunks                = 86400
	MaxActiveRecordingsPerOwner       = 32
	MaxActiveRecordingsGlobal         = 2048
	MaxConcurrentChunkWrites          = 64
	MaxConcurrentAssemblies           = 16
	MinRecordingFreeBytes       int64 = 128 << 20
	MaxSnapshotMessages               = 120
	MaxSnapshotContentChars           = 8192
	staleRecordingAfter               = 6 * time.Hour
	staleSweepInterval                = 5 * time.Minute
	statsCacheTTL                     = 5 * time.Second
)

// Error is a validation error safe to return to an API caller.
type Error = storepkg.Error

// ErrNotFound means the recording is missing or not owned by the caller.
var ErrNotFound = storepkg.ErrNotFound

// ErrStorageFull means recording writes were stopped to preserve enough free
// disk space for SQLite and the main voice gateway.
var ErrStorageFull = errors.New("recording storage safety reserve reached")

// ErrCapacity means the bounded active-recording pool is full. It is kept
// separate from validation errors so callers can retry later.
var ErrCapacity = errors.New("recording active capacity reached")

// ErrCallSessionNotFound means recording creation was not bound to a call
// session owned by the requester.
var ErrCallSessionNotFound = errors.New("owned call session not found")

// ErrCallSessionInactive means the bound call was already released before the
// recording row could be created.
var ErrCallSessionInactive = errors.New("call session is not active")

type Item struct {
	ID                string `json:"id"`
	Owner             string `json:"-"`
	ConversationID    string `json:"conversation_id"`
	ConversationTitle string `json:"conversation_title"`
	VoiceSessionID    string `json:"voice_session_id"`
	MIMEType          string `json:"mime_type"`
	FileExt           string `json:"-"`
	Status            string `json:"status"`
	ChunkCount        int    `json:"chunk_count"`
	ByteSize          int64  `json:"byte_size"`
	DurationMS        int64  `json:"duration_ms"`
	ErrorMessage      string `json:"error_message,omitempty"`
	CreatedAt         string `json:"created_at"`
	UpdatedAt         string `json:"updated_at"`
	CompletedAt       string `json:"completed_at,omitempty"`
	MessageCount      int    `json:"message_count"`
	AudioAvailable    bool   `json:"audio_available"`
}

type Message struct {
	ClientID  string `json:"client_id"`
	Role      string `json:"role"`
	Content   string `json:"content"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type Detail struct {
	Recording Item      `json:"recording"`
	Messages  []Message `json:"messages"`
}

type CreateInput struct {
	ConversationID string
	VoiceSessionID string
	CallOwner      string
	MIMEType       string
}

type CompleteInput struct {
	ChunkCount     int
	DurationMS     int64
	VoiceSessionID string
	Failed         bool
	ErrorMessage   string
}

type ListFilter struct {
	Query  string
	Status string
	Limit  int
}

type Stats struct {
	Total      int   `json:"total"`
	Recording  int   `json:"recording"`
	Completed  int   `json:"completed"`
	Incomplete int   `json:"incomplete"`
	Failed     int   `json:"failed"`
	ByteSize   int64 `json:"byte_size"`
}

type Store struct {
	db      *storepkg.DB
	baseDir string

	operationMu sync.Mutex
	operations  map[string]*recordingOperation

	sweepMu   sync.Mutex
	lastSweep time.Time

	statsMu       sync.Mutex
	statsCache    Stats
	statsCachedAt time.Time

	storageMu       sync.Mutex
	storageReserved int64
	chunkWriteSlots chan struct{}
	assemblySlots   chan struct{}
}

type recordingOperation struct {
	mu   sync.Mutex
	refs int
}

func NewStore(db *storepkg.DB, baseDir string) (*Store, error) {
	baseDir = strings.TrimSpace(baseDir)
	if baseDir == "" {
		return nil, errors.New("recording directory is empty")
	}
	if err := os.MkdirAll(filepath.Join(baseDir, "chunks"), 0o700); err != nil {
		return nil, fmt.Errorf("create recording directory: %w", err)
	}
	if err := os.Chmod(baseDir, 0o700); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("secure recording directory: %w", err)
	}
	return &Store{
		db:              db,
		baseDir:         baseDir,
		operations:      make(map[string]*recordingOperation),
		chunkWriteSlots: make(chan struct{}, MaxConcurrentChunkWrites),
		assemblySlots:   make(chan struct{}, MaxConcurrentAssemblies),
	}, nil
}

func (s *Store) Create(owner string, input CreateInput) (Item, error) {
	owner = strings.TrimSpace(owner)
	input.ConversationID = strings.TrimSpace(input.ConversationID)
	input.VoiceSessionID = truncate(strings.TrimSpace(input.VoiceSessionID), 160)
	input.CallOwner = strings.TrimSpace(input.CallOwner)
	mimeType, ext, err := normalizeMIME(input.MIMEType)
	if err != nil {
		return Item{}, err
	}
	if owner == "" || input.ConversationID == "" {
		return Item{}, &Error{Message: "conversation_id is required"}
	}
	_, _ = s.RecoverStale()
	if err := s.ensureStorageReserve(MaxChunkBytes); err != nil {
		return Item{}, err
	}

	id := "rec_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	chunkDir := s.chunkDir(id)
	if err := os.MkdirAll(chunkDir, 0o700); err != nil {
		return Item{}, fmt.Errorf("create recording chunk directory: %w", err)
	}

	s.db.Lock()
	tx, err := s.db.Conn().Begin()
	var title string
	if err == nil {
		err = tx.QueryRow(
			`SELECT title FROM conversations WHERE id = ? AND owner = ?`,
			input.ConversationID, owner,
		).Scan(&title)
	}
	if err == nil && input.CallOwner != "" {
		var callStatus string
		callErr := tx.QueryRow(
			`SELECT status FROM call_sessions WHERE voice_session_id = ? AND owner = ?`,
			input.VoiceSessionID, input.CallOwner,
		).Scan(&callStatus)
		switch {
		case errors.Is(callErr, sql.ErrNoRows):
			err = ErrCallSessionNotFound
		case callErr != nil:
			err = callErr
		case callStatus != callsessions.StatusActive:
			err = ErrCallSessionInactive
		}
	}
	var globalActive, ownerActive int
	if err == nil {
		err = tx.QueryRow(
			`SELECT COUNT(*), COALESCE(SUM(CASE WHEN owner = ? THEN 1 ELSE 0 END), 0)
			 FROM recordings WHERE status = ?`,
			owner, StatusRecording,
		).Scan(&globalActive, &ownerActive)
	}
	if err == nil && (ownerActive >= MaxActiveRecordingsPerOwner || globalActive >= MaxActiveRecordingsGlobal) {
		err = ErrCapacity
	}
	if err == nil {
		_, err = tx.Exec(`
			INSERT INTO recordings (
				id, owner, conversation_id, conversation_title, voice_session_id,
				mime_type, file_ext, status
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			id, owner, input.ConversationID, truncate(title, 120), input.VoiceSessionID,
			mimeType, ext, StatusRecording,
		)
	}
	if err == nil {
		err = tx.Commit()
	} else if tx != nil {
		_ = tx.Rollback()
	}
	s.db.Unlock()
	if errors.Is(err, sql.ErrNoRows) {
		_ = os.RemoveAll(chunkDir)
		return Item{}, ErrNotFound
	}
	if err != nil {
		_ = os.RemoveAll(chunkDir)
		if errors.Is(err, ErrCapacity) ||
			errors.Is(err, ErrCallSessionNotFound) || errors.Is(err, ErrCallSessionInactive) {
			return Item{}, err
		}
		var validationError *Error
		if errors.As(err, &validationError) {
			return Item{}, validationError
		}
		return Item{}, fmt.Errorf("create recording: %w", err)
	}
	s.invalidateStats()
	return s.GetOwned(owner, id)
}

func (s *Store) AddChunk(owner, id string, sequence int, reader io.Reader) (Item, error) {
	id = normalizeID(id)
	if id == "" {
		return Item{}, ErrNotFound
	}
	if sequence < 0 || sequence >= MaxRecordingChunks {
		return Item{}, &Error{Message: "invalid recording chunk sequence"}
	}
	unlock := s.lockRecording(id)
	defer unlock()
	item, err := s.getUploadTarget(owner, id)
	if err != nil {
		return Item{}, err
	}
	if item.Status != StatusRecording {
		return Item{}, &Error{Message: "recording is already finalized"}
	}
	releaseSlot, err := acquireRecordingSlot(s.chunkWriteSlots)
	if err != nil {
		return Item{}, err
	}
	defer releaseSlot()
	if err := os.MkdirAll(s.chunkDir(item.ID), 0o700); err != nil {
		return Item{}, fmt.Errorf("create recording chunk directory: %w", err)
	}
	path := s.chunkPath(item.ID, sequence)
	if _, err := os.Stat(path); err == nil {
		return item, nil // idempotent retry after a lost HTTP response
	} else if !errors.Is(err, os.ErrNotExist) {
		return Item{}, fmt.Errorf("inspect recording chunk: %w", err)
	}
	releaseStorage, err := s.reserveStorage(MaxChunkBytes)
	if err != nil {
		return Item{}, err
	}
	defer releaseStorage()
	file, err := os.CreateTemp(s.chunkDir(item.ID), ".upload-*")
	if err != nil {
		return Item{}, fmt.Errorf("create temporary recording chunk: %w", err)
	}
	tempPath := file.Name()
	_ = file.Chmod(0o600)
	written, copyErr := io.Copy(file, io.LimitReader(reader, MaxChunkBytes+1))
	closeErr := file.Close()
	if copyErr != nil || closeErr != nil || written == 0 || written > MaxChunkBytes {
		_ = os.Remove(tempPath)
		if copyErr != nil {
			return Item{}, fmt.Errorf("write recording chunk: %w", copyErr)
		}
		if closeErr != nil {
			return Item{}, fmt.Errorf("close recording chunk: %w", closeErr)
		}
		if written > MaxChunkBytes {
			return Item{}, &Error{Message: "recording chunk is too large"}
		}
		return Item{}, &Error{Message: "recording chunk is empty"}
	}
	usage, err := s.readChunkUsage(item.ID)
	if err != nil {
		_ = os.Remove(tempPath)
		return Item{}, err
	}
	if usage+written > MaxRecordingBytes {
		_ = os.Remove(tempPath)
		return Item{}, &Error{Message: "recording exceeds the maximum size"}
	}
	if err := s.writeChunkUsage(item.ID, usage+written); err != nil {
		_ = os.Remove(tempPath)
		return Item{}, err
	}
	if err := os.Rename(tempPath, path); err != nil {
		_ = s.writeChunkUsage(item.ID, usage)
		_ = os.Remove(tempPath)
		return Item{}, fmt.Errorf("publish recording chunk: %w", err)
	}

	// Per-chunk progress is intentionally not written to SQLite. Complete()
	// derives the authoritative count and byte size from files, keeping the
	// shared single-connection database out of the live recording data path.
	return item, nil
}

func (s *Store) getUploadTarget(owner, id string) (Item, error) {
	owner = strings.TrimSpace(owner)
	id = normalizeID(id)
	if id == "" {
		return Item{}, ErrNotFound
	}
	var item Item
	s.db.Lock()
	err := s.db.Conn().QueryRow(
		`SELECT id, owner, status FROM recordings WHERE id = ? AND owner = ?`,
		id, owner,
	).Scan(&item.ID, &item.Owner, &item.Status)
	s.db.Unlock()
	if errors.Is(err, sql.ErrNoRows) {
		return Item{}, ErrNotFound
	}
	if err != nil {
		return Item{}, fmt.Errorf("authorize recording chunk: %w", err)
	}
	return item, nil
}

func (s *Store) Complete(owner, id string, input CompleteInput) (Item, error) {
	id = normalizeID(id)
	if id == "" {
		return Item{}, ErrNotFound
	}
	unlock := s.lockRecording(id)
	defer unlock()
	item, err := s.GetOwned(owner, id)
	if err != nil {
		return Item{}, err
	}
	if item.Status != StatusRecording {
		return item, nil // idempotent completion retry after a lost response
	}
	if input.ChunkCount < 0 || input.ChunkCount > MaxRecordingChunks {
		return Item{}, &Error{Message: "invalid recording chunk count"}
	}
	if input.DurationMS < 0 {
		input.DurationMS = 0
	}
	input.VoiceSessionID = truncate(strings.TrimSpace(input.VoiceSessionID), 160)
	if input.VoiceSessionID != "" && item.VoiceSessionID != "" && input.VoiceSessionID != item.VoiceSessionID {
		return Item{}, &Error{Message: "voice_session_id does not match recording"}
	}
	input.ErrorMessage = truncate(strings.TrimSpace(input.ErrorMessage), 500)
	releaseSlot, err := acquireRecordingSlot(s.assemblySlots)
	if err != nil {
		return Item{}, err
	}
	defer releaseSlot()

	status := StatusCompleted
	if input.Failed {
		status = StatusIncomplete
	}
	finalPath := s.audioPath(item.ID, item.FileExt)
	var assembledBytes int64
	assembledChunks := 0
	for sequence := 0; sequence < input.ChunkCount; sequence++ {
		info, statErr := os.Stat(s.chunkPath(item.ID, sequence))
		if statErr != nil {
			if !errors.Is(statErr, os.ErrNotExist) {
				return Item{}, fmt.Errorf("inspect recording chunk: %w", statErr)
			}
			status = StatusIncomplete
			if input.ErrorMessage == "" {
				input.ErrorMessage = "one or more audio chunks are missing"
			}
			break
		}
		if info.Size() <= 0 || assembledBytes+info.Size() > MaxRecordingBytes {
			status = StatusIncomplete
			if input.ErrorMessage == "" {
				input.ErrorMessage = "recording exceeds the maximum size"
			}
			break
		}
		assembledBytes += info.Size()
		assembledChunks++
	}

	var tempPath string
	var releaseStorage func()
	if assembledBytes > 0 {
		releaseStorage, err = s.reserveStorage(assembledBytes)
		if err != nil {
			if !errors.Is(err, ErrStorageFull) {
				return Item{}, err
			}
			status = StatusFailed
			input.ErrorMessage = "recording stopped to preserve the storage safety reserve"
			assembledBytes = 0
			assembledChunks = 0
		}
	}
	if releaseStorage != nil {
		defer releaseStorage()
	}
	if assembledBytes > 0 {
		out, createErr := os.CreateTemp(s.baseDir, item.ID+".assemble-*")
		if createErr != nil {
			return Item{}, fmt.Errorf("create assembled recording: %w", createErr)
		}
		tempPath = out.Name()
		_ = out.Chmod(0o600)
		var copiedBytes int64
		for sequence := 0; sequence < assembledChunks; sequence++ {
			chunk, openErr := os.Open(s.chunkPath(item.ID, sequence))
			if openErr != nil {
				_ = out.Close()
				_ = os.Remove(tempPath)
				return Item{}, fmt.Errorf("open recording chunk: %w", openErr)
			}
			n, copyErr := io.Copy(out, chunk)
			_ = chunk.Close()
			if copyErr != nil {
				_ = out.Close()
				_ = os.Remove(tempPath)
				return Item{}, fmt.Errorf("assemble recording chunk: %w", copyErr)
			}
			copiedBytes += n
		}
		if closeErr := out.Close(); closeErr != nil {
			_ = os.Remove(tempPath)
			return Item{}, fmt.Errorf("close assembled recording: %w", closeErr)
		}
		if copiedBytes != assembledBytes {
			_ = os.Remove(tempPath)
			return Item{}, fmt.Errorf("assembled recording size changed during completion")
		}
	}
	if assembledBytes == 0 {
		_ = os.Remove(finalPath)
		status = StatusFailed
		if input.ErrorMessage == "" {
			input.ErrorMessage = "no audio data was received"
		}
	} else if err := os.Rename(tempPath, finalPath); err != nil {
		_ = os.Remove(tempPath)
		return Item{}, fmt.Errorf("publish assembled recording: %w", err)
	}

	s.db.Lock()
	tx, err := s.db.Conn().Begin()
	if err == nil {
		_, err = tx.Exec(`
			UPDATE recordings SET
				conversation_title = COALESCE(
					(SELECT title FROM conversations WHERE id = ? AND owner = ?),
					conversation_title
				),
				status = ?, chunk_count = ?, byte_size = ?, duration_ms = ?,
				error_message = ?, completed_at = CURRENT_TIMESTAMP,
				updated_at = CURRENT_TIMESTAMP
			WHERE id = ? AND owner = ?`,
			item.ConversationID, strings.TrimSpace(owner), status, assembledChunks,
			assembledBytes, input.DurationMS, input.ErrorMessage, item.ID, strings.TrimSpace(owner),
		)
	}
	if err == nil {
		_, err = tx.Exec(`DELETE FROM recording_messages WHERE recording_id = ?`, item.ID)
	}
	if err == nil {
		_, err = tx.Exec(`
			INSERT INTO recording_messages (recording_id, client_id, role, content, created_at, updated_at)
			SELECT ?, client_id, role, substr(content, 1, ?), created_at, updated_at
			FROM (
				SELECT id, client_id, role, content, created_at, updated_at
				FROM conversation_messages
				WHERE conversation_id = ?
				ORDER BY id DESC
				LIMIT ?
			) recent
			ORDER BY id`, item.ID, MaxSnapshotContentChars, item.ConversationID, MaxSnapshotMessages)
	}
	if err == nil {
		err = tx.Commit()
	} else if tx != nil {
		_ = tx.Rollback()
	}
	s.db.Unlock()
	if err != nil {
		return Item{}, fmt.Errorf("complete recording metadata: %w", err)
	}
	_ = os.RemoveAll(s.chunkDir(item.ID))
	s.invalidateStats()
	return s.GetOwned(owner, item.ID)
}

func (s *Store) GetOwned(owner, id string) (Item, error) {
	owner = strings.TrimSpace(owner)
	id = normalizeID(id)
	if id == "" {
		return Item{}, ErrNotFound
	}
	s.db.Lock()
	row := s.db.Conn().QueryRow(`
		SELECT id, owner, conversation_id, conversation_title, voice_session_id,
			mime_type, file_ext, status, chunk_count, byte_size, duration_ms,
			error_message, created_at, updated_at, completed_at,
			(SELECT COUNT(*) FROM recording_messages rm WHERE rm.recording_id = recordings.id)
		FROM recordings WHERE id = ? AND owner = ?`, id, owner)
	item, err := scanItem(row)
	s.db.Unlock()
	return itemOrError(item, err)
}

func (s *Store) GetAdmin(id string) (Detail, error) {
	id = normalizeID(id)
	if id == "" {
		return Detail{}, ErrNotFound
	}
	s.db.Lock()
	item, err := s.getAdminItemUnlocked(id)
	if err != nil {
		s.db.Unlock()
		return Detail{}, err
	}
	messages, err := s.messagesUnlocked(item)
	s.db.Unlock()
	if err != nil {
		return Detail{}, err
	}
	item.AudioAvailable = item.ByteSize > 0
	return Detail{Recording: item, Messages: messages}, nil
}

func (s *Store) List(filter ListFilter) ([]Item, error) {
	_, _ = s.RecoverStale()
	limit := filter.Limit
	if limit <= 0 {
		limit = 200
	}
	if limit > 500 {
		limit = 500
	}
	clauses := []string{"1 = 1"}
	args := make([]any, 0, 5)
	args = append(args, StatusRecording)
	if status := strings.TrimSpace(filter.Status); status != "" && status != "all" {
		clauses = append(clauses, "status = ?")
		args = append(args, status)
	}
	if query := strings.TrimSpace(filter.Query); query != "" {
		like := "%" + query + "%"
		clauses = append(clauses, `(id LIKE ? OR conversation_id LIKE ? OR conversation_title LIKE ? OR voice_session_id LIKE ? OR owner LIKE ?)`)
		args = append(args, like, like, like, like, like)
	}
	args = append(args, limit)
	statement := `
		SELECT id, owner, conversation_id, conversation_title, voice_session_id,
			mime_type, file_ext, status, chunk_count, byte_size, duration_ms,
			error_message, created_at, updated_at, completed_at,
			CASE WHEN recordings.status = ? THEN
				(SELECT COUNT(*) FROM conversation_messages cm WHERE cm.conversation_id = recordings.conversation_id)
			ELSE
				(SELECT COUNT(*) FROM recording_messages rm WHERE rm.recording_id = recordings.id)
			END
		FROM recordings WHERE ` + strings.Join(clauses, " AND ") + `
		ORDER BY created_at DESC, id DESC LIMIT ?`
	s.db.Lock()
	rows, err := s.db.Conn().Query(statement, args...)
	if err != nil {
		s.db.Unlock()
		return nil, fmt.Errorf("list recordings: %w", err)
	}
	items := make([]Item, 0)
	for rows.Next() {
		item, scanErr := scanItem(rows)
		if scanErr != nil {
			_ = rows.Close()
			s.db.Unlock()
			return nil, fmt.Errorf("read recording: %w", scanErr)
		}
		item.AudioAvailable = item.ByteSize > 0
		items = append(items, item)
	}
	err = rows.Err()
	_ = rows.Close()
	s.db.Unlock()
	if err != nil {
		return nil, fmt.Errorf("iterate recordings: %w", err)
	}
	return items, nil
}

func (s *Store) Stats() (Stats, error) {
	_, _ = s.RecoverStale()
	s.statsMu.Lock()
	defer s.statsMu.Unlock()
	if !s.statsCachedAt.IsZero() && time.Since(s.statsCachedAt) < statsCacheTTL {
		return s.statsCache, nil
	}

	s.db.Lock()
	var stats Stats
	err := s.db.Conn().QueryRow(`
		SELECT COUNT(*),
			COALESCE(SUM(CASE WHEN status = 'recording' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN status = 'completed' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN status = 'incomplete' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(byte_size), 0)
		FROM recordings`).Scan(
		&stats.Total, &stats.Recording, &stats.Completed,
		&stats.Incomplete, &stats.Failed, &stats.ByteSize,
	)
	s.db.Unlock()
	if err != nil {
		return Stats{}, fmt.Errorf("recording stats: %w", err)
	}
	s.statsCache = stats
	s.statsCachedAt = time.Now()
	return stats, nil
}

func (s *Store) OpenAudio(id string) (*os.File, Item, error) {
	id = normalizeID(id)
	if id == "" {
		return nil, Item{}, ErrNotFound
	}
	var item Item
	s.db.Lock()
	err := s.db.Conn().QueryRow(`
		SELECT id, mime_type, file_ext, status, byte_size
		FROM recordings WHERE id = ?`, id,
	).Scan(&item.ID, &item.MIMEType, &item.FileExt, &item.Status, &item.ByteSize)
	s.db.Unlock()
	if errors.Is(err, sql.ErrNoRows) {
		return nil, Item{}, ErrNotFound
	}
	if err != nil {
		return nil, Item{}, fmt.Errorf("get recording audio metadata: %w", err)
	}
	item.AudioAvailable = item.ByteSize > 0
	if !item.AudioAvailable {
		return nil, Item{}, ErrNotFound
	}
	file, err := os.Open(s.audioPath(item.ID, item.FileExt))
	if errors.Is(err, os.ErrNotExist) {
		return nil, Item{}, ErrNotFound
	}
	if err != nil {
		return nil, Item{}, fmt.Errorf("open recording audio: %w", err)
	}
	return file, item, nil
}

func (s *Store) getAdminItemUnlocked(id string) (Item, error) {
	row := s.db.Conn().QueryRow(`
		SELECT id, owner, conversation_id, conversation_title, voice_session_id,
			mime_type, file_ext, status, chunk_count, byte_size, duration_ms,
			error_message, created_at, updated_at, completed_at,
			(SELECT COUNT(*) FROM recording_messages rm WHERE rm.recording_id = recordings.id)
		FROM recordings WHERE id = ?`, id)
	item, err := scanItem(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Item{}, ErrNotFound
	}
	if err != nil {
		return Item{}, fmt.Errorf("get recording: %w", err)
	}
	item.AudioAvailable = item.ByteSize > 0
	return item, nil
}

func (s *Store) Delete(id string) error {
	id = normalizeID(id)
	if id == "" {
		return ErrNotFound
	}
	unlock := s.lockRecording(id)
	defer unlock()
	s.db.Lock()
	var ext string
	err := s.db.Conn().QueryRow(`SELECT file_ext FROM recordings WHERE id = ?`, id).Scan(&ext)
	s.db.Unlock()
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("find recording for delete: %w", err)
	}
	if err := os.Remove(s.audioPath(id, ext)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("delete recording audio: %w", err)
	}
	if err := os.RemoveAll(s.chunkDir(id)); err != nil {
		return fmt.Errorf("delete recording chunks: %w", err)
	}
	s.db.Lock()
	result, err := s.db.Conn().Exec(`DELETE FROM recordings WHERE id = ?`, id)
	s.db.Unlock()
	if err != nil {
		return fmt.Errorf("delete recording metadata: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check deleted recording: %w", err)
	}
	if count == 0 {
		return ErrNotFound
	}
	s.invalidateStats()
	return nil
}

func (s *Store) messagesUnlocked(item Item) ([]Message, error) {
	statement := `
		SELECT client_id, role, content, created_at, updated_at
		FROM (
			SELECT id, client_id, role, substr(content, 1, ?) AS content, created_at, updated_at
			FROM recording_messages
			WHERE recording_id = ?
			ORDER BY id DESC
			LIMIT ?
		) recent
		ORDER BY id`
	args := []any{MaxSnapshotContentChars, item.ID, MaxSnapshotMessages}
	if item.Status == StatusRecording && item.MessageCount == 0 {
		statement = `
			SELECT client_id, role, content, created_at, updated_at
			FROM (
				SELECT id, client_id, role, substr(content, 1, ?) AS content, created_at, updated_at
				FROM conversation_messages
				WHERE conversation_id = ?
				ORDER BY id DESC
				LIMIT ?
			) recent
			ORDER BY id`
		args = []any{MaxSnapshotContentChars, item.ConversationID, MaxSnapshotMessages}
	}
	rows, err := s.db.Conn().Query(statement, args...)
	if err != nil {
		return nil, fmt.Errorf("list recording messages: %w", err)
	}
	defer rows.Close()
	messages := make([]Message, 0)
	for rows.Next() {
		var message Message
		if err := rows.Scan(&message.ClientID, &message.Role, &message.Content, &message.CreatedAt, &message.UpdatedAt); err != nil {
			return nil, fmt.Errorf("read recording message: %w", err)
		}
		messages = append(messages, message)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate recording messages: %w", err)
	}
	return messages, nil
}

// RecoverInterrupted finalizes every recording left active by an earlier
// process. No browser upload can survive a server restart, so these rows are
// authoritative orphans and must not remain undeletable forever.
func (s *Store) RecoverInterrupted() (int, error) {
	if err := s.cleanupTemporaryFiles(); err != nil {
		return 0, err
	}
	return s.recoverActive(true)
}

// RecoverStale finalizes live-process orphans whose chunk directory has not
// changed for a conservative interval. The sweep is throttled because Create,
// List, and Stats may all call it.
func (s *Store) RecoverStale() (int, error) {
	now := time.Now()
	s.sweepMu.Lock()
	if !s.lastSweep.IsZero() && now.Sub(s.lastSweep) < staleSweepInterval {
		s.sweepMu.Unlock()
		return 0, nil
	}
	s.lastSweep = now
	s.sweepMu.Unlock()
	return s.recoverActive(false)
}

func (s *Store) recoverActive(all bool) (int, error) {
	type activeRecording struct {
		id    string
		owner string
	}
	s.db.Lock()
	rows, err := s.db.Conn().Query(`SELECT id, owner FROM recordings WHERE status = ?`, StatusRecording)
	if err != nil {
		s.db.Unlock()
		return 0, fmt.Errorf("list interrupted recordings: %w", err)
	}
	active := make([]activeRecording, 0)
	for rows.Next() {
		var item activeRecording
		if scanErr := rows.Scan(&item.id, &item.owner); scanErr != nil {
			_ = rows.Close()
			s.db.Unlock()
			return 0, fmt.Errorf("read interrupted recording: %w", scanErr)
		}
		active = append(active, item)
	}
	err = rows.Err()
	_ = rows.Close()
	s.db.Unlock()
	if err != nil {
		return 0, fmt.Errorf("iterate interrupted recordings: %w", err)
	}

	recovered := 0
	var firstErr error
	for _, item := range active {
		if !all && !s.recordingIsStale(item.id, time.Now()) {
			continue
		}
		if recoveryErr := s.failInterrupted(item.owner, item.id); recoveryErr != nil {
			if firstErr == nil {
				firstErr = recoveryErr
			}
			continue
		}
		recovered++
	}
	return recovered, firstErr
}

func (s *Store) recordingIsStale(id string, now time.Time) bool {
	info, err := os.Stat(s.chunkDir(id))
	if errors.Is(err, os.ErrNotExist) {
		return true
	}
	if err != nil {
		return false
	}
	return now.Sub(info.ModTime()) >= staleRecordingAfter
}

func (s *Store) failInterrupted(owner, id string) error {
	chunkCount, err := s.interruptedChunkCount(id)
	if err != nil {
		return err
	}
	_, err = s.Complete(owner, id, CompleteInput{
		ChunkCount:   chunkCount,
		Failed:       true,
		ErrorMessage: "recording interrupted before completion",
	})
	if err != nil {
		return fmt.Errorf("finalize interrupted recording: %w", err)
	}
	return nil
}

func (s *Store) interruptedChunkCount(id string) (int, error) {
	entries, err := os.ReadDir(s.chunkDir(id))
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("list interrupted recording chunks: %w", err)
	}
	highest := -1
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".part") {
			continue
		}
		sequence, parseErr := strconv.Atoi(strings.TrimSuffix(entry.Name(), ".part"))
		if parseErr == nil && sequence >= 0 && sequence < MaxRecordingChunks && sequence > highest {
			highest = sequence
		}
	}
	return highest + 1, nil
}

func (s *Store) cleanupTemporaryFiles() error {
	entries, err := os.ReadDir(s.baseDir)
	if err != nil {
		return fmt.Errorf("list recording directory: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.Contains(entry.Name(), ".assemble-") {
			if err := os.Remove(filepath.Join(s.baseDir, entry.Name())); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("remove interrupted recording assembly: %w", err)
			}
		}
	}

	active := make(map[string]struct{})
	s.db.Lock()
	rows, err := s.db.Conn().Query(`SELECT id FROM recordings WHERE status = ?`, StatusRecording)
	if err == nil {
		for rows.Next() {
			var id string
			if scanErr := rows.Scan(&id); scanErr != nil {
				err = scanErr
				break
			}
			active[id] = struct{}{}
		}
		if err == nil {
			err = rows.Err()
		}
		_ = rows.Close()
	}
	s.db.Unlock()
	if err != nil {
		return fmt.Errorf("list active recording chunks: %w", err)
	}

	chunkRoot := filepath.Join(s.baseDir, "chunks")
	chunkDirs, err := os.ReadDir(chunkRoot)
	if err != nil {
		return fmt.Errorf("list recording chunk directories: %w", err)
	}
	for _, chunkDir := range chunkDirs {
		path := filepath.Join(chunkRoot, chunkDir.Name())
		if !chunkDir.IsDir() {
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("remove stray recording chunk file: %w", err)
			}
			continue
		}
		if _, ok := active[chunkDir.Name()]; !ok {
			if err := os.RemoveAll(path); err != nil {
				return fmt.Errorf("remove finalized recording chunks: %w", err)
			}
			continue
		}
		files, readErr := os.ReadDir(path)
		if readErr != nil {
			return fmt.Errorf("list active recording chunks: %w", readErr)
		}
		for _, file := range files {
			if file.IsDir() || (!strings.HasPrefix(file.Name(), ".upload-") && !strings.HasPrefix(file.Name(), ".usage-")) {
				continue
			}
			if err := os.Remove(filepath.Join(path, file.Name())); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("remove interrupted recording upload: %w", err)
			}
		}
	}
	return nil
}

func (s *Store) lockRecording(id string) func() {
	s.operationMu.Lock()
	operation := s.operations[id]
	if operation == nil {
		operation = &recordingOperation{}
		s.operations[id] = operation
	}
	operation.refs++
	s.operationMu.Unlock()

	operation.mu.Lock()
	return func() {
		operation.mu.Unlock()
		s.operationMu.Lock()
		operation.refs--
		if operation.refs == 0 {
			delete(s.operations, id)
		}
		s.operationMu.Unlock()
	}
}

func (s *Store) ensureStorageReserve(required int64) error {
	release, err := s.reserveStorage(required)
	if err != nil {
		return err
	}
	release()
	return nil
}

func (s *Store) reserveStorage(required int64) (func(), error) {
	if required < 0 {
		required = 0
	}
	s.storageMu.Lock()
	defer s.storageMu.Unlock()
	var stat syscall.Statfs_t
	if err := syscall.Statfs(s.baseDir, &stat); err != nil {
		return nil, fmt.Errorf("inspect recording storage: %w", err)
	}
	available := int64(stat.Bavail) * int64(stat.Bsize)
	if available < required || s.storageReserved > available-required ||
		available-required-s.storageReserved < MinRecordingFreeBytes {
		return nil, ErrStorageFull
	}
	s.storageReserved += required
	var once sync.Once
	return func() {
		once.Do(func() {
			s.storageMu.Lock()
			s.storageReserved -= required
			if s.storageReserved < 0 {
				s.storageReserved = 0
			}
			s.storageMu.Unlock()
		})
	}, nil
}

func acquireRecordingSlot(slots chan struct{}) (func(), error) {
	select {
	case slots <- struct{}{}:
		return func() { <-slots }, nil
	default:
		return nil, ErrCapacity
	}
}

func (s *Store) usagePath(id string) string {
	return filepath.Join(s.chunkDir(id), ".bytes")
}

func (s *Store) readChunkUsage(id string) (int64, error) {
	raw, err := os.ReadFile(s.usagePath(id))
	if err == nil {
		value, parseErr := strconv.ParseInt(strings.TrimSpace(string(raw)), 10, 64)
		if parseErr == nil && value >= 0 {
			return value, nil
		}
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return 0, fmt.Errorf("read recording usage: %w", err)
	}
	entries, err := os.ReadDir(s.chunkDir(id))
	if err != nil {
		return 0, fmt.Errorf("list recording chunks: %w", err)
	}
	var total int64
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".part") {
			continue
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			return 0, fmt.Errorf("inspect recording usage: %w", infoErr)
		}
		total += info.Size()
	}
	return total, nil
}

func (s *Store) writeChunkUsage(id string, value int64) error {
	file, err := os.CreateTemp(s.chunkDir(id), ".usage-*")
	if err != nil {
		return fmt.Errorf("create recording usage marker: %w", err)
	}
	tempPath := file.Name()
	_ = file.Chmod(0o600)
	_, writeErr := io.WriteString(file, strconv.FormatInt(value, 10))
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		_ = os.Remove(tempPath)
		if writeErr != nil {
			return fmt.Errorf("write recording usage marker: %w", writeErr)
		}
		return fmt.Errorf("close recording usage marker: %w", closeErr)
	}
	if err := os.Rename(tempPath, s.usagePath(id)); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("publish recording usage marker: %w", err)
	}
	return nil
}

func (s *Store) invalidateStats() {
	s.statsMu.Lock()
	s.statsCachedAt = time.Time{}
	s.statsMu.Unlock()
}

func (s *Store) audioPath(id, ext string) string {
	return filepath.Join(s.baseDir, id+"."+ext)
}

func (s *Store) chunkDir(id string) string {
	return filepath.Join(s.baseDir, "chunks", id)
}

func (s *Store) chunkPath(id string, sequence int) string {
	return filepath.Join(s.chunkDir(id), fmt.Sprintf("%08d.part", sequence))
}

func normalizeMIME(raw string) (string, string, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch {
	case strings.HasPrefix(value, "audio/webm"):
		return rawOr(value, "audio/webm"), "webm", nil
	case strings.HasPrefix(value, "audio/mp4"):
		return rawOr(value, "audio/mp4"), "m4a", nil
	case strings.HasPrefix(value, "audio/ogg"):
		return rawOr(value, "audio/ogg"), "ogg", nil
	default:
		return "", "", &Error{Message: "unsupported recording MIME type"}
	}
}

func rawOr(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return truncate(value, 120)
}

func normalizeID(id string) string {
	id = strings.TrimSpace(id)
	if !strings.HasPrefix(id, "rec_") || len(id) > 80 || strings.ContainsAny(id, "/\\") {
		return ""
	}
	return id
}

func scanItem(scanner storepkg.Scanner) (Item, error) {
	var item Item
	err := scanner.Scan(
		&item.ID, &item.Owner, &item.ConversationID, &item.ConversationTitle,
		&item.VoiceSessionID, &item.MIMEType, &item.FileExt, &item.Status,
		&item.ChunkCount, &item.ByteSize, &item.DurationMS, &item.ErrorMessage,
		&item.CreatedAt, &item.UpdatedAt, &item.CompletedAt, &item.MessageCount,
	)
	item.AudioAvailable = item.ByteSize > 0
	return item, err
}

func itemOrError(item Item, err error) (Item, error) {
	if errors.Is(err, sql.ErrNoRows) {
		return Item{}, ErrNotFound
	}
	if err != nil {
		return Item{}, fmt.Errorf("get recording: %w", err)
	}
	return item, nil
}

func truncate(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) <= limit {
		return value
	}
	return value[:limit]
}

// ChunkSequence parses the strict decimal sequence used by the upload route.
func ChunkSequence(raw string) (int, error) {
	if raw == "" || len(raw) > 5 {
		return 0, &Error{Message: "invalid recording chunk sequence"}
	}
	sequence, err := strconv.Atoi(raw)
	if err != nil || sequence < 0 || sequence >= MaxRecordingChunks {
		return 0, &Error{Message: "invalid recording chunk sequence"}
	}
	return sequence, nil
}
