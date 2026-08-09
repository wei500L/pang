import * as THREE from "/static/vendor/three/three.module.min.js";
import { AudioFeatureExtractor } from "./audio-features.js";

const MANIFEST_URL = "/static/models/agent-particles.json";
const PULSE_COUNT = 6;
const QUALITY_COUNTS = [65536, 36864, 16384];

const vertexShader = /* glsl */ `
  attribute vec2 normalOct;
  attribute vec4 particleColor;
  attribute vec2 particleSeed;

  uniform float uTime;
  uniform float uPixelRatio;
  uniform float uPointSize;
  uniform float uEnergy;
  uniform float uAttack;
  uniform float uLow;
  uniform float uMid;
  uniform float uHigh;
  uniform float uFlux;
  uniform float uSpeech;
  uniform float uPresence;
  uniform float uStateEnergy;
  uniform float uMotionScale;
  uniform float uLayer;
  uniform vec4 uPulses[${PULSE_COUNT}];
  uniform float uPulseAmplitudes[${PULSE_COUNT}];

  varying vec3 vColor;
  varying float vAlpha;
  varying float vEnergy;
  varying float vHalo;

  vec3 decodeOct(vec2 value) {
    vec3 normal = vec3(value.xy, 1.0 - abs(value.x) - abs(value.y));
    if (normal.z < 0.0) {
      normal.xy = (1.0 - abs(normal.yx)) * sign(normal.xy + vec2(0.00001));
    }
    return normalize(normal);
  }

  vec3 pulseField(vec3 basePosition, float phase) {
    float crestField = 0.0;
    float wakeField = 0.0;
    float impactField = 0.0;
    for (int i = 0; i < ${PULSE_COUNT}; i += 1) {
      float age = uTime - uPulses[i].w;
      float valid = step(0.0, age) * step(age, 3.6);
      float amplitude = uPulseAmplitudes[i];
      float spatialWarp = sin(dot(basePosition, vec3(5.3, 3.7, 7.1)) + phase * 0.24 + float(i) * 1.73);
      float distanceToAnchor = distance(basePosition, uPulses[i].xyz) + spatialWarp * (0.018 + uMid * 0.012);
      float front = age * (0.52 + uLow * 0.18 + amplitude * 0.08);
      float width = 0.055 + uMid * 0.055 + age * 0.012;
      float crest = exp(-pow((distanceToAnchor - front) / width, 2.0));
      float wakeCenter = max(0.0, front - 0.16 - age * 0.035);
      float wakeWidth = 0.17 + age * 0.055;
      float wake = exp(-pow((distanceToAnchor - wakeCenter) / wakeWidth, 2.0));
      float impact = exp(-distanceToAnchor * distanceToAnchor * 34.0) * exp(-age * 4.6);
      float decay = exp(-age * 0.68) * amplitude * valid;
      crestField += crest * decay;
      wakeField += wake * decay * 0.42;
      impactField += impact * amplitude * valid;
    }
    return clamp(vec3(crestField, wakeField, impactField), 0.0, 1.45);
  }

  void main() {
    vec3 basePosition = position;
    vec3 normal = decodeOct(normalOct);
    float phase = particleSeed.x * 6.2831853;
    float secondary = particleSeed.y * 6.2831853;

    vec3 reference = abs(normal.y) > 0.85 ? vec3(1.0, 0.0, 0.0) : vec3(0.0, 1.0, 0.0);
    vec3 tangent = normalize(cross(normal, reference));
    vec3 bitangent = normalize(cross(normal, tangent));

    float breath = sin(uTime * 0.72 + phase) * 0.5 + 0.5;
    float slowFlow = sin(dot(basePosition, vec3(3.1, 4.4, 2.7)) + uTime * 0.46 + secondary);
    vec3 idleOffset = normal * (breath - 0.5) * 0.010;
    idleOffset += tangent * slowFlow * 0.0045 + bitangent * cos(uTime * 0.38 + phase) * 0.0035;

    vec3 pulseFields = pulseField(basePosition, phase);
    float pulseCrest = pulseFields.x;
    float pulseWake = pulseFields.y;
    float localImpact = pulseFields.z;
    float pulse = clamp(pulseCrest + pulseWake * 0.72 + localImpact * 0.85, 0.0, 1.5);
    float broadWave = sin(basePosition.y * 8.5 + basePosition.x * 2.6 - uTime * (1.25 + uLow * 1.35));
    float surfaceFlow = sin(dot(basePosition, vec3(6.4, 4.8, 3.1)) - uTime * (1.5 + uMid * 1.8) + phase * 0.12);
    float fineWave = sin(dot(basePosition, vec3(15.0, 10.5, 7.0)) - uTime * 5.4 + secondary);
    float pressure = uLow * (0.008 + uEnergy * 0.020) * broadWave;
    float propagated = uMid * (pulseCrest * 0.072 + pulseWake * 0.032 + localImpact * 0.045);
    float residualMotion = uPresence * 0.0045 * sin(dot(basePosition, vec3(4.1, 6.0, 3.4)) - uTime * 1.15);
    vec3 voiceOffset = normal * (pressure + propagated + residualMotion);
    voiceOffset += tangent * uMid * (0.009 + pulse * 0.020) * surfaceFlow;
    voiceOffset += bitangent * (uHigh * 0.008 + uFlux * 0.018 * pulse) * fineWave;

    float detachThreshold = mix(0.978, 0.935, clamp(uEnergy * 0.7 + uFlux * 0.65, 0.0, 1.0));
    float detachMask = step(detachThreshold, particleSeed.x) * smoothstep(0.20, 0.76, uEnergy + uAttack * 0.38 + uFlux * 0.42);
    float detachEnvelope = detachMask * uSpeech * (uEnergy * 0.052 + uAttack * 0.040 + uFlux * 0.032 + pulse * 0.040);
    detachEnvelope *= 0.72 + 0.28 * sin(uTime * 2.2 + secondary);
    vec3 detachDirection = normalize(normal * 0.78 + tangent * sin(secondary) * 0.32 + bitangent * cos(phase) * 0.22);
    vec3 detachOffset = detachDirection * detachEnvelope;

    vec3 transformedPosition = basePosition + (idleOffset + voiceOffset + detachOffset) * uMotionScale;
    vec4 modelPosition = modelMatrix * vec4(transformedPosition, 1.0);
    vec4 viewPosition = viewMatrix * modelPosition;
    gl_Position = projectionMatrix * viewPosition;

    float perspective = clamp(2.5 / max(0.5, -viewPosition.z), 0.55, 2.25);
    float localEnergy = clamp(uEnergy * 0.20 + pulseCrest * 0.62 + pulseWake * 0.22 + localImpact * 0.72 + uFlux * (0.08 + pulse * 0.30), 0.0, 1.45);
    float energySize = 1.0 + localEnergy * 0.48 + uAttack * localImpact * 0.32;
    float haloSize = mix(1.0, 2.45 + detachMask * 1.25, uLayer);
    float particleScale = mix(0.84, 1.12, particleSeed.y);
    gl_PointSize = clamp(uPointSize * uPixelRatio * perspective * energySize * haloSize * particleScale, 1.0, 14.0);

    vec3 sourceColor = pow(max(particleColor.rgb, vec3(0.0)), vec3(2.2));
    float sourceLuma = dot(sourceColor, vec3(0.2126, 0.7152, 0.0722));
    sourceColor = mix(vec3(sourceLuma), sourceColor, 1.20);
    sourceColor = max((sourceColor - vec3(0.085)) * 1.24 + vec3(0.085), vec3(0.0)) * 1.12;
    float sculpturalLight = 0.76 + max(dot(normal, normalize(vec3(-0.32, 0.48, 0.82))), 0.0) * 0.38;
    sourceColor *= sculpturalLight;
    vec3 warmEnergy = vec3(1.0, 0.78, 0.52);
    vec3 paleEnergy = vec3(1.0, 0.93, 0.82);
    vec3 energyColor = mix(warmEnergy, paleEnergy, clamp(uHigh * 0.36 + uFlux * 0.58, 0.0, 0.82));
    float colorLift = clamp(uEnergy * 0.10 + localEnergy * 0.46 + uAttack * localImpact * 0.22 + uStateEnergy * 0.10, 0.0, 0.68);
    vColor = mix(sourceColor, energyColor, colorLift);
    vEnergy = clamp(uEnergy * 0.36 + localEnergy * 0.88 + uPresence * 0.10, 0.0, 1.45);
    vHalo = max(detachMask * (0.22 + uEnergy + uFlux * 0.4), pulseCrest * 0.54 + pulseWake * 0.18 + localImpact * 0.78 + uFlux * pulse * 0.26);
    float sourceAlpha = max(0.1, particleColor.a);
    float densityVariation = mix(0.82, 1.0, particleSeed.x);
    vAlpha = sourceAlpha * densityVariation * mix(0.80 + uStateEnergy * 0.07, 0.13 + vHalo * 0.46, uLayer);
  }
`;

