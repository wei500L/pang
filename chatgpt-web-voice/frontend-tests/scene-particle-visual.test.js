import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

import {
  SCENE_PARTICLE_DEFAULTS,
  buildParticleAttributes,
  calculateVisualWeight,
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
  assert.ok(Math.abs(attributes.positions[0] + 1) < 0.01);
  assert.equal(attributes.positions[1], 1);
  assert.equal(attributes.positions[2], 0);
  assert.ok(attributes.positions[3] >= -1 && attributes.positions[3] <= -0.68);
  assert.equal(attributes.positions[4], -1);
  assert.equal(attributes.positions[5], 0);
  assert.equal(attributes.colors[0], 1);
  assert.ok(Math.abs(attributes.colors[1] - 128 / 255) < 1e-6);
  assert.equal(attributes.colors[2], 0);
  assert.ok(Math.abs(attributes.opacities[1] - 128 / 255) < 1e-6);
  assert.equal(attributes.worldWidth, 4);
  assert.equal(attributes.worldHeight, 4);
  assert.equal(attributes.visualWeights.length, attributes.count);
  assert.equal(attributes.edges.length, attributes.count);
  assert.equal(attributes.luminances.length, attributes.count);
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

test("visual weighting favors edges, chroma, and midtones over flat bright backgrounds", () => {
  const flatBright = calculateVisualWeight({ luminance: 0.86, chroma: 0.01, edge: 0.005 });
  const coloredSubject = calculateVisualWeight({ luminance: 0.34, chroma: 0.48, edge: 0.06 });
  const subjectEdge = calculateVisualWeight({ luminance: 0.32, chroma: 0.12, edge: 0.72 });
  const usefulDark = calculateVisualWeight({ luminance: 0.1, chroma: 0.08, edge: 0.18 });
  assert.ok(coloredSubject > flatBright);
  assert.ok(subjectEdge > flatBright);
  assert.ok(usefulDark > flatBright);
  [flatBright, coloredSubject, subjectEdge, usefulDark].forEach((value) => assert.ok(value >= 0.22 && value <= 1));
});

test("content-aware attributes and jitter are deterministic, bounded, and break the regular grid", () => {
  const width = 8;
  const height = 8;
  const pixels = new Uint8ClampedArray(width * height * 4);
  for (let y = 0; y < height; y += 1) {
    for (let x = 0; x < width; x += 1) {
      const offset = (y * width + x) * 4;
      const subject = x >= 3 && x <= 4;
      pixels[offset] = subject ? 190 : 245;
      pixels[offset + 1] = subject ? 72 : 243;
      pixels[offset + 2] = subject ? 42 : 238;
      pixels[offset + 3] = 255;
    }
  }
  const first = buildParticleAttributes({ width, height, data: pixels }, { step: 2 });
  const second = buildParticleAttributes({ width, height, data: pixels }, { step: 2 });
  assert.deepEqual(first.positions, second.positions);
  assert.deepEqual(first.visualWeights, second.visualWeights);
  assert.deepEqual(first.edges, second.edges);
  assert.equal(first.positions.length, first.count * 3);
  assert.equal(first.visualWeights.length, first.count);
  assert.equal(first.edges.length, first.count);
  first.visualWeights.forEach((value) => assert.ok(value >= 0.219 && value <= 1));
  first.edges.forEach((value) => assert.ok(value >= 0 && value <= 1));
  assert.ok(Array.from(first.positions).some((value, index) => index % 3 !== 2 && Math.abs(value * 2 - Math.round(value * 2)) > 1e-4));
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

test("shader implements content-aware bounded depth, curl flow, highlight compression, and circular points", () => {
  assert.match(particleModule, /attribute float aVisualWeight/);
  assert.match(particleModule, /attribute float aEdge/);
  assert.match(particleModule, /attribute float aLuminance/);
  assert.match(particleModule, /float snoise\(vec3 v\)/);
  assert.match(particleModule, /vec3 curlNoise\(vec3 p, float epsilon\)/);
  assert.match(particleModule, /return vec3\(dPsiDy, -dPsiDx, depthBreath \* 0\.28\)/);
  assert.match(particleModule, /float vulnerability = clamp\(\(1\.0 - aVisualWeight\)/);
  assert.match(particleModule, /float dispersionCurve = uDispersion \* uDispersion/);
  assert.match(particleModule, /highlightCompression = mix\(1\.0, 0\.72/);
  assert.match(particleModule, /displaced\.z = clamp\(displaced\.z, -1\.05, 1\.18\)/);
  assert.match(particleModule, /gl_PointSize = clamp\(uPointSize \* uPixelRatio \* perspectiveScale/);
  assert.match(particleModule, /float radius = length\(centered\)/);
  assert.match(particleModule, /if \(radius > 0\.5\) discard/);
  assert.match(particleModule, /#include <colorspace_fragment>/);
});

test("core and halo share geometry while using distinct blend states", () => {
  assert.match(particleModule, /this\.corePoints = new THREE\.Points\(this\.geometry, this\.coreMaterial\)/);
  assert.match(particleModule, /this\.haloPoints = new THREE\.Points\(this\.geometry, this\.haloMaterial\)/);
  assert.match(particleModule, /transparent: true/);
  assert.match(particleModule, /depthWrite: false/);
  assert.match(particleModule, /depthTest: true/);
  assert.match(particleModule, /layer > 0\.5 \? THREE\.AdditiveBlending : THREE\.NormalBlending/);
  assert.match(particleModule, /geometry\.setAttribute\("aVisualWeight"/);
  assert.match(particleModule, /geometry\.setAttribute\("aEdge"/);
});

test("runtime quality, visibility, context recovery, reduced motion, presets, and debug metrics are wired", () => {
  assert.match(particleModule, /new IntersectionObserver/);
  assert.match(particleModule, /this\.intersecting = Boolean/);
  assert.match(particleModule, /if \(this\.qualityLevel === 1\) this\.dprScale = 0\.78/);
  assert.match(particleModule, /this\.haloPoints\.visible = this\.haloEnabled/);
  assert.match(particleModule, /this\.detailScale = 0/);
  assert.match(particleModule, /buildParticleAttributes\(this\.lastImageData, \{ step: nextStep/);
  assert.match(particleModule, /webglcontextrestored/);
  assert.match(particleModule, /async restoreContext\(\)/);
  assert.match(particleModule, /restored: true/);
  assert.match(particleModule, /uMotionScale\.value = this\.reducedMotion \? 0 : 1/);
  assert.match(particleModule, /if \(!this\.reducedMotion \|\| this\.outgoingLayers\?\.length\) this\.schedule\(\)/);
  assert.match(particleModule, /setPreset\(name\)/);
  assert.match(particleModule, /still: Object\.freeze/);
  assert.match(particleModule, /ethereal: SCENE_PARTICLE_DEFAULTS/);
  assert.match(particleModule, /dissolve: Object\.freeze/);
  assert.match(particleModule, /FPS · DPR/);
});

test("async image versions and lifecycle cleanup prevent stale installs and accumulated resources", () => {
  assert.match(particleModule, /const version = \+\+this\.loadVersion/);
  assert.match(particleModule, /this\.loadController\?\.abort\(\)/);
  assert.match(particleModule, /version !== this\.loadVersion \|\| this\.disposed/);
  assert.match(particleModule, /this\.intersectionObserver\?\.disconnect\(\)/);
  assert.match(particleModule, /this\.resizeObserver\?\.disconnect\(\)/);
  assert.match(particleModule, /this\.releaseRenderResources\(\)/);
  assert.match(voiceHTML, /sceneParticleRequestVersion \+= 1/);
  assert.match(voiceHTML, /removeEventListener\('scene-particle-ready', sceneHandleParticleReady\)/);
  assert.match(voiceHTML, /addEventListener\('scene-particle-ready', sceneHandleParticleReady\)/);
  assert.match(voiceHTML, /requestVersion !== sceneParticleRequestVersion/);
  assert.match(voiceHTML, /typeof sceneParticleVisual\.canRender === 'function'/);
});

test("scene UI lazy-loads one particle stage with fullscreen and static-image fallback", () => {
  assert.match(voiceHTML, /id="sceneParticleStage"/);
  assert.match(voiceHTML, /id="sceneParticleDialog"/);
  assert.match(voiceHTML, /import\('\/static\/scene-particle-visual\/index\.js'\)/);
  assert.match(voiceHTML, /window\.PangSceneParticleVisual/);
  assert.match(voiceHTML, /sceneParticleDialogMount\.appendChild\(sceneParticleStage\)/);
  assert.match(voiceHTML, /sceneVisualSlot\.insertBefore\(sceneParticleStage/);
  assert.match(voiceHTML, /sceneImage\.src = project\.image_url/);
  assert.match(voiceHTML, /sceneSetAgentVisualActive\(!particlesActive\)/);
  assert.match(voiceHTML, /sceneSetAgentVisualActive\(true\)/);
  assert.match(voiceRoomCSS, /\.scene-visual-slot\.is-particle-ready \.scene-image \{[\s\S]{0,180}opacity: 0\.07/);
  assert.match(voiceRoomCSS, /object-fit: contain/);
  assert.match(voiceRoomCSS, /\.scene-particle-stage\.is-immersive/);
  assert.match(voiceRoomCSS, /touch-action: pan-y/);
  assert.match(voiceRoomCSS, /touch-action: none/);
});
