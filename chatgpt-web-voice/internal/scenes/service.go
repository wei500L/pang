package scenes

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/dyhhhhhh/chatgpt-web-voice/internal/conversations"
)

// PromptVersion identifies the exact prompt-set used for a scene.
const PromptVersion = "scene-prompts-v2"

// Service orchestrates the scene lifecycle: draft creation from saved
// conversation messages, user edits, generation enqueueing, and file lifecycle
// management. It holds no HTTP knowledge and never reads recordings.
type Service struct {
	store           *Store
	imagesDir       string
	text            TextOrchestrator
	image           ImageGenerator
	worker          *Worker
	logger          *slog.Logger
	imageProvider   string
	imageModel      string
	textConfigured  bool
	imageConfigured bool
}

// NewService builds the scene service. imageProviderName/imageModel are the
// image provider family and IMAGE_MODEL recorded on scene rows for
// traceability; the text model is only tracked in structured startup logs.
func NewService(store *Store, imagesDir string, text TextOrchestrator, image ImageGenerator, logger *slog.Logger, imageProviderName, imageModel string, textConfigured, imageConfigured bool) *Service {
	return &Service{
		store:           store,
		imagesDir:       imagesDir,
		text:            text,
		image:           image,
		logger:          logger,
		imageProvider:   imageProviderName,
		imageModel:      imageModel,
		textConfigured:  textConfigured,
		imageConfigured: imageConfigured,
	}
}

// AttachWorker links the bounded queue after construction (the worker needs
// the service, and the service needs the worker for enqueueing).
func (s *Service) AttachWorker(worker *Worker) { s.worker = worker }

// ProvidersConfigured reports whether both external providers are fully
// configured. It deliberately does NOT depend on the runtime worker, so the
// assembly order (create worker when providers are ready) has no circular
// dependency.
func (s *Service) ProvidersConfigured() bool {
	return s.textConfigured && s.text != nil &&
		s.imageConfigured && s.image != nil
}

// checkTextConfigured returns the precise 503 capability error when the text
// orchestrator cannot be called.
func (s *Service) checkTextConfigured() error {
	if !s.textConfigured || s.text == nil {
		return ErrTextNotConfigured
	}
	return nil
}

// checkImageConfigured returns the precise 503 capability error when the image
// provider cannot be called.
func (s *Service) checkImageConfigured() error {
	if !s.imageConfigured || s.image == nil {
		return ErrImageNotConfigured
	}
	return nil
}

// checkGenerationReady returns the first missing capability for the full
// generation loop: text provider, image provider, and the runtime worker.
// Credentials never fall back across the two providers.
func (s *Service) checkGenerationReady() error {
	if err := s.checkTextConfigured(); err != nil {
		return err
	}
	if err := s.checkImageConfigured(); err != nil {
		return err
	}
	if s.worker == nil {
		return ErrImageNotConfigured
	}
	return nil
}

// Configured reports whether the full scene loop can run.
func (s *Service) Configured() bool {
	return s.checkGenerationReady() == nil
}

// MarkInterruptedOnStartup fails jobs left over from a previous process.
func (s *Service) MarkInterruptedOnStartup() (int, error) {
	return s.store.MarkInterruptedOnStartup()
}

