# Pangdonglai Enterprise Source Audit — System Prompt Knowledge Base

This document records which source materials were consulted, assessed, and selected for the enterprise-mode system prompt. It supplements the original `pangdonglai-source-audit.md` (which focused on personal-mode content selection) with enterprise-specific analysis. This file is for development reference only and does NOT enter the model context.

---

## Enterprise Mode Content Selection Overview

The enterprise prompt draws from the same comprehensive source inventory as the personal-mode prompt (see `pangdonglai-source-audit.md` for the complete 33-file inventory). However, content selection priorities differ significantly:

| Category | Personal Mode | Enterprise Mode |
|----------|--------------|-----------------|
| Core faith/goal/principle | Used for personal life framing | Used for organizational framing |
| Happy Life State framework | Core framework (8 modules) | Referenced only for work/rest philosophy |
| Love/partnership, parenting, home | Fully adopted | **Excluded** (out of scope) |
| Finance, health, safety | Fully adopted for personal context | Referenced only for organizational policy implications |
| Enterprise philosophy (经营理念) | Excluded | **Fully adopted** |
| Enterprise-as-school philosophy | Adopted for personal application | **Fully adopted as central concept** |
| Management philosophy | Excluded | **Fully adopted** |
| Operational standards (商品/环境/人员/服务/系统) | Excluded | **Adopted as organizational design reference** |
| Profit/distribution philosophy | Excluded | **Fully adopted** |
| Enterprise evolution/history | Adopted for factual Q&A | **Fully adopted for factual Q&A** |
| Yu Donglai's speeches/books | Personal life content adopted | **Organizational content adopted** |
| Corporate stories | Illustration material | Illustration material (organizational examples only) |
| Third-party analysis | Summarized only | Summarized only (organizational analysis only) |
| Older handbook (指导手册) | Historical reference only | Historical reference — legacy belief system noted |

---

## Enterprise-Specific Content Selection

### A-Level Materials Adopted for Enterprise Prompt

| File | Personal Mode | Enterprise Mode | Notes |
|------|-------------|-----------------|-------|
| A01 胖东来文化理念.md | Adopted (core definitions) | Adopted (core definitions) | Faith/goal/principle definitions are shared foundation |
| A05 幸福生命状态（2024）.md | Adopted as primary framework | Partially adopted | Work philosophy and 道德观 adopted; personal life modules (爱情/居家/父母/孩子) excluded |
| A06 文化理念分享（关于生活）.md | Adopted | **Excluded** | Personal life focus; not relevant to enterprise mode |
| A07 文化理念分享（关于企业）.md | **Excluded** | **Fully adopted** | Enterprise-focused poster content; key aphorisms and operational philosophy |
| A08 胖东来经营理念.md | Excluded | **Fully adopted** | Core operational philosophy: 商品/环境/人员/服务/系统 standards |
| A19 胖东来是一所学校，而非一个企业.md | Adopted (selected) | **Fully adopted** | Yu Donglai's 2021 speech; enterprise-as-school, democratic management, work-life philosophy |
| A20 胖东来企业文化指导手册.PDF.md | Historical reference | Historical reference | Legacy belief system (公平、自由、快乐、博爱); noted as superseded |
| A16 新员工文化培训——关于工作.md | Adopted | Partially adopted | Work philosophy, 净心, craftsmanship; personal stories excluded |
| A17 员工手册.md | Partially adopted | Partially adopted | Leave policies noted for factual Q&A; specific welfare amounts excluded |
| A21 企业文化理念门店植入参考标准.md | Excluded | **Adopted as reference** | Physical culture implementation; demonstrates "culture made visible" |
| A03 幸福生命状态手册.md | Adopted | **Excluded** | Personal life assessment framework; not relevant to enterprise mode |

### B-Level Materials Adopted for Enterprise Prompt

| File | Personal Mode | Enterprise Mode | Notes |
|------|-------------|-----------------|-------|
| B03 美好之路（于东来）.md | Partially adopted (life philosophy) | **Fully adopted (management/organizational content)** | Management philosophy, wage philosophy, profit distribution, team building, enterprise adjustment methodology |
| B01 走在信仰的路上——东来随笔.md | Adopted | Partially adopted | Organizational wisdom sections adopted; personal life reflections excluded |
| B02 心向阳光-于东来.md | Adopted with caution | Adopted with caution | Autobiographical context for understanding founder's evolution |
| B04 爱的传道者-于东来.md | Adopted | Partially adopted | Management teachings adopted; personal letters excluded |

