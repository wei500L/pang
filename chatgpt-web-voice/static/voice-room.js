const root = document.documentElement;
const body = document.body;
const reducedMotion = window.matchMedia("(prefers-reduced-motion: reduce)");
const coarsePointer = window.matchMedia("(pointer: coarse)");

const memory = Number(navigator.deviceMemory || 4);
const cores = Number(navigator.hardwareConcurrency || 4);
const narrow = window.matchMedia("(max-width: 700px)").matches;
const lowQuality = reducedMotion.matches || narrow || memory <= 2 || cores <= 4;
root.dataset.roomQuality = lowQuality ? "low" : memory >= 8 && cores >= 8 ? "high" : "medium";

let pointerX = 0;
let pointerY = 0;
let targetX = 0;
let targetY = 0;
let raf = 0;
let disposed = false;

function applyParallax() {
  raf = 0;
  if (disposed) return;
  pointerX += (targetX - pointerX) * 0.065;
  pointerY += (targetY - pointerY) * 0.065;
  const nearlyStill = Math.abs(targetX - pointerX) < 0.008 && Math.abs(targetY - pointerY) < 0.008;

  root.style.setProperty("--room-parallax-x", `${(pointerX * 13).toFixed(2)}px`);
  root.style.setProperty("--room-parallax-y", `${(pointerY * 9.5).toFixed(2)}px`);
  root.style.setProperty("--room-parallax-soft-x", `${(pointerX * 4.6).toFixed(2)}px`);
  root.style.setProperty("--room-parallax-soft-y", `${(pointerY * 3.4).toFixed(2)}px`);
  root.style.setProperty("--room-parallax-shadow-x", `${(pointerX * -5.6).toFixed(2)}px`);
  root.style.setProperty("--room-parallax-shadow-y", `${(pointerY * -4).toFixed(2)}px`);
  root.style.setProperty("--room-parallax-fore-x", `${(pointerX * 10).toFixed(2)}px`);
  root.style.setProperty("--room-parallax-fore-y", `${(pointerY * 7.2).toFixed(2)}px`);
  root.style.setProperty("--room-parallax-particle-x", `${(pointerX * 3).toFixed(2)}px`);
  root.style.setProperty("--room-parallax-particle-y", `${(pointerY * 2.2).toFixed(2)}px`);

  if (!nearlyStill) raf = requestAnimationFrame(applyParallax);
}

function queueParallax() {
  if (!raf) raf = requestAnimationFrame(applyParallax);
}

function onPointerMove(event) {
  if (reducedMotion.matches || coarsePointer.matches || document.hidden) return;
  targetX = (event.clientX / Math.max(1, window.innerWidth) - 0.5) * 2;
  targetY = (event.clientY / Math.max(1, window.innerHeight) - 0.5) * 2;
  queueParallax();
}

function onPointerLeave() {
  targetX = 0;
  targetY = 0;
  queueParallax();
}

function syncMuteState() {
  const muteButton = document.getElementById("btnMute");
  const muted = Boolean(muteButton && muteButton.classList.contains("is-muted"));
  body?.toggleAttribute("data-mic-muted", muted);
}

const muteButton = document.getElementById("btnMute");
const muteObserver = muteButton
  ? new MutationObserver(syncMuteState)
  : null;
muteObserver?.observe(muteButton, { attributes: true, attributeFilter: ["class", "aria-pressed"] });
syncMuteState();

window.addEventListener("pointermove", onPointerMove, { passive: true });
window.addEventListener("pointerleave", onPointerLeave, { passive: true });

window.addEventListener("pagehide", () => {
  disposed = true;
  if (raf) cancelAnimationFrame(raf);
  muteObserver?.disconnect();
  window.removeEventListener("pointermove", onPointerMove);
  window.removeEventListener("pointerleave", onPointerLeave);
}, { once: true });