// CreateDraft builds a bounded recent conversation window, asks the text
// orchestrator for candidates, validates the result, and persists the draft.
// The conversation and its messages are read by the server from SQLite; the
// client never uploads a transcript.
func (s *Service) CreateDraft(ctx context.Context, owner string, conversation conversations.Conversation) (Project, error) {
	owner = normalizeOwner(owner)
	conversationID := strings.TrimSpace(conversation.ID)
	if conversationID == "" {
		return Project{}, &Error{Message: "conversation is required"}
	}
	if normalizeMode(conversation.Mode) != "personal" {
		return Project{}, &Error{Message: "scene generation only supports personal conversations"}
	}
	// Draft creation only needs the text orchestrator; it must keep working
	// even when the image provider key is missing.
	if err := s.checkTextConfigured(); err != nil {
		return Project{}, err
	}
	excerpt := buildConversationExcerpt(conversation.Messages)
	userCount, assistantCount := countRealMessages(conversation.Messages)
	if userCount == 0 || assistantCount == 0 {
		return Project{}, &Error{Message: "need at least one user message and one assistant message to create a scene"}
	}

	result, err := s.text.GenerateCandidates(ctx, CandidateInput{
		Mode:    normalizeMode(conversation.Mode),
		Excerpt: excerpt,
	})
	if err != nil {
		return Project{}, err
	}
	validated, err := validateCandidateResult(result)
	if err != nil {
		return Project{}, err
	}

	project := Project{
		ConversationID:  conversationID,
		Owner:           owner,
		Mode:            normalizeMode(conversation.Mode),
		ApprovedSummary: truncateRunes(result.ApprovedSummary, MaxApprovedSummaryRunes),
		Tensions:        truncateStringList(result.Tensions, MaxTensionRunes),
		CultureLens:     truncateText(result.CultureLens, MaxLensRunes),
		Candidates:      validated,
		RiskFlags:       truncateStringList(result.RiskFlags, 60),
		PromptVersion:   PromptVersion,
	}
	if !result.CanGenerate {
		project.Status = StatusBlocked
		project.BlockedReason = truncateRunes(result.BlockedReason, 300)
		if project.BlockedReason == "" {
			project.BlockedReason = "当前对话还不适合生成生活情境，可以继续聊一会儿再试。"
		}
	} else {
		project.Status = StatusDraft
	}
	return s.store.Create(project)
}

// GetOwned returns one scene.
func (s *Service) GetOwned(owner, id string) (Project, error) {
	return s.store.GetOwned(owner, id)
}

// ListByConversation returns all scenes of one owned conversation.
func (s *Service) ListByConversation(owner, conversationID string) ([]Project, error) {
	return s.store.ListByConversation(owner, conversationID)
}

// UpdateDraft applies the user-edited summary and the selected candidate id.
// The candidate is resolved from the stored candidate list; clients can never
// submit their own candidate JSON.
func (s *Service) UpdateDraft(owner, id string, summary string, selectedCandidateID string) (Project, error) {
	project, err := s.store.GetOwned(owner, id)
	if err != nil {
		return Project{}, err
	}
	switch project.Status {
	case StatusDraft, StatusFailed, StatusCompleted:
	default:
		return Project{}, &Error{Message: "scene can only be edited before generation"}
	}
	summary = strings.TrimSpace(summary)
	if len([]rune(summary)) > MaxApprovedSummaryRunes {
		return Project{}, &Error{Message: "approved summary is too long"}
	}
	if summary == "" {
		return Project{}, &Error{Message: "approved summary is required"}
	}
	var selected *Candidate
	selectedCandidateID = strings.TrimSpace(selectedCandidateID)
	if selectedCandidateID != "" {
		found := false
		for i := range project.Candidates {
			if project.Candidates[i].ID == selectedCandidateID {
				candidate := project.Candidates[i]
				selected = &candidate
				found = true
				break
			}
		}
		if !found {
			return Project{}, &Error{Message: "selected candidate is not part of this scene"}
		}
	}
	return s.store.UpdateDraftContent(owner, id, summary, selected)
}