const fragmentShader = /* glsl */ `
  uniform float uLayer;
  uniform float uLightTheme;

  varying vec3 vColor;
  varying float vAlpha;
  varying float vEnergy;
  varying float vHalo;

  void main() {
    vec2 centered = gl_PointCoord - vec2(0.5);
    float distanceFromCenter = length(centered);
    float core = smoothstep(0.48, 0.23, distanceFromCenter);
    float hotCore = exp(-distanceFromCenter * distanceFromCenter * 34.0);
    float energizedRim = smoothstep(0.48, 0.34, distanceFromCenter) - smoothstep(0.32, 0.20, distanceFromCenter);
    float glow = exp(-distanceFromCenter * distanceFromCenter * 13.5);
    if (uLayer > 0.5 && vHalo < 0.045) discard;
    float shape = mix(core * 0.74 + hotCore * 0.46 + energizedRim * vEnergy * 0.18, glow, uLayer);
    float themeAlpha = mix(1.0, 1.12, uLightTheme);
    float alpha = shape * vAlpha * themeAlpha;
    if (alpha < 0.012) discard;
    vec3 color = vColor * (1.12 + vEnergy * mix(0.46, 0.78, uLayer));
    color *= mix(1.0, 0.74, uLightTheme);
    gl_FragColor = vec4(color, alpha);
    #include <tonemapping_fragment>
    #include <colorspace_fragment>
  }
`;