### D-Level Materials Adopted for Enterprise Prompt

| File | Treatment |
|------|-----------|
| D01 胖东来，你要怎么学？.md | Summarized only; "非权力影响力" model noted as author's framework; practical learning methodology referenced |
| D02 觉醒胖东来.md | Summarized only; "三飞轮" model noted as author's framework; stakeholder analysis referenced; specific service examples excluded |

---

## Enterprise Mode Content Selection Principles

1. **Organization-first filter:** Content is selected only if it directly informs organizational design, management practice, team dynamics, or enterprise purpose. Personal life development content (relationships, parenting, individual health, home decoration) is excluded unless it illuminates a workplace policy implication.

2. **Principle over practice:** PDL's specific operational practices (exact wage figures, store counts, leave days, store closure schedules) are noted as context but NOT presented as benchmarks. The underlying principles (fairness in compensation, adequate rest, sustainable pace) are what travel.

3. **Current over historical:** The 2022-2024 framework (自由·爱 + 经营理念) takes precedence over the pre-2015 framework (公平、自由、快乐、博爱). Historical evolution is documented but clearly time-stamped.

4. **Yu Donglai's own words preferred for management philosophy:** For organizational content, B03 (美好之路) is the single richest source — it contains his direct teachings to other entrepreneurs about how to adjust, structure, and lead organizations. This is prioritized over third-party interpretation.

5. **Third-party frameworks excluded from runtime:** The "three flywheels" (文化理念/分配机制/运营系统), "non-power influence," and Maslow-based analyses are external authors' constructs. The enterprise prompt references the underlying realities these frameworks describe without adopting the frameworks themselves.

---

## Enterprise Cultural Knowledge Compression

The enterprise prompt's Section 4 (Core Cultural Knowledge) was compressed as follows:

| Source Material Volume | Compressed To | Compression Ratio |
|-----------------------|---------------|-------------------|
| ~500K+ characters (all enterprise-relevant sources) | ~40K characters (Section 4) | ~12:1 |
| A07 (Enterprise posters): 37 pages | 4.9-4.12 aphorisms + service philosophy | Condensed to key principles |
| A08 (经营理念): 82 lines, 5 dimensions | 4.6 work philosophy + 4.9 service philosophy | Dimensions integrated into principles |
| B03 (美好之路): 382 PDF pages | 4.7 management philosophy (8 principles) + 4.10 profit philosophy | ~40:1 compression |
| A19 (学校 speech): ~15,000 chars | 4.5 enterprise-as-school philosophy | ~5:1 compression |

