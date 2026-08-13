import * as THREE from "/static/vendor/three/three.module.min.js";
import {
  SCENE_PARTICLE_DEFAULTS,
  SCENE_PARTICLE_RANGES,
  buildParticleAttributes,
  chooseSceneParticleStep,
  normalizeSceneParticleUniforms,
} from "./core.js?v=20260813-8";

const PRESETS = Object.freeze({
  still: SCENE_PARTICLE_DEFAULTS,
  ethereal: Object.freeze({ uFlowSpeed: 0.22, uDispersion: 0.1, uDepth: 1.45, uPointSize: 1.36 }),
  dissolve: Object.freeze({ uFlowSpeed: 0.58, uDispersion: 1.15, uDepth: 1.85, uPointSize: 1.42 }),
});

const vertexShader = /* glsl */ `
  attribute vec3 color;
  attribute vec3 aRandom;
  attribute float aOpacity;
  attribute float aVisualWeight;
  attribute float aEdge;
  attribute float aLuminance;

  uniform float uTime;
  uniform float uFlowSpeed;
  uniform float uDispersion;
  uniform float uDepth;
  uniform float uPointSize;
  uniform float uPixelRatio;
  uniform float uReveal;
  uniform float uOpacity;
  uniform float uMotionScale;
  uniform float uLayer;
  uniform float uHaloScale;
  uniform float uDetailScale;
  uniform float uWorldWidth;
  uniform float uWorldHeight;
  uniform vec3 uPointer;
  uniform float uPointerStrength;
  uniform float uPressStrength;
  uniform float uPressVelocity;
  uniform float uPulseTime;
  uniform vec3 uPulsePosition;
  uniform float uPulseStrength;

  varying vec3 vColor;
  varying float vOpacity;
  varying float vVisualWeight;
  varying float vLuminance;

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

  vec3 curlNoise(vec3 p, float epsilon) {
    float x1 = snoise(p + vec3(epsilon, 0.0, 0.0));
    float x0 = snoise(p - vec3(epsilon, 0.0, 0.0));
    float y1 = snoise(p + vec3(0.0, epsilon, 0.0));
    float y0 = snoise(p - vec3(0.0, epsilon, 0.0));
    float dPsiDx = (x1 - x0) / (2.0 * epsilon);
    float dPsiDy = (y1 - y0) / (2.0 * epsilon);
    float depthBreath = snoise(p * 0.72 + vec3(13.7, -5.2, 8.1));
    return vec3(dPsiDy, -dPsiDx, depthBreath * 0.28);
  }

  vec3 srgbToLinear(vec3 value) {
    vec3 low = value / 12.92;
    vec3 high = pow((value + 0.055) / 1.055, vec3(2.4));
    return mix(low, high, step(vec3(0.04045), value));
  }

  void main() {
    vec3 linearColor = srgbToLinear(color);
    float chroma = max(linearColor.r, max(linearColor.g, linearColor.b)) - min(linearColor.r, min(linearColor.g, linearColor.b));
    float highlightCompression = mix(1.0, 0.72, smoothstep(0.58, 0.96, aLuminance) * (1.0 - smoothstep(0.035, 0.2, chroma)));
    
    float halfW = max(0.001, uWorldWidth * 0.5);
    float halfH = max(0.001, uWorldHeight * 0.5);
    float centerDist = length(position.xy) / max(halfW, halfH);
    float centerBloom = exp(-centerDist * centerDist * 2.5);
    
    vColor = linearColor * highlightCompression * (1.0 + centerBloom * 0.6);
    vVisualWeight = aVisualWeight;
    vLuminance = aLuminance;

    vec3 displaced = position;
    float darkRecess = 1.0 - smoothstep(0.0, 0.25, aLuminance);
    float brightLimit = smoothstep(0.5, 0.9, aLuminance);
    float depthShape = (aLuminance - 0.4) * 1.5 + aVisualWeight * 0.6 - darkRecess * 0.5 + brightLimit * 0.4;
    displaced.z += clamp(depthShape * uDepth, -1.8, 1.8);

    float driftTime = uTime * uFlowSpeed * uMotionScale;
    vec3 flowPosition = position * 0.31 + vec3(0.0, driftTime * 0.16, driftTime * 0.08) + aRandom * 0.045;
    vec3 broadCurl = curlNoise(flowPosition, 0.17);
    vec3 detailCurl = vec3(
      sin(flowPosition.y * 3.7 + driftTime * 0.09),
      cos(flowPosition.x * 3.2 - driftTime * 0.08),
      sin((flowPosition.x + flowPosition.y) * 2.6 + driftTime * 0.05)
    ) * (0.035 * uDetailScale);
    vec3 flow = normalize(broadCurl + vec3(0.0001)) * min(1.15, length(broadCurl)) + detailCurl;
    float vulnerability = clamp((1.0 - aVisualWeight) * 0.9 + aEdge * (0.16 + (1.0 - aVisualWeight) * 0.34), 0.05, 1.0);
    float dispersionCurve = uDispersion * uDispersion * (0.16 + uDispersion * 0.08);
    float subjectGate = smoothstep(0.16, 1.15, uDispersion + vulnerability * 0.48);
    float flowAmount = dispersionCurve * mix(0.16, 1.0, vulnerability) * subjectGate;
    displaced += flow * flowAmount;
    displaced.z += broadCurl.z * uDispersion * 0.026 * (0.24 + vulnerability);

    float localReveal = smoothstep(0.02 + uLayer * 0.2 + (1.0 - aVisualWeight) * 0.12, 0.72 + uLayer * 0.22, uReveal);
    float enter = 1.0 - localReveal;
    vec3 gatherDirection = normalize(vec3(position.xy * 0.38, 1.1 + aRandom.z * 0.22));
    displaced -= vec3(position.xy, 0.0) * enter * (0.64 + uLayer * 0.12);
    displaced += gatherDirection * enter * (0.48 + length(position.xy) * 0.08 + uLayer * 0.32);
    displaced.z += enter * (1.0 + (1.0 - aVisualWeight) * 0.72 + aRandom.z * 0.2);

    vec2 pointerDelta = displaced.xy - uPointer.xy;
    float pointerDistance = length(pointerDelta);
    float pointerFalloff = exp(-pointerDistance * pointerDistance * 2.1);
    vec2 radial = pointerDistance > 0.0001 ? pointerDelta / pointerDistance : vec2(0.0);
    vec2 tangent = vec2(-radial.y, radial.x);
    float pointerNoise = 1.0;
    if (uPointerStrength + uPressStrength > 0.01) pointerNoise = 0.82 + snoise(vec3(pointerDelta * 0.72, uTime * 0.12)) * 0.18;
    displaced.xy += tangent * pointerFalloff * uPointerStrength * 0.045 * pointerNoise;
    displaced.xy += (tangent * (0.34 + vulnerability * 0.66) + radial * 0.18) * pointerFalloff * uPressStrength * 0.34;
    displaced.z += pointerFalloff * (uPointerStrength * 0.035 + uPressStrength * 0.28 + uPressVelocity * 0.04) * (0.55 + vulnerability * 0.45);

    float pulseAge = max(0.0, uTime - uPulseTime);
    float pulseRadius = pulseAge * 0.78;
    float pulseWarp = 0.0;
    if (pulseAge < 4.0 && uPulseStrength > 0.01) pulseWarp = snoise(vec3(displaced.xy * 0.5, pulseAge * 0.16)) * 0.08;
    float warpedDistance = length(displaced.xy - uPulsePosition.xy) + pulseWarp;
    float pulseBand = exp(-pow((warpedDistance - pulseRadius) / (0.22 + pulseAge * 0.08), 2.0));
    float pulseDecay = exp(-pulseAge * 1.45) * uPulseStrength;
    displaced.xy += tangent * pulseBand * pulseDecay * 0.035;
    displaced.z += pulseBand * pulseDecay * 0.12;

    float nx = abs(position.x) / halfW;
    float ny = abs(position.y) / halfH;
    float frame = max(nx, ny);
    float corner = nx * ny;
    float grain = snoise(vec3(position.xy * 2.5, aRandom.z * 2.0));
    float edgeMix = frame * 0.8 + corner * 0.2 + grain * 0.25;
    float edgeKeep = 1.0 - smoothstep(0.6, 1.1, edgeMix);
    displaced += flow * (1.0 - edgeKeep) * 0.18 * uMotionScale;

    displaced.z = clamp(displaced.z, -1.8, 1.8);
    vec4 mvPosition = modelViewMatrix * vec4(displaced, 1.0);
    gl_Position = projectionMatrix * mvPosition;
    float perspectiveScale = clamp(7.0 / max(2.0, -mvPosition.z), 0.72, 1.28);
    float structureSize = mix(0.9, 1.08, aVisualWeight) * mix(0.96, 1.06, aEdge);
    float layerScale = mix(1.0, 1.55 * uHaloScale, uLayer);
    gl_PointSize = clamp(uPointSize * uPixelRatio * perspectiveScale * structureSize * layerScale, 0.75, mix(6.5, 11.0, uLayer));
    float densityAlpha = mix(0.55, 1.0, aVisualWeight);
    vOpacity = aOpacity * densityAlpha * uOpacity * localReveal * mix(0.04, 1.0, edgeKeep);
  }
`;