class SpringValue {
  constructor(value = 0) {
    this.value = value;
    this.velocity = 0;
    this.target = value;
  }

  step(dt, stiffness = 56, damping = 14) {
    const acceleration = (this.target - this.value) * stiffness - this.velocity * damping;
    this.velocity += acceleration * dt;
    this.value += this.velocity * dt;
    return this.value;
  }

  reset(value = 0) {
    this.value = value;
    this.velocity = 0;
    this.target = value;
  }
}

class AgentVisual {
  constructor(container) {
    this.container = container;
    this.extractor = new AudioFeatureExtractor();
    this.state = "idle";
    this.disposed = false;
    this.visible = !document.hidden;
    this.lastFrame = 0;
    this.frameSamples = [];
    this.qualityIndex = this.initialQualityIndex();
    this.pulseCursor = 0;
    this.anchorCursor = 0;
    this.lastPulseAt = -Infinity;
    this.pulses = Array.from({ length: PULSE_COUNT }, () => ({ anchor: new THREE.Vector3(), startedAt: -100, amplitude: 0 }));
    this.springs = Object.fromEntries(["energy", "attack", "low", "mid", "high", "flux", "speech", "presence", "state"].map((key) => [key, new SpringValue()]));
    this.reducedMotion = window.matchMedia("(prefers-reduced-motion: reduce)");
    this.motionScale = this.reducedMotion.matches ? 0.38 : 1;
    this.onVisibility = () => {
      this.visible = !document.hidden;
      if (this.visible) {
        this.lastFrame = performance.now();
        this.schedule();
      }
    };
    document.addEventListener("visibilitychange", this.onVisibility);
  }

  initialQualityIndex() {
    const memory = Number(navigator.deviceMemory || 4);
    const cores = Number(navigator.hardwareConcurrency || 4);
    const mobile = window.matchMedia("(max-width: 700px), (pointer: coarse)").matches;
    const reduced = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
    if (mobile || reduced || memory <= 2 || cores <= 4) return 2;
    if (memory >= 8 && cores >= 8) return 0;
    return 1;
  }

