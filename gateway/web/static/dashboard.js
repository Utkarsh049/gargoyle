/**
 * Gargoyle API Gateway - Dashboard Client Logic
 * Handles real-time clock, canvas isometric 3D wireframe rendering, and modal triggers.
 */

document.addEventListener('DOMContentLoaded', () => {
  initClock();
  initTopologyCanvas();
});

// -----------------------------------------------------------------------------
// Real-time Clock & Date Widget
// -----------------------------------------------------------------------------
function initClock() {
  const digitsEl = document.getElementById('liveClockDigits');
  const dateEl = document.getElementById('liveDateWidget');

  function update() {
    const now = new Date();
    
    if (digitsEl) {
      const hours = now.getHours();
      const minutes = String(now.getMinutes()).padStart(2, '0');
      const ampm = hours >= 12 ? 'PM' : 'AM';
      const formattedHours = hours % 12 || 12;
      digitsEl.textContent = `${formattedHours}:${minutes} ${ampm}`;
    }

    if (dateEl) {
      const day = now.getDate();
      const monthNames = [
        "January", "February", "March", "April", "May", "June",
        "July", "August", "September", "October", "November", "December"
      ];
      dateEl.textContent = `${day} ${monthNames[now.getMonth()]}`;
    }
  }

  update();
  setInterval(update, 1000);
}

// -----------------------------------------------------------------------------
// Canvas: Isometric 3D Gateway Topology Wireframe
// -----------------------------------------------------------------------------
function initTopologyCanvas() {
  const canvas = document.getElementById('topologyCanvas');
  if (!canvas) return;

  const ctx = canvas.getContext('2d');
  let animationFrameId;
  let angle = 0;

  function resize() {
    const rect = canvas.getBoundingClientRect();
    canvas.width = rect.width * window.devicePixelRatio;
    canvas.height = rect.height * window.devicePixelRatio;
    ctx.scale(window.devicePixelRatio, window.devicePixelRatio);
  }

  resize();
  window.addEventListener('resize', resize);

  // Isometric 3D Projection
  function project(x, y, z, cx, cy) {
    // 30 degree isometric angle
    const isoX = (x - z) * Math.cos(Math.PI / 6);
    const isoY = (x + z) * Math.sin(Math.PI / 6) - y;
    return { x: cx + isoX, y: cy + isoY };
  }

  function drawNode(p, size, color, glow = true) {
    ctx.save();
    if (glow) {
      ctx.shadowColor = '#7CE57B';
      ctx.shadowBlur = 8;
    }
    ctx.fillStyle = color;
    ctx.beginPath();
    ctx.arc(p.x, p.y, size, 0, Math.PI * 2);
    ctx.fill();
    ctx.restore();
  }

  function drawLine(p1, p2, color, width = 1, dashed = false) {
    ctx.save();
    ctx.strokeStyle = color;
    ctx.lineWidth = width;
    if (dashed) ctx.setLineDash([3, 3]);
    ctx.beginPath();
    ctx.moveTo(p1.x, p1.y);
    ctx.lineTo(p2.x, p2.y);
    ctx.stroke();
    ctx.restore();
  }

  function render() {
    const rect = canvas.getBoundingClientRect();
    const w = rect.width;
    const h = rect.height;
    const cx = w / 2;
    const cy = h / 2 + 10;

    ctx.clearRect(0, 0, w, h);

    angle += 0.015;

    // Isometric Room Grid Lines
    const size = 50;
    const gridColor = 'rgba(255, 255, 255, 0.08)';
    const neonGreen = '#7CE57B';
    const accentLine = 'rgba(124, 229, 123, 0.4)';

    // Floor Base Quad
    const p00 = project(-size, 0, -size, cx, cy);
    const p10 = project(size, 0, -size, cx, cy);
    const p11 = project(size, 0, size, cx, cy);
    const p01 = project(-size, 0, size, cx, cy);

    drawLine(p00, p10, gridColor);
    drawLine(p10, p11, gridColor);
    drawLine(p11, p01, gridColor);
    drawLine(p01, p00, gridColor);

    // Gateway Core Isometric Cube (Center)
    const cubeH = 22;
    const cSize = 18;
    const c00_b = project(-cSize, 0, -cSize, cx, cy);
    const c10_b = project(cSize, 0, -cSize, cx, cy);
    const c11_b = project(cSize, 0, cSize, cx, cy);
    const c01_b = project(-cSize, 0, cSize, cx, cy);

    const c00_t = project(-cSize, cubeH, -cSize, cx, cy);
    const c10_t = project(cSize, cubeH, -cSize, cx, cy);
    const c11_t = project(cSize, cubeH, cSize, cx, cy);
    const c01_t = project(-cSize, cubeH, cSize, cx, cy);

    // Pillars
    drawLine(c00_b, c00_t, accentLine);
    drawLine(c10_b, c10_t, neonGreen, 1.5);
    drawLine(c11_b, c11_t, neonGreen, 1.5);
    drawLine(c01_b, c01_t, accentLine);

    // Top Roof Quad
    drawLine(c00_t, c10_t, neonGreen, 1.2);
    drawLine(c10_t, c11_t, neonGreen, 1.2);
    drawLine(c11_t, c01_t, neonGreen, 1.2);
    drawLine(c01_t, c00_t, neonGreen, 1.2);

    // Server Rack Verticals / Mesh
    const rack1 = project(-35, 30, -10, cx, cy);
    const rack1_b = project(-35, 0, -10, cx, cy);
    drawLine(rack1_b, rack1, 'rgba(255, 255, 255, 0.2)', 1, true);

    const rack2 = project(35, 30, -10, cx, cy);
    const rack2_b = project(35, 0, -10, cx, cy);
    drawLine(rack2_b, rack2, 'rgba(255, 255, 255, 0.2)', 1, true);

    // Active Data Pulse Node orbiting Core
    const orbitR = 38;
    const pulseX = Math.cos(angle) * orbitR;
    const pulseZ = Math.sin(angle) * orbitR;
    const pulseY = 12 + Math.sin(angle * 2) * 4;
    const pulsePos = project(pulseX, pulseY, pulseZ, cx, cy);

    drawNode(pulsePos, 3.5, '#FFFFFF', true);
    drawNode(c11_t, 2.5, neonGreen, true);

    animationFrameId = requestAnimationFrame(render);
  }

  render();
}

// -----------------------------------------------------------------------------
// Modal Helpers
// -----------------------------------------------------------------------------
window.openCreateClientModal = function() {
  const modal = document.getElementById('createClientModal');
  if (modal) modal.style.display = 'flex';
};

window.closeCreateClientModal = function() {
  const modal = document.getElementById('createClientModal');
  if (modal) modal.style.display = 'none';
};

window.copyToClipboard = function(text) {
  navigator.clipboard.writeText(text).then(() => {
    alert('API Key copied to clipboard!');
  });
};
