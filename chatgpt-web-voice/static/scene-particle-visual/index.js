import * as THREE from "/static/vendor/three/three.module.min.js";
import {
  SCENE_PARTICLE_DEFAULTS,
  SCENE_PARTICLE_RANGES,
  buildParticleAttributes,
  chooseSceneParticleStep,
  normalizeSceneParticleUniforms,
} from "./core.js";

const vertexShader = /* glsl */ `
  attribute vec3 color;
  attribute vec3 aRandom;
  attribute float aOpacity;

  uniform float uTime;
  uniform float uFlowSpeed;
  uniform float uDispersion;
  uniform float uDepth;
  uniform float uPointSize;
  uniform float uPixelRatio;
  uniform float uReveal;
  uniform float uOpacity;
  uniform float uMotionScale;
  uniform vec3 uPointer;
  uniform float uPointerStrength;
  uniform float uPressStrength;
  uniform float uPulseTime;
  uniform vec3 uPulsePosition;
  uniform float uPulseStrength;

  varying vec3 vColor;
  varying float vOpacity;

  vec4 permute(vec4 x) { return mod(((x * 34.0) + 1.0) * x, 289.0); }
  vec4 taylorInvSqrt(vec4 r) { return 1.79284291400159 - 0.85373472095314 * r; }

  float snoise(vec3 v) {
    const vec2 C = vec2(1.0 / 6.0, 1.0 / 3.0);
    const vec4 D = vec4(0.0, 0.5, 1.0, 2.0);
    vec3 i = floor(v + dot(v, C.yyy));
    vec3 x0 = v - i + dot(i, C.xxx);
    vec3 g = step(x0.yzx, x0.xyz);
    vec3 l = 1.0 - g;
    vec3 i1 = min(g.xyz, l.zxy);
    vec3 i2 = max(g.xyz, l.zxy);
    vec3 x1 = x0 - i1 + C.xxx;
    vec3 x2 = x0 - i2 + C.yyy;
    vec3 x3 = x0 - D.yyy;
    i = mod(i, 289.0);
    vec4 p = permute(permute(permute(
      i.z + vec4(0.0, i1.z, i2.z, 1.0))
      + i.y + vec4(0.0, i1.y, i2.y, 1.0))
      + i.x + vec4(0.0, i1.x, i2.x, 1.0));
    float n_ = 1.0 / 7.0;
    vec3 ns = n_ * D.wyz - D.xzx;
    vec4 j = p - 49.0 * floor(p * ns.z * ns.z);
    vec4 x_ = floor(j * ns.z);
    vec4 y_ = floor(j - 7.0 * x_);
    vec4 x = x_ * ns.x + ns.yyyy;
    vec4 y = y_ * ns.x + ns.yyyy;
    vec4 h = 1.0 - abs(x) - abs(y);
    vec4 b0 = vec4(x.xy, y.xy);
    vec4 b1 = vec4(x.zw, y.zw);
    vec4 s0 = floor(b0) * 2.0 + 1.0;
    vec4 s1 = floor(b1) * 2.0 + 1.0;
    vec4 sh = -step(h, vec4(0.0));
    vec4 a0 = b0.xzyw + s0.xzyw * sh.xxyy;
    vec4 a1 = b1.xzyw + s1.xzyw * sh.zzww;
    vec3 p0 = vec3(a0.xy, h.x);
    vec3 p1 = vec3(a0.zw, h.y);
    vec3 p2 = vec3(a1.xy, h.z);
    vec3 p3 = vec3(a1.zw, h.w);
    vec4 norm = taylorInvSqrt(vec4(dot(p0, p0), dot(p1, p1), dot(p2, p2), dot(p3, p3)));
    p0 *= norm.x;
    p1 *= norm.y;
    p2 *= norm.z;
    p3 *= norm.w;
    vec4 m = max(0.6 - vec4(dot(x0, x0), dot(x1, x1), dot(x2, x2), dot(x3, x3)), 0.0);
    m *= m;
    return 42.0 * dot(m * m, vec4(dot(p0, x0), dot(p1, x1), dot(p2, x2), dot(p3, x3)));
  }

  vec3 fluidNoise(vec3 p) {
    return vec3(
      snoise(p + vec3(0.0, 17.1, 3.7)),
      snoise(p + vec3(11.4, 0.0, 8.2)),
      snoise(p + vec3(6.3, 13.7, 0.0))
    );
  }

  void main() {
    vColor = color;
    vOpacity = aOpacity * uOpacity;
    float luminance = dot(color, vec3(0.2126, 0.7152, 0.0722));
    vec3 displaced = position;
    displaced.z += (luminance - 0.28) * uDepth;

    float driftTime = uTime * uFlowSpeed * uMotionScale;
    vec3 noisePosition = position * 0.56 + aRandom * 1.35 + vec3(0.0, driftTime, driftTime * 0.43);
    vec3 flow = fluidNoise(noisePosition);
    float randomWeight = 0.58 + dot(abs(aRandom), vec3(0.14));
    displaced += flow * uDispersion * randomWeight;

    float enter = 1.0 - uReveal;
    displaced += aRandom * enter * (0.42 + length(position.xy) * 0.12);
    displaced.z += enter * (1.2 + aRandom.z * 0.8);

    vec2 pointerDelta = displaced.xy - uPointer.xy;
    float pointerDistance = length(pointerDelta);
    float pointerFalloff = exp(-pointerDistance * pointerDistance * 3.2);
    vec2 pointerDirection = pointerDistance > 0.0001 ? pointerDelta / pointerDistance : vec2(0.0);
    float ripple = sin(pointerDistance * 16.0 - uTime * 5.0) * 0.055;
    displaced.xy += pointerDirection * pointerFalloff * (uPointerStrength * ripple + uPressStrength * 0.28);
    displaced.z += pointerFalloff * (uPointerStrength * 0.13 + uPressStrength * 0.5) * (0.55 + aRandom.z * 0.45);

    float pulseAge = max(0.0, uTime - uPulseTime);
    float pulseRadius = pulseAge * 1.35;
    float pulseBand = exp(-pow((length(displaced.xy - uPulsePosition.xy) - pulseRadius) * 8.0, 2.0));
    displaced.z += pulseBand * uPulseStrength * exp(-pulseAge * 1.8) * 0.38;

    vec4 mvPosition = modelViewMatrix * vec4(displaced, 1.0);
    gl_Position = projectionMatrix * mvPosition;
    float perspectiveScale = 6.0 / max(0.75, -mvPosition.z);
    gl_PointSize = clamp(uPointSize * uPixelRatio * perspectiveScale * (0.72 + luminance * 0.5), 1.0, 22.0);
  }
`;

