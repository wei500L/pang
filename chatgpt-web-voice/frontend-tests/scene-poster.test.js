import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

const voiceHTML = await readFile(new URL("../static/voice.html", import.meta.url), "utf8");
const voiceRoomCSS = await readFile(new URL("../static/voice-room.css", import.meta.url), "utf8");

test("completed scene keeps the image first and treats essay as supporting copy", () => {
  const completed = voiceHTML.slice(voiceHTML.indexOf('id="sceneStateCompleted"'), voiceHTML.indexOf('id="sceneHistory"'));
  assert.match(voiceHTML, /id="sceneParticleField"/);
  assert.match(completed, /id="sceneEssayBlock"/);
  assert.match(completed, /id="sceneClosing"/);
  assert.match(completed, /id="btnSceneImmersive"/);
  assert.match(completed, /class="scene-figure"/);
  assert.match(completed, /id="sceneImage"/);
  assert.doesNotMatch(completed, /id="sceneParticleStage"/);
  assert.ok(
    voiceHTML.indexOf('id="sceneParticleField"') < voiceHTML.indexOf('id="sceneStateCompleted"'),
    "particle field should live behind the conversation, not inside the studio card"
  );
  assert.doesNotMatch(voiceHTML, /id="sceneSeriesLabel"/);
  assert.match(voiceRoomCSS, /\.scene-completed\.has-essay|\.has-essay/);
  assert.match(voiceRoomCSS, /\.scene-immersive-caption/);
  assert.match(voiceRoomCSS, /\.scene-particle-field/);
  assert.doesNotMatch(voiceRoomCSS, /\.scene-completed\.is-poster/);
  assert.doesNotMatch(voiceRoomCSS, /\.scene-immersive-essay/);
});

test("essay layout falls back to caption when essay is absent", () => {
  assert.match(voiceHTML, /function sceneHasEssay\(/);
  assert.match(voiceHTML, /classList\.toggle\('has-essay', hasEssay\)/);
  assert.match(voiceHTML, /sceneCaption\.hidden = hasEssay/);
  assert.match(voiceHTML, /sceneEssayBlock\.hidden = !hasEssay/);
  assert.match(voiceHTML, /sceneImmersiveEssay\.hidden = !hasEssay/);
});

test("completed scene becomes a dedicated reader above a compact voice dock", () => {
  assert.match(voiceRoomCSS, /\[data-scene-state="completed"\] \.welcome-state/);
  assert.match(voiceRoomCSS, /\.scene-completed\.has-essay \{[\s\S]{0,160}grid-template-columns:/);
  assert.match(voiceRoomCSS, /\.scene-completed\.has-essay \.scene-figure/);
  assert.match(voiceRoomCSS, /\[data-scene-state="completed"\] \.composer-dock/);
  assert.match(voiceRoomCSS, /\[data-scene-state="completed"\] \.voice-wave \{[\s\S]{0,60}display: none/);
  assert.match(voiceHTML, /voice-room\.css\?v=scene-reader-/);
});
