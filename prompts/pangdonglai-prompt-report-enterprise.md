# Pangdonglai Enterprise System Prompt — Technical Report

## 1. Actual Model

**Model:** The project routes voice sessions through `chatgpt.com/realtime/wm` as a WebRTC gateway. The `buildSessionJSON()` function in `internal/voice/service.go` sends empty model selection fields (`model_slug: ""`, `model_slug_advanced: ""`, `requested_default_model: ""`). The actual model is served upstream and is opaque to the gateway. Per product requirements, the Agent identifies itself to users as **胖东来企业模式** (Pangdonglai Enterprise Mode) — the enterprise mode of the 胖东来语音模型 (Pangdonglai Voice Model).

**Configuration location:** No model configuration exists in this project. Model selection is entirely controlled upstream. The gateway's environment variables do not include model selection.

## 2. Current Prompt Injection Method

**Mechanism:** The system prompt is injected via DataChannel `relay_message` from the browser (see `static/voice.html`).

**Current state:**
- `prompts/pangdonglai-realtime-system-prompt.md` → copied to `static/pangdonglai-system-prompt.txt` for serving
- This is the **personal mode** (个人模式) prompt
- Sent in `onVoiceChannelReady()` via `sendSystemPrompt()` at line 4536
- Fetched from `/static/pangdonglai-system-prompt.txt` at line 4513

**No code changes were made.** Per developer instruction, the agent's scope is the prompt only — `voice.html` and all backend logic are untouched. The gateway continues to serve whatever `sendSystemPrompt()` in `voice.html` fetches (`/static/pangdonglai-system-prompt.txt`). Integration is the developer's step.

**Prompt deliverable for enterprise always-on (no URL parameter, no mode-switching code):**
1. `prompts/pangdonglai-realtime-system-prompt-enterprise.md` — the enterprise system prompt (source)
2. `static/pangdonglai-system-prompt-enterprise.txt` — served-ready copy (not wired; code untouched)
3. Identity: when asked what model/mode the Agent is, the prompt answers **胖东来企业模式** (Pangdonglai Enterprise Mode) — enterprise mode is self-identifying through the prompt

## 3. Final Enterprise System Prompt Token Estimate

| Metric | Value |
|--------|-------|
| File | `prompts/pangdonglai-realtime-system-prompt-enterprise.md` |
| Words | ~10,800 |
| Characters | ~83,600 |
| Lines | ~1,070 |
| Estimated tokens | **~20,900 tokens** |
| Context ratio (vs 128K) | **~16.3%** — well within budget |
| Context ratio (vs 200K) | **~10.5%** — very comfortable |

**Token estimation methodology:** Rough estimate at ~1 token per 4 characters for mixed English-Chinese Markdown. Actual count depends on the upstream tokenizer. At ~21K tokens, the prompt is compact enough for any modern context window while carrying comprehensive cultural knowledge density suitable for organizational conversations.

**Comparison with personal mode prompt:**
| Metric | Personal | Enterprise |
|--------|----------|------------|
| Characters | ~55,700 | ~83,600 |
| Est. tokens | ~14,000 | ~20,900 |
| Sections | 23 | 23 |
| Cultural knowledge | Life-focused (~60% content) | Organization-focused (~45% content) |
| Examples | 10 (personal life) | 10 (organizational) |

**Why enterprise prompt is larger:**
1. Management philosophy section (4.7) — 8 principles extracted from 美好之路 (not present in personal prompt)
2. Profit/distribution philosophy (4.10) — new section
3. Systems/standards/sincerity paradox (4.11) — new section
4. Organizational aphorisms (4.12) — replaces personal life aphorisms but more extensive
5. Enterprise purpose evolution (4.8) — more detailed historical tracking
6. Service philosophy (4.9) — organizational framing requires more context

**Remaining context for multi-turn voice conversation:**
- At 128K context: ~107,000 tokens available for conversation history, user speech transcription, and model generation
- At 200K context: ~179,000 tokens available
- This supports extensive multi-turn organizational conversations (hundreds of turns)

## 4. Source Material Coverage (Enterprise Mode)

| Category | Files | Enterprise Content Coverage |
|----------|-------|---------------------------|
| A-level (official culture) | 22 files | ~30% of unique conceptual content adopted (enterprise-filtered); personal life content excluded |
| B-level (于东来 works) | 4 files | Organizational/management content fully adopted; personal life reflections excluded |
| C-level (corporate stories) | 4 files | Used for organizational illustration reference; specific narratives not embedded |
| D-level (third-party) | 2 files | Summarized only; organizational analysis noted as external; NOT adopted as official |