// Generate validates the draft, marks it queued, and enqueues the bounded
// async job. Brief composition happens inside the job's composing phase so the
// handler returns 202 quickly; the client polls GET scene.
func (s *Service) Generate(ctx context.Context, owner, id string) (Project, error) {
	owner = normalizeOwner(owner)
	project, err := s.store.GetOwned(owner, id)
	if err != nil {
		return Project{}, err
	}
	if err := s.checkGenerationReady(); err != nil {
		return Project{}, err
	}
	if project.Status == StatusBlocked {
		return Project{}, &Error{Message: "blocked scenes cannot be generated"}
	}
	if project.ActiveGeneration() {
		return Project{}, ErrBusy
	}
	if strings.TrimSpace(project.ApprovedSummary) == "" {
		return Project{}, &Error{Message: "approved summary is required before generation"}
	}
	if project.SelectedCandidate == nil {
		return Project{}, &Error{Message: "select one candidate before generation"}
	}

	prepared, updated, err := s.store.PrepareGeneration(owner, id, s.imageProvider, s.imageModel, PromptVersion)
	if err != nil {
		return Project{}, err
	}
	if !prepared {
		return Project{}, ErrBusy
	}
	if err := s.worker.Enqueue(updated.ID); err != nil {
		_ = s.store.RevertGeneration(owner, id)
		return Project{}, err
	}
	s.logger.Info("scene_generation_enqueued", "scene_id", updated.ID, "attempt", updated.GenerationAttempt)
	return updated, nil
}

// Regenerate creates a fresh scene from the same approved summary and selected
// moment, keeping parent_scene_id for provenance, and immediately enqueues it.
// The parent's composed brief is reused so it is literally "the same moment";
// if the parent has none, the job composes one in its composing phase.
func (s *Service) Regenerate(ctx context.Context, owner, id string) (Project, error) {
	owner = normalizeOwner(owner)
	parent, err := s.store.GetOwned(owner, id)
	if err != nil {
		return Project{}, err
	}
	if err := s.checkGenerationReady(); err != nil {
		return Project{}, err
	}
	if strings.TrimSpace(parent.ApprovedSummary) == "" || parent.SelectedCandidate == nil {
		return Project{}, &Error{Message: "the source scene has no approved summary or selected moment"}
	}

	child := Project{
		ConversationID:    parent.ConversationID,
		ParentSceneID:     parent.ID,
		Owner:             parent.Owner,
		Mode:              parent.Mode,
		Status:            StatusDraft,
		ApprovedSummary:   parent.ApprovedSummary,
		Tensions:          parent.Tensions,
		CultureLens:       parent.CultureLens,
		Candidates:        parent.Candidates,
		SelectedCandidate: parent.SelectedCandidate,
		RiskFlags:         parent.RiskFlags,
		PromptVersion:     PromptVersion,
	}
	child.sceneBrief = parent.SceneBrief()
	created, err := s.store.Create(child)
	if err != nil {
		return Project{}, err
	}

	prepared, updated, err := s.store.PrepareGeneration(created.Owner, created.ID, s.imageProvider, s.imageModel, PromptVersion)
	if err != nil {
		return Project{}, err
	}
	if !prepared {
		return Project{}, ErrBusy
	}
	if err := s.worker.Enqueue(updated.ID); err != nil {
		_ = s.store.RevertGeneration(updated.Owner, updated.ID)
		return Project{}, err
	}
	s.logger.Info("scene_regenerated", "scene_id", updated.ID, "parent_scene_id", parent.ID, "attempt", updated.GenerationAttempt)
	return updated, nil
}

// OpenImage returns the generated file of an owner-scoped completed scene.
func (s *Service) OpenImage(owner, id string) (*os.File, Project, error) {
	return s.store.OpenImage(owner, id)
}

// DeleteOwned removes one scene and its image file.
func (s *Service) DeleteOwned(owner, id string) error {
	path, err := s.store.DeleteOwned(owner, id)
	if err != nil {
		return err
	}
	if path != "" {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			// Logged without the absolute path; the database row is already gone.
			s.logger.Warn("scene_image_cleanup_failed", "scene_id", id)
		}
	}
	s.logger.Info("scene_deleted", "scene_id", id)
	return nil
}

