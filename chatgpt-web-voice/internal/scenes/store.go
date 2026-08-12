package scenes

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"

	storepkg "github.com/dyhhhhhh/chatgpt-web-voice/internal/store"
)

const sceneSelectColumns = `id, conversation_id, COALESCE(parent_scene_id, ''), owner, mode, status,
	approved_summary, tensions_json, COALESCE(culture_lens, ''),
	candidates_json, selected_candidate_json, scene_brief_json,
	caption, micro_action, disclaimer, prompt_version, provider, model,
	COALESCE(image_path, ''), COALESCE(image_mime, ''), COALESCE(image_width, 0),
	COALESCE(image_height, 0), error_message, COALESCE(blocked_reason, ''),
	COALESCE(risk_flags, ''), COALESCE(generation_attempt, 0),
	created_at, updated_at, COALESCE(completed_at, '')`

// Store is the SQLite-backed scene repository. Generated image bytes are never
// stored here; the store only keeps the file path for the service to manage.
type Store struct {
	db *storepkg.DB
}

// NewStore wraps an already-opened store database.
func NewStore(db *storepkg.DB) *Store {
	return &Store{db: db}
}

// Create inserts a new scene project row. All JSON payloads are serialized
// from Go structs here; the API layer never receives raw client JSON.
func (s *Store) Create(project Project) (Project, error) {
	project.ID = strings.TrimSpace(project.ID)
	project.Owner = normalizeOwner(project.Owner)
	project.ConversationID = strings.TrimSpace(project.ConversationID)
	if project.ID == "" {
		project.ID = "scn_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	}
	project.Status = normalizeStatus(project.Status)
	if project.Status == "" {
		project.Status = StatusDraft
	}
	project.Mode = normalizeMode(project.Mode)
	project.ApprovedSummary = truncateRunes(project.ApprovedSummary, MaxApprovedSummaryRunes)
	project.CultureLens = truncateText(project.CultureLens, MaxLensRunes)
	project.Tensions = truncateStringList(project.Tensions, MaxTensionRunes)
	tensionsJSON, err := marshalTensions(project.Tensions)
	if err != nil {
		return Project{}, err
	}
	candidatesJSON, err := marshalCandidates(project.Candidates)
	if err != nil {
		return Project{}, err
	}
	selectedJSON, err := marshalCandidate(project.SelectedCandidate)
	if err != nil {
		return Project{}, err
	}
	briefJSON, err := marshalBrief(project.sceneBrief)
	if err != nil {
		return Project{}, err
	}

	s.db.Lock()
	defer s.db.Unlock()
	if _, err := s.db.Conn().Exec(`
		INSERT INTO scene_projects (
			id, conversation_id, parent_scene_id, owner, mode, status,
			approved_summary, tensions_json, culture_lens,
			candidates_json, selected_candidate_json, scene_brief_json,
			caption, micro_action, disclaimer, prompt_version, provider, model,
			error_message, blocked_reason, risk_flags, generation_attempt
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		project.ID, project.ConversationID, truncateText(project.ParentSceneID, 128), project.Owner, project.Mode, project.Status,
		project.ApprovedSummary, tensionsJSON, project.CultureLens,
		candidatesJSON, selectedJSON, briefJSON,
		truncateRunes(project.Caption, MaxCaptionRunes), truncateRunes(project.MicroAction, MaxMicroActionRunes),
		truncateRunes(project.Disclaimer, 200), truncateText(project.PromptVersion, 80),
		truncateText(project.Provider, 80), truncateText(project.Model, 160),
		truncateRunes(project.ErrorMessage, MaxInternalErrorRunes),
		truncateRunes(project.BlockedReason, 300), joinRiskFlags(project.RiskFlags),
		project.GenerationAttempt,
	); err != nil {
		return Project{}, fmt.Errorf("create scene project: %w", err)
	}
	return s.getUnlocked(project.Owner, project.ID)
}

// GetOwned returns one scene filtered by owner.
func (s *Store) GetOwned(owner, id string) (Project, error) {
	owner = normalizeOwner(owner)
	id = normalizeID(id)
	if id == "" {
		return Project{}, ErrNotFound
	}
	s.db.Lock()
	defer s.db.Unlock()
	return s.getUnlocked(owner, id)
}

// ListByConversation returns all scenes of one owned conversation, newest
// first. All reads are filtered by owner.
func (s *Store) ListByConversation(owner, conversationID string) ([]Project, error) {
	owner = normalizeOwner(owner)
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		return nil, &Error{Message: "conversation_id is required"}
	}
	s.db.Lock()
	defer s.db.Unlock()
	rows, err := s.db.Conn().Query(
		"SELECT "+sceneSelectColumns+" FROM scene_projects WHERE owner = ? AND conversation_id = ? ORDER BY updated_at DESC, id DESC LIMIT 200",
		owner, conversationID,
	)
	if err != nil {
		return nil, fmt.Errorf("list scene projects: %w", err)
	}
	defer rows.Close()
	items := make([]Project, 0, 8)
	for rows.Next() {
		item, scanErr := scanProject(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("read scene project: %w", scanErr)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate scene projects: %w", err)
	}
	return items, nil
}

// GetUnscoped loads a scene by id for the internal worker. The worker only
// ever consumes ids it enqueued itself.
func (s *Store) GetUnscoped(id string) (Project, error) {
	id = normalizeID(id)
	if id == "" {
		return Project{}, ErrNotFound
	}
	s.db.Lock()
	defer s.db.Unlock()
	row := s.db.Conn().QueryRow("SELECT "+sceneSelectColumns+" FROM scene_projects WHERE id = ?", id)
	project, err := scanProject(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Project{}, ErrNotFound
	}
	if err != nil {
		return Project{}, fmt.Errorf("get scene project for worker: %w", err)
	}
	return project, nil
}

// UpdateDraftContent persists the user-edited approved summary and the
// selected candidate (resolved server-side from the stored candidates list).
func (s *Store) UpdateDraftContent(owner, id string, summary string, selected *Candidate) (Project, error) {
	owner = normalizeOwner(owner)
	id = normalizeID(id)
	if id == "" {
		return Project{}, ErrNotFound
	}
	summary = truncateRunes(summary, MaxApprovedSummaryRunes)
	selectedJSON, err := marshalCandidate(selected)
	if err != nil {
		return Project{}, err
	}
	s.db.Lock()
	defer s.db.Unlock()
	result, err := s.db.Conn().Exec(`
		UPDATE scene_projects SET
			approved_summary = ?, selected_candidate_json = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND owner = ? AND status IN (?, ?, ?)`,
		summary, selectedJSON, id, owner, StatusDraft, StatusFailed, StatusCompleted,
	)
	if err != nil {
		return Project{}, fmt.Errorf("update scene draft: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return Project{}, fmt.Errorf("check updated scene draft: %w", err)
	}
	if count == 0 {
		// Distinguish "not found" from "wrong state" for the handler.
		if _, getErr := s.getUnlocked(owner, id); getErr != nil {
			return Project{}, getErr
		}
		return Project{}, &Error{Message: "scene can only be edited before generation"}
	}
	return s.getUnlocked(owner, id)
}

// PrepareGeneration atomically moves a scene from draft/failed to queued while
// recording provider metadata. It returns false when the scene is already
// active or missing, so the queue can never double-start.
func (s *Store) PrepareGeneration(owner, id string, provider, model, promptVersion string) (bool, Project, error) {
	owner = normalizeOwner(owner)
	id = normalizeID(id)
	if id == "" {
		return false, Project{}, ErrNotFound
	}
	s.db.Lock()
	defer s.db.Unlock()
	result, err := s.db.Conn().Exec(`
		UPDATE scene_projects SET
			provider = ?, model = ?, prompt_version = ?,
			status = ?, generation_attempt = generation_attempt + 1,
			error_message = '', completed_at = '', updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND owner = ? AND status IN (?, ?, ?)`,
		truncateText(provider, 80), truncateText(model, 160), truncateText(promptVersion, 80),
		StatusQueued, id, owner, StatusDraft, StatusFailed, StatusCompleted,
	)
	if err != nil {
		return false, Project{}, fmt.Errorf("prepare scene generation: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return false, Project{}, fmt.Errorf("check prepared scene generation: %w", err)
	}
	if count == 0 {
		if _, getErr := s.getUnlocked(owner, id); errors.Is(getErr, ErrNotFound) {
			return false, Project{}, getErr
		}
		return false, Project{}, ErrBusy
	}
	project, err := s.getUnlocked(owner, id)
	return true, project, err
}

// SaveBrief persists the composed SceneBrief and its provenance metadata while
// the job is in the composing phase. A stale row (no longer composing) is a
// state conflict and returns an error so the worker stops the job.
func (s *Store) SaveBrief(owner, id string, brief SceneBrief, provider, model, promptVersion string) error {
	owner = normalizeOwner(owner)
	id = normalizeID(id)
	if id == "" {
		return ErrNotFound
	}
	briefJSON, err := marshalBrief(brief)
	if err != nil {
		return err
	}
	s.db.Lock()
	defer s.db.Unlock()
	result, err := s.db.Conn().Exec(`
		UPDATE scene_projects SET
			scene_brief_json = ?, provider = ?, model = ?, prompt_version = ?,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND owner = ? AND status = ?`,
		briefJSON, truncateText(provider, 80), truncateText(model, 160), truncateText(promptVersion, 80),
		id, owner, StatusComposing,
	)
	if err != nil {
		return fmt.Errorf("save scene brief: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check saved scene brief: %w", err)
	}
	if count == 0 {
		return fmt.Errorf("save scene brief: %w", ErrStateConflict)
	}
	return nil
}

// RevertGeneration moves a queued scene back to draft after a failed enqueue.
func (s *Store) RevertGeneration(owner, id string) error {
	owner = normalizeOwner(owner)
	id = normalizeID(id)
	if id == "" {
		return ErrNotFound
	}
	s.db.Lock()
	defer s.db.Unlock()
	if _, err := s.db.Conn().Exec(`
		UPDATE scene_projects SET status = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND owner = ? AND status = ?`,
		StatusDraft, id, owner, StatusQueued,
	); err != nil {
		return fmt.Errorf("revert scene generation: %w", err)
	}
	return nil
}

// AdvanceGeneration is the worker's CAS between queued/composing/generating.
// A zero-row update is a normal CAS miss (another actor already moved or
// removed the row) and returns false with a nil error; SQL execution or
// RowsAffected failures are returned as wrapped internal errors so database
// faults are never mistaken for state conflicts.
func (s *Store) AdvanceGeneration(owner, id, from, to string) (bool, error) {
	owner = normalizeOwner(owner)
	id = normalizeID(id)
	if id == "" {
		return false, nil
	}
	s.db.Lock()
	defer s.db.Unlock()
	result, err := s.db.Conn().Exec(`
		UPDATE scene_projects SET status = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND owner = ? AND status = ?`,
		to, id, owner, from,
	)
	if err != nil {
		return false, fmt.Errorf("advance scene generation: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("check advanced scene generation: %w", err)
	}
	return count > 0, nil
}

// CompleteGeneration is the terminal CAS: only a row still in the generating
// state may move to completed. A zero-row update means the state changed
// underneath the worker (shutdown mark or deletion) and returns ErrStateConflict
// so the caller removes the just-published file.
func (s *Store) CompleteGeneration(owner, id string, imagePath, mime string, width, height int, caption, microAction, disclaimer string) error {
	owner = normalizeOwner(owner)
	id = normalizeID(id)
	if id == "" {
		return ErrNotFound
	}
	s.db.Lock()
	defer s.db.Unlock()
	result, err := s.db.Conn().Exec(`
		UPDATE scene_projects SET
			status = ?, image_path = ?, image_mime = ?, image_width = ?, image_height = ?,
			caption = ?, micro_action = ?, disclaimer = ?,
			error_message = '', completed_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND owner = ? AND status = ?`,
		StatusCompleted,
		truncateText(imagePath, 512), truncateText(mime, 64), width, height,
		truncateRunes(caption, MaxCaptionRunes), truncateRunes(microAction, MaxMicroActionRunes),
		truncateRunes(disclaimer, 200),
		id, owner, StatusGenerating,
	)
	if err != nil {
		return fmt.Errorf("complete scene generation: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check completed scene generation: %w", err)
	}
	if count == 0 {
		return fmt.Errorf("complete scene generation: %w", ErrStateConflict)
	}
	return nil
}

// FailGeneration marks an active job failed. The status guard prevents a late
// worker from rewriting a row that already reached a terminal state
// (completed/blocked/draft) or was deleted; a zero-row update returns
// ErrStateConflict instead of pretending the write succeeded.
func (s *Store) FailGeneration(owner, id, message string) error {
	owner = normalizeOwner(owner)
	id = normalizeID(id)
	if id == "" {
		return ErrNotFound
	}
	s.db.Lock()
	defer s.db.Unlock()
	result, err := s.db.Conn().Exec(`
		UPDATE scene_projects SET
			status = ?, error_message = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND owner = ? AND status IN (?, ?, ?)`,
		StatusFailed, truncateErr(message), id, owner, StatusQueued, StatusComposing, StatusGenerating,
	)
	if err != nil {
		return fmt.Errorf("fail scene generation: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check failed scene generation: %w", err)
	}
	if count == 0 {
		return fmt.Errorf("fail scene generation: %w", ErrStateConflict)
	}
	return nil
}

// IsInStatus reports whether the owned scene is currently in the given status.
// Used by the worker to re-confirm the row before publishing a file. A missing
// row returns false with a nil error; database failures are returned so they
// are not mistaken for a benign state mismatch.
func (s *Store) IsInStatus(owner, id, status string) (bool, error) {
	owner = normalizeOwner(owner)
	id = normalizeID(id)
	if id == "" || status == "" {
		return false, nil
	}
	s.db.Lock()
	defer s.db.Unlock()
	var one int
	err := s.db.Conn().QueryRow(
		"SELECT 1 FROM scene_projects WHERE id = ? AND owner = ? AND status = ? LIMIT 1",
		id, owner, status,
	).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check scene status: %w", err)
	}
	return true, nil
}

// MarkInterruptedOnStartup fails every scene left queued/composing/generating
// by an earlier process. External requests cannot be resumed across restarts.
func (s *Store) MarkInterruptedOnStartup() (int, error) {
	s.db.Lock()
	defer s.db.Unlock()
	result, err := s.db.Conn().Exec(`
		UPDATE scene_projects SET
			status = ?, error_message = ?, updated_at = CURRENT_TIMESTAMP
		WHERE status IN (?, ?, ?)`,
		StatusFailed, "generation interrupted by server restart", StatusQueued, StatusComposing, StatusGenerating,
	)
	if err != nil {
		return 0, fmt.Errorf("mark interrupted scene generation: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("check interrupted scene generation: %w", err)
	}
	return int(count), nil
}

// OpenImage returns the generated image file for an owner-scoped completed
// scene. The real path is never exposed to callers, and open failures are
// sanitized so the underlying *os.PathError (which contains the absolute local
// path) never reaches logs or public responses.
func (s *Store) OpenImage(owner, id string) (*os.File, Project, error) {
	project, err := s.GetOwned(owner, id)
	if err != nil {
		return nil, Project{}, err
	}
	if project.Status != StatusCompleted || project.imagePath == "" {
		return nil, Project{}, ErrNotFound
	}
	file, err := os.Open(project.imagePath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, Project{}, ErrNotFound
	}
	if err != nil {
		// Deliberately not wrapped with %w: the wrapped *os.PathError would
		// carry the absolute image path into the error chain that gets logged.
		return nil, Project{}, errors.New("scene image is unavailable")
	}
	return file, project, nil
}

// DeleteOwned removes one scene and returns its image path for cleanup.
// Active scenes (queued/composing/generating) cannot be deleted: the worker
// may still be writing their image file, and deleting the row first would turn
// that file into a permanent orphan. The status read and the row deletion are
// serialized under the same store lock the worker uses for its state
// transitions, so no active job can slip in between.
func (s *Store) DeleteOwned(owner, id string) (string, error) {
	owner = normalizeOwner(owner)
	id = normalizeID(id)
	if id == "" {
		return "", ErrNotFound
	}
	s.db.Lock()
	defer s.db.Unlock()
	var status, imagePath string
	err := s.db.Conn().QueryRow(
		"SELECT status, COALESCE(image_path, '') FROM scene_projects WHERE id = ? AND owner = ?",
		id, owner,
	).Scan(&status, &imagePath)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("read scene for delete: %w", err)
	}
	if status == StatusQueued || status == StatusComposing || status == StatusGenerating {
		return "", ErrBusy
	}
	if _, err := s.db.Conn().Exec("DELETE FROM scene_projects WHERE id = ? AND owner = ?", id, owner); err != nil {
		return "", fmt.Errorf("delete scene project: %w", err)
	}
	return strings.TrimSpace(imagePath), nil
}

// DeleteConversation transactionally removes one owned conversation and all of
// its scene metadata (plus messages via the FK cascade) in a single SQLite
// transaction, returning the collected image paths so the service can remove
// the files after the commit. The whole operation holds the shared store lock,
// so no scene can be created or started, and no worker transition can
// interleave, between the active check and the row deletion.
//
// Refusals:
//   - conversation missing or not owned → ErrNotFound (unchanged 404 behavior);
//   - any scene still queued/composing/generating → ErrBusy (409): its worker
//     could otherwise publish an image file after the rows are gone.
func (s *Store) DeleteConversation(owner, conversationID string) ([]string, error) {
	owner = normalizeOwner(owner)
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		return nil, &Error{Message: "conversation_id is required"}
	}
	s.db.Lock()
	defer s.db.Unlock()

	tx, err := s.db.Conn().Begin()
	if err != nil {
		return nil, fmt.Errorf("begin conversation delete: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var one int
	err = tx.QueryRow(
		"SELECT 1 FROM conversations WHERE id = ? AND owner = ? LIMIT 1",
		conversationID, owner,
	).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("check conversation for delete: %w", err)
	}

	err = tx.QueryRow(`
		SELECT 1 FROM scene_projects
		WHERE owner = ? AND conversation_id = ? AND status IN (?, ?, ?)
		LIMIT 1`,
		owner, conversationID, StatusQueued, StatusComposing, StatusGenerating,
	).Scan(&one)
	if err == nil {
		return nil, ErrBusy
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("check active conversation scenes: %w", err)
	}

	rows, err := tx.Query(
		"SELECT COALESCE(image_path, '') FROM scene_projects WHERE owner = ? AND conversation_id = ?",
		owner, conversationID,
	)
	if err != nil {
		return nil, fmt.Errorf("list scene images for cleanup: %w", err)
	}
	var paths []string
	for rows.Next() {
		var path string
		if scanErr := rows.Scan(&path); scanErr != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("read scene image for cleanup: %w", scanErr)
		}
		if path = strings.TrimSpace(path); path != "" {
			paths = append(paths, path)
		}
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return nil, fmt.Errorf("iterate scene images for cleanup: %w", err)
	}

	// Deleting the conversation row cascades to conversation_messages and
	// scene_projects (FK ON DELETE CASCADE). The row existence was already
	// confirmed inside the same transaction, so a non-1 RowsAffected is an
	// internal inconsistency, not a user-visible state.
	result, err := tx.Exec("DELETE FROM conversations WHERE id = ? AND owner = ?", conversationID, owner)
	if err != nil {
		return nil, fmt.Errorf("delete conversation: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("check deleted conversation: %w", err)
	}
	if count != 1 {
		return nil, fmt.Errorf("delete conversation: expected one row, affected %d", count)
	}

	// Explicitly assert the FK cascade removed every scene row. This keeps the
	// transactional guarantee from silently depending on the schema keeping
	// the ON DELETE CASCADE clause; a violation rolls back instead of orphaning
	// scene rows.
	err = tx.QueryRow(
		"SELECT 1 FROM scene_projects WHERE owner = ? AND conversation_id = ? LIMIT 1",
		owner, conversationID,
	).Scan(&one)
	if err == nil {
		return nil, fmt.Errorf("delete conversation: scene rows remained after cascade")
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("verify conversation scene cascade: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit conversation delete: %w", err)
	}
	return paths, nil
}

func (s *Store) getUnlocked(owner, id string) (Project, error) {
	row := s.db.Conn().QueryRow(
		"SELECT "+sceneSelectColumns+" FROM scene_projects WHERE id = ? AND owner = ?",
		id, owner,
	)
	project, err := scanProject(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Project{}, ErrNotFound
	}
	if err != nil {
		return Project{}, fmt.Errorf("get scene project: %w", err)
	}
	return project, nil
}

func scanProject(scanner storepkg.Scanner) (Project, error) {
	var project Project
	var candidatesJSON, selectedJSON, briefJSON string
	var tensionsJSON string
	var blockedReason, riskFlags string
	if err := scanner.Scan(
		&project.ID, &project.ConversationID, &project.ParentSceneID, &project.Owner, &project.Mode, &project.Status,
		&project.ApprovedSummary, &tensionsJSON, &project.CultureLens,
		&candidatesJSON, &selectedJSON, &briefJSON,
		&project.Caption, &project.MicroAction, &project.Disclaimer, &project.PromptVersion,
		&project.Provider, &project.Model, &project.imagePath, &project.ImageMIME,
		&project.ImageWidth, &project.ImageHeight, &project.ErrorMessage, &blockedReason, &riskFlags,
		&project.GenerationAttempt,
		&project.CreatedAt, &project.UpdatedAt, &project.CompletedAt,
	); err != nil {
		return Project{}, err
	}
	// JSON payloads are always written by this package, but reads still
	// validate them defensively instead of trusting stored bytes blindly.
	tensions, err := unmarshalTensions(tensionsJSON)
	if err != nil {
		return Project{}, fmt.Errorf("scene tensions corrupted: %w", err)
	}
	project.Tensions = tensions
	candidates, err := unmarshalCandidates(candidatesJSON)
	if err != nil {
		return Project{}, fmt.Errorf("scene candidates corrupted: %w", err)
	}
	project.Candidates = candidates
	if strings.TrimSpace(selectedJSON) != "" && strings.TrimSpace(selectedJSON) != "{}" {
		selected, err := unmarshalCandidate(selectedJSON)
		if err != nil {
			return Project{}, fmt.Errorf("scene selection corrupted: %w", err)
		}
		project.SelectedCandidate = &selected
	}
	if strings.TrimSpace(briefJSON) != "" && strings.TrimSpace(briefJSON) != "{}" {
		brief, err := unmarshalBrief(briefJSON)
		if err != nil {
			return Project{}, fmt.Errorf("scene brief corrupted: %w", err)
		}
		project.sceneBrief = brief
	}
	project.BlockedReason = blockedReason
	project.RiskFlags = splitRiskFlags(riskFlags)
	if project.Status == StatusCompleted && project.imagePath != "" && project.ImageMIME != "" {
		project.ImageURL = "/api/scenes/" + project.ID + "/image"
	}
	return project, nil
}

func joinRiskFlags(flags []string) string {
	clean := truncateStringList(flags, 60)
	if len(clean) == 0 {
		return ""
	}
	return strings.Join(clean, ",")
}

func splitRiskFlags(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var flags []string
	for _, part := range strings.Split(raw, ",") {
		if part = strings.TrimSpace(part); part != "" {
			flags = append(flags, truncateText(part, 60))
		}
	}
	return flags
}

func marshalTensions(tensions []string) (string, error) {
	if len(tensions) == 0 {
		return "[]", nil
	}
	raw, err := json.Marshal(tensions)
	if err != nil {
		return "", &Error{Message: "scene tensions serialization failed"}
	}
	return string(raw), nil
}

func unmarshalTensions(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" || raw == "[]" {
		return nil, nil
	}
	var tensions []string
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&tensions); err != nil {
		return nil, err
	}
	return tensions, nil
}

func marshalCandidates(candidates []Candidate) (string, error) {
	if len(candidates) == 0 {
		return "[]", nil
	}
	raw, err := json.Marshal(candidates)
	if err != nil {
		return "", &Error{Message: "scene candidates serialization failed"}
	}
	return string(raw), nil
}

func marshalCandidate(candidate *Candidate) (string, error) {
	if candidate == nil {
		return "{}", nil
	}
	raw, err := json.Marshal(candidate)
	if err != nil {
		return "", &Error{Message: "scene selection serialization failed"}
	}
	return string(raw), nil
}

func marshalBrief(brief SceneBrief) (string, error) {
	raw, err := json.Marshal(brief)
	if err != nil {
		return "", &Error{Message: "scene brief serialization failed"}
	}
	return string(raw), nil
}

func unmarshalCandidates(raw string) ([]Candidate, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" || raw == "[]" {
		return nil, nil
	}
	var candidates []Candidate
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&candidates); err != nil {
		return nil, err
	}
	return candidates, nil
}

func unmarshalCandidate(raw string) (Candidate, error) {
	var candidate Candidate
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&candidate); err != nil {
		return Candidate{}, err
	}
	return candidate, nil
}

func unmarshalBrief(raw string) (SceneBrief, error) {
	var brief SceneBrief
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&brief); err != nil {
		return SceneBrief{}, err
	}
	return brief, nil
}

// ImageFilePath validates and builds the storage path for one scene image.
// Only the scene id and a whitelisted extension can appear in the name.
func ImageFilePath(baseDir, id, ext string) (string, error) {
	baseDir = strings.TrimSpace(baseDir)
	id = normalizeID(id)
	ext = strings.ToLower(strings.TrimSpace(ext))
	switch ext {
	case "jpg", "png", "webp":
	default:
		return "", &Error{Message: "unsupported scene image extension"}
	}
	if baseDir == "" || id == "" {
		return "", &Error{Message: "invalid scene image path"}
	}
	return filepath.Join(baseDir, id+"."+ext), nil
}

func normalizeID(id string) string {
	id = strings.TrimSpace(id)
	if !strings.HasPrefix(id, "scn_") || len(id) > 80 || strings.ContainsAny(id, "/\\") {
		return ""
	}
	return id
}

func normalizeOwner(owner string) string {
	owner = strings.TrimSpace(owner)
	if owner == "" {
		return "default"
	}
	return truncateText(owner, 320)
}

func normalizeMode(mode string) string {
	if strings.EqualFold(strings.TrimSpace(mode), "organization") {
		return "organization"
	}
	return "personal"
}

func normalizeStatus(status string) string {
	switch status {
	case StatusDraft, StatusQueued, StatusComposing, StatusGenerating, StatusCompleted, StatusFailed, StatusBlocked:
		return status
	default:
		return ""
	}
}

func truncateText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

func truncateStringList(values []string, limit int) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out = append(out, truncateText(value, limit))
	}
	return out
}