Key compression techniques:
- Dimensions collapsed into principles (A08's 商品/环境/人员/服务/系统 → integrated across 4.7, 4.9, 4.11)
- Stories converted to illustrative principles (specific employee names/events → "one example")
- Repetitive poster content consolidated to aphorism collection (4.12)
- Historical details compressed to timeline of conceptual evolution (4.8)

---

## Version Differences — Enterprise-Relevant Concepts

In addition to the version differences documented in the original audit (自由, 爱, 平等, 善良, 戒恶, 健全人格, 幸福生命状态, etc.), the following enterprise-specific version differences were identified from the source materials:

### Enterprise Purpose (企业存在的目的)
- **Pre-2003:** "用真品换真心" — integrity in commerce
- **2003-2006:** "创造财富，播撒文明，分享快乐" — wealth creation + civilization
- **2006-2015:** "公平、自由、快乐、博爱" — older belief framework
- **2015-present:** "自由·爱" + "传播先进文化理念，培养健全人格" — person-centric purpose
- **Resolution:** Current formulation adopted. The shift from mission-centric to person-centric represents the most significant philosophical evolution.

### 经营理念 (Business Philosophy)
- **Earlier:** "发自内心的喜欢高于一切" (undifferentiated)
- **2024 refinement:** "喜欢·专业" — liking must be paired with professionalism
- **Resolution:** 2024 refinement adopted as primary. The addition of professionalism prevents romanticization of mere enthusiasm.

### 戒恶 (Vices) List
- **Pre-2015:** 5 vices (嫉妒、自私、贪婪、虚伪、自卑)
- **2023+:** 8 vices (adds 无知、束缚、伤害)
- **Resolution:** Current 8-vice list adopted. The expansion to include ignorance, bondage, and harm represents conceptual refinement.

### 奴性文化 Terminology
- **2023:** "恶" sourced from "奴性文化" (servile culture) — harsher term
- **2024:** "恶" sourced from "落后文化" (backward culture) — softer term
- **Resolution:** The softening is noted as a terminological evolution. Both terms describe the same phenomenon; the 2024 term is less confrontational.

### 服务 Philosophy
- **Older handbook:** Service is warm, happy, charming, professional, universal-love-oriented; "customer interest first" when conflicts arise
- **Current:** "服务不是为了取悦顾客" — service is NOT about pleasing customers; it flows from genuinely liking what you do; fairness between customer and employee rights
- **Resolution:** Current formulation adopted. The shift from customer-first to balanced stakeholder dignity is significant.

---

## Content Entering Enterprise Runtime Knowledge Layer

### Fully Adopted (Organizational Context):
1. Enterprise faith: 自由·爱 — organizational framing of all three concepts
2. 10 扬善 virtues and 8 戒恶 vices — organizational manifestations
3. 健全人格 five dimensions — organizational application
4. 道德观 key articles — organizational interpretation
5. Enterprise-as-school philosophy — central organizing concept
6. Work philosophy (喜欢·专业, 净心, flow state) — organizational implications
7. Management philosophy (8 core principles from B03)
8. Profit/distribution philosophy (healthy margins, fair sharing, investment as consumption)
9. Service philosophy (不取悦顾客, fairness between stakeholders)
10. Systems/standards/sincerity paradox
11. Enterprise purpose evolution
12. Key organizational aphorisms

### Adopted in Summarized Form:
1. Operational standards (商品/环境/人员/服务/系统) — principles, not specifications
2. Democratic management practices (竞聘, 民主评议) — methodology, not procedures
3. Yu Donglai's personal evolution as context for philosophical development
4. Historical framework evolution for factual Q&A

### Deliberately Excluded from Enterprise Runtime:
1. All personal life content (love, parenting, home, individual health, personal finance)
2. Specific compensation figures (exact salary bands, specific bonus amounts)
3. Store operational details (square footage, product listings, vendor names)
4. HR policy specifics (exact leave days, welfare amounts, disciplinary procedures)
5. Physical environment specifications (floor materials, equipment brands,装修 standards)
6. COVID-era content
7. Third-party analytical frameworks
8. Copyright-protected book text beyond brief paraphrasing
9. Individual employee/customer stories as specific narratives
10. Store signage and physical culture implementation details

### Uncertain / Cannot Confirm:
1. Current (2026) operational status of all described practices
2. Whether compensation figures cited in 2021-2022 remain current
3. Whether the enterprise mode's organizational framing is consistent with PDL's latest self-understanding
4. Whether Yu Donglai's management teachings to other enterprises (in B03) represent his current thinking

---

## Enterprise Mode — Material Quality Summary

- **Total enterprise-relevant source files:** ~18 (10 A-level + 4 B-level + 2 D-level + 2 additional)
- **Content entering enterprise runtime prompt:** ~30% of unique conceptual content across enterprise-relevant materials (vs. ~60% for personal mode, because enterprise mode is more selective about what constitutes core organizational knowledge)
- **Enterprise-specific compression ratio:** ~12:1 from source to runtime knowledge
- **Cross-reference with personal mode:** Core faith/goal/principle definitions shared; all personal-life modules excluded; management/operations philosophy added
- **Enterprise-unique content added:** Management philosophy (8 principles), profit/distribution philosophy, operational standards, enterprise-as-school as central concept, service philosophy (organizational framing), systems/sincerity paradox

---

*End of enterprise source audit. This file supplements `pangdonglai-source-audit.md` with enterprise-specific content selection analysis.*
