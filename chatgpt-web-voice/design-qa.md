# Design QA — 自由·爱语音对话页

## Comparison target

- Source visual truth: `/Users/oo/Downloads/ChatGPT Image 2026年8月9日 23_49_12.png`
- Implementation route: `http://127.0.0.1:8091/voice`
- Desktop implementation screenshot: `/Users/oo/project/pang/chatgpt-web-voice/tmp/design-qa/voice-desktop-final.png`
- Mobile implementation screenshot: `/Users/oo/project/pang/chatgpt-web-voice/tmp/design-qa/voice-mobile-final.png`
- Tablet implementation screenshot: `/Users/oo/project/pang/chatgpt-web-voice/tmp/design-qa/voice-tablet-768x1024.png`
- Logical desktop viewport: `1692 × 929`; the in-app browser capture surface returned `1417 × 929`, so the reference was cropped to the same visible `1417 × 929` region for pixel-scale comparison.
- Mobile viewport: `390 × 844`
- Tablet viewport: `768 × 1024`
- State: light theme, Chinese UI, one current AI subtitle and one current user subtitle. The reference shows a live listening call while the implementation evidence shows the ready state; structural layout, assets, typography, subtitle placement, and dock geometry were compared directly, while live waveform amplitude and call-label text were excluded from pixel-parity judgment.

## Visual evidence

- Full-view comparison: `/Users/oo/project/pang/chatgpt-web-voice/tmp/design-qa/comparison-full.png`
- Focused header comparison: `/Users/oo/project/pang/chatgpt-web-voice/tmp/design-qa/comparison-header.png`
- Focused hero/particle comparison: `/Users/oo/project/pang/chatgpt-web-voice/tmp/design-qa/comparison-hero.png`
- Focused voice-dock comparison: `/Users/oo/project/pang/chatgpt-web-voice/tmp/design-qa/comparison-dock.png`
- Focused regions were required because the brand wordmark, subtitle surfaces, particle silhouette, icon weight, waveform scale, borders, and dock elevation were too small to judge reliably from the full-view comparison alone.

## Findings

No actionable P0, P1, or P2 findings remain.

### Required fidelity surfaces

- Fonts and typography: the serif display hierarchy, two-line title wrapping, restrained body copy, monospaced subtitle metadata, navigation weight, and small control labels preserve the reference's optical hierarchy. Chinese display text uses `Noto Serif SC`/Songti fallbacks; UI text uses `DM Sans`, `Noto Sans SC`, and system fallbacks. No clipped or truncated primary copy remains at the tested widths.
- Spacing and layout rhythm: the desktop hero, AI subtitle, central DL mark, user subtitle, left session rail, and 920px voice dock retain the reference's overall composition. Desktop and mobile have no horizontal or vertical viewport overflow. Tablet and mobile active-conversation states intentionally hide the welcome copy so subtitles do not collide with it.
- Colors and visual tokens: warm ivory, natural wood, restrained Pangdonglai orange, muted sage/green state colors, warm-gray borders, and soft natural shadows consistently map to the requested brand direction. No black technology background, blue-purple neon, HUD, or heavy glass-card styling is present.
- Image quality and asset fidelity: the supplied natural-room imagery is used as the spatial background. The supplied wordmark and Pangdonglai badge were converted to clean transparent raster assets so their original crop backgrounds do not appear as rectangles. No visible target asset is replaced with custom SVG, emoji, placeholder art, or CSS illustration.
- Copy and content: brand headline, prompt, theme chips, connection state, transcript labels, optional text-input hint, and privacy copy are coherent in the standalone page and consistent across Chinese UI states.
- Icons: visible controls use one Material Symbols Rounded family with aligned optical weight and clear mic, speaker, call, settings, history, add, close, and send states.
- Responsiveness: verified at `1692 × 929`, `768 × 1024`, and `390 × 844`. Final measurements show `scrollWidth === innerWidth` and `scrollHeight === innerHeight` at desktop and mobile. The low-quality mobile path is active at 390px and suppresses expensive atmospheric layers.
- Accessibility: semantic buttons retain labels/titles and pressed/disabled states; the transcript remains an `aria-live` region; focus-visible styling is present; mobile tap targets remain practical; `prefers-reduced-motion` disables atmospheric motion and transitions; text and controls remain readable over the scene.
- Interaction states: settings open/close, mobile session drawer open/close, new conversation, empty state, session switching, disabled ready-state controls, and subtitle restoration were exercised in the browser. Browser console logs were empty after final desktop, tablet, and mobile captures.

## Comparison history

1. Earlier finding — P2 responsive overlap at `390 × 844`.
   - Evidence: the current AI subtitle covered the mobile welcome title and the user subtitle competed with the central mark.
   - Fix: hide `.welcome-copy` only when a transcript exists at the narrow breakpoint, move the user subtitle to a bottom anchor, and preserve the complete hero for empty sessions.
   - Post-fix evidence: `voice-mobile-final.png` shows separated AI/user subtitle layers, a readable central DL mark, a fully visible dock, and `390 × 844` with no overflow.

2. Earlier finding — P2 mobile session drawer was visually open but its controls were behind the generated page overlay.
   - Evidence: hit testing returned `BODY` at the close and new-session button centers.
   - Fix: lift the mobile drawer, remove the workspace stacking-context conflict while open, and make the decorative overlay ignore pointer events.
   - Post-fix evidence: close, new-session, empty-state, and session-restore interactions completed successfully through visible controls.

3. Earlier finding — P2 tablet title/subtitle collision and rectangular badge crop at `768 × 1024`.
   - Evidence: the assistant subtitle obscured the first line of the welcome title; the header badge retained wood pixels outside the badge pill.
   - Fix: extend active-transcript welcome-copy suppression through the 900px breakpoint and use a masked transparent raster badge.
   - Post-fix evidence: `voice-tablet-768x1024.png` shows separated subtitles, the DL mark as the focal layer, a clean badge silhouette, an unobstructed dock, and no viewport overflow.

4. Final desktop comparison.
   - Evidence: `comparison-full.png`, `comparison-header.png`, `comparison-hero.png`, and `comparison-dock.png` show aligned brand direction, composition, type hierarchy, warm palette, imagery, subtitle surfaces, and voice-control geometry.
   - Accepted intentional difference: the final particle silhouette is tighter and more legible than the more explosive reference frame. This follows the product requirement that audio response remain stable, restrained, and recognizably DL-shaped.

## Open questions

- None blocking. A future polish capture can compare the live listening and AI-speaking frames under an actual media session; this QA did not initiate a real upstream voice call or alter the working media connection solely to force a screenshot state.

## Implementation checklist

- [x] Preserve WebRTC, DataChannel, text, subtitle, session, and call-control IDs/handlers.
- [x] Feed microphone and assistant playback analyzers into separate smoothed particle feature channels.
- [x] Add warm room depth, sunlight, leaf shadow, restrained parallax, and device-quality degradation.
- [x] Validate desktop, tablet, mobile, reduced-motion CSS, low-quality CSS, empty state, transcript state, drawers, settings, and console output.
- [x] Run JavaScript syntax checks, audio-feature tests, Go tests, Go production build, duplicate-ID check, and `git diff --check`.

## Follow-up polish

- P3: capture and archive live listening/user-speaking/AI-speaking screenshots during a real demo session so the dynamic waveform and particle attack/release can be reviewed visually alongside the static design comparison.

final result: passed