// DeleteConversation is the single delete path for a conversation that may own
// scenes. The scene metadata is removed inside the same SQLite transaction as
// the conversation row (see Store.DeleteConversation); image files live outside
// the database, so they are only removed after the transaction commits. A
// cleanup failure is logged without paths and does not roll back the committed
// delete.
func (s *Service) DeleteConversation(owner, conversationID string) error {
	paths, err := s.store.DeleteConversation(owner, conversationID)
	if err != nil {
		return err
	}
	for _, path := range paths {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			// Logged without the absolute path; the transaction already committed.
			s.logger.Warn("scene_image_cleanup_failed", "conversation_id", conversationID)
		}
	}
	if len(paths) > 0 {
		s.logger.Info("conversation_scenes_deleted", "conversation_id", conversationID, "images", len(paths))
	}
	return nil
}

// failGeneration records a sanitized failure for an active job. When the job
// context was cancelled (graceful shutdown) the message reflects the shutdown
// instead of the provider error. A CAS miss (the row already left the active
// states) is expected and exits quietly; a real database failure keeps only a
// sanitized warning so the job does not vanish silently — the row stays active
// and is recovered on the next startup.
func (s *Service) failGeneration(ctx context.Context, owner, id string, err error) {
	message := truncateErr(err.Error())
	if ctxErr := ctx.Err(); ctxErr != nil {
		message = "generation interrupted by server shutdown"
	}
	s.logger.Warn("scene_generation_failed", "scene_id", id, "error", message)
	if storeErr := s.store.FailGeneration(owner, id, message); storeErr != nil {
		if errors.Is(storeErr, ErrStateConflict) {
			return // late CAS miss: row is already terminal, nothing to do
		}
		s.logger.Warn("scene_failure_persist_failed", "scene_id", id, "error", "scene failure state could not be persisted")
	}
}

