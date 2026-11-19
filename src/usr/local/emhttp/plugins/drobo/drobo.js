/* Drobo panel for the Unraid Main tab. Plain JS, no dependencies. */
(function () {
  'use strict';
  var panel = document.getElementById('drobo-panel');
  if (!panel) return;
  var timer = null;

  function esc(s) {
    return String(s == null ? '' : s).replace(/[&<>"']/g, function (c) {
      return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c];
    });
  }
  function size(n) {
    if (!n) return '0';
    var u = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'], i = 0;
    while (n >= 1000 && i < u.length - 1) { n /= 1000; i++; }
    return (n >= 100 || i < 3 ? n.toFixed(0) : n.toFixed(n >= 10 ? 1 : 2)) + ' ' + u[i];
  }
  function pct(a, b) { return b ? Math.round(a * 100 / b) : 0; }
  function speed(m) {
    if (!m) return 'unknown';
    if (m >= 1000) return 'USB 3 (' + (m / 1000) + ' Gbps)';
    if (m === 480) return 'USB 2 (480 Mbps)';
    return m + ' Mbps';
  }
  function badge(text, kind, title) {
    return '<span class="drobo-badge drobo-' + kind + '"' + (title ? ' title="' + esc(title) + '"' : '') + '>' + esc(text) + '</span>';
  }

  var healthKind = { Healthy: 'ok', Warning: 'warn', Critical: 'bad' };
  var bayKind = { Good: 'ok', Healed: 'ok', Warning: 'warn', Failed: 'bad', Empty: 'empty', Unknown: 'warn' };
  var levelKind = { alert: 'bad', warning: 'warn', normal: 'info' };

  function renderDrobo(d) {
    var h = '<div class="drobo-unit">';
    // Header line
    h += '<div class="drobo-head"><span class="drobo-name"><i class="fa fa-hdd-o"></i> ' + esc(d.name) + '</span> ';
    if (d.health) h += badge(d.health, healthKind[d.health] || 'warn', 'Overall status the Drobo reports');
    if (d.relayout) h += badge('Rebuilding data protection', 'warn');
    var u = d.usb || {};
    if (u.speed_mbps) h += badge(speed(u.speed_mbps), u.speed_mbps >= 5000 ? 'ok' : 'warn',
      'Negotiated link speed' + (u.port_max_mbps ? '. The port supports up to ' + speed(u.port_max_mbps) : '') + (u.driver ? '. Driver: ' + u.driver : ''));
    if (d.luns && d.luns.length) h += ' <span class="drobo-muted">' + esc(d.luns.map(function (l) { return l.block; }).join(', ')) + '</span>';
    h += '</div>';

    // Findings
    if (d.findings && d.findings.length) {
      h += '<ul class="drobo-findings">';
      d.findings.forEach(function (f) {
        h += '<li class="drobo-' + (levelKind[f.level] || 'info') + '"><b>' + esc(f.title) + '.</b> ' + esc(f.detail) + '</li>';
      });
      h += '</ul>';
    }

    // Bays
    if (d.bays && d.bays.length) {
      h += '<div class="drobo-bays">';
      d.bays.forEach(function (b) {
        var k = bayKind[b.health] || 'warn';
        h += '<div class="drobo-bay drobo-' + k + '">';
        h += '<div class="drobo-bay-top"><span class="drobo-light"></span>' + (b.ssd ? 'Cache' : 'Bay ' + b.number) + ' · ' + esc(b.health) + '</div>';
        if (!b.empty) {
          h += '<div class="drobo-bay-model" title="' + esc('Serial ' + (b.Serial || '?') + ', firmware ' + (b.Firmware || '?')) + '">' + esc(b.Model || 'Unknown drive') + '</div>';
          h += '<div class="drobo-muted">' + size(b.TotalCapacity);
          if (b.TemperatureC) h += ' · ' + b.TemperatureC + '°C';
          if (b.ErrorCount) h += ' · <span class="drobo-text-warn">' + b.ErrorCount + ' errors</span>';
          if (b.ssd && b.LifeRemaining) h += ' · ' + b.LifeRemaining + '% life';
          h += '</div>';
        }
        h += '</div>';
      });
      h += '</div>';
    }

    // Capacity comparison
    var c = d.capacity;
    var vols = d.volumes || [];
    if (c || vols.length) {
      h += '<table class="drobo-cap"><thead><tr><th></th><th>Drobo (real, protected)</th><th>Volume (what Unraid sees)</th></tr></thead><tbody>';
      var vSize = 0, vUsed = 0, vFree = 0;
      vols.forEach(function (v) { vSize += v.size_bytes || 0; vUsed += v.used_bytes || 0; vFree += v.free_bytes || 0; });
      var lunSize = (d.luns || []).reduce(function (a, l) { return a + (l.size_bytes || 0); }, 0);
      h += '<tr><td>Size</td><td>' + (c ? size(c.TotalProtected) : '–') + '</td><td>' + (vSize ? size(vSize) : '–') +
        (lunSize ? ' <span class="drobo-muted">(disk reported as ' + size(lunSize) + ')</span>' : '') + '</td></tr>';
      h += '<tr><td>Used</td><td>' + (c ? size(c.UsedProtected) + ' (' + pct(c.UsedProtected, c.TotalProtected) + '%)' : '–') + '</td><td>' + (vSize ? size(vUsed) : '–') + '</td></tr>';
      h += '<tr><td>Free</td><td>' + (c ? size(c.FreeProtected) : '–') + '</td><td>' + (vSize ? size(vFree) : '–') + '</td></tr>';
      h += '</tbody></table>';
      if (c && c.TotalProtected) {
        var p = pct(c.UsedProtected, c.TotalProtected);
        h += '<div class="drobo-bar" title="' + p + '% of protected space used"><div class="drobo-bar-fill drobo-' + (p >= 95 ? 'bad' : p >= 85 ? 'warn' : 'ok') + '" style="width:' + p + '%"></div></div>';
      }
      h += '<p class="drobo-muted drobo-note">A Drobo tells the computer its disk is bigger than the drives inside, so the volume size never matches. The Drobo column is the real limit. ' +
        'Used space can also differ: the Drobo still counts files deleted from the volume until it reclaims that space itself.</p>';
    }

    // Volumes
    if (vols.length) {
      h += '<div class="drobo-vols">';
      vols.forEach(function (v) {
        h += '<div>' + esc(v.partition) + ' ';
        h += v.mountpoint ? '<a href="/Shares/Browse?dir=' + encodeURIComponent(v.mountpoint) + '">' + esc(v.mountpoint) + '</a> ' + badge(v.read_only ? 'read-only' : 'read-write', v.read_only ? 'warn' : 'ok')
          : badge('not mounted', 'info');
        if (v.fs_type) h += ' <span class="drobo-muted">' + esc(v.fs_type) + '</span>';
        if (v.hfs) h += ' <span class="drobo-muted">· ' + (v.hfs.journaled ? 'journaled' : 'not journaled') + '</span>';
        h += '</div>';
      });
      h += '</div>';
    }
    if (d.error && !(d.findings || []).some(function (f) { return f.code === 'esa'; })) {
      h += '<p class="drobo-text-warn">' + esc(d.error) + '</p>';
    }
    h += '</div>';
    return h;
  }

  function render(r) {
    var h = '';
    if (r.error) h += '<p class="drobo-text-warn">' + esc(r.error) + '</p>';
    if (!r.drobos || !r.drobos.length) {
      h += '<p>No Drobo is connected. If it should be, check its power and USB cable; it can take a minute to appear after power-on.</p>';
    } else {
      r.drobos.forEach(function (d) { h += renderDrobo(d); });
    }
    var t = r.time ? new Date(r.time) : null;
    h += '<div class="drobo-foot drobo-muted">' + (t ? 'Checked ' + t.toLocaleTimeString() + ' · ' : '') +
      '<a href="#" id="drobo-refresh">Refresh</a> · <a href="/Settings/DroboSettings">Settings</a></div>';
    panel.innerHTML = h;
    var a = document.getElementById('drobo-refresh');
    if (a) a.onclick = function (e) { e.preventDefault(); load(true); };
  }

  function load(fresh) {
    var x = new XMLHttpRequest();
    x.open('GET', '/plugins/drobo/include/status.php' + (fresh ? '?fresh=1' : ''));
    x.onload = function () {
      try { render(JSON.parse(x.responseText)); }
      catch (e) { panel.innerHTML = '<p class="drobo-text-warn">Could not read Drobo status: ' + esc(x.responseText.slice(0, 300)) + '</p>'; }
    };
    x.onerror = function () { panel.innerHTML = '<p class="drobo-text-warn">Could not reach the server.</p>'; };
    x.send();
  }

  function schedule() {
    clearInterval(timer);
    timer = setInterval(function () { if (!document.hidden) load(false); }, 30000);
  }
  document.addEventListener('visibilitychange', function () { if (!document.hidden) load(false); });
  load(false);
  schedule();
})();
