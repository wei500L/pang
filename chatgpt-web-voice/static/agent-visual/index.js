import * as THREE from "/static/vendor/three/three.module.min.js";
import { AudioFeatureExtractor } from "./audio-features.js";

const MANIFEST_URL = "/static/models/agent-particles.json";
const PULSE_COUNT = 6;
const QUALITY_COUNTS = [65536, 36864, 16384];
const SPRING_PROFILES = {
  energy: [70, 17],
  attack: [128, 22],
  low: [58, 15],
  mid: [78, 17],
  high: [104, 20],
  flux: [136, 23],
  centroid: [54, 14],
  speech: [44, 14],
  presence: [30, 11],
  state: [48, 14],
  assistant: [36, 12],
  user: [62, 16],
  listening: [30, 11],
  thinking: [36, 12],
  interrupted: [168, 27],
  error: [44, 15],
  muted: [56, 15],
};

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
  uniform float uCentroid;
  uniform float uSpeech;
  uniform float uPresence;
  uniform float uStateEnergy;
  uniform float uAssistantMix;
  uniform float uUserSpeaking;
  uniform float uListening;
  uniform float uThinking;
  uniform float uInterrupted;
  uniform float uError;
  uniform float uMuted;
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
    float spectralCenter = clamp(uCentroid, 0.0, 1.0);
    float centroidBias = spectralCenter * 2.0 - 1.0;

    vec3 reference = abs(normal.y) > 0.85 ? vec3(1.0, 0.0, 0.0) : vec3(0.0, 1.0, 0.0);
    vec3 tangent = normalize(cross(normal, reference));
    vec3 bitangent = normalize(cross(normal, tangent));

    float activeMotion = (1.0 - uMuted * 0.84) * (1.0 - uError * 0.72);
    float breath = sin(uTime * 1.5708 + phase * 0.08) * 0.5 + 0.5;
    float slowFlow = sin(dot(basePosition, vec3(3.1, 4.4, 2.7)) + uTime * 0.34 + secondary);
    basePosition *= 1.0 + uLow * 0.012 * activeMotion - uThinking * 0.0025;
    basePosition.z *= 1.0 + uLow * 0.14 * activeMotion;
    vec3 idleOffset = normal * (breath - 0.5) * mix(0.0045, 0.009, activeMotion);
    idleOffset += tangent * slowFlow * 0.0032 * activeMotion + bitangent * cos(uTime * 0.31 + phase) * 0.0026 * activeMotion;

    vec3 pulseFields = pulseField(basePosition, phase);
    float pulseCrest = pulseFields.x;
    float pulseWake = pulseFields.y;
    float localImpact = pulseFields.z;
    float pulse = clamp(pulseCrest + pulseWake * 0.72 + localImpact * 0.85, 0.0, 1.5);
    float broadWave = sin(basePosition.y * 7.2 + basePosition.x * 2.2 - uTime * (0.82 + uLow * 0.78));
    float assistantRhythm = sin(basePosition.x * 5.6 - basePosition.y * 2.4 - uTime * (1.05 + uMid * 1.08));
    vec3 userAxis = normalize(vec3(0.82, centroidBias * 0.46, 0.28));
    float userRhythm = sin(dot(basePosition, userAxis) * mix(5.2, 9.8, spectralCenter) - uTime * (1.18 + uMid * 1.46) + phase * 0.06);
    float thinkingRhythm = sin(length(basePosition.xy * vec2(0.88, 1.12)) * 9.2 - uTime * 1.22 + secondary * 0.09);
    float surfaceFlow = sin(dot(basePosition, vec3(5.8, 4.2, 2.8)) - uTime * (0.92 + uMid * 1.18) + phase * 0.1);
    float fineWave = sin(dot(basePosition, vec3(13.0, 9.2, 6.4)) - uTime * 3.8 + secondary);
    float pressure = uLow * (0.006 + uEnergy * 0.015) * broadWave * activeMotion;
    float propagated = uMid * (pulseCrest * 0.052 + pulseWake * 0.024 + localImpact * 0.034) * activeMotion;
    float residualMotion = uPresence * 0.0036 * sin(dot(basePosition, vec3(4.1, 6.0, 3.4)) - uTime * 0.95) * activeMotion;
    float userSurface = uUserSpeaking * (uMid * 0.010 + uEnergy * 0.0055) * userRhythm * activeMotion;
    float thinkingGather = uThinking * (0.0038 + uStateEnergy * 0.006) * thinkingRhythm;
    vec3 voiceOffset = normal * (pressure + propagated + residualMotion + userSurface - thinkingGather);
    voiceOffset += tangent * uMid * (0.007 + pulse * 0.014) * mix(surfaceFlow, assistantRhythm, uAssistantMix) * activeMotion;
    voiceOffset += bitangent * (uHigh * 0.005 + uFlux * 0.010 * pulse) * fineWave * activeMotion;
    voiceOffset += (tangent * thinkingRhythm + bitangent * cos(thinkingRhythm + phase)) * uThinking * 0.0038;
    voiceOffset *= 1.0 - uInterrupted * 0.78;

    float detachThreshold = mix(0.994, 0.978, clamp(uEnergy * 0.58 + uFlux * 0.42, 0.0, 1.0));
    float detachMask = step(detachThreshold, particleSeed.x) * smoothstep(0.20, 0.76, uEnergy + uAttack * 0.38 + uFlux * 0.42);
    float detachEnvelope = detachMask * uSpeech * (uEnergy * 0.012 + uAttack * 0.010 + uFlux * 0.008 + pulse * 0.012) * activeMotion;
    detachEnvelope *= 0.76 + 0.24 * sin(uTime * 1.8 + secondary);
    detachEnvelope *= max(0.0, 1.0 - uThinking * 0.72 - uInterrupted * 0.92);
    vec3 detachDirection = normalize(normal * 0.78 + tangent * sin(secondary) * 0.32 + bitangent * cos(phase) * 0.22);
    vec3 detachOffset = detachDirection * detachEnvelope;
    float dustMask = step(0.983, particleSeed.y) * (1.0 - uMuted * 0.72);
    float dustDrift = 0.035 + uHigh * 0.018 + 0.035 * (sin(uTime * 0.42 + phase) * 0.5 + 0.5);
    vec3 ambientDustOffset = dustMask * (normal * dustDrift + tangent * sin(uTime * 0.31 + secondary) * 0.018 + bitangent * cos(uTime * 0.27 + phase) * 0.014);

    vec3 transformedPosition = basePosition + (idleOffset + voiceOffset + detachOffset + ambientDustOffset) * uMotionScale;
    vec4 modelPosition = modelMatrix * vec4(transformedPosition, 1.0);
    vec4 viewPosition = viewMatrix * modelPosition;
    gl_Position = projectionMatrix * viewPosition;

    float perspective = clamp(2.5 / max(0.5, -viewPosition.z), 0.55, 2.25);
    float localEnergy = clamp(uEnergy * 0.20 + pulseCrest * 0.62 + pulseWake * 0.22 + localImpact * 0.72 + uFlux * (0.08 + pulse * 0.30), 0.0, 1.45);
    float energySize = 1.0 + localEnergy * 0.32 + uAttack * localImpact * 0.18;
    float haloSize = mix(1.0, 2.05 + detachMask * 0.7, uLayer);
    float particleScale = mix(0.84, 1.12, particleSeed.y);
    float depthCue = smoothstep(-0.82, 0.82, transformedPosition.z);
    float depthScale = mix(0.78, 1.22, depthCue);
    gl_PointSize = clamp(uPointSize * uPixelRatio * perspective * energySize * haloSize * particleScale * depthScale, 1.0, 14.0);

    vec3 sourceColor = pow(max(particleColor.rgb, vec3(0.0)), vec3(2.2));
    float sourceLuma = dot(sourceColor, vec3(0.2126, 0.7152, 0.0722));
    vec3 lightDirection = normalize(vec3(-0.44 + spectralCenter * 0.24, 0.5 + uHigh * 0.06, 0.82));
    float sculpturalLight = (0.72 + max(dot(normal, lightDirection), 0.0) * 0.42) * mix(0.8, 1.1, depthCue);
    vec3 brandOrange = vec3(1.0, 0.16, 0.018);
    vec3 champagne = vec3(0.72, 0.39, 0.13);
    vec3 softGold = vec3(0.94, 0.57, 0.24);
    vec3 sage = vec3(0.24, 0.46, 0.32);
    float orangeMix = smoothstep(0.18, 0.86, particleSeed.y);
    vec3 brandColor = mix(champagne, brandOrange, orangeMix);
    float sageMask = smoothstep(0.91, 0.985, particleSeed.x) * (0.55 + uListening * 0.45);
    brandColor = mix(brandColor, sage, sageMask * (0.34 + uListening * 0.42));
    vec3 energyColor = mix(brandOrange, softGold, clamp(uAssistantMix * 0.72 + uHigh * 0.24, 0.0, 0.9));
    energyColor = mix(energyColor, softGold, uThinking * 0.24);
    brandColor = mix(brandColor, vec3(0.50, 0.43, 0.34), uError * 0.42);
    float colorLift = clamp(uEnergy * 0.12 + localEnergy * 0.32 + uStateEnergy * 0.08, 0.0, 0.56);
    vColor = mix(brandColor, energyColor, colorLift) * sculpturalLight * mix(0.92, 1.05, sourceLuma);
    vColor = mix(vColor, sage * 0.92, uInterrupted * 0.12);
    vEnergy = clamp(uEnergy * 0.36 + localEnergy * 0.88 + uPresence * 0.10, 0.0, 1.45);
    vHalo = max(detachMask * (0.22 + uEnergy + uFlux * 0.4), pulseCrest * 0.54 + pulseWake * 0.18 + localImpact * 0.78 + uFlux * pulse * 0.26);
    float sourceAlpha = max(0.1, particleColor.a);
    float densityVariation = mix(0.82, 1.0, particleSeed.x);
    vAlpha = sourceAlpha * densityVariation * mix(0.76 + uStateEnergy * 0.05, 0.09 + vHalo * 0.34, uLayer) * mix(0.72, 1.08, depthCue) * mix(1.0, 0.82, uMuted);
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
    float themeAlpha = mix(1.0, 1.06, uLightTheme);
    float alpha = shape * vAlpha * themeAlpha;
    if (alpha < 0.012) discard;
    vec3 color = vColor * (1.04 + vEnergy * mix(0.28, 0.52, uLayer));
    color *= mix(1.0, 0.8, uLightTheme);
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
    this.extractors = {
      mic: new AudioFeatureExtractor(),
      assistant: new AudioFeatureExtractor(),
    };
    this.featureFrames = {
      mic: this.extractors.mic.snapshot(false),
      assistant: this.extractors.assistant.snapshot(false),
    };
    this.state = "idle";
    this.visualState = "idle";
    this.container.dataset.visualState = "idle";
    document.body.dataset.agentVisualState = "idle";
    this.muted = false;
    this.disposed = false;
    this.active = true;
    this.visible = !document.hidden;
    this.lastFrame = 0;
    this.frameSamples = [];
    this.renderValues = Object.create(null);
    this.qualityIndex = this.initialQualityIndex();
    this.pulseCursor = 0;
    this.anchorCursor = 0;
    this.lastPulseAt = -Infinity;
    this.pulses = Array.from({ length: PULSE_COUNT }, () => ({ anchor: new THREE.Vector3(), startedAt: -100, amplitude: 0 }));
    this.springs = Object.fromEntries([
      "energy", "attack", "low", "mid", "high", "flux", "centroid", "speech", "presence", "state", "assistant", "user", "listening", "thinking", "interrupted", "error", "muted",
    ].map((key) => [key, new SpringValue()]));
    this.springs.centroid.reset(0.38);
    this.springEntries = Object.entries(this.springs);
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
    this.lastFrame = performance.now();
    this.render(this.lastFrame);
    this.container.classList.add("is-ready");
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
      uCentroid: { value: 0.38 },
      uSpeech: { value: 0 },
      uPresence: { value: 0 },
      uStateEnergy: { value: 0 },
      uAssistantMix: { value: 0 },
      uUserSpeaking: { value: 0 },
      uListening: { value: 0 },
      uThinking: { value: 0 },
      uInterrupted: { value: 0 },
      uError: { value: 0 },
      uMuted: { value: 0 },
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
    this.anchorsByX = [...this.anchors].sort((a, b) => a.x - b.x);
  }

  observeLayout() {
    this.resizeObserver = new ResizeObserver(() => this.resize());
    this.resizeObserver.observe(this.container);
    this.themeObserver = new MutationObserver(() => this.updateTheme());
    this.themeObserver.observe(document.documentElement, { attributes: true, attributeFilter: ["data-theme"] });
    const transcript = document.querySelector(".transcript");
    if (transcript) {
      const syncTranscriptState = () => {
        this.container.classList.toggle("has-transcript", !transcript.classList.contains("empty"));
      };
      this.transcriptObserver = new MutationObserver(syncTranscriptState);
      this.transcriptObserver.observe(transcript, { attributes: true, attributeFilter: ["class"] });
      syncTranscriptState();
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
    const nextState = state || "idle";
    const previousState = this.state;
    this.state = nextState;
    if (nextState === "interrupted" && previousState !== "interrupted") {
      this.featureFrames.assistant = this.extractors.assistant.reset();
      this.springs.assistant.target = 0;
    }
    const levels = {
      idle: 0,
      connecting: 0.1,
      listening: 0.08,
      "user-speaking": 0.18,
      thinking: 0.12,
      "assistant-speaking": 0.2,
      interrupted: 0.06,
      error: 0.12,
    };
    this.springs.state.target = levels[this.state] ?? 0;
    this.springs.listening.target = this.state === "listening" || this.state === "interrupted" ? 1 : 0;
    this.springs.thinking.target = this.state === "thinking" ? 1 : 0;
    this.springs.interrupted.target = this.state === "interrupted" ? 1 : 0;
    this.springs.error.target = this.state === "error" ? 1 : 0;
    this.updateAudioTargets();
    this.updateVisualState();
  }

  resolveVisualState() {
    const mic = this.featureFrames.mic;
    const assistant = this.featureFrames.assistant;
    if (this.state === "error") return "error";
    if (this.state === "interrupted") return "interrupted";
    if (this.state === "connecting") return "connecting";
    if (this.state === "assistant-speaking" || assistant.speechActive) return "assistant-speaking";
    if (!this.muted && mic.speechActive) return "user-speaking";
    if (this.state === "thinking") return "thinking";
    if (this.state === "listening") return "listening";
    return "idle";
  }

  updateVisualState() {
    const next = this.resolveVisualState();
    if (next === this.visualState) return;
    this.visualState = next;
    this.container.dataset.visualState = next;
    document.body.dataset.agentVisualState = next;
  }

  processMicFrame(timeData, frequencyData, sampleRate, timestamp) {
    return this.processAudioFrame("mic", timeData, frequencyData, sampleRate, timestamp);
  }

  processAssistantFrame(timeData, frequencyData, sampleRate, timestamp) {
    return this.processAudioFrame("assistant", timeData, frequencyData, sampleRate, timestamp);
  }

  processAudioFrame(role, timeData, frequencyData, sampleRate, timestamp) {
    const extractor = this.extractors[role];
    if (!extractor) return null;
    const features = extractor.process(timeData, frequencyData, sampleRate, timestamp);
    this.featureFrames[role] = features;
    const now = Number.isFinite(Number(timestamp)) ? Number(timestamp) : performance.now();
    this.updateAudioTargets();

    const transient = Math.max(features.attack, features.spectralFlux * 1.15);
    if (features.onset) {
      const roleScale = role === "assistant" ? 0.72 : 0.9;
      this.triggerPulse(Math.max(0.24, transient * roleScale, features.energy * 0.7), features.spectralCentroid, role);
      this.lastPulseAt = now;
    } else if (features.speechActive && features.spectralFlux > 0.18 && now - this.lastPulseAt > 280) {
      this.triggerPulse(Math.max(0.18, features.spectralFlux * 0.68, features.attack * 0.56), features.spectralCentroid, role);
      this.lastPulseAt = now;
    }
    this.updateVisualState();
    return features;
  }

  updateAudioTargets() {
    const mic = this.featureFrames.mic;
    const assistant = this.featureFrames.assistant;
    const micPresence = this.muted ? 0 : Math.min(1, mic.energy * 1.35 + (mic.speechActive ? 0.32 : 0));
    const assistantPresence = Math.min(1, assistant.energy * 1.42 + (assistant.speechActive ? 0.38 : 0));
    const assistantSuppressed = this.state === "interrupted";
    const assistantBias = this.state === "assistant-speaking" ? 0.24 : 0;
    const assistantWeight = assistantSuppressed ? 0 : Math.min(1, assistantPresence + assistantBias);
    const micWeight = Math.min(1, micPresence * (1 - assistantWeight * 0.72));
    const total = Math.max(0.0001, micWeight + assistantWeight);
    const assistantMix = Math.min(1, assistantWeight / total);
    const blend = (key) => Math.min(1, (mic[key] * micWeight + assistant[key] * assistantWeight) / total);

    this.springs.energy.target = Math.max(mic.energy * micWeight, assistant.energy * assistantWeight, blend("energy") * 0.82);
    this.springs.attack.target = Math.max(mic.attack * micWeight, assistant.attack * assistantWeight);
    this.springs.low.target = blend("low");
    this.springs.mid.target = blend("mid");
    this.springs.high.target = blend("high");
    this.springs.flux.target = Math.min(1, (mic.spectralFlux * micWeight + assistant.spectralFlux * assistantWeight) / total);
    this.springs.centroid.target = blend("spectralCentroid") || 0.38;
    this.springs.speech.target = mic.speechActive || (!assistantSuppressed && assistant.speechActive) ? 1 : 0;
    this.springs.presence.target = Math.max(micPresence, assistantSuppressed ? 0 : assistantPresence);
    this.springs.assistant.target = assistantMix;
    this.springs.user.target = !this.muted && mic.speechActive && assistantWeight < 0.45 ? 1 : 0;
    this.springs.muted.target = this.muted ? 1 : 0;
    this.updateVisualState();
  }

  setMuted(muted) {
    this.muted = Boolean(muted);
    if (this.muted) this.featureFrames.mic = this.extractors.mic.reset();
    this.updateAudioTargets();
    this.updateVisualState();
  }

  triggerPulse(amplitude, centroid = 0.5, role = "mic") {
    if (!this.anchorsByX?.length) return;
    const pulse = this.pulses[this.pulseCursor % PULSE_COUNT];
    const direction = role === "assistant" ? 1 - centroid : centroid;
    const baseIndex = Math.round(Math.max(0, Math.min(1, direction)) * (this.anchorsByX.length - 1));
    const spread = (this.anchorCursor % 3) - 1;
    const index = Math.max(0, Math.min(this.anchorsByX.length - 1, baseIndex + spread));
    const anchor = this.anchorsByX[index];
    pulse.anchor.copy(anchor);
    pulse.startedAt = this.sharedUniforms?.uTime.value || 0;
    pulse.amplitude = Math.min(1.2, amplitude);
    this.pulseCursor += 1;
    this.anchorCursor += 1;
  }

  resetAudio() {
    this.featureFrames.mic = this.extractors.mic.reset();
    this.featureFrames.assistant = this.extractors.assistant.reset();
    for (const key of ["energy", "attack", "low", "mid", "high", "flux", "speech", "presence", "assistant", "user"]) this.springs[key].target = 0;
    this.springs.centroid.target = 0.38;
    this.updateVisualState();
  }

  setActive(active) {
    this.active = Boolean(active);
    if (!this.active && this.raf) {
      cancelAnimationFrame(this.raf);
      this.raf = 0;
    }
    if (this.active) {
      this.lastFrame = performance.now();
      this.schedule();
    }
  }

  schedule() {
    if (this.disposed || !this.active || !this.visible || this.raf) return;
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
    const values = this.renderValues;
    for (const [key, spring] of this.springEntries) {
      const [stiffness, damping] = SPRING_PROFILES[key] || [56, 14];
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
    this.sharedUniforms.uCentroid.value = Math.max(0, Math.min(1, values.centroid));
    this.sharedUniforms.uSpeech.value = Math.max(0, values.speech);
    this.sharedUniforms.uPresence.value = Math.max(0, values.presence);
    this.sharedUniforms.uStateEnergy.value = Math.max(0, values.state);
    this.sharedUniforms.uAssistantMix.value = Math.max(0, Math.min(1, values.assistant));
    this.sharedUniforms.uUserSpeaking.value = Math.max(0, Math.min(1, values.user));
    this.sharedUniforms.uListening.value = Math.max(0, Math.min(1, values.listening));
    this.sharedUniforms.uThinking.value = Math.max(0, Math.min(1, values.thinking));
    this.sharedUniforms.uInterrupted.value = Math.max(0, Math.min(1, values.interrupted));
    this.sharedUniforms.uError.value = Math.max(0, Math.min(1, values.error));
    this.sharedUniforms.uMuted.value = Math.max(0, Math.min(1, values.muted));
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
    if (document.body.dataset.agentVisualState === this.visualState) delete document.body.dataset.agentVisualState;
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