const fragmentShader = /* glsl */ `
  uniform float uLayer;
  uniform float uHaloAlpha;

  varying vec3 vColor;
  varying float vOpacity;
  varying float vVisualWeight;
  varying float vLuminance;

  void main() {
    vec2 centered = gl_PointCoord - vec2(0.5);
    float radius = length(centered);
    if (radius > 0.5) discard;
    float coreShape = smoothstep(0.5, 0.16, radius);
    float softCore = exp(-radius * radius * 22.0);
    float haloShape = exp(-radius * radius * 10.5) * smoothstep(0.5, 0.04, radius);
    float shape = mix(coreShape * 0.78 + softCore * 0.3, haloShape, uLayer);
    float layerAlpha = mix(0.92, uHaloAlpha * mix(0.52, 1.0, vVisualWeight), uLayer);
    float alpha = shape * vOpacity * layerAlpha;
    if (alpha < 0.008) discard;
    vec3 coreColor = vColor * mix(1.02, 1.14, vVisualWeight);
    vec3 haloColor = vColor * mix(0.78, 1.02, smoothstep(0.08, 0.7, vLuminance));
    gl_FragColor = vec4(mix(coreColor, haloColor, uLayer), alpha);
    #include <tonemapping_fragment>
    #include <colorspace_fragment>
  }
`;

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
    if (signal?.aborted) throw new DOMException("Aborted", "AbortError");
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

