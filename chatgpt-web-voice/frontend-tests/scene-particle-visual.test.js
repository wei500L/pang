import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

import {
  SCENE_PARTICLE_DEFAULTS,
  buildParticleAttributes,
  chooseSceneParticleStep,
  normalizeSceneParticleUniforms,
  stableRandomVector,
} from "../static/scene-particle-visual/core.js";

const particleModule = await readFile(new URL("../static/scene-particle-visual/index.js", import.meta.url), "utf8");
const voiceHTML = await readFile(new URL("../static/voice.html", import.meta.url), "utf8");
const voiceRoomCSS = await readFile(new URL("../static/voice-room.css", import.meta.url), "utf8");

function imageData(width, height, pixels) {
  return { width, height, data: new Uint8ClampedArray(pixels) };
}

test("image sampling centers coordinates, flips Y, normalizes color, and skips transparent pixels", () => {
  const attributes = buildParticleAttributes(imageData(2, 2, [
    255, 128, 0, 255, 20, 30, 40, 0,
    0, 64, 255, 128, 255, 255, 255, 255,
  ]), { step: 1, worldHeight: 4, alphaThreshold: 0.05 });

  assert.equal(attributes.count, 3);
  assert.deepEqual(Array.from(attributes.positions.slice(0, 3)), [-1, 1, 0]);
  assert.deepEqual(Array.from(attributes.positions.slice(3, 6)), [-1, -1, 0]);
  assert.equal(attributes.colors[0], 1);
  assert.ok(Math.abs(attributes.colors[1] - 128 / 255) < 1e-6);
  assert.equal(attributes.colors[2], 0);
  assert.ok(Math.abs(attributes.opacities[1] - 128 / 255) < 1e-6);
  assert.equal(attributes.worldWidth, 4);
  assert.equal(attributes.worldHeight, 4);
});

test("sampling step is honored and fully transparent images fail safely", () => {
  const data = new Uint8ClampedArray(4 * 4 * 4);
  for (let i = 3; i < data.length; i += 4) data[i] = 255;
  const attributes = buildParticleAttributes({ width: 4, height: 4, data }, { step: 2 });
  assert.equal(attributes.count, 4);
  assert.equal(attributes.step, 2);
  assert.throws(() => buildParticleAttributes(imageData(1, 1, [255, 255, 255, 0])), /no visible pixels/);
});

test("stable random vectors are deterministic and bounded", () => {
  const first = stableRandomVector(17, 31);
  const second = stableRandomVector(17, 31);
  assert.deepEqual(first, second);
  assert.equal(first.length, 3);
  first.forEach((value) => assert.ok(value >= -1 && value <= 1));
  assert.notDeepEqual(first, stableRandomVector(18, 31));
});

test("uniform normalization preserves defaults and clamps public ranges", () => {
  assert.deepEqual(normalizeSceneParticleUniforms(), SCENE_PARTICLE_DEFAULTS);
  assert.deepEqual(normalizeSceneParticleUniforms({
    uFlowSpeed: 8,
    uDispersion: -1,
    uDepth: "3.4",
    uPointSize: Infinity,
  }), {
    uFlowSpeed: 2,
    uDispersion: 0,
    uDepth: 3.4,
    uPointSize: 2,
  });
});

test("quality selection follows desktop, ordinary, and constrained budgets", () => {
  assert.equal(chooseSceneParticleStep({ deviceMemory: 8, hardwareConcurrency: 8 }), 4);
  assert.equal(chooseSceneParticleStep({ deviceMemory: 4, hardwareConcurrency: 6 }), 5);
  assert.equal(chooseSceneParticleStep({ mobile: true, deviceMemory: 8, hardwareConcurrency: 8 }), 7);
  assert.equal(chooseSceneParticleStep({ reducedMotion: true, deviceMemory: 8, hardwareConcurrency: 8 }), 7);
});

test("shader and material implement luminance depth, simplex flow, perspective circles, and additive state", () => {
  assert.match(particleModule, /dot\(color, vec3\(0\.2126, 0\.7152, 0\.0722\)\)/);
  assert.match(particleModule, /float snoise\(vec3 v\)/);
  assert.match(particleModule, /vec3 fluidNoise\(vec3 p\)/);
  assert.match(particleModule, /uDispersion \* randomWeight/);
  assert.match(particleModule, /gl_PointSize = clamp\(uPointSize \* uPixelRatio \* perspectiveScale/);
  assert.match(particleModule, /length\(gl_PointCoord - vec2\(0\.5\)\)/);
  assert.match(particleModule, /smoothstep\(0\.26, 0\.5, distanceToCenter\)/);
  assert.match(particleModule, /transparent: true/);
  assert.match(particleModule, /depthWrite: false/);
  assert.match(particleModule, /depthTest: true/);
  assert.match(particleModule, /blending: THREE\.AdditiveBlending/);
});

test("scene UI lazy-loads one particle stage with fullscreen and static-image fallback", () => {
  assert.match(voiceHTML, /id="sceneParticleStage"/);
  assert.match(voiceHTML, /id="sceneParticleDialog"/);
  assert.match(voiceHTML, /import\('\/static\/scene-particle-visual\/index\.js'\)/);
  assert.match(voiceHTML, /window\.PangSceneParticleVisual/);
  assert.match(voiceHTML, /sceneParticleDialogMount\.appendChild\(sceneParticleStage\)/);
  assert.match(voiceHTML, /sceneVisualSlot\.insertBefore\(sceneParticleStage/);
  assert.match(voiceHTML, /sceneImage\.src = project\.image_url/);
  assert.match(voiceHTML, /sceneSetAgentVisualActive\(false\)/);
  assert.match(voiceHTML, /sceneSetAgentVisualActive\(true\)/);
  assert.match(voiceRoomCSS, /\.scene-visual-slot\.is-particle-ready \.scene-image \{[\s\S]{0,180}opacity: 0\.12/);
  assert.match(voiceRoomCSS, /\.scene-particle-stage\.is-immersive/);
  assert.match(voiceRoomCSS, /touch-action: pan-y/);
  assert.match(voiceRoomCSS, /touch-action: none/);
});