  async init() {
    if (!this.supportsWebGL()) throw new Error("WebGL is unavailable");
    const manifestResponse = await fetch(MANIFEST_URL, { credentials: "same-origin" });
    if (!manifestResponse.ok) throw new Error(`particle manifest ${manifestResponse.status}`);
    this.manifest = await manifestResponse.json();
    if (this.manifest.version !== 1) throw new Error(`unsupported particle asset version ${this.manifest.version}`);
    const binaryURL = new URL(this.manifest.binary, manifestResponse.url);
    const binaryResponse = await fetch(binaryURL, { credentials: "same-origin" });
    if (!binaryResponse.ok) throw new Error(`particle binary ${binaryResponse.status}`);
    const buffer = await binaryResponse.arrayBuffer();
    this.buildScene(buffer);
    this.observeLayout();
    this.container.classList.add("is-ready");
    this.lastFrame = performance.now();
    this.schedule();
    return this;
  }

  supportsWebGL() {
    try {
      const canvas = document.createElement("canvas");
      return Boolean(window.WebGL2RenderingContext && canvas.getContext("webgl2"));
    } catch (_) {
      return false;
    }
  }

  buildScene(buffer) {
    this.scene = new THREE.Scene();
    this.camera = new THREE.PerspectiveCamera(34, 1, 0.1, 20);
    this.camera.position.set(0, 0.015, 4.54);
    this.group = new THREE.Group();
    this.group.rotation.set(-0.035, -0.08, 0);
    this.scene.add(this.group);

    this.renderer = new THREE.WebGLRenderer({ alpha: true, antialias: false, powerPreference: "high-performance" });
    this.renderer.setClearColor(0x000000, 0);
    this.renderer.outputColorSpace = THREE.SRGBColorSpace;
    this.renderer.toneMapping = THREE.ACESFilmicToneMapping;
    this.renderer.toneMappingExposure = 1.32;
    this.container.appendChild(this.renderer.domElement);

    const count = this.manifest.count;
    const sections = this.manifest.sections;
    const geometry = new THREE.BufferGeometry();
    geometry.setAttribute("position", new THREE.Int16BufferAttribute(new Int16Array(buffer, sections.position.offset, count * 3), 3, true));
    geometry.setAttribute("normalOct", new THREE.Int16BufferAttribute(new Int16Array(buffer, sections.normal.offset, count * 2), 2, true));
    geometry.setAttribute("particleColor", new THREE.Uint8BufferAttribute(new Uint8Array(buffer, sections.color.offset, count * 4), 4, true));
    geometry.setAttribute("particleSeed", new THREE.Uint16BufferAttribute(new Uint16Array(buffer, sections.seed.offset, count * 2), 2, true));
    geometry.computeBoundingSphere();
    this.geometry = geometry;
    this.applyQuality();

    const sharedUniforms = {
      uTime: { value: 0 },
      uPixelRatio: { value: 1 },
      uPointSize: { value: 1.58 },
      uEnergy: { value: 0 },
      uAttack: { value: 0 },
      uLow: { value: 0 },
      uMid: { value: 0 },
      uHigh: { value: 0 },
      uFlux: { value: 0 },
      uSpeech: { value: 0 },
      uPresence: { value: 0 },
      uStateEnergy: { value: 0 },
      uMotionScale: { value: this.motionScale },
      uLightTheme: { value: 0 },
      uPulses: { value: this.pulses.map((pulse) => new THREE.Vector4(pulse.anchor.x, pulse.anchor.y, pulse.anchor.z, pulse.startedAt)) },
      uPulseAmplitudes: { value: this.pulses.map((pulse) => pulse.amplitude) },
    };
    this.sharedUniforms = sharedUniforms;
    const coreMaterial = new THREE.ShaderMaterial({
      uniforms: { ...sharedUniforms, uLayer: { value: 0 } },
      vertexShader,
      fragmentShader,
      transparent: true,
      depthTest: true,
      depthWrite: true,
      blending: THREE.NormalBlending,
      toneMapped: true,
    });
    const haloMaterial = new THREE.ShaderMaterial({
      uniforms: { ...sharedUniforms, uLayer: { value: 1 } },
      vertexShader,
      fragmentShader,
      transparent: true,
      depthTest: true,
      depthWrite: false,
      blending: THREE.AdditiveBlending,
      toneMapped: true,
    });
    this.materials = [coreMaterial, haloMaterial];
    this.group.add(new THREE.Points(geometry, coreMaterial), new THREE.Points(geometry, haloMaterial));
    this.anchors = (this.manifest.anchors || [[0, 0, 0]]).map((anchor) => new THREE.Vector3(anchor[0], anchor[1], anchor[2]));
  }