const fragmentShader = /* glsl */ `
  varying vec3 vColor;
  varying float vOpacity;

  void main() {
    float distanceToCenter = length(gl_PointCoord - vec2(0.5));
    if (distanceToCenter > 0.5) discard;
    float feather = 1.0 - smoothstep(0.26, 0.5, distanceToCenter);
    float core = 1.0 - smoothstep(0.0, 0.2, distanceToCenter);
    float glow = feather * 0.42 + core * 0.36;
    gl_FragColor = vec4(vColor * (0.78 + core * 0.44), glow * vOpacity);
  }
`;

function waitForTransition(duration) {
  return new Promise((resolve) => window.setTimeout(resolve, duration));
}

function dispatch(container, name, detail = {}) {
  container.dispatchEvent(new CustomEvent(name, { detail }));
}

async function decodeBlob(blob, signal) {
  if (typeof createImageBitmap === "function") {
    try {
      return await createImageBitmap(blob, { colorSpaceConversion: "default", premultiplyAlpha: "default" });
    } catch (_) {
      if (signal?.aborted) throw new DOMException("Aborted", "AbortError");
    }
  }
  const objectURL = URL.createObjectURL(blob);
  try {
    const image = new Image();
    image.decoding = "async";
    image.src = objectURL;
    await image.decode();
    return image;
  } finally {
    URL.revokeObjectURL(objectURL);
  }
}

function imageToImageData(image) {
  const width = image.naturalWidth || image.width;
  const height = image.naturalHeight || image.height;
  const canvas = document.createElement("canvas");
  canvas.width = width;
  canvas.height = height;
  const context = canvas.getContext("2d", { willReadFrequently: true });
  if (!context) throw new Error("2D canvas is unavailable");
  context.clearRect(0, 0, width, height);
  context.drawImage(image, 0, 0, width, height);
  return context.getImageData(0, 0, width, height);
}

