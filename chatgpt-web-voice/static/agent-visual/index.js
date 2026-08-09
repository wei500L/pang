import * as THREE from "/static/vendor/three/three.module.min.js";
import { AudioFeatureExtractor } from "./audio-features.js";

const MANIFEST_URL = "/static/models/agent-particles.json";
const PULSE_COUNT = 4;
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
  uniform float uSpeech;
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

  float pulseField(vec3 basePosition) {
    float field = 0.0;
    for (int i = 0; i < ${PULSE_COUNT}; i += 1) {
      float age = uTime - uPulses[i].w;
      float valid = step(0.0, age) * step(age, 3.2);
      float distanceToAnchor = distance(basePosition, uPulses[i].xyz);
      float front = age * (0.58 + uLow * 0.22);
      float width = 0.10 + uMid * 0.08;
      float ring = exp(-pow((distanceToAnchor - front) / width, 2.0));
      field += ring * exp(-age * 0.72) * uPulseAmplitudes[i] * valid;
    }
    return clamp(field, 0.0, 1.35);
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

    float pulse = pulseField(basePosition);
    float broadWave = sin(basePosition.y * 7.0 - uTime * (1.4 + uLow) + phase * 0.18);
    float fineWave = sin(dot(basePosition, vec3(13.0, 9.0, 6.0)) - uTime * 4.2 + secondary);
    vec3 voiceOffset = normal * (uLow * 0.020 * broadWave + uMid * 0.065 * pulse);
    voiceOffset += tangent * uMid * 0.022 * pulse * slowFlow;
    voiceOffset += bitangent * uHigh * 0.012 * fineWave;

    float detachMask = step(0.94, particleSeed.x) * smoothstep(0.24, 0.72, uEnergy + uAttack * 0.45);
    float detachEnvelope = detachMask * uSpeech * (uEnergy * 0.072 + uAttack * 0.052 + pulse * 0.035);
    vec3 detachDirection = normalize(normal * 0.78 + tangent * sin(secondary) * 0.32 + bitangent * cos(phase) * 0.22);
    vec3 detachOffset = detachDirection * detachEnvelope;

    vec3 transformedPosition = basePosition + (idleOffset + voiceOffset + detachOffset) * uMotionScale;
    vec4 modelPosition = modelMatrix * vec4(transformedPosition, 1.0);
    vec4 viewPosition = viewMatrix * modelPosition;
    gl_Position = projectionMatrix * viewPosition;

    float perspective = clamp(2.5 / max(0.5, -viewPosition.z), 0.55, 2.25);
    float energySize = 1.0 + uEnergy * 0.34 + uAttack * 0.28 + pulse * 0.22;
    float haloSize = mix(1.0, 2.45 + detachMask * 1.25, uLayer);
    gl_PointSize = clamp(uPointSize * uPixelRatio * perspective * energySize * haloSize, 1.0, 14.0);

    vec3 sourceColor = pow(max(particleColor.rgb, vec3(0.0)), vec3(2.2));
    vec3 warmEnergy = vec3(1.0, 0.78, 0.52);
    float colorLift = clamp(uEnergy * 0.22 + pulse * 0.38 + uAttack * 0.16 + uStateEnergy * 0.12, 0.0, 0.62);
    vColor = mix(sourceColor, warmEnergy, colorLift);
    vEnergy = clamp(uEnergy + pulse * 0.7 + uAttack * 0.25, 0.0, 1.35);
    vHalo = max(detachMask * (0.25 + uEnergy), pulse * 0.42 + uAttack * 0.16);
    float sourceAlpha = max(0.1, particleColor.a);
    vAlpha = sourceAlpha * mix(0.72 + uStateEnergy * 0.08, 0.16 + vHalo * 0.44, uLayer);
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
    float core = smoothstep(0.5, 0.10, distanceFromCenter);
    float glow = exp(-distanceFromCenter * distanceFromCenter * 11.0);
    if (uLayer > 0.5 && vHalo < 0.045) discard;
    float shape = mix(core, glow, uLayer);
    float themeAlpha = mix(1.0, 1.12, uLightTheme);
    float alpha = shape * vAlpha * themeAlpha;
    if (alpha < 0.012) discard;
    vec3 color = vColor * (1.0 + vEnergy * mix(0.42, 0.72, uLayer));
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
    this.pulses = Array.from({ length: PULSE_COUNT }, () => ({ anchor: new THREE.Vector3(), startedAt: -100, amplitude: 0 }));
    this.springs = Object.fromEntries(["energy", "attack", "low", "mid", "high", "speech", "state"].map((key) => [key, new SpringValue()]));
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
    this.camera.position.set(0, 0.02, 3.55);
    this.group = new THREE.Group();
    this.group.rotation.set(-0.035, -0.08, 0);
    this.scene.add(this.group);

    this.renderer = new THREE.WebGLRenderer({ alpha: true, antialias: false, powerPreference: "high-performance" });
    this.renderer.setClearColor(0x000000, 0);
    this.renderer.outputColorSpace = THREE.SRGBColorSpace;
    this.renderer.toneMapping = THREE.ACESFilmicToneMapping;
    this.renderer.toneMappingExposure = 1.18;
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
      uPointSize: { value: 2.15 },
      uEnergy: { value: 0 },
      uAttack: { value: 0 },
      uLow: { value: 0 },
      uMid: { value: 0 },
      uHigh: { value: 0 },
      uSpeech: { value: 0 },
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
    if (this.renderer) this.renderer.toneMappingExposure = light ? 0.94 : 1.18;
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
    this.camera.position.z = mobile ? 4.05 : Math.max(3.35, 3.7 - Math.min(0.35, width / Math.max(height, 1) * 0.12));
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
    this.springs.energy.target = features.energy;
    this.springs.attack.target = features.attack;
    this.springs.low.target = features.low;
    this.springs.mid.target = features.mid;
    this.springs.high.target = features.high;
    this.springs.speech.target = features.speechActive ? 1 : 0;
    if (features.onset) this.triggerPulse(Math.max(0.28, features.attack, features.energy * 0.82));
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
    for (const key of ["energy", "attack", "low", "mid", "high", "speech"]) this.springs[key].target = 0;
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
    const values = {};
    for (const [key, spring] of Object.entries(this.springs)) values[key] = spring.step(dt);
    const time = timestamp / 1000;
    this.sharedUniforms.uTime.value = time;
    this.sharedUniforms.uEnergy.value = Math.max(0, values.energy);
    this.sharedUniforms.uAttack.value = Math.max(0, values.attack);
    this.sharedUniforms.uLow.value = Math.max(0, values.low);
    this.sharedUniforms.uMid.value = Math.max(0, values.mid);
    this.sharedUniforms.uHigh.value = Math.max(0, values.high);
    this.sharedUniforms.uSpeech.value = Math.max(0, values.speech);
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
