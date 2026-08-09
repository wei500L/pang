# Design QA — 自由·爱实时语音页精修

## Comparison target

- Source visual truth: `/Users/oo/Downloads/ChatGPT Image 2026年8月9日 23_49_12.png`
- Implementation route used for visual QA: `http://127.0.0.1:8093/voice`
- Desktop implementation screenshot: `/Users/oo/project/pang/chatgpt-web-voice/tmp/design-qa/voice-desktop-audio-refined.png`
- Mobile implementation screenshot: `/Users/oo/project/pang/chatgpt-web-voice/tmp/design-qa/voice-mobile-audio-refined.png`
- Common laptop screenshot: `/Users/oo/project/pang/chatgpt-web-voice/tmp/design-qa/voice-1280x800-audio-refined.png`
- Logical desktop viewport: `1692 × 929`. The in-app capture surface returned the visible `1417 × 929` region, so the reference was cropped to the same region for full-view comparison.
- Additional verified viewports: `1920 × 1080`, `1440 × 900`, `1280 × 800`, and `390 × 844`.
- State: warm light theme with one assistant message and one user message. The source shows live listening while the implementation screenshot shows ready/idle; live waveform amplitude and the active-call label were excluded from static pixel-parity judgment.

## Visual evidence

- Full-view side-by-side comparison: `/Users/oo/project/pang/chatgpt-web-voice/tmp/design-qa/comparison-full-audio-refined.png`
- Header comparison: `/Users/oo/project/pang/chatgpt-web-voice/tmp/design-qa/comparison-header-audio-refined.png`
- Hero and particle comparison: `/Users/oo/project/pang/chatgpt-web-voice/tmp/design-qa/comparison-hero-audio-refined.png`
- Voice dock comparison: `/Users/oo/project/pang/chatgpt-web-voice/tmp/design-qa/comparison-dock-audio-refined.png`
- Focused logo comparison: `/Users/oo/project/pang/chatgpt-web-voice/tmp/design-qa/comparison-logo-audio-refined.png`
- Focused regions were required because the top-right logo edge treatment, particle depth, subtitle surfaces, and dock material could not be judged reliably from the full view alone.

## Findings

No actionable P0, P1, or P2 findings remain.

### Required fidelity surfaces

- Fonts and typography: the serif headline, body copy, navigation, monospaced subtitle metadata, and compact status labels preserve the source hierarchy. Primary text remains unclipped at all tested widths.
- Spacing and layout rhythm: the left history rail, headline block, central DL particle form, floating conversation cards, and bottom control dock retain the source composition. `scrollWidth === innerWidth` and `scrollHeight === innerHeight` at all tested viewports.
- Colors and visual tokens: warm ivory, wood, restrained orange, sage status accents, soft warm-gray borders, and directional morning light remain consistent. No neon, dark-tech, heavy bloom, or glass-heavy restyling was introduced.
- Image quality and asset fidelity: the supplied room images and GLB-derived particle asset remain in use. The top-right badge is no longer a cropped pill bitmap: the original DL pixels were isolated into a transparent monogram, while “胖东来” uses a neutral live-text fallback because the repository has no official SVG, AI, PDF, or higher-resolution wordmark. The logo graphic, text, and supporting container are now independent layers.
- Copy and content: the original brand headline, prompts, topic labels, conversation copy, status text, and controls remain intact.
- Particle material and state behavior: idle keeps a legible DL form; front/back particles now differ in point size, alpha, and directional light. Spectral centroid changes propagation direction and highlight position. User speech, thinking, assistant speech, interruption, and error each have distinct spring-smoothed motion targets without changing the warm palette.
- Audio response: adaptive peak normalization, dynamic noise-floor tracking, nonlinear soft limiting, separate attack/release, spectral flux, onset pulses, low/mid/high bands, and spectral centroid are implemented. Synthetic low-volume and low/high-centroid test frames pass.
- Responsiveness: desktop, common laptop, and mobile layouts have no viewport overflow. The mobile low-quality path is active at `390 × 844`, keeps the dock fully visible, hides the desktop logo lockup, and suppresses expensive atmosphere layers.
- Accessibility and reduced motion: semantic controls, labels, focus-visible styles, `aria-live` transcript/status regions, non-blocking decorative canvas, and reduced-motion behavior remain in place.
- Interaction states: mobile session drawer and settings open/close behavior were exercised. A P2 hidden-panel edge leak found during QA was fixed by moving the closed settings panel fully beyond the viewport.
- Browser console: a fresh final browser tab reported no errors or warnings after the final shader and layout fixes.

## Comparison history

1. Earlier finding — P1 particle canvas disappeared after the shader expansion.
   - Evidence: WebGL reported a vertex shader syntax error because `centroid` is a reserved GLSL qualifier.
   - Fix: rename the local shader value to `spectralCenter` and reload the actual page.
   - Post-fix evidence: the final desktop, laptop, and mobile screenshots all show the DL particle form; the fresh final browser tab has no shader errors.

2. Earlier finding — P2 the closed settings panel left a 14px light strip on the right edge.
   - Evidence: the panel used `right: 14px` with `translateX(100%)`, leaving part of its surface inside the viewport at desktop and mobile sizes.
   - Fix: move the closed panel by `calc(100% + 20px)` while preserving the higher-specificity open-state transform.
   - Post-fix evidence: the final `1280 × 800` and `390 × 844` captures have no right-edge strip and no overflow.

3. Earlier finding — P2 the top-right logo was a single low-resolution pill crop.
   - Evidence: source and previous implementation bundled the DL mark, Chinese wordmark, beige pill, and captured background in one 160 × 68 raster.
   - Fix: extract only the official DL pixels to a transparent high-density asset, render the Chinese name as a neutral fallback, and let CSS own the separate material container and safe spacing.
   - Post-fix evidence: `comparison-logo-audio-refined.png` shows a clean page-matched container with no wood rectangle or object-position crop.

4. Final visual comparison.
   - Evidence: the full, hero, logo, and dock comparison images show the preserved composition, warm natural room, brand hierarchy, card depth, and interaction geometry.
   - Accepted intentional difference: the implementation screenshot is idle and therefore keeps the DL silhouette tighter and quieter than the reference listening frame. The active response is driven by real analyser features rather than a permanently explosive static pose.

## Open questions

- No design blocker remains. A complete end-to-end live-call capture still needs a signed-in gateway session plus an approved OS microphone prompt; the automated environment reached the microphone authorization/connecting state but could not complete that system-level approval.
- The neutral “胖东来” text fallback should be replaced if an official vector or high-resolution transparent wordmark becomes available.

## Implementation checklist

- [x] Preserve the existing WebRTC, DataChannel, authentication, session, subtitle, and control wiring.
- [x] Separate the DL graphic from its container and remove the screenshot-like badge background.
- [x] Add adaptive amplitude normalization and spectral-centroid extraction.
- [x] Map user speech, thinking, assistant speech, interruption, and error into spring-smoothed particle states.
- [x] Add depth-sensitive point size, alpha, lighting, state-aware air/light response, and restrained UI depth.
- [x] Verify desktop, common laptop, mobile, drawer, settings, reduced-quality behavior, overflow, and browser console.
- [x] Run JavaScript syntax checks, five audio-feature tests, and `git diff --check`.

## Follow-up polish

- P3: replace the neutral Chinese wordmark with an official vector asset when the brand source file is available.
- P3: archive a real listening/user-speaking/thinking/assistant-speaking capture during a signed-in demo call for motion-direction review.

final result: passed