// runGeneration is executed by the worker with bounded concurrency. Phases:
// queued → composing (brief orchestration, if not already persisted) →
// generating (image render) → completed/failed.
func (s *Service) runGeneration(ctx context.Context, project Project) {
	owner := project.Owner
	id := project.ID
	started := timeNow()

	advanced, err := s.store.AdvanceGeneration(owner, id, StatusQueued, StatusComposing)
	if err != nil {
		s.logger.Warn("scene_state_transition_failed", "scene_id", id, "stage", "queued-to-composing", "error", "scene state transition failed")
		return
	}
	if !advanced {
		return // normal CAS miss: another actor already moved or removed the row
	}
	brief := project.SceneBrief()
	if strings.TrimSpace(brief.Caption) == "" {
		composed, err := s.text.ComposeBrief(ctx, BriefInput{
			ApprovedSummary: project.ApprovedSummary,
			Candidate:       *project.SelectedCandidate,
			Tensions:        project.Tensions,
			CultureLens:     project.CultureLens,
		})
		if err != nil {
			s.failGeneration(ctx, owner, id, err)
			return
		}
		if err := validateBrief(composed); err != nil {
			s.failGeneration(ctx, owner, id, err)
			return
		}
		if strings.TrimSpace(composed.SeriesLabel) == "" {
			composed.SeriesLabel = DefaultSeriesLabel
		} else {
			composed.SeriesLabel = truncateRunes(composed.SeriesLabel, MaxSeriesLabelRunes)
		}
		brief = composed
		if err := s.store.SaveBrief(owner, id, brief, s.imageProvider, s.imageModel, PromptVersion); err != nil {
			s.failGeneration(ctx, owner, id, errors.New("scene brief storage failed"))
			return
		}
	}
	advanced, err = s.store.AdvanceGeneration(owner, id, StatusComposing, StatusGenerating)
	if err != nil {
		s.logger.Warn("scene_state_transition_failed", "scene_id", id, "stage", "composing-to-generating", "error", "scene state transition failed")
		return
	}
	if !advanced {
		return // normal CAS miss
	}

	imageInput := ImageInput{
		Prompt: BuildImagePrompt(brief),
	}
	result, err := s.image.Generate(ctx, imageInput)
	if err != nil {
		s.failGeneration(ctx, owner, id, err)
		return
	}

	ext, err := mimeExtension(result.MIMEType)
	if err != nil {
		s.failGeneration(ctx, owner, id, err)
		return
	}
	// Providers must return the normalized target canvas; treat anything else
	// as a provider contract violation rather than publishing it.
	if result.Width != TargetImageWidth || result.Height != TargetImageHeight {
		s.failGeneration(ctx, owner, id, errors.New("generated image normalization failed"))
		return
	}
	if err := os.MkdirAll(s.imagesDir, 0o700); err != nil {
		s.failGeneration(ctx, owner, id, errors.New("scene image storage is unavailable"))
		return
	}
	imagePath, err := ImageFilePath(s.imagesDir, id, ext)
	if err != nil {
		s.failGeneration(ctx, owner, id, err)
		return
	}
	// Write to a temporary file, fsync it, close it, and only then atomically
	// rename into place: a crash or power loss after the rename must never
	// leave unsynced bytes under the final scene id.
	temp, err := os.CreateTemp(s.imagesDir, ".scene-upload-*")
	if err != nil {
		s.failGeneration(ctx, owner, id, errors.New("scene image storage is unavailable"))
		return
	}
	tempPath := temp.Name()
	_ = temp.Chmod(0o600)
	if _, err := temp.Write(result.Bytes); err != nil {
		_ = temp.Close()
		_ = os.Remove(tempPath)
		s.failGeneration(ctx, owner, id, errors.New("scene image write failed"))
		return
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		_ = os.Remove(tempPath)
		s.failGeneration(ctx, owner, id, errors.New("scene image write failed"))
		return
	}
	if err := temp.Close(); err != nil {
		_ = os.Remove(tempPath)
		s.failGeneration(ctx, owner, id, errors.New("scene image write failed"))
		return
	}
	// Re-confirm the row is still generating before publishing: the scene may
	// have been interrupted by shutdown (status already terminal), in which
	// case the temp file is simply dropped and nothing is published.
	inStatus, err := s.store.IsInStatus(owner, id, StatusGenerating)
	if err != nil {
		_ = os.Remove(tempPath)
		s.logger.Warn("scene_state_transition_failed", "scene_id", id, "stage", "publish-status-check", "error", "scene state check failed")
		return
	}
	if !inStatus {
		_ = os.Remove(tempPath)
		s.logger.Warn("scene_publish_skipped", "scene_id", id, "reason", "scene is no longer generating")
		return
	}
	if err := os.Rename(tempPath, imagePath); err != nil {
		_ = os.Remove(tempPath)
		s.failGeneration(ctx, owner, id, errors.New("scene image publish failed"))
		return
	}

	// Terminal CAS: only a row still in generating may move to completed. A
	// zero-row update means the state changed underneath us (shutdown mark or
	// delete); the just-published file must be removed immediately.
	if err := s.store.CompleteGeneration(owner, id, imagePath, result.MIMEType, result.Width, result.Height, brief.Caption, brief.MicroAction, brief.Disclaimer); err != nil {
		_ = os.Remove(imagePath)
		return
	}
	// The generation is durably committed. Only now clean up a previous image
	// that lives at a different path (e.g. a different extension from an older
	// attempt). Rename/CAS failures above must never delete the old file. When
	// the paths are identical, the atomic rename already overwrote the old
	// bytes, so nothing is removed here.
	if previous := strings.TrimSpace(project.imagePath); previous != "" && previous != imagePath {
		if err := os.Remove(previous); err != nil && !errors.Is(err, os.ErrNotExist) {
			s.logger.Warn("scene_previous_image_cleanup_failed", "scene_id", id)
		}
	}
	s.logger.Info("scene_generation_completed",
		"scene_id", id,
		"provider", s.imageProvider,
		"model", s.imageModel,
		"duration_ms", timeNow().Sub(started).Milliseconds(),
		"bytes", len(result.Bytes),
		"width", result.Width,
		"height", result.Height,
	)
}

