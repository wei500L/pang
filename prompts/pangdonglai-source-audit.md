# Pangdonglai Source Audit — System Prompt Knowledge Base

This document records every source file consulted, its authority level, quality assessment, and disposition for the runtime system prompt. It is for development reference only and does NOT enter the model context.

---

## Source Inventory

### A-Level: Current Normative Cultural Materials

Primary knowledge base for the Agent. These are the most authoritative current articulations of Pangdonglai culture.

| # | File | Lines | Date/Version | Authority | OCR Quality | Status |
|---|------|-------|-------------|-----------|-------------|--------|
| A01 | 胖东来文化理念.md | 177 | 2023.01 (文化部), 2022.12 (运营) | Official — Culture Dept | Clean | **Adopted** — Core faith/goal/principle definitions, 扬善/戒恶 terminology |
| A02 | 胖东来企业文化理念.md | 221 | No explicit date | Official — ~90% overlap with A01 | Minor OCR issues (尊世→普世, 体现有→体面) | **Redundant** — Content substantially covered by A01; minor variant wording not adopted |
| A03 | 幸福生命状态手册.md | 1654 | 2023 | Official — HR assessment system | Clean | **Adopted** — 7-module assessment framework, reward linkage, detailed rubrics. Note: 2023 edition; 2024 update in A10 takes precedence where they differ |
| A04 | 幸福生命状态手册-胖东来.md | 1499+ | 2023 | Official | Clean | **Redundant** — Near-duplicate of A03 (~95% overlap); minor formatting differences only |
| A05 | 幸福生命状态（2024）.md | 705 | 2024 | Official — Latest edition | Minor OCR at truncation points | **Adopted as primary** — 2024 updates: 道德观 (12 articles), 思想状态 section, 安全 as 8th module, 喜欢·专业 refinement, 幸福的胖东来人标准. Takes precedence over A03 where they differ |
| A06 | 胖东来文化理念分享（关于生活）.md | 565 | Copyright 2015-2024 | Official — Poster/slide collection | Variable; some bilingual artifacts | **Partially adopted** — Aphorisms and life philosophy content adopted; decorative/poster design content discarded |
| A07 | 胖东来文化理念分享（关于企业）.md | 301 | Copyright 2015-2024 | Official — Enterprise-focused | Clean | **降权** — Enterprise/management content; minimal personal-life relevance. Not included in runtime prompt |
| A08 | 胖东来经营理念.md | 82 | 2022.12 (文化小组) | Official — Operations dept | Clean | **降权** — Pure operations content; subset of A01/A02. Not included |
| A09 | 文化分享——关于爱情.md | 271 | No date; trainer presentation | Official — Training dept | Minor OCR; generally clean | **Adopted** — Active/passive love framework, self-love prerequisite, East-West comparison. Core personal-life content |
| A10 | 文化分享——关于理财.md | 225 | No date | Official — Training dept | Clean | **Adopted** — "能力大于欲望" principle, "人生十大奢侈品", salary-based financial plans, housing ≤30% guidance |
| A11 | 文化分享——关于健康.md | 374 | No date | Official — Training dept | Clean (visual page-by-page verified) | **Adopted** — Holistic health framework, mental-physical integration, self-acceptance, Nordic lifestyle inspiration |
| A12 | 文化分享——关于喜欢.md | 158 | No date; trainer: 徐一冉 | Official — Training dept | Clean | **Adopted** — "喜欢" vs mere "工作" distinction, creating vs passive duty, finding value in small acts |
| A13 | 如何与父母相处.md | 217 | No date | Official — Training dept | Clean | **Adopted** — "孝而有道" principle, financial support guidelines (10-20%), independence framework, death/dying perspectives |
| A14 | 如何对待孩子.md | 262 | 2020.01.01 | Official — Training dept | Clean (structured transcription) | **Adopted** — Khalil Gibran poem, critique of exam-oriented education, age-stage guidance, parent self-assessment |
| A15 | 新员工文化培训——关于生活.md | 301 | No date | Official — New employee orientation | **Significant OCR issues**: 休质→体质, 患上眼镜→戴上眼镜, 暖他吃水果→喂他吃水果, and multiple garbled passages | **Partially adopted with caution** — Life planning content adopted; specific garbled passages excluded from direct citation |
| A16 | 新员工文化培训——关于工作.md | 297 | No date | Official — New employee orientation | Clean (structured transcription) | **Adopted** — Work philosophy, 净心 flow state, craftsmanship, handling mistakes |
| A17 | 员工手册.md | 420 | 2023.06.01 effective | Official — HR policy | Clean (text-layer verified) | **Partially adopted** — General employment practices (30-day leave, no-overtime policy) noted for factual Q&A; detailed policy text not embedded |
| A18 | 文化理念分享.md | 589 | Copyright 2015-2024 | Official — Comprehensive slide deck | Variable; decorative slides have bilingual artifacts | **Partially adopted** — Core aphorisms and life philosophy adopted; decorative layout content discarded |
| A19 | 胖东来是一所学校，而非一个企业.md | 369 | 2021.12.20 (于东来 speech) | Official — Verbatim speech transcript | Clean (spoken-language disfluencies are natural, not OCR) | **Adopted** — Enterprise-as-school philosophy, democratic management, personal health narrative, work-life balance principle |
| A20 | 胖东来企业文化指导手册.PDF.md | 654 | Contains 2008 & 2009 letters | **Legacy** — Older edition handbook | Generally clean | **降权 as historical** — Uses older belief system (公平、自由、快乐、博爱), older HR policies, older vice list (5 not 8). Used only for understanding cultural evolution; NOT adopted as current |
| A21 | 企业文化理念门店植入参考标准.md | 205 | No date | Official — Store operations | Clean (visual verified) | **降权** — Store signage and physical environment standards. No personal-life relevance |
| A22 | 新员工培训课件5-《员工人身安全》.md | 413 | 2020 | Official — Safety training | Clean | **降权** — Safety protocols (robbery, theft, fraud, etc.). Minimal personal-life philosophy content |

