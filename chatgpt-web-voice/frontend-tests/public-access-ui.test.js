import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

const voiceHTML = await readFile(new URL("../static/voice.html", import.meta.url), "utf8");
const loginHTML = await readFile(new URL("../static/login.html", import.meta.url), "utf8");
const sessionsHTML = await readFile(new URL("../static/sessions.html", import.meta.url), "utf8");

test("voice page switches management controls by public auth state", () => {
  assert.match(voiceHTML, /id="adminGatewaySection"[^>]*hidden/);
  assert.match(voiceHTML, /id="guestGatewaySection"[^>]*hidden/);
  assert.match(voiceHTML, /\/api\/auth\/session/);
  assert.match(voiceHTML, /\/login\?next=%2Fkeys/);
  assert.doesNotMatch(voiceHTML, /response\.status === 401[\s\S]{0,160}window\.location\.replace\('\/login'\)/);
  assert.match(voiceHTML, /window\.location\.replace\('\/voice'\)/);
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