// mimeExtension maps the accepted image MIME types to file extensions.
func mimeExtension(mime string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(mime)) {
	case "image/jpeg", "image/jpg":
		return "jpg", nil
	case "image/png":
		return "png", nil
	case "image/webp":
		return "webp", nil
	default:
		return "", &Error{Message: "generated image has an unsupported MIME type"}
	}
}

// buildConversationExcerpt keeps a recent window of real messages, drops the
// prompt-injection and greeting artifacts, and caps total size.
func buildConversationExcerpt(messages []conversations.Message) string {
	type row struct {
		role string
		text string
	}
	var rows []row
	for _, message := range messages {
		text := strings.TrimSpace(message.Content)
		if text == "" || looksLikePromptArtifact(text) || looksLikeWelcomeGreeting(text) {
			continue
		}
		text = truncateRunes(text, MaxMessageContentRunes)
		if text == "" {
			continue
		}
		rows = append(rows, row{role: message.Role, text: text})
	}
	if len(rows) > MaxExcerptMessages {
		rows = rows[len(rows)-MaxExcerptMessages:]
	}

	var builder strings.Builder
	totalRunes := 0
	for i := len(rows) - 1; i >= 0; i-- {
		item := rows[i]
		if totalRunes > 0 {
			builder.WriteString("\n")
		}
		label := "用户"
		if item.role == "assistant" {
			label = "伙伴"
		}
		line := label + "：" + item.text
		lineRunes := len([]rune(line))
		remaining := MaxExcerptRunes - totalRunes
		if remaining <= 0 {
			break
		}
		if lineRunes > remaining {
			line = truncateRunes(line, remaining)
		}
		builder.WriteString(line)
		totalRunes += len([]rune(line))
	}
	return strings.TrimSpace(builder.String())
}

func countRealMessages(messages []conversations.Message) (userCount, assistantCount int) {
	for _, message := range messages {
		text := strings.TrimSpace(message.Content)
		if text == "" || looksLikePromptArtifact(text) || looksLikeWelcomeGreeting(text) {
			continue
		}
		switch message.Role {
		case "user":
			userCount++
		case "assistant":
			assistantCount++
		}
	}
	return userCount, assistantCount
}

// looksLikePromptArtifact ignores leftover system-prompt injection rows.
func looksLikePromptArtifact(text string) bool {
	lower := strings.ToLower(text)
	return strings.Contains(lower, "实时语音提示词") ||
		strings.Contains(lower, "system prompt") ||
		strings.Contains(lower, "本轮加载与首次开场") ||
		strings.Contains(lower, "# 自由·爱")
}

// looksLikeWelcomeGreeting ignores the fixed initialization greeting, which is
// not real user situation content.
func looksLikeWelcomeGreeting(text string) bool {
	normalized := strings.ToLower(text)
	normalized = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '\u3400' && r <= '\u9fff') {
			return r
		}
		return -1
	}, normalized)
	return strings.Contains(normalized, "你好我是自由爱以真诚善良尊重自由和爱为底色")
}