### B-Level: Yu Donglai's Own Works

Authoritative for understanding cultural formation and authentic expression. Must note time period and context.

| # | File | Lines/Pages | Author | Type | OCR Quality | Status |
|---|------|------------|--------|------|-------------|--------|
| B01 | 走在信仰的路上——东来随笔.md | ~3771 lines | 于东来 | Personal spiritual/philosophical collection | Good; some image placeholders | **Adopted** — Faith definition, life principles, philosophical reflections. Core aphorisms used |
| B02 | 心向阳光-于东来.md | ~437 PDF pages | 于东来【著】 | Autobiography + Weibo posts (2012-2014) + speeches | **Moderate** — Significant OCR errors in Weibo sections; misrecognized characters throughout | **Adopted with caution** — Personal story (childhood, arrests, business failures, health struggles) used for understanding authenticity. Direct quotes only from clearly readable sections |
| B03 | 美好之路（于东来）.md | ~382 PDF pages | 于东来 著, 联商网 编 | Business lectures (2022) | **Moderate** — Tables, charts, calculations heavily garbled | **Partially adopted** — Personal philosophy content adopted; enterprise management details (salary tables, org charts) discarded |
| B04 | 爱的传道者-于东来.md | ~314 PDF pages | 于东来 | Autobiography + management teachings + letters + media | Good | **Adopted** — Letters and personal reflections; duplicate autobiography content cross-referenced with B02 |

### C-Level: Corporate Stories

Illustrate culture in practice. Individual stories are NOT universal rules.

| # | File | Lines/Pages | Publisher | Year | OCR Quality | Status |
|---|------|------------|-----------|------|-------------|--------|
| C01 | 胖东来故事手册（一）.md | ~1500 lines | 河南胖东来商贸集团, 人力资源部 | 2007 | Good; one name marked "[字迹不清]" | **Adopted as illustration material** — Employee service stories showing cultural values in action. Used as narrative reference, not policy |
| C02 | 胖东来故事手册（二）.md | ~135 PDF pages | 河南胖东来商贸集团 | — | Good | **Adopted as illustration material** — Same treatment as C01 |
| C03 | 爱的路上释放温暖的力量.md | ~223 PDF pages | Internal publication | 2022 (于东来 preface 2022.7.31) | Good | **Adopted as illustration material** — Word frequency analysis (爱 2201次, 幸福 2020次, 美好 1920次) noted |
| C04 | 新乡故事手册——致敬逆行者.md | ~85 PDF pages | Internal publication | 2020 | Good | **降权** — COVID-specific response stories. Contextually narrow; not adopted for general use |

### D-Level: Third-Party Analysis

External observations only. MUST NOT be presented as official position or Yu Donglai's view.

