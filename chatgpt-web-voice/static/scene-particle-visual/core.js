export const SCENE_PARTICLE_DEFAULTS = Object.freeze({
  uFlowSpeed: 0.5,
  uDispersion: 0.2,
  uDepth: 2.0,
  uPointSize: 2.0,
});

export const SCENE_PARTICLE_RANGES = Object.freeze({
  uFlowSpeed: Object.freeze([0, 2]),
  uDispersion: Object.freeze([0, 3]),
  uDepth: Object.freeze([0, 4]),
  uPointSize: Object.freeze([0.5, 6]),
});

function clamp(value, min, max) {
  return Math.min(max, Math.max(min, value));
}

export function normalizeSceneParticleUniforms(values = {}, current = SCENE_PARTICLE_DEFAULTS) {
  const normalized = {};
  for (const [key, range] of Object.entries(SCENE_PARTICLE_RANGES)) {
    const raw = values[key];
    const candidate = raw === null || raw === "" ? NaN : Number(raw);
    const fallback = Number.isFinite(Number(current[key])) ? Number(current[key]) : SCENE_PARTICLE_DEFAULTS[key];
    normalized[key] = Number.isFinite(candidate) ? clamp(candidate, range[0], range[1]) : clamp(fallback, range[0], range[1]);
  }
  return normalized;
}

function hash01(x, y, channel) {
  let value = Math.imul((x + 1) ^ Math.imul(channel + 11, 374761393), 668265263);
  value = Math.imul(value ^ Math.imul(y + 1, 2246822519), 3266489917);
  value ^= value >>> 13;
  value = Math.imul(value, 1274126177);
  value ^= value >>> 16;
  return (value >>> 0) / 4294967295;
}

export function stableRandomVector(x, y) {
  return [
    hash01(x, y, 0) * 2 - 1,
    hash01(x, y, 1) * 2 - 1,
    hash01(x, y, 2) * 2 - 1,
  ];
}

export function buildParticleAttributes(imageData, options = {}) {
  const width = Number(imageData && imageData.width);
  const height = Number(imageData && imageData.height);
  const data = imageData && imageData.data;
  if (!Number.isInteger(width) || width <= 0 || !Number.isInteger(height) || height <= 0 || !data || data.length < width * height * 4) {
    throw new TypeError("valid ImageData is required");
  }

  const requestedStep = Number(options.step);
  const step = Number.isFinite(requestedStep) ? Math.max(1, Math.floor(requestedStep)) : 5;
  const alphaThreshold = clamp(Number.isFinite(Number(options.alphaThreshold)) ? Number(options.alphaThreshold) : 0.05, 0, 1);
  const worldHeight = Number.isFinite(Number(options.worldHeight)) && Number(options.worldHeight) > 0 ? Number(options.worldHeight) : 4;
  const scale = worldHeight / height;
  const positions = [];
  const colors = [];
  const randoms = [];
  const opacities = [];

  for (let y = 0; y < height; y += step) {
    for (let x = 0; x < width; x += step) {
      const offset = (y * width + x) * 4;
      const alpha = data[offset + 3] / 255;
      if (alpha < alphaThreshold) continue;
      positions.push(
        (x - (width - 1) * 0.5) * scale,
        ((height - 1) * 0.5 - y) * scale,
        0,
      );
      colors.push(data[offset] / 255, data[offset + 1] / 255, data[offset + 2] / 255);
      randoms.push(...stableRandomVector(x, y));
      opacities.push(alpha);
    }
  }

  if (!opacities.length) throw new Error("image contains no visible pixels");
  return {
    positions: new Float32Array(positions),
    colors: new Float32Array(colors),
    randoms: new Float32Array(randoms),
    opacities: new Float32Array(opacities),
    count: opacities.length,
    step,
    width,
    height,
    worldWidth: width * scale,
    worldHeight,
  };
}

export function chooseSceneParticleStep(environment = {}) {
  const mobile = Boolean(environment.mobile);
  const reducedMotion = Boolean(environment.reducedMotion);
  const memory = Number(environment.deviceMemory || 4);
  const cores = Number(environment.hardwareConcurrency || 4);
  if (mobile || reducedMotion || memory <= 2 || cores <= 4) return 7;
  if (memory >= 8 && cores >= 8) return 4;
  return 5;
}