function createSharedUniforms(values, reducedMotion) {
  return {
    uTime: { value: 0 },
    uFlowSpeed: { value: values.uFlowSpeed },
    uDispersion: { value: values.uDispersion },
    uDepth: { value: values.uDepth },
    uPointSize: { value: values.uPointSize },
    uPixelRatio: { value: 1 },
    uReveal: { value: reducedMotion ? 1 : 0 },
    uOpacity: { value: 1 },
    uMotionScale: { value: reducedMotion ? 0 : 1 },
    uHaloScale: { value: 1 },
    uHaloAlpha: { value: 0.14 },
    uDetailScale: { value: 1 },
    uWorldWidth: { value: 6 },
    uWorldHeight: { value: 4 },
    uPointer: { value: new THREE.Vector3() },
    uPointerStrength: { value: 0 },
    uPressStrength: { value: 0 },
    uPressVelocity: { value: 0 },
    uPulseTime: { value: -100 },
    uPulsePosition: { value: new THREE.Vector3() },
    uPulseStrength: { value: 0 },
  };
}

function createLayerMaterial(sharedUniforms, layer) {
  return new THREE.ShaderMaterial({
    uniforms: { ...sharedUniforms, uLayer: { value: layer } },
    vertexShader,
    fragmentShader,
    transparent: true,
    depthWrite: false,
    depthTest: true,
    blending: layer > 0.5 ? THREE.AdditiveBlending : THREE.NormalBlending,
    toneMapped: true,
  });
}

