const clamp01 = (value) => Math.max(0, Math.min(1, value));

function smooth(current, target, dt, attack, release) {
  const time = target > current ? attack : release;
  const amount = 1 - Math.exp(-Math.max(0, dt) / Math.max(0.001, time));
  return current + (target - current) * amount;
}

function bandEnergy(data, sampleRate, fftSize, lowHz, highHz) {
  const nyquist = sampleRate * 0.5;
  const start = Math.max(0, Math.floor((lowHz / nyquist) * data.length));
  const end = Math.min(data.length, Math.ceil((highHz / nyquist) * data.length));
  if (end <= start) return 0;
  let sum = 0;
  for (let i = start; i < end; i += 1) {
    const value = data[i] / 255;
    sum += value * value;
  }
  return Math.sqrt(sum / (end - start));
}

export class AudioFeatureExtractor {
  constructor() {
    this.reset();
  }

  reset() {
    this.energy = 0;
    this.attack = 0;
    this.low = 0;
    this.mid = 0;
    this.high = 0;
    this.flux = 0;
    this.noiseFloor = 0.008;
    this.speechActive = false;
    this.speechHoldUntil = 0;
    this.lastOnsetAt = -Infinity;
    this.lastTimestamp = 0;
    this.previousSpectrum = null;
    this.previousTargetEnergy = 0;
    return this.snapshot(false);
  }

  process(timeData, frequencyData, sampleRate, timestamp) {
    const parsedTimestamp = Number(timestamp);
    const now = Number.isFinite(parsedTimestamp) ? parsedTimestamp : performance.now();
    const dt = this.lastTimestamp ? Math.min(0.1, Math.max(1 / 240, (now - this.lastTimestamp) / 1000)) : 1 / 60;
    this.lastTimestamp = now;

    let sum = 0;
    for (let i = 0; i < timeData.length; i += 1) {
      const value = (timeData[i] - 128) / 128;
      sum += value * value;
    }
    const rms = Math.sqrt(sum / Math.max(1, timeData.length));

    if (!this.speechActive || rms < this.noiseFloor * 2.4) {
      const floorRate = rms < this.noiseFloor ? 1.5 : 0.22;
      this.noiseFloor += (rms - this.noiseFloor) * Math.min(1, dt * floorRate);
      this.noiseFloor = Math.max(0.002, Math.min(0.08, this.noiseFloor));
    }

    const activeFloor = this.noiseFloor * 1.25 + 0.0035;
    const range = Math.max(0.035, this.noiseFloor * 4.5);
    const rawEnergy = clamp01((rms - activeFloor) / range);
    const targetEnergy = Math.pow(rawEnergy, 0.72);

    const fftSize = Math.max(2, frequencyData.length * 2);
    const lowRaw = bandEnergy(frequencyData, sampleRate, fftSize, 80, 280);
    const midRaw = bandEnergy(frequencyData, sampleRate, fftSize, 280, 2200);
    const highRaw = bandEnergy(frequencyData, sampleRate, fftSize, 2200, Math.min(7200, sampleRate * 0.45));
    const spectralGate = 0.025 + this.noiseFloor * 0.65;
    const gateMix = 0.2 + targetEnergy * 0.8;
    const lowTarget = clamp01((lowRaw - spectralGate) * 3.1) * gateMix;
    const midTarget = clamp01((midRaw - spectralGate) * 2.75) * gateMix;
    const highTarget = clamp01((highRaw - spectralGate) * 3.4) * gateMix;

    if (!this.previousSpectrum || this.previousSpectrum.length !== frequencyData.length) {
      this.previousSpectrum = new Float32Array(frequencyData.length);
    }
    let positiveDelta = 0;
    for (let i = 0; i < frequencyData.length; i += 1) {
      const current = frequencyData[i] / 255;
      positiveDelta += Math.max(0, current - this.previousSpectrum[i]);
      this.previousSpectrum[i] = current;
    }
    const fluxTarget = clamp01((positiveDelta / Math.max(1, frequencyData.length)) * 8.5) * gateMix;
    const derivative = Math.max(0, (targetEnergy - this.previousTargetEnergy) / dt);
    const attackTarget = clamp01(derivative * 0.12 + fluxTarget * 0.9);
    this.previousTargetEnergy = targetEnergy;

    const wasSpeaking = this.speechActive;
    if (targetEnergy > 0.12 || attackTarget > 0.2) {
      this.speechActive = true;
      this.speechHoldUntil = now + 260;
    } else if (targetEnergy < 0.052 && now > this.speechHoldUntil) {
      this.speechActive = false;
    }

    const onsetCandidate = (!wasSpeaking && this.speechActive) || attackTarget > 0.34;
    const onset = onsetCandidate && now - this.lastOnsetAt >= 140;
    if (onset) this.lastOnsetAt = now;

    this.energy = smooth(this.energy, targetEnergy, dt, 0.035, 0.42);
    this.attack = smooth(this.attack, attackTarget, dt, 0.018, 0.24);
    this.low = smooth(this.low, lowTarget, dt, 0.065, 0.38);
    this.mid = smooth(this.mid, midTarget, dt, 0.045, 0.31);
    this.high = smooth(this.high, highTarget, dt, 0.025, 0.22);
    this.flux = smooth(this.flux, fluxTarget, dt, 0.025, 0.2);
    return this.snapshot(onset);
  }

  snapshot(onset) {
    return {
      energy: this.energy,
      attack: this.attack,
      low: this.low,
      mid: this.mid,
      high: this.high,
      spectralFlux: this.flux,
      speechActive: this.speechActive,
      onset: Boolean(onset),
      noiseFloor: this.noiseFloor,
    };
  }
}

export { bandEnergy, clamp01 };
