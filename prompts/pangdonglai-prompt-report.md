# Pangdonglai System Prompt — Technical Report

## 1. Actual Model

**Model:** The project routes voice sessions through `chatgpt.com/realtime/wm` as a WebRTC gateway. The `buildSessionJSON()` function in `internal/voice/service.go` sends:
- `model_slug: ""` (empty — upstream default)
- `model_slug_advanced: ""` (empty)
- `requested_default_model: ""` (empty)
- `backend_reasoning_effort: "instant"`
- `conversation_mode: { kind: "primary_assistant" }`

The actual model is served upstream and is opaque to the gateway. Per product requirements, the Agent identifies itself to users as **胖东来语音模型** (Pangdonglai Voice Model) and never mentions any external model provider.

**Context window:** Larger than the standard 128K context; exact figure not specified here. The system prompt at ~14,000 tokens is well within budget regardless.

**Configuration location:** There is no model configuration in this project. Model selection is entirely controlled upstream. The gateway's `VOICE_*` environment variables do not include model selection.

## 2. Current Prompt Injection Method

**There is no existing system prompt or instruction injection anywhere in the codebase.**

The chatgpt-web-voice gateway is a transparent WebRTC/SDP proxy. It:
1. Receives an SDP offer from the browser
2. Selects a ChatGPT Web access token from the account pool
3. Forwards the SDP offer + session JSON to `chatgpt.com/realtime/wm`
4. Returns the SDP answer to the browser
5. WebRTC media and DataChannel flow directly between browser and chatgpt.com (not through the gateway)

The gateway does NOT inject, modify, or inspect:
- System prompts
- Instructions
- Conversation history
- DataChannel message content

**The only mechanism for injecting instructions is the DataChannel `relay_message` sent from the browser.**

In `static/voice.html`, after the WebRTC DataChannel opens:
1. `dc.onopen` fires (line 5429)
2. `onVoiceChannelReady()` is called (line 4495)
3. `sendVoicePreviewGreeting()` sends a test phrase via `relay_message` (line 4476)

This is the injection point. The current message sent is the `previewPrompt` i18n string:
- Chinese: "请只用当前语音说：你好。不要添加其他内容。"
- English: "Using only the current voice, say: hello. Do not add anything else."

## 3. Final System Prompt Token Estimate

| Metric | Value |
|--------|-------|
| File | `prompts/pangdonglai-realtime-system-prompt.md` |
| Words | ~7,400 |
| Characters | ~55,700 |
| Lines | ~630 |
| Estimated tokens | **~14,000 tokens** |
| Context ratio (vs 128K) | **~11%** — well within budget even for standard context; ample room for larger windows |
| Estimated multi-turn voice capacity | Several hundred conversational turns |

**Token estimation methodology:** Rough estimate at ~1 token per 4 characters for mixed English-Chinese Markdown text. Actual count depends on the specific tokenizer. At ~14K tokens, the prompt is compact enough for any modern context window while carrying sufficient cultural knowledge density.

## 4. Source Material Coverage

| Category | Files | Content Coverage |
|----------|-------|-----------------|
| A-level (official culture) | 22 files | ~60% of unique conceptual content adopted; enterprise operations excluded |
| B-level (于东来 works) | 4 files | Personal philosophy and autobiographical content adopted; management lectures partially adopted |
| C-level (corporate stories) | 4 files | Used for tone calibration and illustration; specific narratives not embedded |
| D-level (third-party) | 2 files | Summarized only; NOT adopted as official content |

**Content adopted into runtime knowledge layer:**
- Complete cultural framework (faith, goal, principle)
- 10 virtues and 8 vices with definitions
- Happy Life State 8-module framework with levels
- 2024 Moral Code key articles
- Work philosophy and life philosophy aphorisms
- Parent/child/love/finance/health practical guidance
- Historical evolution notes
- Common misunderstandings and deifications

**Content deliberately excluded:**
- Enterprise management philosophy and operational standards
- HR policy details (specific leave days, compensation tables)
- Store operations (merchandising, environment, service protocols)
- Safety training protocols
- Third-party analytical frameworks
- Copyright-protected book text
- Individual employee stories as specific narratives
- COVID-era content

## 5. Discovered Material Conflicts

### Resolved Conflicts:

1. **2023 vs 2024 Happy Life State editions:** 2024 adopted as primary. Key differences documented in source audit. Where 2023 rubrics are more detailed (爱情/居家/父母/孩子 modules), both editions are substantially identical.

2. **Legacy (pre-2015) vs Modern (2023+) framework:** Older belief system (公平、自由、快乐、博爱) replaced by current (自由·爱). Older vice list (5 items) expanded to current (8 items). Current framework adopted; evolution noted for historical context.

3. **"奴性文化" vs "落后文化":** The 2023 edition uses the harsher term "奴性文化" as the source of evil; 2024 softens to "落后文化". The 2024 terminology adopted. This is a significant terminological shift reflecting a less confrontational framing.

4. **Third-party interpretations vs official positions:** External authors' analytical frameworks (王慧中's influence model, 刘杨's three flywheels) are clearly identified as external analysis and excluded from runtime knowledge.

### Unresolved Tensions (noted but not resolved in materials):

1. **Freedom philosophy vs strict employee discipline:** The official culture emphasizes freedom and non-bondage, but described employee management includes strict rules with termination for violations. This tension is inherent in the materials and not reconciled. The prompt does not attempt to resolve it.

2. **Yu Donglai's personal evolution:** His earlier stance (firing employees who wanted reduced hours) contradicts his later philosophy (7-hour workday, mandatory rest). This is framed as personal growth in his own narrative.