export class SceneParticleVisual {
  constructor(container, options = {}) {
    if (!container) throw new TypeError("SceneParticleVisual requires a container");
    this.container = container;
    this.options = options;
    this.reducedMotionQuery = window.matchMedia("(prefers-reduced-motion: reduce)");
    this.reducedMotion = this.reducedMotionQuery.matches;
    this.active = true;
    this.pageVisible = !document.hidden;
    this.intersecting = true;
    this.disposed = false;
    this.contextLost = false;
    this.immersive = false;
    this.raf = 0;
    this.lastFrame = 0;
    this.transitionStartedAt = 0;
    this.pointerTarget = 0;
    this.pointerValue = 0;
    this.pressTarget = 0;
    this.pressValue = 0;
    this.pressVelocity = 0;
    this.parallaxTarget = new THREE.Vector2();
    this.parallax = new THREE.Vector2();
    this.dragRotation = new THREE.Vector2();
    this.targetDragRotation = new THREE.Vector2();
    this.isDragging = false;
    this.previousPointer = { x: 0, y: 0 };
    this.loadVersion = 0;
    this.loadController = null;
    this.currentURL = "";
    this.lastImageData = null;
    this.particleCount = 0;
    this.worldWidth = 6;
    this.worldHeight = 4;
    this.uniformValues = normalizeSceneParticleUniforms(options.uniforms || {});
    this.baseStep = Number.isFinite(Number(options.step)) ? Math.max(1, Math.floor(Number(options.step))) : chooseSceneParticleStep({
      mobile: window.matchMedia("(max-width: 700px), (pointer: coarse)").matches,
      reducedMotion: this.reducedMotion,
      deviceMemory: navigator.deviceMemory,
      hardwareConcurrency: navigator.hardwareConcurrency,
    });
    this.step = this.baseStep;
    this.qualityLevel = 0;
    this.qualityLabel = "high";
    this.dprScale = 1;
    this.haloEnabled = !this.reducedMotion;
    this.detailScale = 1;
    this.frameSamples = [];
    this.slowWindows = 0;
    this.lastQualityChange = 0;
    this.fps = 0;

    this.onVisibility = () => {
      this.pageVisible = !document.hidden;
      if (this.pageVisible) this.resumeClock();
      else this.cancelFrame();
      this.dispatchActivity();
    };
    this.onReducedMotion = (event) => {
      this.reducedMotion = event.matches;
      if (this.uniforms) {
        this.uniforms.uMotionScale.value = this.reducedMotion ? 0 : 1;
        this.uniforms.uReveal.value = 1;
      }
      this.schedule();
    };
    this.onContextLost = (event) => {
      event.preventDefault();
      this.contextLost = true;
      this.cancelFrame();
      this.container.classList.remove("is-ready");
      this.container.classList.add("is-unavailable");
      dispatch(this.container, "scene-particle-error", { error: new Error("WebGL context lost") });
      this.dispatchActivity();
    };
    this.onContextRestored = () => this.restoreContext();
    this.onPointerMove = (event) => this.handlePointerMove(event);
    this.onPointerEnter = (event) => this.handlePointerMove(event);
    this.onPointerLeave = () => {
      this.pointerTarget = 0;
      this.pressTarget = 0;
      this.parallaxTarget.set(0, 0);
      this.isDragging = false;
    };
    this.onPointerDown = (event) => {
      if (event.pointerType === "touch" && !this.immersive) return;
      this.isDragging = true;
      this.previousPointer = { x: event.clientX, y: event.clientY };
      this.handlePointerMove(event);
      this.pressTarget = 1;
      this.container.setPointerCapture?.(event.pointerId);
    };
    this.onPointerUp = (event) => {
      this.isDragging = false;
      this.pressTarget = 0;
      if (this.container.hasPointerCapture?.(event.pointerId)) this.container.releasePointerCapture(event.pointerId);
    };

    this.initRenderer();
    this.bindLifecycle();
    this.bindInteraction();
    this.createDebugPanel();
  }

  bindLifecycle() {
    document.addEventListener("visibilitychange", this.onVisibility);
    this.reducedMotionQuery.addEventListener?.("change", this.onReducedMotion);
    this.resizeObserver = new ResizeObserver(() => this.resize());
    this.resizeObserver.observe(this.container);
    if (typeof IntersectionObserver === "function") {
      this.intersectionObserver = new IntersectionObserver((entries) => {
        const entry = entries[entries.length - 1];
        this.intersecting = Boolean(entry?.isIntersecting && entry.intersectionRatio > 0);
        if (this.intersecting) this.resumeClock();
        else this.cancelFrame();
        this.dispatchActivity();
      }, { threshold: [0, 0.01] });
      this.intersectionObserver.observe(this.container);
    }
  }