export class SceneParticleVisual {
  constructor(container, options = {}) {
    if (!container) throw new TypeError("SceneParticleVisual requires a container");
    this.container = container;
    this.options = options;
    this.reducedMotionQuery = window.matchMedia("(prefers-reduced-motion: reduce)");
    this.reducedMotion = this.reducedMotionQuery.matches;
    this.active = true;
    this.visible = !document.hidden;
    this.disposed = false;
    this.raf = 0;
    this.lastFrame = 0;
    this.transitionStartedAt = 0;
    this.fadeOutStartedAt = 0;
    this.fadeOutDuration = 0;
    this.pointerTarget = 0;
    this.pointerValue = 0;
    this.pressTarget = 0;
    this.pressValue = 0;
    this.parallaxTarget = new THREE.Vector2();
    this.parallax = new THREE.Vector2();
    this.loadVersion = 0;
    this.loadController = null;
    this.currentURL = "";
    this.particleCount = 0;
    this.worldWidth = 6;
    this.worldHeight = 4;
    this.uniformValues = normalizeSceneParticleUniforms(options.uniforms || {});
    this.step = Number.isFinite(Number(options.step)) ? Math.max(1, Math.floor(Number(options.step))) : chooseSceneParticleStep({
      mobile: window.matchMedia("(max-width: 700px), (pointer: coarse)").matches,
      reducedMotion: this.reducedMotion,
      deviceMemory: navigator.deviceMemory,
      hardwareConcurrency: navigator.hardwareConcurrency,
    });
    this.onVisibility = () => {
      this.visible = !document.hidden;
      if (this.visible) {
        this.lastFrame = performance.now();
        this.schedule();
      }
    };
    this.onContextLost = (event) => {
      event.preventDefault();
      this.setActive(false);
      this.container.classList.remove("is-ready");
      this.container.classList.add("is-unavailable");
      dispatch(this.container, "scene-particle-error", { error: new Error("WebGL context lost") });
    };
    this.onPointerMove = (event) => this.handlePointerMove(event);
    this.onPointerEnter = (event) => this.handlePointerMove(event);
    this.onPointerLeave = () => {
      this.pointerTarget = 0;
      this.pressTarget = 0;
      this.parallaxTarget.set(0, 0);
    };
    this.onPointerDown = (event) => {
      if (event.pointerType === "touch" && !this.container.closest(".scene-particle-dialog[open]")) return;
      this.handlePointerMove(event);
      this.pressTarget = 1;
      this.container.setPointerCapture?.(event.pointerId);
    };
    this.onPointerUp = (event) => {
      this.pressTarget = 0;
      if (this.container.hasPointerCapture?.(event.pointerId)) this.container.releasePointerCapture(event.pointerId);
    };
    this.initRenderer();
    document.addEventListener("visibilitychange", this.onVisibility);
    this.bindInteraction();
    this.createDebugPanel();
  }

  initRenderer() {
    const probe = document.createElement("canvas");
    if (!window.WebGL2RenderingContext || !probe.getContext("webgl2")) throw new Error("WebGL2 is unavailable");
    this.scene = new THREE.Scene();
    this.camera = new THREE.PerspectiveCamera(33, 1, 0.1, 30);
    this.camera.position.set(0, 0, 7.5);
    this.group = new THREE.Group();
    this.scene.add(this.group);
    this.renderer = new THREE.WebGLRenderer({ alpha: true, antialias: false, powerPreference: "high-performance" });
    this.renderer.setClearColor(0x000000, 0);
    this.renderer.outputColorSpace = THREE.SRGBColorSpace;
    this.renderer.toneMapping = THREE.ACESFilmicToneMapping;
    this.renderer.toneMappingExposure = 1.18;
    this.renderer.domElement.setAttribute("aria-hidden", "true");
    this.renderer.domElement.addEventListener("webglcontextlost", this.onContextLost, false);
    this.container.appendChild(this.renderer.domElement);

    this.uniforms = {
      uTime: { value: 0 },
      uFlowSpeed: { value: this.uniformValues.uFlowSpeed },
      uDispersion: { value: this.uniformValues.uDispersion },
      uDepth: { value: this.uniformValues.uDepth },
      uPointSize: { value: this.uniformValues.uPointSize },
      uPixelRatio: { value: 1 },
      uReveal: { value: this.reducedMotion ? 1 : 0 },
      uOpacity: { value: 1 },
      uMotionScale: { value: this.reducedMotion ? 0 : 1 },
      uPointer: { value: new THREE.Vector3() },
      uPointerStrength: { value: 0 },
      uPressStrength: { value: 0 },
      uPulseTime: { value: -100 },
      uPulsePosition: { value: new THREE.Vector3() },
      uPulseStrength: { value: 0 },
    };
    this.material = new THREE.ShaderMaterial({
      uniforms: this.uniforms,
      vertexShader,
      fragmentShader,
      transparent: true,
      depthWrite: false,
      depthTest: true,
      blending: THREE.AdditiveBlending,
      toneMapped: true,
    });
    this.points = new THREE.Points(new THREE.BufferGeometry(), this.material);
    this.group.add(this.points);
    this.geometry = this.points.geometry;
    this.resizeObserver = new ResizeObserver(() => this.resize());
    this.resizeObserver.observe(this.container);
    this.resize();
  }

