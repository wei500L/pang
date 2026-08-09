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

	"github.com/google/uuid"

	storepkg "github.com/dyhhhhhh/chatgpt-web-voice/internal/store"
)

const (
	StatusRecording  = "recording"
	StatusCompleted  = "completed"
	StatusIncomplete = "incomplete"
	StatusFailed     = "failed"
)

// Error is a validation error safe to return to an API caller.
type Error = storepkg.Error

// ErrNotFound means the recording is missing or not owned by the caller.
var ErrNotFound = storepkg.ErrNotFound

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
	return &Store{db: db, baseDir: baseDir}, nil
}

func (s *Store) Create(owner string, input CreateInput) (Item, error) {
	owner = strings.TrimSpace(owner)
	input.ConversationID = strings.TrimSpace(input.ConversationID)
	input.VoiceSessionID = truncate(strings.TrimSpace(input.VoiceSessionID), 160)
	mimeType, ext, err := normalizeMIME(input.MIMEType)
	if err != nil {
		return Item{}, err
	}
	if owner == "" || input.ConversationID == "" {
		return Item{}, &Error{Message: "conversation_id is required"}
	}

	s.db.Lock()
	var title string
	err = s.db.Conn().QueryRow(
		`SELECT title FROM conversations WHERE id = ? AND owner = ?`,
		input.ConversationID, owner,
	).Scan(&title)
	s.db.Unlock()
	if errors.Is(err, sql.ErrNoRows) {
		return Item{}, ErrNotFound
	}
	if err != nil {
		return Item{}, fmt.Errorf("find recording conversation: %w", err)
	}

	id := "rec_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	chunkDir := s.chunkDir(id)
	if err := os.MkdirAll(chunkDir, 0o700); err != nil {
		return Item{}, fmt.Errorf("create recording chunk directory: %w", err)
	}

	s.db.Lock()
	_, err = s.db.Conn().Exec(`
		INSERT INTO recordings (
			id, owner, conversation_id, conversation_title, voice_session_id,
			mime_type, file_ext, status
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, owner, input.ConversationID, truncate(title, 120), input.VoiceSessionID,
		mimeType, ext, StatusRecording,
	)
	s.db.Unlock()
	if err != nil {
		_ = os.RemoveAll(chunkDir)
		return Item{}, fmt.Errorf("create recording: %w", err)
	}
	return s.GetOwned(owner, id)
}

func (s *Store) AddChunk(owner, id string, sequence int, reader io.Reader) (Item, error) {
	if sequence < 0 || sequence > 100000 {
		return Item{}, &Error{Message: "invalid recording chunk sequence"}
	}
	item, err := s.GetOwned(owner, id)
	if err != nil {
		return Item{}, err
	}
	if item.Status != StatusRecording {
		return Item{}, &Error{Message: "recording is already finalized"}
	}
	if err := os.MkdirAll(s.chunkDir(item.ID), 0o700); err != nil {
		return Item{}, fmt.Errorf("create recording chunk directory: %w", err)
	}
	path := s.chunkPath(item.ID, sequence)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		return item, nil // idempotent retry after a lost HTTP response
	}
	if err != nil {
		return Item{}, fmt.Errorf("create recording chunk: %w", err)
	}
	written, copyErr := io.Copy(file, reader)
	closeErr := file.Close()
	if copyErr != nil || closeErr != nil || written == 0 {
		_ = os.Remove(path)
		if copyErr != nil {
			return Item{}, fmt.Errorf("write recording chunk: %w", copyErr)
		}
		if closeErr != nil {
			return Item{}, fmt.Errorf("close recording chunk: %w", closeErr)
		}
		return Item{}, &Error{Message: "recording chunk is empty"}
	}

	s.db.Lock()
	_, err = s.db.Conn().Exec(`
		UPDATE recordings SET
			chunk_count = chunk_count + 1,
			byte_size = byte_size + ?,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND owner = ?`, written, item.ID, strings.TrimSpace(owner))
	s.db.Unlock()
	if err != nil {
		return Item{}, fmt.Errorf("update recording chunk metadata: %w", err)
	}
	return s.GetOwned(owner, item.ID)
}

func (s *Store) Complete(owner, id string, input CompleteInput) (Item, error) {
	item, err := s.GetOwned(owner, id)
	if err != nil {
		return Item{}, err
	}
	if item.Status != StatusRecording {
		return item, nil // idempotent completion retry after a lost response
	}
	if input.ChunkCount < 0 || input.ChunkCount > 100000 {
		return Item{}, &Error{Message: "invalid recording chunk count"}
	}
	if input.DurationMS < 0 {
		input.DurationMS = 0
	}
	input.VoiceSessionID = truncate(strings.TrimSpace(input.VoiceSessionID), 160)
	input.ErrorMessage = truncate(strings.TrimSpace(input.ErrorMessage), 500)

	status := StatusCompleted
	if input.Failed {
		status = StatusIncomplete
	}
	finalPath := s.audioPath(item.ID, item.FileExt)
	tempPath := finalPath + ".tmp"
	_ = os.Remove(tempPath)
	out, err := os.OpenFile(tempPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return Item{}, fmt.Errorf("create assembled recording: %w", err)
	}
	var assembledBytes int64
	assembledChunks := 0
	for sequence := 0; sequence < input.ChunkCount; sequence++ {
		chunk, openErr := os.Open(s.chunkPath(item.ID, sequence))
		if openErr != nil {
			status = StatusIncomplete
			if input.ErrorMessage == "" {
				input.ErrorMessage = "one or more audio chunks are missing"
			}
			break
		}
		n, copyErr := io.Copy(out, chunk)
		_ = chunk.Close()
		if copyErr != nil {
			_ = out.Close()
			_ = os.Remove(tempPath)
			return Item{}, fmt.Errorf("assemble recording chunk: %w", copyErr)
		}
		assembledBytes += n
		assembledChunks++
	}
	if closeErr := out.Close(); closeErr != nil {
		_ = os.Remove(tempPath)
		return Item{}, fmt.Errorf("close assembled recording: %w", closeErr)
	}
	if assembledBytes == 0 {
		_ = os.Remove(tempPath)
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
				voice_session_id = CASE WHEN ? <> '' THEN ? ELSE voice_session_id END,
				conversation_title = COALESCE(
					(SELECT title FROM conversations WHERE id = ? AND owner = ?),
					conversation_title
				),
				status = ?, chunk_count = ?, byte_size = ?, duration_ms = ?,
				error_message = ?, completed_at = CURRENT_TIMESTAMP,
				updated_at = CURRENT_TIMESTAMP
			WHERE id = ? AND owner = ?`,
			input.VoiceSessionID, input.VoiceSessionID, item.ConversationID, strings.TrimSpace(owner), status, assembledChunks,
			assembledBytes, input.DurationMS, input.ErrorMessage, item.ID, strings.TrimSpace(owner),
		)
	}
	if err == nil {
		_, err = tx.Exec(`DELETE FROM recording_messages WHERE recording_id = ?`, item.ID)
	}
	if err == nil {
		_, err = tx.Exec(`
			INSERT INTO recording_messages (recording_id, client_id, role, content, created_at, updated_at)
			SELECT ?, client_id, role, content, created_at, updated_at
			FROM conversation_messages WHERE conversation_id = ? ORDER BY id`, item.ID, item.ConversationID)
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
	row := s.db.Conn().QueryRow(`
		SELECT id, owner, conversation_id, conversation_title, voice_session_id,
			mime_type, file_ext, status, chunk_count, byte_size, duration_ms,
			error_message, created_at, updated_at, completed_at,
			(SELECT COUNT(*) FROM recording_messages rm WHERE rm.recording_id = recordings.id)
		FROM recordings WHERE id = ?`, id)
	item, err := scanItem(row)
	if err != nil {
		s.db.Unlock()
		if errors.Is(err, sql.ErrNoRows) {
			return Detail{}, ErrNotFound
		}
		return Detail{}, fmt.Errorf("get recording: %w", err)
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
	limit := filter.Limit
	if limit <= 0 {
		limit = 200
	}
	if limit > 500 {
		limit = 500
	}
	clauses := []string{"1 = 1"}
	args := make([]any, 0, 4)
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
			CASE
				WHEN (SELECT COUNT(*) FROM recording_messages rm WHERE rm.recording_id = recordings.id) > 0
				THEN (SELECT COUNT(*) FROM recording_messages rm WHERE rm.recording_id = recordings.id)
				ELSE (SELECT COUNT(*) FROM conversation_messages cm WHERE cm.conversation_id = recordings.conversation_id)
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
	s.db.Lock()
	defer s.db.Unlock()
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
	if err != nil {
		return Stats{}, fmt.Errorf("recording stats: %w", err)
	}
	return stats, nil
}

func (s *Store) OpenAudio(id string) (*os.File, Item, error) {
	detail, err := s.GetAdmin(id)
	if err != nil {
		return nil, Item{}, err
	}
	if !detail.Recording.AudioAvailable {
		return nil, Item{}, ErrNotFound
	}
	file, err := os.Open(s.audioPath(detail.Recording.ID, detail.Recording.FileExt))
	if errors.Is(err, os.ErrNotExist) {
		return nil, Item{}, ErrNotFound
	}
	if err != nil {
		return nil, Item{}, fmt.Errorf("open recording audio: %w", err)
	}
	return file, detail.Recording, nil
}

func (s *Store) Delete(id string) error {
	id = normalizeID(id)
	if id == "" {
		return ErrNotFound
	}
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
	return nil
}

func (s *Store) messagesUnlocked(item Item) ([]Message, error) {
	statement := `
		SELECT client_id, role, content, created_at, updated_at
		FROM recording_messages WHERE recording_id = ? ORDER BY id`
	args := []any{item.ID}
	if item.MessageCount == 0 {
		statement = `
			SELECT client_id, role, content, created_at, updated_at
			FROM conversation_messages WHERE conversation_id = ? ORDER BY id`
		args = []any{item.ConversationID}
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
	if raw == "" || len(raw) > 6 {
		return 0, &Error{Message: "invalid recording chunk sequence"}
	}
	sequence, err := strconv.Atoi(raw)
	if err != nil || sequence < 0 || sequence > 100000 {
		return 0, &Error{Message: "invalid recording chunk sequence"}
	}
	return sequence, nil
}