  initRenderer() {
    const probe = document.createElement("canvas");
    if (!window.WebGL2RenderingContext || !probe.getContext("webgl2")) throw new Error("WebGL2 is unavailable");
    this.scene = new THREE.Scene();
    this.camera = new THREE.PerspectiveCamera(33, 1, 0.1, 40);
    this.camera.position.set(0, 0, 8);
    this.group = new THREE.Group();
    this.scene.add(this.group);
    this.renderer = new THREE.WebGLRenderer({ alpha: true, antialias: false, powerPreference: "high-performance" });
    this.renderer.setClearColor(0x000000, 0);
    this.renderer.outputColorSpace = THREE.SRGBColorSpace;
    this.renderer.toneMapping = THREE.ACESFilmicToneMapping;
    this.renderer.toneMappingExposure = 1.18;
    this.renderer.domElement.setAttribute("aria-hidden", "true");
    this.renderer.domElement.addEventListener("webglcontextlost", this.onContextLost, false);
    this.renderer.domElement.addEventListener("webglcontextrestored", this.onContextRestored, false);
    this.container.appendChild(this.renderer.domElement);
    this.uniforms = createSharedUniforms(this.uniformValues, this.reducedMotion);
    this.geometry = new THREE.BufferGeometry();
    this.coreMaterial = createLayerMaterial(this.uniforms, 0);
    this.haloMaterial = createLayerMaterial(this.uniforms, 1);
    this.corePoints = new THREE.Points(this.geometry, this.coreMaterial);
    this.haloPoints = new THREE.Points(this.geometry, this.haloMaterial);
    this.corePoints.renderOrder = 1;
    this.haloPoints.renderOrder = 2;
    this.group.add(this.corePoints, this.haloPoints);
    this.applyQualitySettings();
    this.resize();
  }

  releaseRenderResources({ removeCanvas = true } = {}) {
    for (const transition of this.outgoingLayers || []) {
      transition.core?.material.dispose();
      transition.halo?.material.dispose();
      transition.geometry?.dispose();
      transition.group?.removeFromParent();
    }
    this.outgoingLayers = [];
    this.geometry?.dispose();
    this.coreMaterial?.dispose();
    this.haloMaterial?.dispose();
    if (this.renderer?.domElement) {
      this.renderer.domElement.removeEventListener("webglcontextlost", this.onContextLost);
      this.renderer.domElement.removeEventListener("webglcontextrestored", this.onContextRestored);
      if (removeCanvas) this.renderer.domElement.remove();
    }
    this.renderer?.dispose();
  }

  async restoreContext() {
    if (this.disposed) return;
    try {
      this.releaseRenderResources();
      this.initRenderer();
      if (this.lastImageData) {
        const attributes = buildParticleAttributes(this.lastImageData, { step: this.step, worldHeight: 4 });
        this.installGeometry(attributes, { transition: false });
        this.uniforms.uReveal.value = 1;
        this.container.classList.remove("is-unavailable", "is-loading");
        this.container.classList.add("is-ready");
        dispatch(this.container, "scene-particle-ready", { count: attributes.count, step: attributes.step, url: this.currentURL, restored: true });
      }
      this.contextLost = false;
      this.resumeClock();
      this.dispatchActivity();
    } catch (error) {
      this.contextLost = true;
      this.container.classList.remove("is-ready");
      this.container.classList.add("is-unavailable");
      dispatch(this.container, "scene-particle-error", { error });
    }
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
    if (event.pointerType === "touch" && !this.immersive) return;
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
    if (this.raycaster.ray.intersectPlane(this.pointerPlane, this.pointerIntersection)) this.uniforms.uPointer.value.copy(this.pointerIntersection);
    this.pointerTarget = 1;
    
    if (this.isDragging) {
      const deltaX = event.clientX - this.previousPointer.x;
      const deltaY = event.clientY - this.previousPointer.y;
      this.targetDragRotation.y += deltaX * 0.005;
      this.targetDragRotation.x += deltaY * 0.005;
      this.targetDragRotation.x = THREE.MathUtils.clamp(this.targetDragRotation.x, -Math.PI / 3, Math.PI / 3);
      this.previousPointer = { x: event.clientX, y: event.clientY };
    } else {
      this.parallaxTarget.set(ndc.y * 0.018, ndc.x * 0.032);
    }
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
      if (signal.aborted || version !== this.loadVersion || this.disposed) {
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
      if (signal.aborted || version !== this.loadVersion || this.disposed) return this;
      this.lastImageData = imageData;
      this.installGeometry(attributes, { transition: Boolean(this.particleCount && !this.reducedMotion) });
      if (signal.aborted || version !== this.loadVersion || this.disposed) return this;
      this.currentURL = nextURL;
      this.container.dataset.particleCount = String(attributes.count);
      this.container.dataset.particleStep = String(attributes.step);
      this.container.classList.remove("is-loading", "is-unavailable");
      this.container.classList.add("is-ready");
      this.transitionStartedAt = performance.now();
      this.uniforms.uOpacity.value = 1;
      this.uniforms.uReveal.value = this.reducedMotion ? 1 : 0;
      this.updateDebugPanel();
      this.setActive(true);
      dispatch(this.container, "scene-particle-ready", { count: attributes.count, step: attributes.step, url: nextURL });
      return this;
    } catch (error) {
      if (error?.name === "AbortError" || version !== this.loadVersion || this.disposed) return this;
      this.container.classList.remove("is-loading", "is-ready");
      this.container.classList.add("is-unavailable");
      dispatch(this.container, "scene-particle-error", { error });
      throw error;
    }
  }