  bindInteraction() {
    this.container.addEventListener("pointermove", this.onPointerMove, { passive: true });
    this.container.addEventListener("pointerenter", this.onPointerEnter, { passive: true });
    this.container.addEventListener("pointerleave", this.onPointerLeave, { passive: true });
    this.container.addEventListener("pointerdown", this.onPointerDown, { passive: true });
    this.container.addEventListener("pointerup", this.onPointerUp, { passive: true });
    this.container.addEventListener("pointercancel", this.onPointerUp, { passive: true });
  }

  handlePointerMove(event) {
    if (!this.geometry || !this.particleCount || this.reducedMotion) return;
    if (event.pointerType === "touch" && !this.container.closest(".scene-particle-dialog[open]")) return;
    const rect = this.container.getBoundingClientRect();
    if (!rect.width || !rect.height) return;
    const ndc = new THREE.Vector2(
      ((event.clientX - rect.left) / rect.width) * 2 - 1,
      -(((event.clientY - rect.top) / rect.height) * 2 - 1),
    );
    this.raycaster ||= new THREE.Raycaster();
    this.pointerPlane ||= new THREE.Plane(new THREE.Vector3(0, 0, 1), 0);
    this.pointerIntersection ||= new THREE.Vector3();
    this.raycaster.setFromCamera(ndc, this.camera);
    if (this.raycaster.ray.intersectPlane(this.pointerPlane, this.pointerIntersection)) {
      this.uniforms.uPointer.value.copy(this.pointerIntersection);
    }
    this.pointerTarget = 1;
    this.parallaxTarget.set(ndc.y * 0.025, ndc.x * 0.045);
  }

  async setImage(url, options = {}) {
    const nextURL = String(url || "").trim();
    if (!nextURL) throw new Error("scene image URL is required");
    const version = ++this.loadVersion;
    this.loadController?.abort();
    this.loadController = new AbortController();
    const signal = this.loadController.signal;
    this.container.classList.add("is-loading");
    this.container.classList.remove("is-unavailable");
    try {
      const response = await fetch(nextURL, { credentials: "same-origin", signal });
      if (!response.ok) throw new Error(`scene image ${response.status}`);
      const blob = await response.blob();
      const image = await decodeBlob(blob, signal);
      if (signal.aborted || version !== this.loadVersion) {
        image.close?.();
        return this;
      }
      const imageData = imageToImageData(image);
      image.close?.();
      const attributes = buildParticleAttributes(imageData, {
        step: options.step || this.step,
        alphaThreshold: options.alphaThreshold,
        worldHeight: 4,
      });
      if (signal.aborted || version !== this.loadVersion) return this;
      if (this.particleCount && !this.reducedMotion) {
        this.fadeOutStartedAt = performance.now();
        this.fadeOutDuration = 180;
        await waitForTransition(this.fadeOutDuration);
        if (signal.aborted || version !== this.loadVersion) return this;
      }
      this.installGeometry(attributes);
      this.currentURL = nextURL;
      this.container.dataset.particleCount = String(attributes.count);
      this.container.dataset.particleStep = String(attributes.step);
      this.container.classList.remove("is-loading");
      this.container.classList.add("is-ready");
      this.transitionStartedAt = performance.now();
      this.fadeOutDuration = 0;
      this.uniforms.uOpacity.value = 1;
      this.uniforms.uReveal.value = this.reducedMotion ? 1 : 0;
      this.updateDebugPanel();
      this.setActive(true);
      dispatch(this.container, "scene-particle-ready", { count: attributes.count, step: attributes.step, url: nextURL });
      return this;
    } catch (error) {
      if (error?.name === "AbortError" || version !== this.loadVersion) return this;
      this.container.classList.remove("is-loading", "is-ready");
      this.container.classList.add("is-unavailable");
      dispatch(this.container, "scene-particle-error", { error });
      throw error;
    }
  }