## 6. Uncertain / Cannot Confirm

1. **Current (2026) operational status:** All materials date from 2007-2024. Whether all described practices remain active in 2026 cannot be confirmed from the materials.

2. **Exact current employee count, store count, revenue:** Not reliably extractable from the materials.

3. **Specific OCR-damaged passages:** Several paragraphs in A15 (新员工文化培训——关于生活) and B02 (心向阳光-于东来 Weibo sections) are too garbled for confident citation.

4. **Story embellishment:** C-level corporate stories are edited publications. Whether individual stories contain embellishment for narrative effect cannot be determined.

5. **Copyright boundary for published books:** D01 and D02 are commercially published books. While factual information about Pangdonglai culture is not copyrightable, the authors' specific analytical language and frameworks may be. The prompt summarizes concepts rather than quoting text.

## 7. Recommended Injection Approach

### Primary Recommendation: Frontend DataChannel Injection

**Injection point:** `static/voice.html`, function `onVoiceChannelReady()` (line 4495) or `sendVoicePreviewGreeting()` (line 4476).

**Approach:** After the DataChannel opens, send the system prompt as the first `relay_message` before any user interaction. This uses the same mechanism as the existing voice preview greeting.

**Specific modification (minimum change):**

1. Serve the system prompt text via a lightweight API endpoint or embedded static resource
2. In `onVoiceChannelReady()`, fetch the system prompt and send it as a `relay_message`
3. After sending, proceed with normal conversation (optionally send a brief greeting)

**Why this approach:**
- The DataChannel `relay_message` is the ONLY mechanism available for sending content to the model
- The gateway has no server-side prompt injection capability
- chatgpt.com's web realtime endpoint does not expose a system prompt or instructions field
- The "priming turn" pattern is already established by the voice preview greeting

### Alternative: Server-Side Session JSON

The `buildSessionJSON()` function in `internal/voice/service.go` could potentially accept additional fields. However:
- The chatgpt.com realtime/wm endpoint's accepted fields are undocumented
- Experimenting with unknown fields risks breaking the connection
- The DataChannel approach is proven and already in use

**Recommendation:** Use the DataChannel approach. It is the lowest-risk, most controllable method.

## 8. Code Integration Status

**Not yet integrated.** The system prompt file exists at `prompts/pangdonglai-realtime-system-prompt.md` but has not been wired into the application.

### Integration Required:

1. **Serve the system prompt:** Add a lightweight endpoint (e.g., `GET /api/voice/system-prompt`) or serve as static file from `static/` directory.

2. **Modify voice.html:** In the DataChannel open handler, fetch and send the system prompt as the first `relay_message`:
   - After `dc.onopen` fires and `onVoiceChannelReady()` is called
   - Send system prompt as a priming message BEFORE the voice preview greeting
   - The message should use the existing `sendRelayMessage()` mechanism
   - Format using `buildBaseUserMessage()` with appropriate metadata

3. **Language handling:** The system prompt is in English (model instruction language). The i18n system in voice.html handles UI strings only. The system prompt text itself does not need i18n — it is always sent in English regardless of UI locale.

### Files to Modify:

| File | Change | Risk |
|------|--------|------|
| `static/voice.html` | Add system prompt fetch + send in `onVoiceChannelReady()` | Low — additive change to existing flow |
| `internal/api/` or `static/` | Add endpoint or static file serving | Low — new endpoint, no existing behavior changed |

### Files NOT to Modify:

- `internal/voice/service.go` — SDP proxy, no prompt injection
- `internal/config/config.go` — No prompt-related config exists or needed
- `internal/voice/config.go` — Voice/language config only
- Any WebRTC, audio, or auth code — Unrelated to prompt injection
- `static/voice-room.js` — Parallax/atmosphere effects only
- `static/app.css` / `static/voice-room.css` — Styling only

## 9. Prompt Quality Assessment

### Internal Consistency Check:
- ✅ No contradictory "must/never/always" rules across sections
- ✅ Behavioral rules in Sections 7-21 are consistent with identity in Sections 1-3
- ✅ Cultural knowledge in Section 4 aligns with fact/attribution rules in Sections 5, 15-16
- ✅ Voice interaction rules (Sections 8-9) are consistent with conversation flow (Section 21)
- ✅ "Another possibility" generation (Section 13) aligns with micro-action design (Section 14)
- ✅ Personal mode (Section 11) properly excludes organizational/enterprise content
- ✅ Prohibited behaviors (Section 20) are non-redundant with other sections

### Redundancy Check:
- Merged: "先理解人" rule appears in Identity (§1), Values (§7), Personal Mode (§11), and Flow (§21) — kept in each as they serve different functions (identity definition, value commitment, mode specification, procedural step)
- Merged: Language/tone rules consolidated in §8; voice-specific rules in §9
- Merged: Citation and attribution consolidated in §15
- Removed: Duplicate "don't fabricate" rules — kept in §20 (Prohibited Behaviors) with cross-reference
- Kept: Intentional reinforcement of core principles (user autonomy, factual honesty, voice-appropriate length) — each appearance serves a distinct structural purpose

### Compression Applied:
- Cultural knowledge: Condensed from ~500K+ characters of source material to ~30K characters of structured knowledge
- Behavioral rules: Merged overlapping constraints into single-source sections
- Examples: 10 scenarios covering the required cases, each concise (4-8 lines)
- Self-check: 10-item checklist, not a paragraph per item

---

*End of prompt report. Last updated: 2026-08-10.*
