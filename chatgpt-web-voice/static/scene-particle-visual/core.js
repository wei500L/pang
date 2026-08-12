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

function smoothstep(edge0, edge1, value) {
  const x = clamp((value - edge0) / Math.max(1e-6, edge1 - edge0), 0, 1);
  return x * x * (3 - 2 * x);
}

function srgbToLinear(value) {
  return value <= 0.04045 ? value / 12.92 : Math.pow((value + 0.055) / 1.055, 2.4);
}

function pixelMetrics(data, offset) {
  const r = data[offset] / 255;
  const g = data[offset + 1] / 255;
  const b = data[offset + 2] / 255;
  const luminance = 0.2126 * srgbToLinear(r) + 0.7152 * srgbToLinear(g) + 0.0722 * srgbToLinear(b);
  const chroma = Math.max(r, g, b) - Math.min(r, g, b);
  return { r, g, b, luminance, chroma };
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

export function calculateVisualWeight({ luminance, chroma, edge, alpha = 1 }) {
  const midtone = 1 - Math.min(1, Math.abs(luminance - 0.36) / 0.46);
  const usefulDark = smoothstep(0.012, 0.2, luminance) * (1 - smoothstep(0.58, 0.94, luminance));
  const flatHighlight = smoothstep(0.62, 0.92, luminance)
    * (1 - smoothstep(0.025, 0.18, edge))
    * (1 - smoothstep(0.04, 0.28, chroma));
  const weight = 0.28
    + smoothstep(0.018, 0.28, edge) * 0.42
    + smoothstep(0.035, 0.42, chroma) * 0.25
    + midtone * 0.2
    + usefulDark * 0.12
    - flatHighlight * 0.18;
  return clamp(weight * (0.72 + clamp(alpha, 0, 1) * 0.28), 0.22, 1);
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
  const visualWeights = [];
  const edges = [];
  const sampledLuminances = [];
  const luminances = new Float32Array(width * height);
  const chromas = new Float32Array(width * height);

  for (let y = 0; y < height; y += 1) {
    for (let x = 0; x < width; x += 1) {
      const pixelIndex = y * width + x;
      const metrics = pixelMetrics(data, pixelIndex * 4);
      luminances[pixelIndex] = metrics.luminance;
      chromas[pixelIndex] = metrics.chroma;
    }
  }

  const sampleLuminance = (x, y) => luminances[clamp(y, 0, height - 1) * width + clamp(x, 0, width - 1)];

  for (let y = 0; y < height; y += step) {
    for (let x = 0; x < width; x += step) {
      const offset = (y * width + x) * 4;
      const alpha = data[offset + 3] / 255;
      if (alpha < alphaThreshold) continue;
      const sampleRadius = Math.max(1, Math.floor(step * 0.72));
      const left = sampleLuminance(x - sampleRadius, y);
      const right = sampleLuminance(x + sampleRadius, y);
      const top = sampleLuminance(x, y - sampleRadius);
      const bottom = sampleLuminance(x, y + sampleRadius);
      const diagonalA = sampleLuminance(x - sampleRadius, y - sampleRadius) - sampleLuminance(x + sampleRadius, y + sampleRadius);
      const diagonalB = sampleLuminance(x + sampleRadius, y - sampleRadius) - sampleLuminance(x - sampleRadius, y + sampleRadius);
      const gradientX = right - left + (diagonalB - diagonalA) * 0.35;
      const gradientY = bottom - top - (diagonalA + diagonalB) * 0.35;
      const edge = clamp(Math.hypot(gradientX, gradientY) * 1.85, 0, 1);
      const luminance = luminances[y * width + x];
      const chroma = chromas[y * width + x];
      const visualWeight = calculateVisualWeight({ luminance, chroma, edge, alpha });
      const random = stableRandomVector(x, y);
      const jitterScale = step * 0.32;
      const jitterX = random[0] * jitterScale;
      const jitterY = random[1] * jitterScale;
      positions.push(
        (clamp(x + jitterX, 0, width - 1) - (width - 1) * 0.5) * scale,
        ((height - 1) * 0.5 - clamp(y + jitterY, 0, height - 1)) * scale,
        0,
      );
      colors.push(data[offset] / 255, data[offset + 1] / 255, data[offset + 2] / 255);
      randoms.push(...random);
      opacities.push(alpha);
      visualWeights.push(visualWeight);
      edges.push(edge);
      sampledLuminances.push(luminance);
    }
  }

  if (!opacities.length) throw new Error("image contains no visible pixels");
  return {
    positions: new Float32Array(positions),
    colors: new Float32Array(colors),
    randoms: new Float32Array(randoms),
    opacities: new Float32Array(opacities),
    visualWeights: new Float32Array(visualWeights),
    edges: new Float32Array(edges),
    luminances: new Float32Array(sampledLuminances),
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