  installGeometry(attributes, { transition = true } = {}) {
    const geometry = new THREE.BufferGeometry();
    geometry.setAttribute("position", new THREE.BufferAttribute(attributes.positions, 3));
    geometry.setAttribute("color", new THREE.BufferAttribute(attributes.colors, 3));
    geometry.setAttribute("aRandom", new THREE.BufferAttribute(attributes.randoms, 3));
    geometry.setAttribute("aOpacity", new THREE.BufferAttribute(attributes.opacities, 1));
    geometry.setAttribute("aVisualWeight", new THREE.BufferAttribute(attributes.visualWeights, 1));
    geometry.setAttribute("aEdge", new THREE.BufferAttribute(attributes.edges, 1));
    geometry.setAttribute("aLuminance", new THREE.BufferAttribute(attributes.luminances, 1));
    geometry.computeBoundingSphere();

    const previous = this.geometry;
    if (transition && previous?.getAttribute("position")?.count) this.createOutgoingLayers(previous);
    else previous?.dispose();
    this.geometry = geometry;
    this.corePoints.geometry = geometry;
    this.haloPoints.geometry = geometry;
    this.particleCount = attributes.count;
    this.step = attributes.step;
    this.worldWidth = attributes.worldWidth;
    this.worldHeight = attributes.worldHeight;
    if (this.uniforms) {
      this.uniforms.uWorldWidth.value = this.worldWidth;
      this.uniforms.uWorldHeight.value = this.worldHeight;
    }
    this.resize();
  }

  createOutgoingLayers(geometry) {
    this.outgoingLayers ||= [];
    const uniforms = createSharedUniforms(this.uniformValues, false);
    uniforms.uReveal.value = 1;
    uniforms.uDispersion.value = Math.max(0.7, this.uniformValues.uDispersion);
    uniforms.uWorldWidth.value = this.worldWidth;
    uniforms.uWorldHeight.value = this.worldHeight;
    const coreMaterial = createLayerMaterial(uniforms, 0);
    const haloMaterial = createLayerMaterial(uniforms, 1);
    const core = new THREE.Points(geometry, coreMaterial);
    const halo = new THREE.Points(geometry, haloMaterial);
    const group = new THREE.Group();
    group.add(core, halo);
    this.scene.add(group);
    this.outgoingLayers.push({ group, geometry, core, halo, uniforms, startedAt: performance.now(), duration: 720 });
  }

  setUniforms(values = {}) {
    this.uniformValues = normalizeSceneParticleUniforms(values, this.uniformValues);
    for (const key of Object.keys(SCENE_PARTICLE_DEFAULTS)) this.uniforms[key].value = this.uniformValues[key];
    this.updateDebugPanel();
    this.schedule();
    return this.getUniforms();
  }

  getUniforms() {
    return { ...this.uniformValues };
  }

  setPreset(name) {
    const preset = PRESETS[String(name || "")];
    if (!preset) throw new RangeError(`unknown scene particle preset: ${name}`);
    this.preset = String(name);
    return this.setUniforms(preset);
  }

  setActive(active) {
    this.active = Boolean(active);
    if (!this.active) this.cancelFrame();
    else this.resumeClock();
    this.dispatchActivity();
  }

  setImmersive(immersive) {
    this.immersive = Boolean(immersive);
    this.resize();
    if (this.immersive && !this.reducedMotion) this.pulse({ x: 0, y: 0 }, 0.42);
  }