  installGeometry(attributes) {
    const geometry = new THREE.BufferGeometry();
    geometry.setAttribute("position", new THREE.BufferAttribute(attributes.positions, 3));
    geometry.setAttribute("color", new THREE.BufferAttribute(attributes.colors, 3));
    geometry.setAttribute("aRandom", new THREE.BufferAttribute(attributes.randoms, 3));
    geometry.setAttribute("aOpacity", new THREE.BufferAttribute(attributes.opacities, 1));
    geometry.computeBoundingSphere();
    const previous = this.geometry;
    this.geometry = geometry;
    this.points.geometry = geometry;
    this.particleCount = attributes.count;
    this.step = attributes.step;
    this.worldWidth = attributes.worldWidth;
    this.worldHeight = attributes.worldHeight;
    previous?.dispose();
    this.resize();
  }

  setUniforms(values = {}) {
    this.uniformValues = normalizeSceneParticleUniforms(values, this.uniformValues);
    for (const key of Object.keys(SCENE_PARTICLE_DEFAULTS)) this.uniforms[key].value = this.uniformValues[key];
    this.updateDebugPanel();
    return this.getUniforms();
  }

  getUniforms() {
    return { ...this.uniformValues };
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

  pulse(position = { x: 0, y: 0 }, strength = 0.6) {
    const x = Number(position.x ?? position[0] ?? 0);
    const y = Number(position.y ?? position[1] ?? 0);
    this.uniforms.uPulsePosition.value.set(Number.isFinite(x) ? x : 0, Number.isFinite(y) ? y : 0, 0);
    this.uniforms.uPulseStrength.value = Math.min(1.5, Math.max(0, Number(strength) || 0));
    this.uniforms.uPulseTime.value = performance.now() / 1000;
    this.schedule();
  }

  resize() {
    if (!this.renderer || !this.camera) return;
    const width = Math.max(1, this.container.clientWidth);
    const height = Math.max(1, this.container.clientHeight);
    const mobile = width < 700;
    const dpr = Math.min(window.devicePixelRatio || 1, mobile ? 1.25 : 1.5);
    this.renderer.setPixelRatio(dpr);
    this.renderer.setSize(width, height, false);
    this.camera.aspect = width / height;
    const halfFov = THREE.MathUtils.degToRad(this.camera.fov * 0.5);
    const verticalDistance = (this.worldHeight * 0.5) / Math.tan(halfFov);
    const horizontalDistance = (this.worldWidth * 0.5) / (Math.tan(halfFov) * this.camera.aspect);
    this.camera.position.z = Math.max(verticalDistance, horizontalDistance) * 1.12;
    this.camera.updateProjectionMatrix();
    this.uniforms.uPixelRatio.value = dpr;
  }

  schedule() {
    if (this.disposed || !this.active || !this.visible || !this.particleCount || this.raf) return;
    this.raf = requestAnimationFrame((time) => {
      this.raf = 0;
      this.render(time);
      this.schedule();
    });
  }

  render(timestamp) {
    if (!this.renderer || !this.scene || !this.camera || !this.particleCount) return;
    const dt = Math.min(0.05, Math.max(1 / 240, (timestamp - this.lastFrame) / 1000 || 1 / 60));
    this.lastFrame = timestamp;
    const smoothing = 1 - Math.exp(-dt * 8);
    this.pointerValue += (this.pointerTarget - this.pointerValue) * smoothing;
    this.pressValue += (this.pressTarget - this.pressValue) * smoothing;
    this.parallax.lerp(this.parallaxTarget, 1 - Math.exp(-dt * 5));
    this.uniforms.uTime.value = timestamp / 1000;
    this.uniforms.uPointerStrength.value = this.reducedMotion ? 0 : this.pointerValue;
    this.uniforms.uPressStrength.value = this.reducedMotion ? 0 : this.pressValue;
    if (!this.reducedMotion) {
      const elapsed = Math.max(0, timestamp - this.transitionStartedAt);
      const progress = Math.min(1, elapsed / 1800);
      this.uniforms.uReveal.value = 1 - Math.pow(1 - progress, 3);
      this.group.rotation.x = this.parallax.x + Math.sin(timestamp * 0.00013) * 0.012;
      this.group.rotation.y = this.parallax.y + Math.sin(timestamp * 0.00017) * 0.018;
    }
    if (this.fadeOutDuration > 0) {
      const fade = Math.min(1, (timestamp - this.fadeOutStartedAt) / this.fadeOutDuration);
      this.uniforms.uOpacity.value = 1 - fade;
    }
    this.renderer.render(this.scene, this.camera);
  }

  createDebugPanel() {
    if (new URLSearchParams(window.location.search).get("scene_particles_debug") !== "1") return;
    const panel = document.createElement("aside");
    panel.className = "scene-particle-debug";
    panel.setAttribute("aria-label", "Scene particle debug controls");
    const title = document.createElement("strong");
    title.textContent = "Scene particles";
    panel.appendChild(title);
    this.debugInputs = {};
    for (const [key, range] of Object.entries(SCENE_PARTICLE_RANGES)) {
      const label = document.createElement("label");
      const name = document.createElement("span");
      name.textContent = key;
      const input = document.createElement("input");
      input.type = "range";
      input.min = String(range[0]);
      input.max = String(range[1]);
      input.step = key === "uPointSize" ? "0.1" : "0.01";
      input.value = String(this.uniformValues[key]);
      input.addEventListener("input", () => this.setUniforms({ [key]: input.value }));
      label.append(name, input);
      panel.appendChild(label);
      this.debugInputs[key] = input;
    }
    this.debugCount = document.createElement("output");
    panel.appendChild(this.debugCount);
    const actions = document.createElement("div");
    const reset = document.createElement("button");
    reset.type = "button";
    reset.textContent = "Reset";
    reset.onclick = () => this.setUniforms(SCENE_PARTICLE_DEFAULTS);
    const pause = document.createElement("button");
    pause.type = "button";
    pause.textContent = "Pause";
    pause.onclick = () => {
      this.setActive(!this.active);
      pause.textContent = this.active ? "Pause" : "Resume";
    };
    actions.append(reset, pause);
    panel.appendChild(actions);
    document.body.appendChild(panel);
    this.debugPanel = panel;
    this.updateDebugPanel();
  }

  updateDebugPanel() {
    if (!this.debugPanel) return;
    for (const [key, input] of Object.entries(this.debugInputs)) input.value = String(this.uniformValues[key]);
    this.debugCount.textContent = `${this.particleCount.toLocaleString()} particles · step ${this.step}`;
  }

  dispose() {
    if (this.disposed) return;
    this.disposed = true;
    this.loadVersion += 1;
    this.loadController?.abort();
    if (this.raf) cancelAnimationFrame(this.raf);
    document.removeEventListener("visibilitychange", this.onVisibility);
    this.resizeObserver?.disconnect();
    this.container.removeEventListener("pointermove", this.onPointerMove);
    this.container.removeEventListener("pointerenter", this.onPointerEnter);
    this.container.removeEventListener("pointerleave", this.onPointerLeave);
    this.container.removeEventListener("pointerdown", this.onPointerDown);
    this.container.removeEventListener("pointerup", this.onPointerUp);
    this.container.removeEventListener("pointercancel", this.onPointerUp);
    this.renderer?.domElement.removeEventListener("webglcontextlost", this.onContextLost);
    this.geometry?.dispose();
    this.material?.dispose();
    this.renderer?.dispose();
    this.renderer?.domElement.remove();
    this.debugPanel?.remove();
    this.container.classList.remove("is-ready", "is-loading");
  }
}

export { vertexShader, fragmentShader };