// validateCandidateResult enforces the structured protocol: can_generate
// scenes must carry exactly three distinct candidates; blocked scenes must not.
func validateCandidateResult(result CandidateResult) ([]Candidate, error) {
	if !result.CanGenerate {
		return nil, nil
	}
	if len(result.Candidates) != CandidateCount {
		return nil, &ErrProviderResponse{Message: "text model must return exactly 3 candidates"}
	}
	seen := make(map[string]struct{}, len(result.Candidates))
	validated := make([]Candidate, 0, len(result.Candidates))
	for _, candidate := range result.Candidates {
		candidate.ID = strings.TrimSpace(candidate.ID)
		if candidate.ID == "" {
			candidate.ID = fmt.Sprintf("moment_%d", len(validated)+1)
		}
		if err := requireNonEmpty(candidate.Title, "candidate title is required"); err != nil {
			return nil, &ErrProviderResponse{Message: "text model returned a candidate without a title"}
		}
		if !hasIfIWerePrefix(candidate.Title) {
			return nil, &ErrProviderResponse{Message: "text model returned a candidate without a 假如我是 title"}
		}
		if err := requireNonEmpty(candidate.Moment, "candidate moment is required"); err != nil {
			return nil, &ErrProviderResponse{Message: "text model returned a candidate without a moment"}
		}
		if _, exists := seen[candidate.ID]; exists {
			return nil, &ErrProviderResponse{Message: "text model returned duplicate candidates"}
		}
		seen[candidate.ID] = struct{}{}
		candidate.Title = truncateRunes(candidate.Title, 120)
		candidate.Moment = truncateRunes(candidate.Moment, 240)
		candidate.VisibleChange = truncateRunes(candidate.VisibleChange, 240)
		candidate.RetainedCost = truncateRunes(candidate.RetainedCost, 240)
		candidate.WhyThisScene = truncateRunes(candidate.WhyThisScene, 240)
		validated = append(validated, candidate)
	}
	return validated, nil
}

// validateBrief checks the composed brief has everything needed for rendering.
func validateBrief(brief SceneBrief) error {
	if err := requireNonEmpty(brief.EssayTitle, "scene brief essay title is required"); err != nil {
		return &ErrProviderResponse{Message: "text model returned a brief without an essay title"}
	}
	if !hasIfIWerePrefix(brief.EssayTitle) {
		return &ErrProviderResponse{Message: "text model returned an essay title without a 假如我是 prefix"}
	}
	if err := requireNonEmpty(brief.Essay, "scene brief essay is required"); err != nil {
		return &ErrProviderResponse{Message: "text model returned a brief without an essay"}
	}
	if err := requireNonEmpty(brief.Closing, "scene brief closing is required"); err != nil {
		return &ErrProviderResponse{Message: "text model returned a brief without a closing"}
	}
	if err := requireNonEmpty(brief.Caption, "scene brief caption is required"); err != nil {
		return &ErrProviderResponse{Message: "text model returned a brief without a caption"}
	}
	if err := requireNonEmpty(brief.MicroAction, "scene brief micro action is required"); err != nil {
		return &ErrProviderResponse{Message: "text model returned a brief without a micro action"}
	}
	if err := requireNonEmpty(brief.Place, "scene brief place is required"); err != nil {
		return &ErrProviderResponse{Message: "text model returned a brief without a place"}
	}
	if err := requireNonEmpty(brief.Action, "scene brief action is required"); err != nil {
		return &ErrProviderResponse{Message: "text model returned a brief without an action"}
	}
	if err := requireNonEmpty(brief.Disclaimer, "scene brief disclaimer is required"); err != nil {
		return &ErrProviderResponse{Message: "text model returned a brief without a disclaimer"}
	}
	if len([]rune(brief.EssayTitle)) > MaxEssayTitleRunes {
		return &ErrProviderResponse{Message: "scene brief essay title is too long"}
	}
	if len([]rune(brief.Essay)) > MaxEssayRunes {
		return &ErrProviderResponse{Message: "scene brief essay is too long"}
	}
	if len([]rune(brief.Closing)) > MaxClosingRunes {
		return &ErrProviderResponse{Message: "scene brief closing is too long"}
	}
	if len([]rune(brief.Caption)) > MaxCaptionRunes {
		return &ErrProviderResponse{Message: "scene brief caption is too long"}
	}
	if len([]rune(brief.MicroAction)) > MaxMicroActionRunes {
		return &ErrProviderResponse{Message: "scene brief micro action is too long"}
	}
	return nil
}

// hasIfIWerePrefix reports whether title starts with 假如我是, ignoring
// regular and full-width spaces so "假如 我是……" still matches.
func hasIfIWerePrefix(title string) bool {
	compact := strings.NewReplacer(" ", "", "　", "", "\u00a0", "").Replace(strings.TrimSpace(title))
	return strings.HasPrefix(compact, "假如我是")
}