  pulse(position = { x: 0, y: 0 }, strength = 0.6) {
    if (!this.uniforms) return;
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
    const maxDpr = mobile ? 1.25 : 1.5;
    const dpr = Math.max(0.75, Math.min(window.devicePixelRatio || 1, maxDpr) * this.dprScale);
    this.renderer.setPixelRatio(dpr);
    this.renderer.setSize(width, height, false);
    this.camera.aspect = width / height;
    const halfFov = THREE.MathUtils.degToRad(this.camera.fov * 0.5);
    const verticalDistance = (this.worldHeight * 0.5) / Math.tan(halfFov);
    const horizontalDistance = (this.worldWidth * 0.5) / (Math.tan(halfFov) * this.camera.aspect);
    const framingDistance = this.immersive
      ? Math.max(verticalDistance, horizontalDistance)
      : Math.min(verticalDistance, horizontalDistance);
    const framingScale = this.immersive ? 1.28 : 0.9;
    this.camera.position.z = Math.max(3.8, framingDistance * framingScale + 0.55);
    this.camera.near = Math.max(0.1, this.camera.position.z - 4.0);
    this.camera.far = this.camera.position.z + 5.0;
    this.camera.updateProjectionMatrix();
    this.uniforms.uPixelRatio.value = dpr;
    this.currentDpr = dpr;
    this.updateDebugPanel();
  }

  canRender() {
    return !this.disposed && !this.contextLost && this.active && this.pageVisible && this.intersecting && this.particleCount > 0;
  }

  dispatchActivity() {
    dispatch(this.container, "scene-particle-activity", { active: this.canRender() });
  }

  cancelFrame() {
    if (this.raf) cancelAnimationFrame(this.raf);
    this.raf = 0;
  }

  resumeClock() {
    this.lastFrame = performance.now();
    this.schedule();
  }

  schedule() {
    if (!this.canRender() || this.raf) return;
    this.raf = requestAnimationFrame((time) => {
      this.raf = 0;
      this.render(time);
      if (!this.reducedMotion || this.outgoingLayers?.length) this.schedule();
    });
  }

  render(timestamp) {
    if (!this.renderer || !this.scene || !this.camera || !this.particleCount) return;
    const frameMs = Math.min(80, Math.max(1, timestamp - this.lastFrame || 16.67));
    const dt = Math.min(0.05, Math.max(1 / 240, frameMs / 1000));
    this.lastFrame = timestamp;
    const pointerSmoothing = 1 - Math.exp(-dt * 7);
    this.pointerValue += (this.pointerTarget - this.pointerValue) * pointerSmoothing;
    const springAcceleration = (this.pressTarget - this.pressValue) * 34 - this.pressVelocity * 8.5;
    this.pressVelocity += springAcceleration * dt;
    this.pressValue += this.pressVelocity * dt;
    this.pressValue = THREE.MathUtils.clamp(this.pressValue, -0.08, 1.08);
    this.parallax.lerp(this.parallaxTarget, 1 - Math.exp(-dt * 4));
    this.dragRotation.lerp(this.targetDragRotation, 1 - Math.exp(-dt * 8));
    this.uniforms.uTime.value = timestamp / 1000;
    this.uniforms.uPointerStrength.value = this.reducedMotion ? 0 : this.pointerValue;
    this.uniforms.uPressStrength.value = this.reducedMotion ? 0 : Math.max(0, this.pressValue);
    this.uniforms.uPressVelocity.value = this.reducedMotion ? 0 : this.pressVelocity;
    if (!this.reducedMotion) {
      const elapsed = Math.max(0, timestamp - this.transitionStartedAt);
      const progress = Math.min(1, elapsed / 2050);
      this.uniforms.uReveal.value = 1 - Math.pow(1 - progress, 3);
      const drift = this.immersive ? 1 : 0.42;
      this.group.rotation.x = this.parallax.x + this.dragRotation.x + Math.sin(timestamp * 0.00009) * 0.006 * drift;
      this.group.rotation.y = this.parallax.y + this.dragRotation.y + Math.sin(timestamp * 0.00011) * 0.009 * drift;
    }
    this.updateOutgoingLayers(timestamp);
    this.renderer.render(this.scene, this.camera);
    this.monitorPerformance(frameMs, timestamp);
  }

  updateOutgoingLayers(timestamp) {
    if (!this.outgoingLayers?.length) return;
    this.outgoingLayers = this.outgoingLayers.filter((transition) => {
      const progress = Math.min(1, (timestamp - transition.startedAt) / transition.duration);
      transition.uniforms.uTime.value = timestamp / 1000;
      transition.uniforms.uOpacity.value = 1 - progress * progress;
      transition.uniforms.uDispersion.value = 0.7 + progress * 1.1;
      transition.group.position.z = -progress * 0.18;
      if (progress < 1) return true;
      transition.core.material.dispose();
      transition.halo.material.dispose();
      transition.geometry.dispose();
      transition.group.removeFromParent();
      return false;
    });
  }

