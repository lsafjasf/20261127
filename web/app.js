(() => {
  "use strict";

  const MAX_DIMENSION = 1240;
  const CLIP_LEVEL = 250;
  const NEAR_CLIP_LEVEL = 240;
  const OVEREXPOSURE_EV = 2.0;
  const SCALE_MIN = -5.5;
  const SCALE_MAX = 5.5;

  const BANDS = [
    { min: -Infinity, max: -4.5, name: "深阴影", label: "≤−5", color: "#321253" },
    { min: -4.5, max: -3.5, name: "极暗", label: "−4", color: "#472378" },
    { min: -3.5, max: -2.5, name: "暗部", label: "−3", color: "#24499d" },
    { min: -2.5, max: -1.5, name: "阴影", label: "−2", color: "#1464a2" },
    { min: -1.5, max: -0.5, name: "低中间调", label: "−1", color: "#087c8d" },
    { min: -0.5, max: 0.5, name: "参考灰", label: "0", color: "#16906d" },
    { min: 0.5, max: 1.5, name: "亮中间调", label: "+1", color: "#87b62e" },
    { min: 1.5, max: 2.5, name: "高光", label: "+2", color: "#d8c22d" },
    { min: 2.5, max: 3.5, name: "强高光", label: "+3", color: "#eb812c" },
    { min: 3.5, max: 4.5, name: "近裁切", label: "+4", color: "#d83b41" },
    { min: 4.5, max: Infinity, name: "裁切", label: "≥+5", color: "#f02d77" },
  ];

  const BAND_RGB = BANDS.map((band) => {
    const value = Number.parseInt(band.color.slice(1), 16);
    return [(value >> 16) & 255, (value >> 8) & 255, value & 255];
  });

  const dom = {
    viewport: document.querySelector("#viewport"),
    canvasFrame: document.querySelector("#canvasFrame"),
    canvas: document.querySelector("#imageCanvas"),
    overlayLayer: document.querySelector("#overlayLayer"),
    referenceMarker: document.querySelector("#referenceMarker"),
    markerEV: document.querySelector("#markerEV"),
    regionLabels: document.querySelector("#regionLabels"),
    probeChip: document.querySelector("#probeChip"),
    loadingLayer: document.querySelector("#loadingLayer"),
    sceneDescription: document.querySelector("#sceneDescription"),
    viewBadge: document.querySelector("#viewBadge"),
    fileInput: document.querySelector("#fileInput"),
    eyedropperButton: document.querySelector("#eyedropperButton"),
    eyedropperLabel: document.querySelector("#eyedropperLabel"),
    resetButton: document.querySelector("#resetButton"),
    referenceCard: document.querySelector(".reference-card"),
    referenceOffset: document.querySelector("#referenceOffset"),
    offsetOutput: document.querySelector("#offsetOutput"),
    referenceHint: document.querySelector("#referenceHint"),
    referenceSwatch: document.querySelector("#referenceSwatch"),
    referenceLuma: document.querySelector("#referenceLuma"),
    referencePercent: document.querySelector("#referencePercent"),
    referencePointLabel: document.querySelector("#referencePointLabel"),
    probeCoord: document.querySelector("#probeCoord"),
    probeEV: document.querySelector("#probeEV"),
    probeZone: document.querySelector("#probeZone"),
    probeR: document.querySelector("#probeR"),
    probeG: document.querySelector("#probeG"),
    probeB: document.querySelector("#probeB"),
    probeLuma: document.querySelector("#probeLuma"),
    probeState: document.querySelector("#probeState"),
    clipChannelText: document.querySelector("#clipChannelText"),
    clipChannels: [...document.querySelectorAll("#clipChannels > span")],
    hudClip: document.querySelector("#hudClip"),
    hudOver: document.querySelector("#hudOver"),
    hudSpecular: document.querySelector("#hudSpecular"),
    totalClip: document.querySelector("#totalClip"),
    clipPctR: document.querySelector("#clipPctR"),
    clipPctG: document.querySelector("#clipPctG"),
    clipPctB: document.querySelector("#clipPctB"),
    clipBarR: document.querySelector("#clipBarR"),
    clipBarG: document.querySelector("#clipBarG"),
    clipBarB: document.querySelector("#clipBarB"),
    scaleBar: document.querySelector("#scaleBar"),
    scaleTicks: document.querySelector("#scaleTicks"),
    scalePointer: document.querySelector("#scalePointer"),
    scalePointerDot: document.querySelector("#scalePointerDot"),
    scalePointerLabel: document.querySelector("#scalePointerLabel"),
    recalcBadge: document.querySelector(".recalc-badge"),
    recalcText: document.querySelector("#recalcText"),
    imageStatus: document.querySelector("#imageStatus"),
  };

  const state = {
    view: "false",
    overlays: { reference: true, clip: true, overexposure: true },
    picking: false,
    offset: 0,
    originalWidth: 0,
    originalHeight: 0,
    width: 0,
    height: 0,
    sourceCanvas: document.createElement("canvas"),
    sourceData: null,
    linearR: null,
    linearG: null,
    linearB: null,
    luma: null,
    refLuma: 0.18,
    reference: { x: 0.5, y: 0.5, label: "参考点" },
    defaultReference: null,
    referenceRGB: [118, 118, 118],
    analysis: null,
    probe: null,
    pointerInside: false,
    frameWidth: 0,
    frameHeight: 0,
    renderQueued: false,
    loading: false,
  };

  const ctx = dom.canvas.getContext("2d", { willReadFrequently: true });
  const offscreen = document.createElement("canvas");
  const offscreenCtx = offscreen.getContext("2d", { willReadFrequently: true });
  let candidateMask = null;
  let regionIds = null;
  let regionTypes = null;
  let bfsStack = null;

  function clamp(value, min, max) {
    return Math.max(min, Math.min(max, value));
  }

  function srgbToLinear(value) {
    const normalized = value / 255;
    return normalized <= 0.04045
      ? normalized / 12.92
      : Math.pow((normalized + 0.055) / 1.055, 2.4);
  }

  function formatEV(value, digits = 2) {
    if (!Number.isFinite(value)) return "—";
    const normalized = Math.abs(value) < 0.5 * Math.pow(10, -digits) ? 0 : value;
    const sign = normalized >= 0 ? "+" : "−";
    return `${sign}${Math.abs(normalized).toFixed(digits)}`;
  }

  function sampleNormalizedPoint(nx, ny) {
    if (!state.sourceData) return;
    const width = state.width;
    const height = state.height;
    const cx = clamp(Math.round(nx * (width - 1)), 0, width - 1);
    const cy = clamp(Math.round(ny * (height - 1)), 0, height - 1);
    const radius = 2;
    let r = 0;
    let g = 0;
    let b = 0;
    let linear = 0;
    let count = 0;

    for (let y = Math.max(0, cy - radius); y <= Math.min(height - 1, cy + radius); y += 1) {
      for (let x = Math.max(0, cx - radius); x <= Math.min(width - 1, cx + radius); x += 1) {
        const index = y * width + x;
        const pixel = index * 4;
        r += state.sourceData.data[pixel];
        g += state.sourceData.data[pixel + 1];
        b += state.sourceData.data[pixel + 2];
        linear += state.luma[index];
        count += 1;
      }
    }

    state.refLuma = Math.max(linear / count, 0.0001);
    state.referenceRGB = [r / count, g / count, b / count].map((value) => Math.round(value));
  }

  function buildReferenceState() {
    sampleNormalizedPoint(state.reference.x, state.reference.y);
    const [r, g, b] = state.referenceRGB;
    dom.referenceSwatch.style.background = `rgb(${r}, ${g}, ${b})`;
    dom.referenceLuma.textContent = `L ${(state.refLuma * 100).toFixed(1)}%`;
    dom.referencePercent.textContent = `${Math.round(state.refLuma * 100)}%`;
    dom.referencePointLabel.textContent = state.reference.label;
    dom.referenceMarker.style.left = `${state.reference.x * 100}%`;
    dom.referenceMarker.style.top = `${state.reference.y * 100}%`;
    dom.markerEV.textContent = `EV ${formatEV(state.offset, 1)}`;
    dom.referenceMarker.classList.toggle("is-hidden", !state.overlays.reference);
  }

  function setOffset(value) {
    state.offset = roundToTenth(clamp(Number(value) || 0, -3, 3));
    dom.referenceOffset.value = String(state.offset);
    dom.offsetOutput.textContent = `${formatEV(state.offset, 1)} EV`;
    dom.markerEV.textContent = `EV ${formatEV(state.offset, 1)}`;
    scheduleRender();
  }

  function roundToTenth(value) {
    return Math.round(value * 10) / 10;
  }

  function buildScale() {
    dom.scaleBar.replaceChildren();
    BANDS.forEach((band) => {
      const item = document.createElement("span");
      item.className = "scale-band";
      item.style.background = band.color;
      item.title = `${band.name} · ${band.label} EV`;
      item.dataset.band = band.name;
      dom.scaleBar.append(item);
    });

    dom.scaleTicks.replaceChildren();
    for (let ev = -4; ev <= 4; ev += 1) {
      const tick = document.createElement("span");
      tick.className = "scale-tick";
      tick.textContent = ev > 0 ? `+${ev}` : `${ev}`;
      tick.style.left = `${((ev - SCALE_MIN) / (SCALE_MAX - SCALE_MIN)) * 100}%`;
      dom.scaleTicks.append(tick);
    }
  }

  async function loadScene() {
    setLoading(true);
    try {
      const response = await fetch("/api/scene", { cache: "no-store" });
      if (!response.ok) throw new Error(`scene ${response.status}`);
      const info = await response.json();
      state.defaultReference = {
        x: info.reference?.x ?? 0.5,
        y: info.reference?.y ?? 0.5,
        label: info.reference?.label ?? "场景参考点",
      };
      if (info.description) dom.sceneDescription.textContent = info.description;
      const image = new Image();
      image.src = info.imageUrl;
      await image.decode();
      await useImage(image, {
        width: info.width || image.naturalWidth,
        height: info.height || image.naturalHeight,
        name: "棚拍样片",
      });
    } catch (error) {
      console.error(error);
      dom.sceneDescription.textContent = "样片载入失败";
      dom.referenceHint.textContent = "无法读取内置样片，请重新启动本地服务。";
    } finally {
      setLoading(false);
    }
  }

  async function useImage(image, meta = {}) {
    setLoading(true);
    const originalWidth = meta.width || image.naturalWidth || image.width;
    const originalHeight = meta.height || image.naturalHeight || image.height;
    const scale = Math.min(1, MAX_DIMENSION / Math.max(originalWidth, originalHeight));
    const width = Math.max(2, Math.round(originalWidth * scale));
    const height = Math.max(2, Math.round(originalHeight * scale));

    state.originalWidth = originalWidth;
    state.originalHeight = originalHeight;
    state.width = width;
    state.height = height;
    state.sourceCanvas.width = width;
    state.sourceCanvas.height = height;
    const sourceContext = state.sourceCanvas.getContext("2d", { willReadFrequently: true });
    sourceContext.imageSmoothingEnabled = true;
    sourceContext.imageSmoothingQuality = "high";
    sourceContext.clearRect(0, 0, width, height);
    sourceContext.drawImage(image, 0, 0, width, height);
    state.sourceData = sourceContext.getImageData(0, 0, width, height);

    dom.canvas.width = width;
    dom.canvas.height = height;
    dom.canvasFrame.style.setProperty("--scene-aspect", String(width / height));
    offscreen.width = width;
    offscreen.height = height;

    buildLinearBuffers();
    const reference = state.defaultReference || { x: 0.5, y: 0.5, label: "画面中心参考 · 5×5 平均" };
    state.reference = { ...reference };
    state.offset = 0;
    dom.referenceOffset.value = "0";
    dom.offsetOutput.textContent = "0.0 EV";
    state.probe = {
      x: Math.round(state.reference.x * (width - 1)),
      y: Math.round(state.reference.y * (height - 1)),
    };
    buildReferenceState();
    fitFrame();
    analyzeAndRender();
    updateProbe(state.probe.x, state.probe.y);
    const imageName = meta.name || "已载入照片";
    dom.sceneDescription.textContent = meta.description || `${imageName} · 本地分析`;
    dom.imageStatus.textContent = `${imageName} ${originalWidth} × ${originalHeight}`;
    dom.referenceHint.textContent = state.defaultReference
      ? "参考灰锚定 18% 灰卡；拖动曝光参考偏移可即时重算。"
      : "点击画面中的中性区域，重新锚定参考灰。";
    setLoading(false);
  }

  function buildLinearBuffers() {
    const length = state.width * state.height;
    const data = state.sourceData.data;
    state.linearR = new Float32Array(length);
    state.linearG = new Float32Array(length);
    state.linearB = new Float32Array(length);
    state.luma = new Float32Array(length);
    for (let index = 0; index < length; index += 1) {
      const pixel = index * 4;
      const r = srgbToLinear(data[pixel]);
      const g = srgbToLinear(data[pixel + 1]);
      const b = srgbToLinear(data[pixel + 2]);
      state.linearR[index] = r;
      state.linearG[index] = g;
      state.linearB[index] = b;
      state.luma[index] = r * 0.2126 + g * 0.7152 + b * 0.0722;
    }
    candidateMask = new Uint8Array(length);
    regionIds = new Int32Array(length);
    regionTypes = new Uint8Array(4096);
    bfsStack = new Int32Array(length);
  }

  function analyzeScene() {
    const width = state.width;
    const height = state.height;
    const length = width * height;
    const raw = state.sourceData.data;
    const refLuma = Math.max(state.refLuma, 0.000001);
    const offset = state.offset;
    const clipCounts = [0, 0, 0];
    const evBuffer = new Float32Array(length);
    let anyClipPixels = 0;
    let allClipPixels = 0;
    let localAreas = 0;
    let specularAreas = 0;

    candidateMask.fill(0);
    regionIds.fill(0);

    for (let index = 0; index < length; index += 1) {
      const pixel = index * 4;
      const r = raw[pixel];
      const g = raw[pixel + 1];
      const b = raw[pixel + 2];
      const cr = r >= CLIP_LEVEL;
      const cg = g >= CLIP_LEVEL;
      const cb = b >= CLIP_LEVEL;
      if (cr) clipCounts[0] += 1;
      if (cg) clipCounts[1] += 1;
      if (cb) clipCounts[2] += 1;
      if (cr || cg || cb) anyClipPixels += 1;
      if (cr && cg && cb) allClipPixels += 1;

      const ev = Math.log2(Math.max(state.luma[index], 1e-8) / refLuma) + offset;
      evBuffer[index] = ev;
      const maxChannel = Math.max(r, g, b);
      if (ev >= OVEREXPOSURE_EV && maxChannel >= NEAR_CLIP_LEVEL) {
        candidateMask[index] = 1;
      }
    }

    let regionCount = 0;
    const regions = [];
    for (let seed = 0; seed < length; seed += 1) {
      if (!candidateMask[seed]) continue;
      regionCount += 1;
      const id = regionCount;
      let stackTop = 0;
      bfsStack[stackTop++] = seed;
      candidateMask[seed] = 0;
      regionIds[seed] = id;
      let area = 0;
      let minX = width;
      let maxX = 0;
      let minY = height;
      let maxY = 0;
      let sumX = 0;
      let sumY = 0;
      let peakEV = -Infinity;
      let clipAny = 0;
      let clipAll = 0;

      while (stackTop > 0) {
        const current = bfsStack[--stackTop];
        const x = current % width;
        const y = (current / width) | 0;
        area += 1;
        sumX += x;
        sumY += y;
        if (x < minX) minX = x;
        if (x > maxX) maxX = x;
        if (y < minY) minY = y;
        if (y > maxY) maxY = y;
        const ev = evBuffer[current];
        if (ev > peakEV) peakEV = ev;
        const pixel = current * 4;
        const cr = raw[pixel] >= CLIP_LEVEL;
        const cg = raw[pixel + 1] >= CLIP_LEVEL;
        const cb = raw[pixel + 2] >= CLIP_LEVEL;
        if (cr || cg || cb) clipAny += 1;
        if (cr && cg && cb) clipAll += 1;

        if (x > 0) {
          const next = current - 1;
          if (candidateMask[next]) {
            candidateMask[next] = 0;
            regionIds[next] = id;
            bfsStack[stackTop++] = next;
          }
        }
        if (x + 1 < width) {
          const next = current + 1;
          if (candidateMask[next]) {
            candidateMask[next] = 0;
            regionIds[next] = id;
            bfsStack[stackTop++] = next;
          }
        }
        if (y > 0) {
          const next = current - width;
          if (candidateMask[next]) {
            candidateMask[next] = 0;
            regionIds[next] = id;
            bfsStack[stackTop++] = next;
          }
        }
        if (y + 1 < height) {
          const next = current + width;
          if (candidateMask[next]) {
            candidateMask[next] = 0;
            regionIds[next] = id;
            bfsStack[stackTop++] = next;
          }
        }
      }

      const boxWidth = maxX - minX + 1;
      const boxHeight = maxY - minY + 1;
      const fillRatio = area / (boxWidth * boxHeight);
      const isLocal = area >= 300 && boxWidth >= 30 && boxHeight >= 30 && fillRatio >= 0.55;
      if (isLocal) localAreas += 1;
      else specularAreas += 1;
      regions[id] = {
        id,
        area,
        minX,
        minY,
        maxX,
        maxY,
        boxWidth,
        boxHeight,
        centroidX: sumX / area,
        centroidY: sumY / area,
        peakEV,
        clipRatio: clipAny / area,
        allClipRatio: clipAll / area,
        type: isLocal ? 1 : 2,
      };
    }

    if (regionTypes.length < regions.length + 1) {
      regionTypes = new Uint8Array(regions.length + 1);
    } else {
      regionTypes.fill(0);
    }
    for (let id = 1; id < regions.length; id += 1) {
      const region = regions[id];
      if (region) regionTypes[id] = region.type;
    }

    return {
      regions,
      clipCounts,
      anyClipPixels,
      allClipPixels,
      localAreas,
      specularAreas,
      totalPixels: length,
      ev: evBuffer,
      localLabels: regions
        .filter((region) => region && region.type === 1 && region.area >= 900)
        .sort((a, b) => b.area - a.area)
        .slice(0, 4),
    };
  }

  function fastBandIndex(ev) {
    if (ev < -4.5) return 0;
    if (ev < -3.5) return 1;
    if (ev < -2.5) return 2;
    if (ev < -1.5) return 3;
    if (ev < -0.5) return 4;
    if (ev < 0.5) return 5;
    if (ev < 1.5) return 6;
    if (ev < 2.5) return 7;
    if (ev < 3.5) return 8;
    if (ev < 4.5) return 9;
    return 10;
  }

  function renderNow() {
    if (!state.sourceData) return;
    const started = performance.now();
    state.analysis = analyzeScene();
    renderFrame(state.analysis);
    renderRegionLabels(state.analysis);
    updateStats(state.analysis);
    updateProbeFromState();
    const elapsed = Math.max(1, Math.round(performance.now() - started));
    dom.recalcText.textContent = `重算 ${elapsed} ms`;
    dom.recalcBadge.classList.remove("is-busy");
  }

  function scheduleRender() {
    if (state.renderQueued) return;
    state.renderQueued = true;
    dom.recalcBadge.classList.add("is-busy");
    dom.recalcText.textContent = "重算中";
    requestAnimationFrame(() => {
      state.renderQueued = false;
      renderNow();
    });
  }

  function analyzeAndRender() {
    renderNow();
  }

  function renderFrame(analysis) {
    const width = state.width;
    const height = state.height;
    const raw = state.sourceData.data;
    const output = new ImageData(width, height);
    const pixels = output.data;
    const showClip = state.overlays.clip;
    const showOverexposure = state.overlays.overexposure;
    const evBuffer = analysis.ev;
    const regions = analysis.regions;

    for (let y = 0; y < height; y += 1) {
      for (let x = 0; x < width; x += 1) {
        const index = y * width + x;
        const pixel = index * 4;
        const band = fastBandIndex(evBuffer[index]);
        const base = BAND_RGB[band];
        let outR = base[0];
        let outG = base[1];
        let outB = base[2];

        const r = raw[pixel];
        const g = raw[pixel + 1];
        const b = raw[pixel + 2];
        const cr = r >= CLIP_LEVEL;
        const cg = g >= CLIP_LEVEL;
        const cb = b >= CLIP_LEVEL;
        const clipped = cr || cg || cb;
        const clipPattern = (cr ? 1 : 0) | (cg ? 2 : 0) | (cb ? 4 : 0);
        const regionId = regionIds[index];
        const regionType = regionId ? regionTypes[regionId] : 0;

        if (showOverexposure && regionType === 1) {
          const edge = hasNeighborOfType(x, y, width, height, 1);
          if (edge) {
            outR = 255;
            outG = 226;
            outB = 174;
          } else if (showClip && clipped && hasDifferentClipNeighbor(x, y, width, height)) {
            [outR, outG, outB] = clipOverlayColor(clipPattern);
          } else if (((x + y) & 7) < 2) {
            outR = Math.round(outR * 0.35 + 255 * 0.65);
            outG = Math.round(outG * 0.35 + 146 * 0.65);
            outB = Math.round(outB * 0.35 + 52 * 0.65);
          } else {
            outR = Math.round(outR * 0.74);
            outG = Math.round(outG * 0.62);
            outB = Math.round(outB * 0.50);
          }
        } else if (showOverexposure && regionType === 2) {
          const edge = hasNeighborOfType(x, y, width, height, 2);
          if (edge) {
            outR = 98;
            outG = 255;
            outB = 229;
          } else {
            outR = 247;
            outG = 255;
            outB = 252;
          }
        } else if (showClip && clipped) {
          if (((x + y) & 3) === 0) {
            [outR, outG, outB] = clipOverlayColor(clipPattern);
          } else {
            outR = Math.round(outR * 0.68 + 255 * 0.32);
            outG = Math.round(outG * 0.68 + 255 * 0.32);
            outB = Math.round(outB * 0.68 + 255 * 0.32);
          }
        }

        pixels[pixel] = outR;
        pixels[pixel + 1] = outG;
        pixels[pixel + 2] = outB;
        pixels[pixel + 3] = 255;
      }
    }

    ctx.clearRect(0, 0, width, height);
    ctx.imageSmoothingEnabled = false;
    ctx.putImageData(output, 0, 0);

    if (state.view === "original") {
      ctx.drawImage(state.sourceCanvas, 0, 0);
    } else if (state.view === "split") {
      ctx.save();
      ctx.beginPath();
      ctx.rect(0, 0, Math.floor(width / 2), height);
      ctx.clip();
      ctx.drawImage(state.sourceCanvas, 0, 0);
      ctx.restore();
    }

    dom.referenceMarker.classList.toggle("is-hidden", !state.overlays.reference);
  }

  function hasNeighborOfType(x, y, width, height, wantedType) {
    const left = x > 0 ? regionTypes[regionIds[y * width + x - 1]] : 0;
    const right = x + 1 < width ? regionTypes[regionIds[y * width + x + 1]] : 0;
    const up = y > 0 ? regionTypes[regionIds[(y - 1) * width + x]] : 0;
    const down = y + 1 < height ? regionTypes[regionIds[(y + 1) * width + x]] : 0;
    return left !== wantedType || right !== wantedType || up !== wantedType || down !== wantedType;
  }

  function hasDifferentClipNeighbor(x, y, width, height) {
    const raw = state.sourceData.data;
    const patternAt = (px, py) => {
      const pixel = (py * width + px) * 4;
      return (
        (raw[pixel] >= CLIP_LEVEL ? 1 : 0) |
        (raw[pixel + 1] >= CLIP_LEVEL ? 2 : 0) |
        (raw[pixel + 2] >= CLIP_LEVEL ? 4 : 0)
      );
    };
    const current = patternAt(x, y);
    if (x > 0 && patternAt(x - 1, y) !== current) return true;
    if (x + 1 < width && patternAt(x + 1, y) !== current) return true;
    if (y > 0 && patternAt(x, y - 1) !== current) return true;
    if (y + 1 < height && patternAt(x, y + 1) !== current) return true;
    return false;
  }

  function clipOverlayColor(pattern) {
    switch (pattern) {
      case 1: return [255, 83, 107];
      case 2: return [116, 242, 91];
      case 3: return [255, 230, 115];
      case 4: return [90, 181, 255];
      case 5: return [249, 116, 224];
      case 6: return [91, 245, 222];
      default: return [255, 250, 242];
    }
  }

  function renderRegionLabels(analysis) {
    dom.regionLabels.replaceChildren();
    if (!state.overlays.overexposure || state.view === "original") return;
    const placed = [];
    analysis.localLabels.forEach((region, index) => {
      const label = document.createElement("span");
      label.className = "region-label";
      const clipLabel = region.allClipRatio >= 0.45
        ? " · RGB"
        : region.clipRatio >= 0.10
          ? " · CLIP"
          : "";
      label.textContent =
        `过曝 ${String(index + 1).padStart(2, "0")} · ${formatEV(region.peakEV, 1)}${clipLabel}`;
      label.title =
        `局部过曝区 ${index + 1} · 峰值 ${formatEV(region.peakEV, 2)} EV` +
        (clipLabel ? " · 存在通道裁切" : "");
      label.style.left = `${clamp((region.centroidX / state.width) * 100, 6, 94)}%`;
      label.style.top = `${clamp((region.centroidY / state.height) * 100, 7, 93)}%`;
      dom.regionLabels.append(label);
      const bounds = label.getBoundingClientRect();
      const overlaps = placed.some((other) =>
        bounds.left < other.right + 4 &&
        bounds.right > other.left - 4 &&
        bounds.top < other.bottom + 3 &&
        bounds.bottom > other.top - 3
      );
      if (overlaps) {
        label.remove();
      } else {
        placed.push(bounds);
      }
    });
  }

  function updateStats(analysis) {
    const total = analysis.totalPixels;
    const clipPct = (analysis.anyClipPixels / total) * 100;
    dom.hudClip.textContent = `${clipPct.toFixed(1)}%`;
    dom.hudOver.textContent = `${analysis.localAreas} 处`;
    dom.hudSpecular.textContent = `${analysis.specularAreas} 点`;
    dom.totalClip.textContent = `${clipPct.toFixed(2)}%`;
    ["R", "G", "B"].forEach((channel, index) => {
      const pct = (analysis.clipCounts[index] / total) * 100;
      dom[`clipPct${channel}`].textContent = `${pct.toFixed(1)}%`;
      dom[`clipBar${channel}`].style.width = `${clamp(pct * 2.8, 0, 100)}%`;
    });
  }

  function updateProbeFromState() {
    if (!state.probe || !state.analysis) return;
    const x = clamp(Math.round(state.probe.x), 0, state.width - 1);
    const y = clamp(Math.round(state.probe.y), 0, state.height - 1);
    const index = y * state.width + x;
    const pixel = index * 4;
    const data = state.sourceData.data;
    const r = data[pixel];
    const g = data[pixel + 1];
    const b = data[pixel + 2];
    const ev = state.analysis.ev[index];
    const band = BANDS[fastBandIndex(ev)];
    const clipped = [r >= CLIP_LEVEL, g >= CLIP_LEVEL, b >= CLIP_LEVEL];
    const anyClip = clipped.some(Boolean);
    const allClip = clipped.every(Boolean);
    const regionId = regionIds[index];
    const regionType = regionId ? regionTypes[regionId] : 0;

    let probeState = band.name;
    if (regionType === 2) probeState = "镜面高光";
    else if (regionType === 1) probeState = "局部过曝";
    else if (allClip) probeState = "全通道裁切";
    else if (anyClip) probeState = "通道裁切";

    dom.probeCoord.textContent =
      `x ${Math.round((x / Math.max(1, state.width - 1)) * state.originalWidth)} · ` +
      `y ${Math.round((y / Math.max(1, state.height - 1)) * state.originalHeight)}`;
    dom.probeEV.textContent = formatEV(ev);
    dom.probeZone.textContent = probeState;
    dom.probeR.textContent = String(r);
    dom.probeG.textContent = String(g);
    dom.probeB.textContent = String(b);
    dom.probeLuma.textContent = `${(state.luma[index] * 100).toFixed(1)}%`;
    dom.probeState.textContent = probeState;
    dom.probeState.style.color = anyClip || regionType ? "#ffc26e" : "";

    dom.clipChannels.forEach((chip, channel) => {
      chip.classList.toggle("is-clipped", clipped[channel]);
    });
    if (allClip) dom.clipChannelText.textContent = "R G B 全通道裁切";
    else if (anyClip) {
      const names = ["R", "G", "B"].filter((_, channel) => clipped[channel]);
      dom.clipChannelText.textContent = `${names.join(" + ")} 通道裁切`;
    } else {
      dom.clipChannelText.textContent = "无通道裁切";
    }

    const scalePosition = ((clamp(ev, SCALE_MIN, SCALE_MAX) - SCALE_MIN) / (SCALE_MAX - SCALE_MIN)) * 100;
    dom.scalePointer.style.left = `${scalePosition}%`;
    dom.scalePointerDot.style.background = band.color;
    dom.scalePointerLabel.textContent = `指针 EV ${formatEV(ev)} · ${band.name}`;

    if (state.pointerInside && !dom.probeChip.hidden) {
      dom.probeChip.innerHTML =
        `<strong>EV ${formatEV(ev)}</strong><br>${probeState} · rgb(${r},${g},${b})`;
    }
  }

  function updateProbe(x, y) {
    state.probe = { x, y };
    updateProbeFromState();
  }

  function handleCanvasPointer(event) {
    if (!state.sourceData) return;
    const rect = dom.canvasFrame.getBoundingClientRect();
    const nx = clamp((event.clientX - rect.left) / rect.width, 0, 0.99999);
    const ny = clamp((event.clientY - rect.top) / rect.height, 0, 0.99999);
    const x = Math.floor(nx * state.width);
    const y = Math.floor(ny * state.height);
    state.pointerInside = true;
    dom.probeChip.hidden = false;
    dom.probeChip.style.left = `${nx * 100}%`;
    dom.probeChip.style.top = `${ny * 100}%`;
    dom.probeChip.style.transform =
      nx > 0.71 ? "translate(calc(-100% - 13px), -50%)" : "translate(13px, -50%)";
    updateProbe(x, y);
  }

  function handleCanvasClick(event) {
    if (!state.picking || !state.sourceData) return;
    const rect = dom.canvasFrame.getBoundingClientRect();
    const nx = clamp((event.clientX - rect.left) / rect.width, 0, 1);
    const ny = clamp((event.clientY - rect.top) / rect.height, 0, 1);
    state.reference = {
      x: nx,
      y: ny,
      label: "手动参考 · 5×5 平均",
    };
    state.offset = 0;
    dom.referenceOffset.value = "0";
    dom.offsetOutput.textContent = "0.0 EV";
    buildReferenceState();
    setPicking(false);
    dom.referenceHint.textContent = "已重新锚定参考灰；可继续调整曝光参考偏移。";
    scheduleRender();
  }

  function setPicking(enabled) {
    state.picking = enabled;
    dom.referenceCard.classList.toggle("is-picking", enabled);
    dom.eyedropperButton.classList.toggle("is-active", enabled);
    dom.eyedropperLabel.textContent = enabled ? "点击画面取样" : "拾取参考灰";
    dom.referenceHint.textContent = enabled
      ? "在画面中的中性灰区域单击；取 5×5 像素平均。"
      : "点击画面中的中性区域，重新锚定参考灰。";
  }

  function fitFrame() {
    if (!state.width || !state.height) return;
    const rect = dom.viewport.getBoundingClientRect();
    const style = getComputedStyle(dom.viewport);
    const paddingX = Number.parseFloat(style.paddingLeft) + Number.parseFloat(style.paddingRight);
    const paddingY = Number.parseFloat(style.paddingTop) + Number.parseFloat(style.paddingBottom);
    const availableWidth = Math.max(120, rect.width - paddingX);
    const availableHeight = Math.max(120, rect.height - paddingY);
    const aspect = state.width / state.height;
    let width = availableWidth;
    let height = width / aspect;
    if (height > availableHeight) {
      height = availableHeight;
      width = height * aspect;
    }
    state.frameWidth = Math.floor(width);
    state.frameHeight = Math.floor(height);
    dom.canvasFrame.style.width = `${state.frameWidth}px`;
    dom.canvasFrame.style.height = `${state.frameHeight}px`;
  }

  async function loadUploadedFile(file) {
    if (!file || !file.type.startsWith("image/")) return;
    setLoading(true);
    const url = URL.createObjectURL(file);
    try {
      const image = new Image();
      image.src = url;
      await image.decode();
      state.defaultReference = {
        x: 0.5,
        y: 0.5,
        label: "画面中心 · 待选参考灰",
      };
      await useImage(image, {
        width: image.naturalWidth,
        height: image.naturalHeight,
        name: file.name,
        description: `${file.name} · 本地分析`,
      });
    } catch (error) {
      console.error(error);
      dom.referenceHint.textContent = "这张图片无法解码，请尝试 PNG、JPEG 或 WebP。";
    } finally {
      URL.revokeObjectURL(url);
      setLoading(false);
    }
  }

  function setLoading(loading) {
    state.loading = loading;
    dom.loadingLayer.classList.toggle("is-visible", loading);
  }

  function bindEvents() {
    dom.referenceOffset.addEventListener("input", (event) => setOffset(event.target.value));

    document.querySelectorAll("[data-view]").forEach((button) => {
      button.addEventListener("click", () => {
        state.view = button.dataset.view;
        document.querySelectorAll("[data-view]").forEach((item) => {
          item.classList.toggle("is-active", item === button);
        });
        dom.canvasFrame.classList.toggle("is-split", state.view === "split");
        dom.viewBadge.textContent =
          state.view === "false" ? "FALSE COLOR" : state.view === "original" ? "ORIGINAL" : "SPLIT VIEW";
        scheduleRender();
      });
    });

    document.querySelectorAll("[data-overlay]").forEach((button) => {
      button.addEventListener("click", () => {
        const key = button.dataset.overlay;
        state.overlays[key] = !state.overlays[key];
        button.classList.toggle("is-on", state.overlays[key]);
        if (key === "reference") {
          dom.referenceMarker.classList.toggle("is-hidden", !state.overlays.reference);
        } else {
          scheduleRender();
        }
      });
    });

    dom.canvasFrame.addEventListener("pointermove", handleCanvasPointer);
    dom.canvasFrame.addEventListener("pointerleave", () => {
      state.pointerInside = false;
      dom.probeChip.hidden = true;
    });
    dom.canvasFrame.addEventListener("pointerdown", handleCanvasClick);

    dom.eyedropperButton.addEventListener("click", () => setPicking(!state.picking));
    dom.resetButton.addEventListener("click", () => {
      if (state.defaultReference) state.reference = { ...state.defaultReference };
      setPicking(false);
      state.offset = 0;
      dom.referenceOffset.value = "0";
      dom.offsetOutput.textContent = "0.0 EV";
      buildReferenceState();
      state.probe = {
        x: Math.round(state.reference.x * (state.width - 1)),
        y: Math.round(state.reference.y * (state.height - 1)),
      };
      dom.referenceHint.textContent = "已回到场景参考点。";
      scheduleRender();
    });

    dom.fileInput.addEventListener("change", (event) => {
      const [file] = event.target.files;
      if (file) loadUploadedFile(file);
      event.target.value = "";
    });

    let dragDepth = 0;
    dom.canvasFrame.addEventListener("dragenter", (event) => {
      event.preventDefault();
      dragDepth += 1;
      dom.canvasFrame.classList.add("is-dragging");
    });
    dom.canvasFrame.addEventListener("dragover", (event) => {
      event.preventDefault();
      event.dataTransfer.dropEffect = "copy";
    });
    dom.canvasFrame.addEventListener("dragleave", () => {
      dragDepth = Math.max(0, dragDepth - 1);
      if (!dragDepth) dom.canvasFrame.classList.remove("is-dragging");
    });
    dom.canvasFrame.addEventListener("drop", (event) => {
      event.preventDefault();
      dragDepth = 0;
      dom.canvasFrame.classList.remove("is-dragging");
      const [file] = event.dataTransfer.files;
      if (file) loadUploadedFile(file);
    });

    window.addEventListener("resize", fitFrame);
    if ("ResizeObserver" in window) {
      new ResizeObserver(fitFrame).observe(dom.viewport);
    }
  }

  buildScale();
  bindEvents();
  loadScene();
})();
