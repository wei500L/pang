import test from "node:test";
import assert from "node:assert/strict";

import { AudioFeatureExtractor } from "../static/agent-visual/audio-features.js";

function frame(amplitude, frequencyLevel = 0, length = 512) {
  const time = new Uint8Array(length);
  const frequency = new Uint8Array(length / 2);
  for (let i = 0; i < time.length; i += 1) {
    time[i] = Math.max(0, Math.min(255, Math.round(128 + Math.sin(i * 0.19) * amplitude * 128)));
  }
  frequency.fill(Math.max(0, Math.min(255, Math.round(frequencyLevel * 255))));
  return { time, frequency };
}

test("silence remains idle after calibration", () => {
  const extractor = new AudioFeatureExtractor();
  let features;
  const quiet = frame(0.004, 0.005);
  for (let i = 0; i < 180; i += 1) {
    features = extractor.process(quiet.time, quiet.frequency, 48000, i * 16.67);
  }
  assert.ok(features.energy < 0.04, `energy=${features.energy}`);
  assert.equal(features.speechActive, false);
});

test("speech onset reacts quickly and then decays continuously", () => {
  const extractor = new AudioFeatureExtractor();
  const quiet = frame(0.004, 0.005);
  const voice = frame(0.18, 0.32);
  for (let i = 0; i < 90; i += 1) extractor.process(quiet.time, quiet.frequency, 48000, i * 16.67);
  let sawOnset = false;
  let features;
  for (let i = 90; i < 96; i += 1) {
    features = extractor.process(voice.time, voice.frequency, 48000, i * 16.67);
    sawOnset ||= features.onset;
  }
  assert.equal(sawOnset, true);
  assert.equal(features.speechActive, true);
  assert.ok(features.energy > 0.45, `energy=${features.energy}`);
  const peak = features.energy;
  for (let i = 96; i < 190; i += 1) features = extractor.process(quiet.time, quiet.frequency, 48000, i * 16.67);
  assert.ok(features.energy < peak);
  assert.ok(features.energy < 0.08, `decayed energy=${features.energy}`);
  assert.equal(features.speechActive, false);
});

test("short pauses retain the speech gate without retriggering immediately", () => {
  const extractor = new AudioFeatureExtractor();
  const voice = frame(0.15, 0.28);
  const quiet = frame(0.002, 0.003);
  let features = extractor.process(voice.time, voice.frequency, 48000, 1000);
  assert.equal(features.speechActive, true);
  features = extractor.process(quiet.time, quiet.frequency, 48000, 1120);
  assert.equal(features.speechActive, true);
  features = extractor.process(quiet.time, quiet.frequency, 48000, 1420);
  assert.equal(features.speechActive, false);
});