| # | File | Pages | Author | Publisher | Year | Status |
|---|------|-------|--------|-----------|------|--------|
| D01 | 胖东来，你要怎么学？.md | ~256 | 王慧中 (Pangdonglai 文化战略顾问 at time) | 龙门书局 (Science Press) | 2014 | **降权 — Copyright-protected.** Summarized only. Author's analytical frameworks are her own, not PDL official. 于东来's foreword ("爱的传道者") is quoted more freely |
| D02 | 觉醒胖东来.md | ~213 | 刘杨 (independent researcher) | 中国广播影视出版社 | 2023 | **降权 — Copyright-protected.** Summarized only. "三个飞轮" model, Maslow-based analysis are author's frameworks. External critiques noted but not adopted as factual |

---

## Version Differences: Key Concepts

### 自由 (Freedom)
- **2024 edition:** Bounded by law and humanitarian principles; guarantees equal rights. "不束缚自己、更不束缚他人"
- **2023 edition:** Same core definition; less explicit about the "万万事万物" extension
- **Legacy (pre-2015):** Part of "公平、自由、快乐、博爱" — less philosophically developed
- **Resolution:** 2024 definition adopted as primary. Earlier versions noted when historically relevant.

### 爱 (Love)
- **2024 edition:** Adds "爱万事万物" explicitly. Active, based on equality and free will.
- **2023 edition:** Same core; "万事万物" implicit but not stated.
- **Legacy:** "博爱" used as one of four faith pillars.
- **Resolution:** 2024 definition adopted. The expansion to "all things" is noted as a 2024 refinement.

### 平等 (Equality)
- Consistent across all versions. Foundation of both freedom and love.
- **Resolution:** Stable concept. No version conflict.

### 善良 (Goodness/Kindness)
- **2024 edition:** Embedded in 道德观 (Moral Code), 扬善 framework
- **Earlier materials:** More emphasis on 善良 as individual virtue; less systematized
- **Resolution:** 2024 systematization adopted. Earlier emphasis on individual practice retained as complementary.

### 戒恶 (Abstain from Evil)
- **2024 edition:** 8 vices with precise definitions. "恶" sourced from "落后文化" (backward culture)
- **2023 edition:** Same 8 vices. "恶" sourced from "奴性文化" (servile culture) — harsher term
- **Legacy (pre-2015):** Only 5 vices (嫉妒、自私、贪婪、虚伪、自卑)
- **Resolution:** 2024 edition adopted. The softening from "奴性文化" to "落后文化" is noted as a terminological evolution. The expansion from 5 to 8 vices represents conceptual refinement.

### 健全人格 (Healthy Personality)
- **2024 edition:** First explicit five-dimension definition (人格权/世界观/生命观/价值观/自然观)
- **2023 edition:** Mentioned as goal but not systematically defined
- **Legacy:** Not a central concept
- **Resolution:** 2024 definition adopted as the clearest articulation.

### 幸福生命状态 (Happy Life State)
- **2024 edition:** 8 modules (adds 安全), expanded 思想状态 section, 道德观, 工作状态 4-dimension rubric
- **2023 edition:** 7 modules, no 道德观, simpler framework
- **Resolution:** 2024 edition adopted as primary. 2023 retained where rubrics are more detailed (爱情/居家/父母/孩子 modules are largely identical).

### 企业与人的关系 (Enterprise-Person Relationship)
- **2024/2023:** 企业是学校，不是企业 — enterprise serves human flourishing
- **Legacy:** "创造快乐、分享快乐、传播快乐" — mission-focused, less individual-centric
- **Resolution:** Current formulation adopted. The evolution from mission-centric to individual-centric framing is a significant historical shift.

### 工作与生活的关系 (Work-Life Relationship)
- **2024/2023:** Work serves life. 7-hour workday. Tuesday closure. 30-day leave.
- **Legacy:** Less generous policies. Yu Donglai's earlier stance was more work-focused.
- **Resolution:** Current standards adopted. The evolution reflects Yu Donglai's own personal development from work-centric to life-centric values.

### 企业存在的目的 (Purpose of Enterprise)
- **2024/2023:** 传播先进文化理念，培养健全人格 — cultural transmission through commerce
- **Legacy:** 创造财富，播撒文明，分享快乐 — more traditional CSR framing
- **Resolution:** Current formulation adopted. The shift from "creating wealth and spreading civilization" to "cultivating healthy personalities" is the most significant philosophical evolution.

---

## Duplicate Identification and Resolution

### Near-Duplicates (>80% overlap):

