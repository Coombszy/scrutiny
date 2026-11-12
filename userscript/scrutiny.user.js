// ==UserScript==
// @name         Goto Scrutiny
// @namespace    https://github.com/Coombszy/scrutiny
// @version      0.1.0
// @description  On a GitHub pull request, adds a "Goto Scrutiny" button when Scrutiny is running locally.
// @match        https://github.com/*
// @connect      localhost
// @grant        GM_xmlhttpRequest
// @grant        GM_openInTab
// @grant        GM_getValue
// @grant        GM_setValue
// @grant        GM_registerMenuCommand
// @grant        GM_addStyle
// @run-at       document-idle
// @noframes
// ==/UserScript==

(function () {
  "use strict";

  const BUTTON_ID = "scrutiny-goto";
  const RECHECK_MS = 5000;

  const port = () => GM_getValue("port", 7878);
  const base = () => `http://localhost:${port()}`;

  GM_registerMenuCommand("Set Scrutiny port", () => {
    const got = prompt("Port Scrutiny serves on", String(port()));
    const n = Number(got);
    if (got !== null && Number.isInteger(n) && n >= 1 && n <= 65535) {
      GM_setValue("port", n);
      lastCheck = 0;
      update();
    }
  });

  // pullRequest is the PR the current page belongs to, on any of its tabs.
  function pullRequest() {
    const m = location.pathname.match(/^\/([^/]+)\/([^/]+)\/pull\/(\d+)(?:\/|$)/);
    return m && { owner: m[1], repo: m[2], number: m[3] };
  }

  // Scrutiny's /health answers any origin; GM_xmlhttpRequest is not bound
  // by github.com's CORS or CSP.
  let running = false;
  let lastCheck = 0;
  let checking = null;
  function checkRunning() {
    if (Date.now() - lastCheck < RECHECK_MS) return Promise.resolve(running);
    if (checking) return checking;
    checking = new Promise((resolve) => {
      GM_xmlhttpRequest({
        method: "GET",
        url: `${base()}/health`,
        timeout: 1500,
        onload: (res) => {
          let ok = false;
          try {
            ok = res.status === 200 && JSON.parse(res.responseText).app === "scrutiny";
          } catch (_) {}
          resolve(ok);
        },
        onerror: () => resolve(false),
        ontimeout: () => resolve(false),
      });
    }).then((ok) => {
      running = ok;
      lastCheck = Date.now();
      checking = null;
      return ok;
    });
    return checking;
  }

  // Opening the PR with GM_openInTab makes it a browser-initiated navigation
  // (Sec-Fetch-Site: none), which Scrutiny allows; a link from github.com
  // would be refused as cross-site.
  function open(pr) {
    GM_openInTab(`${base()}/github.com/${pr.owner}/${pr.repo}/pull/${pr.number}`, { active: true, insert: true });
  }

  // The button's specular edge light is ported from React Bits'
  // SpecularButton (https://reactbits.dev): a WebGL canvas around the button
  // draws a light that circles its rounded edge.
  GM_addStyle(`
    .scrutiny-specular {
      --sb-radius: 6px;
      position: relative;
      isolation: isolate;
      display: inline-flex;
      align-items: center;
      justify-content: center;
      margin: 0;
      padding: 5px 12px;
      border: 0;
      border-radius: var(--sb-radius);
      font: inherit;
      font-size: 12px;
      font-weight: 500;
      line-height: 20px;
      letter-spacing: 0.01em;
      white-space: nowrap;
      color: var(--fgColor-default, #f5f5f5);
      background: color-mix(in srgb, var(--fgColor-default, #ffffff) 4%, transparent);
      box-shadow: 0 10px 24px -14px rgba(0, 0, 0, 0.45);
      cursor: pointer;
      outline: none;
      -webkit-tap-highlight-color: transparent;
      transition: transform 0.25s cubic-bezier(0.2, 0.8, 0.2, 1);
    }
    .scrutiny-specular:active { transform: scale(0.97); }
    .scrutiny-specular:focus-visible {
      outline: 2px solid color-mix(in srgb, var(--fgColor-default, #f5f5f5) 55%, transparent);
      outline-offset: 3px;
    }
    .scrutiny-specular__light {
      position: absolute;
      inset: -24px;
      z-index: 1;
      display: block;
      width: calc(100% + 48px);
      height: calc(100% + 48px);
      pointer-events: none;
    }
    .scrutiny-specular__label { position: relative; z-index: 2; }
  `);

  const LIGHT = {
    radius: 6,
    intensity: 1,
    rotationSeconds: 120, // one full circuit of the edge
    shineSize: 10,
    shineFade: 34,
    thickness: 1.2,
    glow: 1,
  };
  const PAD = 24;

  const VERTEX = `#version 300 es
in vec2 aPosition;
void main() {
  gl_Position = vec4(aPosition, 0.0, 1.0);
}`;

  const FRAGMENT = `#version 300 es
precision highp float;
uniform vec2 uCenter;
uniform vec2 uHalf;
uniform float uRadius;
uniform float uPx;
uniform float uSpot;
uniform vec2 uEntry;
uniform vec3 uLineColor;
uniform vec3 uBaseColor;
uniform float uIntensity;
uniform vec2 uWindow;
uniform float uThickness;
uniform float uGlow;
out vec4 outColor;

float roundedBox(vec2 p, vec2 b, float r) {
  vec2 q = abs(p) - b + r;
  return length(max(q, 0.0)) + min(max(q.x, q.y), 0.0) - r;
}

float loopPosition(vec2 p, vec2 b, float r) {
  vec2 inner = b - r;
  float arc = 1.5707963 * r;
  float quarter = inner.x + inner.y + arc;
  vec2 a = abs(p);
  vec2 q = a - inner;
  float s = q.x > 0.0 && q.y > 0.0
    ? inner.y + atan(q.y, q.x) * r
    : (q.x > q.y ? min(a.y, inner.y) : inner.y + arc + inner.x - min(a.x, inner.x));
  if (p.x < 0.0) s = p.y >= 0.0 ? 2.0 * quarter - s : 2.0 * quarter + s;
  else if (p.y < 0.0) s = 4.0 * quarter - s;
  return s / max(4.0 * quarter, 0.0001);
}

float lobe(float position, float center) {
  float gap = abs(fract(position - center + 0.5) - 0.5);
  return 1.0 - smoothstep(uWindow.x, uWindow.x + uWindow.y, gap);
}

void main() {
  vec2 p = gl_FragCoord.xy - uCenter;
  float d = roundedBox(p, uHalf, uRadius);
  float position = loopPosition(p, uHalf, uRadius);
  float key = lobe(position, uSpot);
  float back = lobe(position, uSpot + 0.5);
  float streak = key + back * 0.45;
  float width = max(uThickness * uPx * mix(1.0, max(key, back), 0.6), 0.35);
  float band = smoothstep(-width - 0.6, -width + 0.6, d) * (1.0 - smoothstep(-0.6, 0.6, d));
  float hair = smoothstep(-uPx - 0.6, -uPx + 0.6, d) * (1.0 - smoothstep(-0.6, 0.6, d));
  float inside = 1.0 - smoothstep(-0.6, 0.6, d);
  vec2 pool = (p - uEntry) / (uHalf.y * vec2(1.6, 1.1));
  float glow = streak * 0.2 * exp(-abs(d + width * 0.5) / (5.0 * uPx)) + inside * 0.08 * exp(-dot(pool, pool));
  float light = clamp((band * streak + glow * uGlow) * uIntensity, 0.0, 1.0);
  vec4 base = vec4(uBaseColor, 1.0) * hair * 0.6;
  outColor = vec4(uLineColor, 1.0) * light + base * (1.0 - light);
}`;

  const UNIFORMS = ["uCenter", "uHalf", "uRadius", "uPx", "uSpot", "uEntry", "uLineColor", "uBaseColor",
    "uIntensity", "uWindow", "uThickness", "uGlow"];

  const wrap = (v) => v - Math.floor(v);

  function loopPosition(x, y, halfX, halfY, r) {
    const innerX = halfX - r;
    const innerY = halfY - r;
    const arc = (Math.PI / 2) * r;
    const quarter = innerX + innerY + arc;
    const ax = Math.abs(x);
    const ay = Math.abs(y);
    const qx = ax - innerX;
    const qy = ay - innerY;
    let s;
    if (qx > 0 && qy > 0) s = innerY + Math.atan2(qy, qx) * r;
    else if (qx > qy) s = Math.min(ay, innerY);
    else s = innerY + arc + innerX - Math.min(ax, innerX);
    if (x < 0) s = y >= 0 ? 2 * quarter - s : 2 * quarter + s;
    else if (y < 0) s = 4 * quarter - s;
    return wrap(s / Math.max(4 * quarter, 0.0001));
  }

  function rimPoint(position, halfX, halfY, r) {
    const innerX = halfX - r;
    const innerY = halfY - r;
    const arc = (Math.PI / 2) * r;
    const quarter = innerX + innerY + arc;
    let s = wrap(position) * 4 * quarter;
    let signX = 1;
    let signY = 1;
    if (s > 3 * quarter) {
      s = 4 * quarter - s;
      signY = -1;
    } else if (s > 2 * quarter) {
      s -= 2 * quarter;
      signX = -1;
      signY = -1;
    } else if (s > quarter) {
      s = 2 * quarter - s;
      signX = -1;
    }
    let x = innerX - (s - innerY - arc);
    let y = halfY;
    if (s <= innerY) {
      x = halfX;
      y = s;
    } else if (s <= innerY + arc) {
      const angle = (s - innerY) / Math.max(r, 0.0001);
      x = innerX + Math.cos(angle) * r;
      y = innerY + Math.sin(angle) * r;
    }
    return [x * signX, y * signY];
  }

  function compile(gl, type, source) {
    const shader = gl.createShader(type);
    if (!shader) return null;
    gl.shaderSource(shader, source);
    gl.compileShader(shader);
    if (gl.getShaderParameter(shader, gl.COMPILE_STATUS)) return shader;
    gl.deleteShader(shader);
    return null;
  }

  function link(gl) {
    const vertex = compile(gl, gl.VERTEX_SHADER, VERTEX);
    const fragment = compile(gl, gl.FRAGMENT_SHADER, FRAGMENT);
    const program = gl.createProgram();
    if (!vertex || !fragment || !program) return null;
    gl.attachShader(program, vertex);
    gl.attachShader(program, fragment);
    gl.bindAttribLocation(program, 0, "aPosition");
    gl.linkProgram(program);
    gl.deleteShader(vertex);
    gl.deleteShader(fragment);
    if (gl.getProgramParameter(program, gl.LINK_STATUS)) return program;
    gl.deleteProgram(program);
    return null;
  }

  // specular lights button's edge from canvas for as long as the page lives.
  // The button is made once and moved, never recreated, so this runs once.
  // Without WebGL2 the button is left plain.
  function specular(button, canvas) {
    const gl = canvas.getContext("webgl2", { alpha: true, premultipliedAlpha: true, antialias: false, depth: false, stencil: false });
    const program = gl && link(gl);
    if (!program) return;
    const uniforms = {};
    for (const name of UNIFORMS) uniforms[name] = gl.getUniformLocation(program, name);
    const vao = gl.createVertexArray();
    gl.bindVertexArray(vao);
    gl.bindBuffer(gl.ARRAY_BUFFER, gl.createBuffer());
    gl.bufferData(gl.ARRAY_BUFFER, new Float32Array([-1, -1, 3, -1, -1, 3]), gl.STATIC_DRAW);
    gl.enableVertexAttribArray(0);
    gl.vertexAttribPointer(0, 2, gl.FLOAT, false, 0, 0);
    gl.bindVertexArray(null);

    const probe = document.createElement("canvas").getContext("2d", { willReadFrequently: true });
    const colors = new Map();
    const view = { width: 1, height: 1, ratio: 1 };
    const reduce = window.matchMedia?.("(prefers-reduced-motion: reduce)").matches ?? false;
    const state = { spot: 0, origin: 0, press: 0, ready: false };
    // The light and the edge line take GitHub's text and border colours, so
    // the button suits both light and dark themes.
    const palette = { line: "#ffffff", base: "#525252" };
    let raf = 0;
    let last = 0;
    let visible = true;

    const toRgb = (value) => {
      if (colors.has(value)) return colors.get(value);
      let rgb = [1, 1, 1];
      if (probe) {
        probe.clearRect(0, 0, 1, 1);
        probe.fillStyle = "#000000";
        probe.fillStyle = value;
        probe.fillRect(0, 0, 1, 1);
        const [r, g, b] = probe.getImageData(0, 0, 1, 1).data;
        rgb = [r / 255, g / 255, b / 255];
      }
      colors.set(value, rgb);
      return rgb;
    };

    const cornerRadius = () => Math.min(LIGHT.radius, view.width / 2, view.height / 2);

    const restSpot = () => {
      const r = cornerRadius();
      const inset = r * (1 - Math.SQRT1_2);
      return loopPosition(inset - view.width / 2, view.height / 2 - inset, view.width / 2, view.height / 2, r);
    };

    const draw = () => {
      const { width, height } = view;
      const scaleX = canvas.width / (width + PAD * 2);
      const scaleY = canvas.height / (height + PAD * 2);
      gl.viewport(0, 0, canvas.width, canvas.height);
      gl.clearColor(0, 0, 0, 0);
      gl.clear(gl.COLOR_BUFFER_BIT);
      gl.useProgram(program);
      gl.uniform2f(uniforms.uCenter, (PAD + width / 2) * scaleX, (PAD + height / 2) * scaleY);
      gl.uniform2f(uniforms.uHalf, (width / 2) * scaleX, (height / 2) * scaleY);
      gl.uniform1f(uniforms.uRadius, cornerRadius() * scaleX);
      gl.uniform1f(uniforms.uPx, view.ratio);
      const [rimX, rimY] = rimPoint(state.spot, width / 2, height / 2, cornerRadius());
      const pull = 1 - Math.min(0.35 * (height / 2), Math.hypot(rimX, rimY)) / Math.max(Math.hypot(rimX, rimY), 0.0001);
      gl.uniform1f(uniforms.uSpot, state.spot);
      gl.uniform2f(uniforms.uEntry, rimX * pull * scaleX, rimY * pull * scaleY);
      gl.uniform3fv(uniforms.uLineColor, toRgb(palette.line));
      gl.uniform3fv(uniforms.uBaseColor, toRgb(palette.base));
      gl.uniform1f(uniforms.uIntensity, LIGHT.intensity * (1 + state.press * 0.8));
      gl.uniform2f(uniforms.uWindow, LIGHT.shineSize / 360 + state.press * 0.06, LIGHT.shineFade / 360 + 0.0001);
      gl.uniform1f(uniforms.uThickness, LIGHT.thickness);
      gl.uniform1f(uniforms.uGlow, LIGHT.glow);
      gl.bindVertexArray(vao);
      gl.drawArrays(gl.TRIANGLES, 0, 3);
      gl.bindVertexArray(null);
    };

    // frame moves the light steadily round the edge, anticlockwise from the
    // top-left corner; with reduced motion it stays put. Its place comes from
    // the clock, not from adding up frame times, so when GitHub stalls the
    // page it carries on from where it should be rather than from where it
    // stopped.
    const frame = (now) => {
      raf = 0;
      if (!visible || !button.isConnected) return;
      const dt = last ? Math.min(0.05, (now - last) / 1000) : 1 / 60;
      last = now;
      const turns = now / 1000 / LIGHT.rotationSeconds;
      if (!state.ready) {
        state.origin = restSpot() - turns;
        state.ready = true;
      }
      state.spot = reduce ? restSpot() : wrap(state.origin + turns);
      state.press *= Math.exp(-dt * 5);
      draw();
      if (!reduce || state.press > 0.002) raf = requestAnimationFrame(frame);
      else last = 0;
    };

    const wake = () => {
      if (!raf && visible && button.isConnected) raf = requestAnimationFrame(frame);
    };

    // measure runs whenever the button is (re)attached or resized.
    const measure = (entry) => {
      const box = entry?.borderBoxSize?.[0];
      view.width = Math.max(1, box ? box.inlineSize : button.offsetWidth);
      view.height = Math.max(1, box ? box.blockSize : button.offsetHeight);
      view.ratio = Math.min(window.devicePixelRatio || 1, 3);
      canvas.width = Math.max(1, Math.round((view.width + PAD * 2) * view.ratio));
      canvas.height = Math.max(1, Math.round((view.height + PAD * 2) * view.ratio));
      const style = getComputedStyle(button);
      palette.line = style.color || palette.line;
      palette.base = style.getPropertyValue("--borderColor-default").trim() || palette.base;
      if (state.ready) draw();
      wake();
    };

    new ResizeObserver((entries) => measure(entries[entries.length - 1])).observe(button);
    new IntersectionObserver((entries) => {
      visible = entries.some((e) => e.isIntersecting);
      if (visible) {
        last = 0;
        wake();
      }
    }).observe(button);
    button.addEventListener("pointerdown", () => {
      state.press = 1;
      wake();
    });
  }

  // host is the one Goto Scrutiny button for the page's life: GitHub
  // re-renders its header, and render moves this back in rather than making
  // another. It opens whichever PR is showing when clicked.
  let host = null;
  function goto() {
    if (host) return host;
    host = document.createElement("span");
    host.id = BUTTON_ID;
    const b = document.createElement("button");
    b.type = "button";
    b.className = "scrutiny-specular";
    b.title = "Open this pull request in Scrutiny";
    const canvas = document.createElement("canvas");
    canvas.className = "scrutiny-specular__light";
    canvas.setAttribute("aria-hidden", "true");
    const label = document.createElement("span");
    label.className = "scrutiny-specular__label";
    label.textContent = "Goto Scrutiny";
    b.append(canvas, label);
    host.append(b);
    b.addEventListener("click", (e) => {
      e.preventDefault();
      const pr = pullRequest();
      if (pr) open(pr);
    });
    specular(b, canvas);
    return host;
  }

  // The button goes at the right end of the PR header's state line
  // ("Merged · … merged 2 commits into main"); failing that, before the
  // header's action buttons (Edit, Code); failing both, it floats at the top
  // right.
  const HEADER_META = ['[data-component="PageHeader.Description"]', ".gh-header-show .gh-header-meta", ".gh-header-meta"];
  const HEADER_ACTIONS = [".gh-header-actions", '[data-component="PH_Actions"]', '[class*="PageHeader-Actions"]'];

  const find = (sels) => sels.map((sel) => document.querySelector(sel)).find(Boolean);

  // spot is where the button belongs on the page as it is now.
  function spot() {
    const meta = find(HEADER_META);
    if (meta) return { el: meta, kind: "meta" };
    const actions = find(HEADER_ACTIONS);
    if (actions) return { el: actions, kind: "actions" };
    return { el: document.body, kind: "floating" };
  }

  function place(h, { el, kind }) {
    h.style.cssText = "";
    if (kind === "meta") {
      // Pinned to the row's right end rather than laid out with it, so it
      // never wraps onto a line of its own; the padding keeps the row's text
      // clear of it.
      if (getComputedStyle(el).position === "static") el.style.position = "relative";
      Object.assign(h.style, { position: "absolute", top: "0", bottom: "0", right: "0", display: "flex", alignItems: "center" });
      el.append(h);
      el.style.paddingRight = `${h.offsetWidth + 16}px`;
    } else if (kind === "actions") {
      Object.assign(h.style, { display: "inline-flex", marginRight: "8px" });
      el.prepend(h);
    } else {
      Object.assign(h.style, { position: "fixed", top: "72px", right: "16px", zIndex: "100" });
      el.append(h);
    }
  }

  // render makes the page show the button on a PR when Scrutiny was last
  // seen running, in the best spot the page has so far. It never waits, so
  // GitHub re-rendering its header is undone before it is seen.
  function render() {
    // Drop copies left by an earlier version of this script.
    document.querySelectorAll(`#${BUTTON_ID}`).forEach((e) => e !== host && e.remove());
    if (!pullRequest() || !running) {
      host?.remove();
      return;
    }
    const h = goto();
    const where = spot();
    if (h.parentElement !== where.el) place(h, where);
  }

  async function update() {
    await checkRunning();
    render();
  }

  // GitHub navigates without full page loads and re-renders its header, so
  // render on every DOM change (cheap, no request) and re-check that Scrutiny
  // is running on navigation, on focus and once the last check is stale.
  new MutationObserver(() => {
    render();
    if (pullRequest() && Date.now() - lastCheck >= RECHECK_MS) update();
  }).observe(document.body, { childList: true, subtree: true });
  document.addEventListener("turbo:load", update);
  window.addEventListener("popstate", update);
  // Notice Scrutiny starting or stopping when the tab comes back into focus.
  document.addEventListener("visibilitychange", () => document.visibilityState === "visible" && update());
  update();
})();