  observeLayout() {
    this.resizeObserver = new ResizeObserver(() => this.resize());
    this.resizeObserver.observe(this.container);
    this.themeObserver = new MutationObserver(() => this.updateTheme());
    this.themeObserver.observe(document.documentElement, { attributes: true, attributeFilter: ["data-theme"] });
    const welcome = document.getElementById("welcomeState");
    if (welcome) {
      this.transcriptObserver = new MutationObserver(() => {
        this.container.classList.toggle("has-transcript", welcome.classList.contains("hidden"));
      });
      this.transcriptObserver.observe(welcome, { attributes: true, attributeFilter: ["class"] });
    }
    this.updateTheme();
    this.resize();
  }

  updateTheme() {
    const light = document.documentElement.getAttribute("data-theme") === "light";
    if (this.sharedUniforms) this.sharedUniforms.uLightTheme.value = light ? 1 : 0;
    if (this.renderer) this.renderer.toneMappingExposure = light ? 1.02 : 1.32;
  }

  resize() {
    if (!this.renderer || !this.camera) return;
    const width = Math.max(1, this.container.clientWidth);
    const height = Math.max(1, this.container.clientHeight);
    const mobile = width < 700;
    const maxDpr = mobile ? 1.25 : 1.5;
    const dpr = Math.min(window.devicePixelRatio || 1, maxDpr);
    this.renderer.setPixelRatio(dpr);
    this.renderer.setSize(width, height, false);
    this.camera.aspect = width / height;
    this.camera.position.z = mobile
      ? 4.90
      : Math.max(4.30, 4.66 - Math.min(0.24, width / Math.max(height, 1) * 0.07));
    this.camera.updateProjectionMatrix();
    if (this.sharedUniforms) this.sharedUniforms.uPixelRatio.value = dpr;
  }

  applyQuality() {
    if (!this.geometry || !this.manifest) return;
    const count = Math.min(this.manifest.count, QUALITY_COUNTS[this.qualityIndex]);
    this.geometry.setDrawRange(0, count);
    this.container.dataset.quality = ["high", "medium", "low"][this.qualityIndex];
  }

  setState(state) {
    this.state = state || "idle";
    const levels = { idle: 0, connecting: 0.16, listening: 0.1, "assistant-speaking": 0.28, error: 0.18 };
    this.springs.state.target = levels[this.state] ?? 0;
  }

  processMicFrame(timeData, frequencyData, sampleRate, timestamp) {
    const features = this.extractor.process(timeData, frequencyData, sampleRate, timestamp);
    const now = Number.isFinite(Number(timestamp)) ? Number(timestamp) : performance.now();
    this.springs.energy.target = features.energy;
    this.springs.attack.target = features.attack;
    this.springs.low.target = features.low;
    this.springs.mid.target = features.mid;
    this.springs.high.target = features.high;
    this.springs.flux.target = features.spectralFlux;
    this.springs.speech.target = features.speechActive ? 1 : 0;
    this.springs.presence.target = features.speechActive
      ? Math.min(1, 0.22 + features.energy * 0.72 + features.spectralFlux * 0.28)
      : 0;

    const transient = Math.max(features.attack, features.spectralFlux * 1.15);
    if (features.onset) {
      this.triggerPulse(Math.max(0.34, transient, features.energy * 0.86));
      this.lastPulseAt = now;
    } else if (features.speechActive && features.spectralFlux > 0.16 && now - this.lastPulseAt > 210) {
      this.triggerPulse(Math.max(0.22, features.spectralFlux * 0.88, features.attack * 0.72));
      this.lastPulseAt = now;
    }
    return features;
  }

  triggerPulse(amplitude) {
    if (!this.anchors?.length) return;
    const pulse = this.pulses[this.pulseCursor % PULSE_COUNT];
    const anchor = this.anchors[this.anchorCursor % this.anchors.length];
    pulse.anchor.copy(anchor);
    pulse.startedAt = this.sharedUniforms?.uTime.value || 0;
    pulse.amplitude = Math.min(1.2, amplitude);
    this.pulseCursor += 1;
    this.anchorCursor = (this.anchorCursor + 3) % this.anchors.length;
  }

