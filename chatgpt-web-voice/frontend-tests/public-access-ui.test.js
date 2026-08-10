import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import vm from "node:vm";

const voiceHTML = await readFile(new URL("../static/voice.html", import.meta.url), "utf8");
const voiceRoomCSS = await readFile(new URL("../static/voice-room.css", import.meta.url), "utf8");
const voiceRoomJS = await readFile(new URL("../static/voice-room.js", import.meta.url), "utf8");
const loginHTML = await readFile(new URL("../static/login.html", import.meta.url), "utf8");
const sessionsHTML = await readFile(new URL("../static/sessions.html", import.meta.url), "utf8");
const recordsHTML = await readFile(new URL("../static/records.html", import.meta.url), "utf8");

test("voice page switches management controls by public auth state", () => {
  assert.match(voiceHTML, /id="adminGatewaySection"[^>]*hidden/);
  assert.match(voiceHTML, /id="guestGatewaySection"[^>]*hidden/);
  assert.match(voiceHTML, /\/api\/auth\/session/);
  assert.match(voiceHTML, /\/login\?next=%2Fkeys/);
  assert.doesNotMatch(voiceHTML, /response\.status === 401[\s\S]{0,160}window\.location\.replace\('\/login'\)/);
  assert.match(voiceHTML, /window\.location\.replace\('\/voice'\)/);
});