  monitorPerformance(frameMs, timestamp) {
    if (this.reducedMotion || timestamp - this.transitionStartedAt < 2500) return;
    this.frameSamples.push(frameMs);
    if (this.frameSamples.length > 120) this.frameSamples.shift();
    if (this.frameSamples.length < 90) return;
    const sorted = [...this.frameSamples].sort((a, b) => a - b);
    const trimmed = sorted.slice(5, -5);
    const average = trimmed.reduce((sum, value) => sum + value, 0) / trimmed.length;
    this.fps = 1000 / average;
    if (average > 22.5) this.slowWindows += 1;
    else this.slowWindows = Math.max(0, this.slowWindows - 1);
    this.frameSamples.length = 0;
    if (this.slowWindows >= 3 && timestamp - this.lastQualityChange > 4500) {
      this.slowWindows = 0;
      this.degradeQuality();
      this.lastQualityChange = timestamp;
    }
    this.updateDebugPanel();
  }

  degradeQuality() {
    if (this.qualityLevel >= 4) return;
    this.qualityLevel += 1;
    if (this.qualityLevel === 1) this.dprScale = 0.78;
    else if (this.qualityLevel === 2) {
      this.haloEnabled = false;
      this.uniforms.uHaloAlpha.value = 0.12;
      this.uniforms.uHaloScale.value = 0.78;
    } else if (this.qualityLevel === 3) {
      this.detailScale = 0;
    } else if (this.qualityLevel === 4 && this.lastImageData) {
      const nextStep = Math.min(10, this.step + 2);
      const attributes = buildParticleAttributes(this.lastImageData, { step: nextStep, worldHeight: 4 });
      this.installGeometry(attributes, { transition: false });
      this.container.dataset.particleCount = String(attributes.count);
      this.container.dataset.particleStep = String(attributes.step);
    }
    this.qualityLabel = ["high", "balanced", "core", "low-detail", "reduced-density"][this.qualityLevel];
    this.applyQualitySettings();
    this.resize();
  }

  applyQualitySettings() {
    if (!this.uniforms) return;
    this.uniforms.uDetailScale.value = this.detailScale;
    this.uniforms.uHaloScale.value = this.haloEnabled ? 0.94 : 0.68;
    this.uniforms.uHaloAlpha.value = this.haloEnabled ? 0.18 : 0.1;
    if (this.haloPoints) this.haloPoints.visible = this.haloEnabled;
    this.updateDebugPanel();
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
    reset.onclick = () => this.setPreset("still");
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
    this.debugCount.textContent = `${Math.round(this.fps || 0)} FPS · DPR ${(this.currentDpr || 1).toFixed(2)} · ${this.qualityLabel} · Core on / Halo ${this.haloEnabled ? "on" : "off"} · ${this.particleCount.toLocaleString()} particles · step ${this.step}`;
  }

  dispose() {
    if (this.disposed) return;
    this.disposed = true;
    this.loadVersion += 1;
    this.loadController?.abort();
    this.cancelFrame();
    document.removeEventListener("visibilitychange", this.onVisibility);
    this.reducedMotionQuery.removeEventListener?.("change", this.onReducedMotion);
    this.resizeObserver?.disconnect();
    this.intersectionObserver?.disconnect();
    this.container.removeEventListener("pointermove", this.onPointerMove);
    this.container.removeEventListener("pointerenter", this.onPointerEnter);
    this.container.removeEventListener("pointerleave", this.onPointerLeave);
    this.container.removeEventListener("pointerdown", this.onPointerDown);
    this.container.removeEventListener("pointerup", this.onPointerUp);
    this.container.removeEventListener("pointercancel", this.onPointerUp);
    this.releaseRenderResources();
    this.debugPanel?.remove();
    this.container.classList.remove("is-ready", "is-loading");
    delete this.container.dataset.particleCount;
    delete this.container.dataset.particleStep;
    this.lastImageData = null;
    this.dispatchActivity();
  }
}

export { PRESETS as SCENE_PARTICLE_PRESETS, vertexShader, fragmentShader };
