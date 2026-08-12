// Package scenes implements the "另一种可能 · 生活的一帧" scene subsystem:
// after a personal-mode conversation ends, the user may turn their situation
// into one visible ordinary-life frame. It is an independent orchestration and
// generation subsystem that never touches the WebRTC state machine and never
// reuses the ChatGPT Web account pool for image generation.
package scenes

import (
	"context"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/dyhhhhhh/chatgpt-web-voice/internal/conversations"
	storepkg "github.com/dyhhhhhh/chatgpt-web-voice/internal/store"
)

// Scene project lifecycle states.
const (
	StatusDraft      = "draft"
	StatusQueued     = "queued"
	StatusComposing  = "composing"
	StatusGenerating = "generating"
	StatusCompleted  = "completed"
	StatusFailed     = "failed"
	StatusBlocked    = "blocked"
)

const (
	// CandidateCount is the fixed number of ordinary-life moments.
	CandidateCount = 3
	// MaxApprovedSummaryRunes bounds the user-editable situation summary.
	MaxApprovedSummaryRunes = 600
	// Excerpt limits fed to the text orchestrator (recent-window policy).
	MaxExcerptMessages = 40
	MaxExcerptRunes    = 16000
	// MaxBriefRunes bounds one composed scene brief before storage.
	MaxBriefRunes = 4000
	// MaxCaptionRunes / MaxMicroActionRunes bound user-visible scene copy.
	MaxCaptionRunes     = 200
	MaxMicroActionRunes = 300
	// MaxTensionRunes / MaxLensRunes bound candidate metadata fields.
	MaxTensionRunes = 80
	MaxLensRunes    = 200
	// MaxInternalErrorRunes bounds persisted error text (never full upstream).
	MaxInternalErrorRunes = 400
	// MaxMessageContentRunes bounds one transcript message kept for scenes.
	MaxMessageContentRunes = 2000
)

// ErrNotFound means a scene does not exist or is not owned by the caller.
var ErrNotFound = storepkg.ErrNotFound

// ErrTextNotConfigured means the scene text orchestrator is not configured.
var ErrTextNotConfigured = errors.New("scene text orchestration is not configured")

// ErrImageNotConfigured means the scene image provider is not configured.
var ErrImageNotConfigured = errors.New("scene image generation is not configured")

// ErrQueueFull means the bounded generation queue is full.
var ErrQueueFull = errors.New("scene generation queue is full")

// ErrBusy means the scene is already queued/composing/generating.
var ErrBusy = errors.New("scene generation is already running")

// ErrStateConflict means a terminal state transition hit a stale expected
// status (e.g. CompleteGeneration on a row that is no longer generating).
// Internal to the worker path; never mapped directly to a public HTTP error.
var ErrStateConflict = errors.New("scene state conflict")

// Error is a validation error safe to surface to API callers.
type Error = storepkg.Error

// Candidate is one ordinary-life moment the user may choose.
type Candidate struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	Moment        string `json:"moment"`
	VisibleChange string `json:"visible_change"`
	RetainedCost  string `json:"retained_cost"`
	WhyThisScene  string `json:"why_this_scene"`
}

// CandidateResult is the strict-JSON response of the candidate orchestration.
type CandidateResult struct {
	CanGenerate     bool        `json:"can_generate"`
	BlockedReason   string      `json:"blocked_reason"`
	ApprovedSummary string      `json:"approved_summary"`
	Tensions        []string    `json:"tensions"`
	CultureLens     string      `json:"culture_lens"`
	Candidates      []Candidate `json:"candidates"`
	RiskFlags       []string    `json:"risk_flags"`
}

// SceneBrief is the strict-JSON scene description used to build the final
// image prompt. It contains only the composed scene, not the raw transcript.
type SceneBrief struct {
	SceneGoal           string   `json:"scene_goal"`
	CultureLens         string   `json:"culture_lens"`
	Time                string   `json:"time"`
	Place               string   `json:"place"`
	Subject             string   `json:"subject"`
	Action              string   `json:"action"`
	Relationships       string   `json:"relationships"`
	RetainedTension     string   `json:"retained_tension"`
	RealityCost         string   `json:"reality_cost"`
	EmotionalDelta      string   `json:"emotional_delta"`
	Camera              string   `json:"camera"`
	Lighting            string   `json:"lighting"`
	NegativeConstraints []string `json:"negative_constraints"`
	Caption             string   `json:"caption"`
	MicroAction         string   `json:"micro_action"`
	Disclaimer          string   `json:"disclaimer"`
}

// CandidateInput is what the candidate orchestrator receives: a bounded recent
// conversation window, never the full transcript.
type CandidateInput struct {
	Mode    string
	Excerpt string
}

// BriefInput is what the brief orchestrator receives: the approved summary and
// the selected moment only.
type BriefInput struct {
	ApprovedSummary string
	Candidate       Candidate
	Tensions        []string
	CultureLens     string
}