test("voice page prioritizes visual assets without warming the microphone on load", () => {
  assert.match(voiceHTML, /rel="preload" as="image" href="\/static\/assets\/voice-room\/natural-room-wide\.png"/);
  assert.match(voiceHTML, /rel="preload" as="fetch" href="\/static\/models\/agent-particles\.bin"/);
  assert.doesNotMatch(voiceHTML, /setTimeout\(warmMicPermission/);
  assert.match(voiceHTML, /if \(languageMenuNeedsRender\) renderLanguageMenuOptions\(\)/);
});

test("conversation mode is session-bound and prompt initialization gates user input", () => {
  assert.match(voiceHTML, /id="conversationModeSwitch"/);
  assert.match(voiceHTML, /\.conversation-mode-switch \{[\s\S]{0,420}pointer-events: auto/);
  assert.match(voiceHTML, /data-conversation-mode="personal"/);
  assert.match(voiceHTML, /data-conversation-mode="organization"/);
  assert.match(voiceHTML, /prompt_injected_for/);
  assert.match(voiceHTML, /pangdonglai-system-prompt\.txt/);
  assert.match(voiceHTML, /pangdonglai-system-prompt-enterprise\.txt/);
  assert.doesNotMatch(voiceHTML, /system_prompt: true/);
  assert.match(voiceHTML, /ignoredPromptMessageIds\.has/);
  assert.match(voiceHTML, /conversation mode cannot change after the conversation starts|conversationHasStarted/);
  assert.match(voiceHTML, /return !!inCall && isDataChannelReady\(\) && contextReady && !sending/);
  assert.match(voiceHTML, /await preloadSystemPrompt\(conversationMode\);[\s\S]{0,180}localStream = await requestMic\(\)/);
  assert.match(voiceHTML, /var welcomeComplete = waitForPromptWelcome\(\);[\s\S]{0,180}await welcomeComplete;[\s\S]{0,80}setContextReady\(true\)/);
  assert.match(voiceHTML, /notePromptWelcomeState\(st\)/);
  assert.match(voiceHTML, /function onVoiceChannelReady\(\) \{\s*if \(!inCall \|\| !isDataChannelReady\(\)\) return/);
  assert.doesNotMatch(voiceHTML, /fetch\('\/static\/pangdonglai-system-prompt\.txt'\)[\s\S]{0,260}sendVoicePreviewGreeting\(\)/);
});

test("text-only QA mode uses a silent media track and never starts microphone recording", () => {
  assert.match(voiceHTML, /new URLSearchParams\(window\.location\.search\)\.get\('text_only'\) === '1'/);
  assert.match(voiceHTML, /audioCtx\.createMediaStreamDestination\(\)/);
  assert.match(voiceHTML, /gain\.gain\.value = 0/);
  assert.match(voiceHTML, /if \(textOnlySession \|\| microphoneRecording/);
  assert.match(voiceHTML, /if \(silentMicSource\) \{[\s\S]{0,220}silentMicSource\.stop\(\)[\s\S]{0,220}silentMicSource = null/);
});

test("room background keeps motion while avoiding layout-driven drift", () => {
  assert.match(voiceRoomJS, /roomBackgroundImage\.decode\(\)/);
  assert.match(voiceRoomCSS, /data-room-background-state="loading"/);
  const drift = voiceRoomCSS.match(/@keyframes room-leaf-drift \{[\s\S]*?\n\}/)?.[0] || "";
  assert.match(drift, /transform:\s*translate3d/);
  assert.doesNotMatch(drift, /margin:/);
});

test("settings overlay is outside the workspace stacking context", () => {
  assert.match(voiceHTML, /<\/main>\s*<\/div>\s*<aside id="settingsPanel"/);
});

test("administrator login requires and submits Turnstile", () => {
  assert.match(loginHTML, /challenges\.cloudflare\.com\/turnstile\/v0\/api\.js/);
  assert.match(loginHTML, /id="turnstileWidget"/);
  assert.match(loginHTML, /turnstile_token:\s*turnstileToken/);
  assert.match(loginHTML, /\/api\/auth\/config/);
  assert.match(loginHTML, /window\.location\.replace\(data\.redirect \|\| '\/keys'\)/);
});

test("admin session inventory exposes guest stats and filtering", () => {
  assert.match(sessionsHTML, /id="guestCount"/);
  assert.match(sessionsHTML, /<option value="guest">游客<\/option>/);
  assert.match(sessionsHTML, /stats\.guest/);
});

test("voice page records the microphone through a bounded best-effort upload queue", () => {
  assert.match(voiceHTML, /new MediaRecorder\(stream/);
  assert.match(voiceHTML, /RECORDING_CHUNK_MS = 5000/);
  assert.match(voiceHTML, /RECORDING_AUDIO_BITS_PER_SECOND = 16000/);
  assert.match(voiceHTML, /RECORDING_MAX_CHUNKS = 86400/);
  assert.match(voiceHTML, /RECORDING_MAX_PENDING_BYTES = 32 << 20/);
  assert.match(voiceHTML, /RECORDING_GLOBAL_MAX_PENDING_BYTES = 64 << 20/);
  assert.match(voiceHTML, /RECORDING_FETCH_TIMEOUT_MS = 8000/);
  assert.match(voiceHTML, /abortBackgroundRecordingUploads\('new call took priority/);
  assert.doesNotMatch(voiceHTML, /microphone recording skipped while previous recording finalizes/);
  assert.match(voiceHTML, /state\.uploadDisabled = true/);
  assert.match(voiceHTML, /startMicrophoneRecording\(\)\.catch/);
  assert.match(voiceHTML, /finishMicrophoneRecording\(updateText \? 'hangup' : 'transport_end'\)/);
  assert.match(voiceHTML, /completionError\.retryAfterMs/);
  assert.match(voiceHTML, /callStartController\.abort\(\)/);
  assert.match(voiceHTML, /retrying voice session without stale account binding/);
  assert.match(voiceHTML, /Recording is strictly best-effort/);
});

test("streaming conversation persistence is coalesced and bounded", () => {
  assert.match(voiceHTML, /CONVERSATION_WRITE_DEBOUNCE_MS = 1500/);
  assert.match(voiceHTML, /CONVERSATION_WRITE_TIMEOUT_MS = 8000/);
  assert.match(voiceHTML, /conversationPendingWrites\.set\(/);
  assert.match(voiceHTML, /priority: 'low'/);
  assert.match(voiceHTML, /async function flushPendingConversationWrites\(\)/);
  assert.match(voiceHTML, /flushPendingConversationWrites\(\)\.catch[\s\S]{0,180}setTimeout\(resolve, 1200\)/);
});

test("streaming updates keep only the latest payload while a write is pending", async () => {
  const block = voiceHTML.match(/  function scheduleConversationWriteFlush\([^)]*\) \{[\s\S]*?\n  function resetCurrentSessionHistory\(\)/)?.[0]
    ?.replace(/\n  function resetCurrentSessionHistory\(\)$/, "");
  assert.ok(block, "conversation persistence block not found");
  const requests = [];
  const context = {
    AbortController,
    Map,
    Promise,
    Array,
    JSON,
    String,
    encodeURIComponent,
    setTimeout: () => 1,
    clearTimeout: () => {},
    conversationRequest: async (path, options) => {
      requests.push({ path, body: JSON.parse(options.body) });
      return {};
    },
    toast: () => {},
    t: (key) => key,
    newClientMessageId: () => "generated",
  };
  vm.runInNewContext(`
    var conversationReady = true;
    var activeSessionId = "cv_test";
    var conversationWriteChain = Promise.resolve();
    var conversationWriteTimer = 0;
    var conversationPendingWrites = new Map();
    var conversationWriteInFlight = false;
    var CONVERSATION_WRITE_DEBOUNCE_MS = 1500;
    var CONVERSATION_WRITE_TIMEOUT_MS = 8000;
    var conversationSyncErrorShown = false;
    ${block}
    globalThis.persistence = {
      persistConversationMessage,
      drainPendingConversationWrites,
      flushPendingConversationWrites,
      pending: conversationPendingWrites
    };
  `, context);

  context.persistence.persistConversationMessage({ clientId: "msg_test", role: "assistant", content: "Hel" });
  context.persistence.persistConversationMessage({ clientId: "msg_test", role: "assistant", content: "Hello" });
  assert.equal(context.persistence.pending.size, 1);
  assert.equal(requests.length, 0);
  await context.persistence.drainPendingConversationWrites();
  assert.equal(requests.length, 1);
  assert.equal(requests[0].body.content, "Hello");
  assert.equal(context.persistence.pending.size, 0);

  let retryAttempts = 0;
  context.conversationRequest = async () => {
    retryAttempts += 1;
    if (retryAttempts === 1) {
      const error = new Error("rate limited");
      error.status = 429;
      error.retryAfterMs = 1000;
      throw error;
    }
    return {};
  };
  context.persistence.persistConversationMessage({ clientId: "msg_retry", role: "user", content: "Retry me" });
  await context.persistence.drainPendingConversationWrites();
  assert.equal(context.persistence.pending.size, 1);
  await context.persistence.flushPendingConversationWrites();
  assert.equal(retryAttempts, 2);
  assert.equal(context.persistence.pending.size, 0);

  let repeatedFailures = 0;
  context.conversationRequest = async () => {
    repeatedFailures += 1;
    if (repeatedFailures < 4) {
      const error = new Error("temporary failure");
      error.status = 503;
      throw error;
    }
    return {};
  };
  context.persistence.persistConversationMessage({ clientId: "msg_multi_retry", role: "assistant", content: "Eventually saved" });
  await context.persistence.flushPendingConversationWrites();
  assert.equal(repeatedFailures, 4);
  assert.equal(context.persistence.pending.size, 0);
});

test("admin recording page supports playback, transcript inspection, and deletion", () => {
  assert.match(recordsHTML, /\/api\/admin\/recordings/);
  assert.match(recordsHTML, /<audio id="detailAudio"[^>]*controls/);
  assert.match(recordsHTML, /id="transcriptList"/);
  assert.match(recordsHTML, /method: 'DELETE'/);
});