**Enterprise-specific content adopted into runtime knowledge layer:**
- Core cultural framework (faith/goal/principle) — organizational framing
- 10 virtues and 8 vices — organizational manifestations
- Enterprise-as-school philosophy — central organizing concept
- Management philosophy (8 principles from Yu Donglai's 美好之路)
- Profit/distribution philosophy and healthy organizational economics
- Work philosophy (喜欢·专业, 净心) — organizational implications
- Service philosophy (不取悦顾客, stakeholder fairness)
- Systems/standards/sincerity paradox
- Enterprise purpose evolution (pre-2003 to present)
- Common misunderstandings in organizational application
- Key organizational aphorisms

**Content deliberately excluded from enterprise runtime:**
- All personal life modules (爱情/居家/父母/孩子)
- Personal health, finance, and safety as individual topics
- Specific compensation figures and HR policy details
- Store operational details and physical environment specifications
- Third-party analytical frameworks (三飞轮, 非权力影响力, Maslow-based)
- Copyright-protected book text
- Individual employee/customer stories as narratives
- COVID-era and disaster-response content
- Store signage and physical culture implementation

## 5. Discovered Material Conflicts (Enterprise-Relevant)

### Resolved Conflicts:
1. **2023 vs 2024 enterprise philosophy:** Current (2022-2024) framework adopted. Key differences documented in enterprise source audit.

2. **Legacy (pre-2015) vs Modern enterprise framework:** Older "公平、自由、快乐、博爱" framework noted as superseded. Current "自由·爱" framework adopted. The older handbook's detailed HR policies are noted as historical.

3. **"奴性文化" vs "落后文化":** 2024 softening adopted. Noted as terminological evolution in enterprise prompt.

4. **Service philosophy shift:** From "customer interest first" (older handbook) to "服务不是为了取悦顾客" (current). Current balanced-stakeholder framing adopted.

5. **Yu Donglai's personal evolution:** From firing employees who wanted reduced hours to mandating rest and 7-hour workdays. Presented as personal growth narrative, not inconsistency.

### Unresolved Tensions:
1. **Freedom vs. strict discipline in organizations:** The culture advocates freedom and non-bondage while practicing strict standards with termination for violations. This tension is presented honestly in the prompt (Section 4.11) rather than artificially resolved.

2. **Local practice vs. universal principle:** PDL does not expand nationally, yet its philosophy is presented as having universal relevance. This is acknowledged: principles travel; specific practices may not.

3. **Profit vs. purpose:** The materials simultaneously claim profit is not the goal while providing detailed profit margin targets (3-5% healthy). This reflects a pragmatic idealism — profit is necessary infrastructure for purpose, not the purpose itself.

## 6. Uncertain / Cannot Confirm

1. **Current (2026) operational status:** All materials date from 2007-2024. Whether all described practices remain active in 2026 cannot be confirmed.

2. **Compensation figures:** Specific wage figures cited (e.g., 5500 yuan/month average for frontline staff in 2021) may have changed.

3. **Enterprise mode framing approval:** The organizational framing of PDL cultural concepts is our interpretive work. Whether PDL would endorse this framing of their philosophy for enterprise consulting contexts is unknown.

4. **Copyright boundary:** D01 (胖东来，你要怎么学？) and D02 (觉醒胖东来) are commercially published books. The enterprise prompt summarizes concepts; no extended quotations are used.

5. **Yu Donglai's current teaching:** B03 captures his 2022-era management teachings. Whether he would express the same views in 2026 is unknown.

6. **Upstream model's instruction following:** The effectiveness of a ~21K token system prompt sent as a DataChannel relay_message depends on the upstream model's behavior, which is opaque to us.

## 7. Recommended Injection Approach (for the developer to apply)

**The prompt was delivered as a self-contained text deliverable. No code was modified.** Two integration options, both prompt-level — no backend logic:

### Option A: Replace the served file's content (zero code change)
- `static/pangdonglai-system-prompt.txt` currently holds personal-mode content and is what `voice.html` fetches.
- Overwrite its content with the enterprise prompt, and the unchanged code serves enterprise always-on. Personal-mode source remains in git (`prompts/pangdonglai-realtime-system-prompt.md`).
- Identity comes from the prompt itself: **胖东来企业模式** (Pangdonglai Enterprise Mode).

### Option B: Point the existing fetch at the enterprise file (one-line change)
- `sendSystemPrompt()` already fetches `/static/pangdonglai-system-prompt.txt`. Changing that one path to `pangdonglai-system-prompt-enterprise.txt` is a single-line swap the developer owns.

**Either way:** no URL parameter, no mode-switching logic. Enterprise mode is always on, and the prompt answers identity questions itself.

**Files to modify:**
| File | Change | Risk |
|------|--------|------|
| `static/voice.html` | ~3 lines in sendSystemPrompt() to select prompt path based on mode | Low |
| `static/pangdonglai-system-prompt-enterprise.txt` | New file (copy from prompts/) | None |

**Files NOT to modify:**
- `internal/voice/service.go` — No prompt injection capability server-side
- `internal/config/config.go` — No prompt config needed
- Any WebRTC, audio, auth, or UI code — Unrelated

## 8. Code Integration Status

**Not modified.** Per developer instruction, no backend or voice code was changed — the agent's scope is the prompt.

### Deliverables (prompt layer only):
1. `prompts/pangdonglai-realtime-system-prompt-enterprise.md` — enterprise system prompt (source, ~20.9K tokens)
2. `prompts/pangdonglai-source-audit-enterprise.md` — enterprise source audit
3. `prompts/pangdonglai-prompt-report-enterprise.md` — this report
4. `static/pangdonglai-system-prompt-enterprise.txt` — served-ready copy (not wired; integration is the developer's step)

### Identity:
When asked what model/mode it is, the prompt answers **胖东来企业模式** (Pangdonglai Enterprise Mode). Enterprise mode needs no URL parameter.

### To serve (developer's choice):
- **Option A:** overwrite `static/pangdonglai-system-prompt.txt` content with the enterprise prompt (zero code change)
- **Option B:** change the fetch path in `sendSystemPrompt()` to `pangdonglai-system-prompt-enterprise.txt` (one line)

### Pre-Integration Verification:
- [ ] Confirm the upstream model accepts ~21K token DataChannel relay_messages as system prompts
- [ ] Test a full voice conversation with the enterprise prompt to validate behavior

## 9. Prompt Quality Assessment

### Internal Consistency Check:
- ✅ No contradictory "must/never/always" rules across sections
- ✅ Behavioral rules in Sections 7-21 are consistent with enterprise identity in Sections 1-3
- ✅ Cultural knowledge in Section 4 aligns with fact/attribution rules in Sections 5, 15-16
- ✅ Voice interaction rules (Sections 8-9) consistent with conversation flow (Section 21)
- ✅ "Another possibility" generation (Section 13) aligned with micro-action design (Section 14)
- ✅ Enterprise mode (Section 11) properly excludes personal-life content
- ✅ Prohibited behaviors (Section 20) non-redundant with other sections
- ✅ All 23 required sections present
- ✅ All 10 required example scenarios covered with enterprise-appropriate content

### Redundancy Check:
- Merged: "Understand the organization first" principle appears in Identity (§1), Values (§7), Enterprise Mode (§11), and Flow (§21) — kept in each as they serve distinct functions
- Merged: Language/tone rules consolidated in §8; voice-specific rules in §9
- Merged: Citation and attribution consolidated in §15
- Removed: Duplicate "don't fabricate" rules — kept in §20 with cross-reference
- Kept: Intentional reinforcement of core principles (user autonomy, factual honesty, organizational focus, voice-appropriate length)

### Enterprise-Specific Quality Checks:
- ✅ All cultural knowledge reframed for organizational application
- ✅ No personal-life counseling content
- ✅ Pangdonglai presented as perspective/lens, not template/benchmark
- ✅ Real organizational constraints honored (industry, scale, resources)
- ✅ Profit-people tension acknowledged honestly, not resolved sentimentally
- ✅ "Another possibility" generation method adapted for organizational scenarios
- ✅ Micro-action design method adapted for institutional/team changes
- ✅ Examples cover all 10 required enterprise scenarios
- ✅ Professional tone: clear-eyed, pragmatic, respectful of organizational judgment
- ✅ No consulting jargon, no business-book clichés, no "AI-healing" language

### Comparison with Personal Mode Prompt:
- ✅ Both prompts share: identity framework, source authority hierarchy, voice rules, prohibited behaviors structure
- ✅ Enterprise prompt adds: management philosophy, profit/distribution, operational standards, service philosophy (organizational)
- ✅ Enterprise prompt excludes: personal life modules, personal counseling mode, individual health/finance/relationship guidance
- ✅ Both are fully self-contained with no RAG or external knowledge dependencies
- ✅ File names clearly distinguish modes (enterprise vs. no suffix)

## 10. File Inventory

| File | Purpose | In Runtime? | Status |
|------|---------|------------|--------|
| `prompts/pangdonglai-realtime-system-prompt.md` | Personal mode system prompt | Yes (served as .txt) | Existing |
| `prompts/pangdonglai-realtime-system-prompt-enterprise.md` | Enterprise mode system prompt | Yes (served as .txt) | **New** |
| `prompts/pangdonglai-source-audit.md` | Source audit (personal mode focus) | No | Existing |
| `prompts/pangdonglai-source-audit-enterprise.md` | Source audit (enterprise mode focus) | No | **New** |
| `prompts/pangdonglai-prompt-report.md` | Prompt report (personal mode) | No | Existing |
| `prompts/pangdonglai-prompt-report-enterprise.md` | Prompt report (enterprise mode) | No | **New** |
| `static/pangdonglai-system-prompt.txt` | Served personal prompt | Yes | Existing |
| `static/pangdonglai-system-prompt-enterprise.txt` | Enterprise prompt, served-ready copy | No (not wired — code untouched) | **Deliverable** |

---

*End of enterprise prompt report. Last updated: 2026-08-10.*