| Files | Overlap | Resolution |
|-------|---------|------------|
| A01 (胖东来文化理念) ≈ A02 (胖东来企业文化理念) | ~90% | A01 adopted as primary; A02's minor variants and evaluation tables noted but not separately adopted |
| A03 (幸福生命状态手册) ≈ A04 (幸福生命状态手册-胖东来) | ~95% | A03 adopted; A04 confirmed as separately-sourced duplicate |
| A08 (胖东来经营理念) ⊂ A01/A02 operations section | 100% | Fully contained in A01; not separately adopted |

### Repeated Content Across Files:

- Core 文化理念 triad appears in A01, A02, A03, A05, A07
- "头等舱船票" parable: A15, A13
- 申红丽 father-with-cancer story: A15, A13
- Nietzsche "起舞" quote: A03, A04, A05, A18, A12
- Resolution: Core content consolidated once in prompt; stories used as illustration material only

---

## OCR Risk Assessment

### High Risk (significant garbled content — avoid direct citation):
- A15 (新员工文化培训——关于生活): Multiple garbled passages in narrative sections. Core principles readable; specific case study details unreliable.
- B02 (心向阳光-于东来): Weibo sections have many misrecognized characters. Autobiography sections are more reliable.
- B03 (美好之路): Tables and financial calculations are heavily corrupted. Philosophical content is readable.

### Moderate Risk (occasional issues — verify before citing):
- A02 (胖东来企业文化理念): Minor character swaps; core content unaffected.
- A05 (幸福生命状态2024): Truncation artifacts at some section boundaries.
- A18 (文化理念分享): Bilingual decorative text has OCR artifacts from slides.

### Low Risk (generally clean):
- All remaining files. Core philosophical content is reliably captured.

---

## Content Entering Runtime Knowledge Layer

The following content was selected for embedding in the system prompt's cultural knowledge section:

### Fully Adopted (verbatim or near-verbatim definitions):
1. Enterprise faith: 自由·爱 and its conceptual relationship to 平等
2. 10 扬善 virtues and 8 戒恶 vices with precise definitions
3. 健全人格 five-dimension definition (2024)
4. 道德观 key articles (2024): 不快乐不道德, 为他人而活不道德, etc.
5. Happy Life State 8-module framework with level descriptions
6. Work philosophy: 发自内心的喜欢高于一切 → 喜欢·专业
7. Core aphorisms: 明世理活自己, 幸福是状态而非心态, etc.
8. "生命在于释放而非修行" principle

### Adopted in Summarized/Paraphrased Form:
1. Love/partnership philosophy (active vs. passive love, self-love prerequisite)
2. Parent relationship framework (孝而有道, financial boundaries)
3. Child-raising philosophy (children belong to society, age-stage guidance)
4. Financial philosophy (能力大于欲望, 人生十大奢侈品, housing ≤30%)
5. Health philosophy (mental-physical integration, self-acceptance)
6. Enterprise-as-school concept (applied to personal life: work as one domain of flourishing)
7. Historical evolution of concepts

### Deliberately Excluded from Runtime:
1. Detailed HR policies (specific leave days, compensation tables, disciplinary procedures)
2. Store operations standards (environment, merchandising, service protocols)
3. Enterprise management philosophy (democratic election systems, organizational design)
4. Store signage and physical environment specifications
5. Safety training protocols
6. COVID-specific stories and pandemic response
7. Third-party analytical frameworks (王慧中's non-power influence model, 刘杨's three flywheels)
8. Copyright-protected book content beyond brief paraphrasing
9. Individual employee stories as specific narratives (used for tone calibration only)
10. Financial reward amounts (RMB figures for happiness bonuses)

### Uncertain / Cannot Confirm:
1. Current exact employee count or revenue figures
2. Current specific store policies beyond what is in the materials
3. Whether all described practices are still active in 2026
4. Exact wording of some OCR-damaged passages in A15 and B02
5. Whether some stories in C-level materials have been embellished in retelling

---

## Material Quality Summary

- **Total source files reviewed:** 33 (22 A-level + 4 B-level + 4 C-level + 2 D-level + 1 additional)
- **Files with significant OCR risk:** 4 (A15, B02, B03, A18 partial)
- **Duplicate/overlapping files:** 3 pairs identified and resolved
- **Copyright-sensitive files:** 2 (D01, D02) — summarized only
- **Legacy/deprecated versions:** 1 (A20) — historical reference only
- **Content entering runtime prompt:** ~60% of unique conceptual content across A-level and B-level materials, focused exclusively on personal-life relevance
- **Content deliberately excluded:** Enterprise management, store operations, HR policy details, third-party frameworks, copyright-protected text, OCR-unreliable passages

---

*End of source audit. This file is for development reference and does not enter the model context.*