  resetAudio() {
    this.extractor.reset();
    for (const key of ["energy", "attack", "low", "mid", "high", "flux", "speech", "presence"]) this.springs[key].target = 0;
  }

  schedule() {
    if (this.disposed || !this.visible || this.raf) return;
    this.raf = requestAnimationFrame((time) => {
      this.raf = 0;
      this.render(time);
      this.schedule();
    });
  }

  render(timestamp) {
    if (!this.renderer || !this.scene || !this.camera) return;
    const dt = Math.min(0.05, Math.max(1 / 240, (timestamp - this.lastFrame) / 1000 || 1 / 60));
    this.lastFrame = timestamp;
    const springProfiles = {
      energy: [70, 17],
      attack: [128, 22],
      low: [58, 15],
      mid: [78, 17],
      high: [104, 20],
      flux: [136, 23],
      speech: [44, 14],
      presence: [30, 11],
      state: [48, 14],
    };
    const values = {};
    for (const [key, spring] of Object.entries(this.springs)) {
      const [stiffness, damping] = springProfiles[key] || [56, 14];
      values[key] = spring.step(dt, stiffness, damping);
    }
    const time = timestamp / 1000;
    this.sharedUniforms.uTime.value = time;
    this.sharedUniforms.uEnergy.value = Math.max(0, values.energy);
    this.sharedUniforms.uAttack.value = Math.max(0, values.attack);
    this.sharedUniforms.uLow.value = Math.max(0, values.low);
    this.sharedUniforms.uMid.value = Math.max(0, values.mid);
    this.sharedUniforms.uHigh.value = Math.max(0, values.high);
    this.sharedUniforms.uFlux.value = Math.max(0, values.flux);
    this.sharedUniforms.uSpeech.value = Math.max(0, values.speech);
    this.sharedUniforms.uPresence.value = Math.max(0, values.presence);
    this.sharedUniforms.uStateEnergy.value = Math.max(0, values.state);
    for (let i = 0; i < PULSE_COUNT; i += 1) {
      const pulse = this.pulses[i];
      this.sharedUniforms.uPulses.value[i].set(pulse.anchor.x, pulse.anchor.y, pulse.anchor.z, pulse.startedAt);
      this.sharedUniforms.uPulseAmplitudes.value[i] = pulse.amplitude;
    }
    const drift = this.motionScale;
    this.group.rotation.x = -0.035 + Math.sin(time * 0.17) * 0.018 * drift;
    this.group.rotation.y = -0.08 + Math.sin(time * 0.13) * 0.045 * drift;
    this.group.rotation.z = Math.sin(time * 0.09) * 0.008 * drift;
    this.renderer.render(this.scene, this.camera);
    this.monitorPerformance(dt);
  }

  monitorPerformance(dt) {
    if (dt > 0.08) return;
    this.frameSamples.push(dt * 1000);
    if (this.frameSamples.length < 180) return;
    const average = this.frameSamples.reduce((sum, value) => sum + value, 0) / this.frameSamples.length;
    this.frameSamples.length = 0;
    if (average > 20 && this.qualityIndex < QUALITY_COUNTS.length - 1) {
      this.qualityIndex += 1;
      this.applyQuality();
    }
  }

  dispose() {
    if (this.disposed) return;
    this.disposed = true;
    if (this.raf) cancelAnimationFrame(this.raf);
    document.removeEventListener("visibilitychange", this.onVisibility);
    this.resizeObserver?.disconnect();
    this.themeObserver?.disconnect();
    this.transcriptObserver?.disconnect();
    this.geometry?.dispose();
    this.materials?.forEach((material) => material.dispose());
    this.renderer?.dispose();
    this.renderer?.domElement.remove();
  }
}

async function mount() {
  const container = document.getElementById("agentVisualLayer");
  if (!container) return null;
  const visual = new AgentVisual(container);
  window.PangAgentVisual = visual;
  try {
    await visual.init();
  } catch (error) {
    container.classList.add("is-unavailable");
    console.warn("Agent visual unavailable", error);
  }
  return visual;
}

window.PangAgentVisualReady = document.readyState === "loading"
  ? new Promise((resolve) => document.addEventListener("DOMContentLoaded", () => mount().then(resolve), { once: true }))
  : mount();

window.addEventListener("pagehide", () => window.PangAgentVisual?.dispose(), { once: true });

export { AgentVisual };