// ImageInput is the final image-generation request. The current product only
// renders one landscape life-film frame from a text prompt: canvas size
// (1536x1024), quality (standard) and count (n=1) are fixed business presets
// owned by the image provider, not per-request parameters.
//
// Future reference images MUST NOT be added to this struct as base64 payloads
// sent to /v1/images/generations. When one or more reference images exist the
// image provider must switch to POST /v1/images/edits (multipart/form-data)
// with repeated `image` file fields whose upload order defines reference
// priority; image models must never be called through /v1/chat/completions.
type ImageInput struct {
	Prompt string
}

// ImageResult is a validated, aspect- and size-normalized generated image.
// Providers guarantee Width==TargetImageWidth (1536) and
// Height==TargetImageHeight (1024); MIMEType always matches the real encoded
// bytes (never a guessed Content-Type).
type ImageResult struct {
	MIMEType string
	Bytes    []byte
	Width    int
	Height   int
}

// TextOrchestrator produces structured scene content. Candidate generation
// receives only the bounded recent excerpt; image generation receives only the
// composed brief, never the raw transcript.
type TextOrchestrator interface {
	GenerateCandidates(ctx context.Context, input CandidateInput) (CandidateResult, error)
	ComposeBrief(ctx context.Context, input BriefInput) (SceneBrief, error)
}

// ImageGenerator renders one scene image from the final prompt.
type ImageGenerator interface {
	Generate(ctx context.Context, input ImageInput) (ImageResult, error)
}

// Project is one scene record (draft and generation job in a single row).
// JSON payload fields are validated by the store on read.
type Project struct {
	ID                string      `json:"id"`
	ConversationID    string      `json:"conversation_id"`
	ParentSceneID     string      `json:"parent_scene_id,omitempty"`
	Owner             string      `json:"-"`
	Mode              string      `json:"mode"`
	Status            string      `json:"status"`
	ApprovedSummary   string      `json:"approved_summary"`
	Tensions          []string    `json:"tensions,omitempty"`
	CultureLens       string      `json:"culture_lens,omitempty"`
	Candidates        []Candidate `json:"candidates"`
	SelectedCandidate *Candidate  `json:"selected_candidate,omitempty"`
	Caption           string      `json:"caption,omitempty"`
	MicroAction       string      `json:"micro_action,omitempty"`
	Disclaimer        string      `json:"disclaimer,omitempty"`
	PromptVersion     string      `json:"prompt_version,omitempty"`
	Provider          string      `json:"provider,omitempty"`
	Model             string      `json:"model,omitempty"`
	ImageURL          string      `json:"image_url,omitempty"`
	ImageMIME         string      `json:"image_mime,omitempty"`
	ImageWidth        int         `json:"image_width,omitempty"`
	ImageHeight       int         `json:"image_height,omitempty"`
	ErrorMessage      string      `json:"error_message,omitempty"`
	GenerationAttempt int         `json:"generation_attempt,omitempty"`
	BlockedReason     string      `json:"blocked_reason,omitempty"`
	RiskFlags         []string    `json:"risk_flags,omitempty"`
	CreatedAt         string      `json:"created_at"`
	UpdatedAt         string      `json:"updated_at"`
	CompletedAt       string      `json:"completed_at,omitempty"`

	// sceneBrief is the persisted SceneBrief used for prompt building.
	sceneBrief SceneBrief
	// imagePath is the absolute path of the generated file (never exposed).
	imagePath string
}

// ActiveGeneration reports whether a job is running or waiting to run.
func (p *Project) ActiveGeneration() bool {
	switch p.Status {
	case StatusQueued, StatusComposing, StatusGenerating:
		return true
	default:
		return false
	}
}

// SceneBrief returns the validated persisted scene brief.
func (p *Project) SceneBrief() SceneBrief { return p.sceneBrief }

// ImagePath returns the absolute path of the generated image file.
func (p *Project) ImagePath() string { return p.imagePath }

// Interface is the scene surface required by the HTTP layer.
type Interface interface {
	Configured() bool
	CreateDraft(ctx context.Context, owner string, conversation conversations.Conversation) (Project, error)
	ListByConversation(owner, conversationID string) ([]Project, error)
	GetOwned(owner, id string) (Project, error)
	UpdateDraft(owner, id string, summary string, selectedCandidateID string) (Project, error)
	Generate(ctx context.Context, owner, id string) (Project, error)
	Regenerate(ctx context.Context, owner, id string) (Project, error)
	OpenImage(owner, id string) (*os.File, Project, error)
	DeleteOwned(owner, id string) error
	// DeleteConversation is the single transactional delete path for a
	// conversation that may own scenes: conversation row, messages and scene
	// metadata are removed in one SQLite transaction, and scene image files
	// are cleaned afterwards.
	DeleteConversation(owner, conversationID string) error
}

// timeNow is overridable for deterministic tests.
var timeNow = time.Now

func truncateRunes(value string, limit int) string {
	runes := []rune(strings.TrimSpace(value))
	if limit <= 0 {
		return ""
	}
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit])
}

func requireNonEmpty(value string, message string) error {
	if strings.TrimSpace(value) == "" {
		return &Error{Message: message}
	}
	return nil
}

func truncateErr(value string) string {
	if value == "" {
		return "scene generation failed"
	}
	return truncateRunes(value, MaxInternalErrorRunes)
}
