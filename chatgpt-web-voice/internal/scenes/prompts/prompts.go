package prompts

import (
	_ "embed"
)

// CandidateSystem instructs the text orchestrator to produce strict JSON with
// exactly three distinct ordinary-life moments from a bounded conversation
// window. The model must be able to refuse generation entirely.
//
//go:embed candidates.txt
var candidateSystemPrompt string

//go:embed brief.txt
var briefSystemPrompt string

//go:embed image-style.txt
var imageStyleBaseline string

// CandidateSystem returns the system prompt for candidate generation.
func CandidateSystem() string { return candidateSystemPrompt }

// BriefSystem returns the system prompt for scene brief composition.
func BriefSystem() string { return briefSystemPrompt }

// ImageStyleBaseline returns the unified visual baseline appended to every
// final image prompt.
func ImageStyleBaseline() string { return imageStyleBaseline }
