package speeds

// clientJS draws the section. It sits beside the numbers it draws, so the plugin
// is one thing to read rather than two that have to agree.
//
// The charts are SVG, fitted to the width of the card: no sideways scrolling, and
// the shape is what is being read, not any single point.
const clientJS = `
// Nothing else in Lola knows about timings: this flag is how the page learns
// that speeds is mounted, so the per-reply line disappears with the plugin.
window.lolaSpeeds = true;

(function () {
  // The section only. The lines under replies are the page's business again.
  var box = document.getElementById('speedsCharts');
  if (!box || box.dataset.wired === '1') return;
  box.dataset.wired = '1';

  var session = document.getElementById('speedsSession');
  var windowSel = document.getElementById('speedsWindow');
  var note = document.getElementById('speedsNote');
  var legendBox = document.getElementById('speedsLegend');

  var palette = ['#2563eb', '#16a34a', '#db2777', '#f59e0b', '#7c3aed', '#0891b2', '#dc2626', '#57534e'];
  var colours = {};
  function colourFor(name) {
    if (!colours[name]) colours[name] = palette[Object.keys(colours).length % palette.length];
    return colours[name];
  }

  function el(name, attrs) {
    var node = document.createElementNS('http://www.w3.org/2000/svg', name);
    for (var k in attrs) node.setAttribute(k, attrs[k]);
    return node;
  }

  // chart draws one measurement over time, filling the width it is given.
  function chart(title, unit, key, points, models, step, ceiling) {
    var width = box.clientWidth;
    if (width < 200) return null;

    var height = 150, left = 52, right = 10, top = 24, bottom = 12;
    var plotW = width - left - right, plotH = height - top - bottom;

    var values = points.map(function (p) { return p[key]; }).filter(function (v) { return v !== null; });
    var peak = values.length ? Math.max.apply(null, values) : 1;
    var max = ceiling ? Math.max(ceiling, Math.ceil(peak / step) * step)
                      : Math.max(step, Math.ceil(peak / step) * step);

    var svg = el('svg', {
      viewBox: '0 0 ' + width + ' ' + height,
      width: '100%',
      height: height,
      style: 'display:block;width:100%;height:' + height + 'px;margin:0 0 10px 0'
    });

    var heading = el('text', { x: left, y: 15, fill: '#111827', 'font-size': 12, 'font-weight': 600 });
    heading.textContent = title + ' (' + unit + ')';
    svg.appendChild(heading);

    // Three lines, not eight: at this size a grid is clutter.
    for (var v = 0; v <= max; v += max) {
      var y = top + plotH - (v / max) * plotH;
      svg.appendChild(el('line', { x1: left, y1: y, x2: left + plotW, y2: y, stroke: 'rgba(0,0,0,0.12)' }));
      var label = el('text', { x: left - 6, y: y + 4, 'text-anchor': 'end', fill: 'rgba(0,0,0,0.5)', 'font-size': 10 });
      label.textContent = Math.round(v);
      svg.appendChild(label);
    }

    models.forEach(function (model) {
      var path = '', started = false;
      points.forEach(function (p, i) {
        if (p.model !== model || p[key] === null) return;
        var x = left + (points.length === 1 ? plotW / 2 : (i / (points.length - 1)) * plotW);
        var y = top + plotH - (Math.min(p[key], max) / max) * plotH;
        path += (started ? ' L ' : 'M ') + x.toFixed(1) + ' ' + y.toFixed(1);
        started = true;
        if (points.length <= 40) {
          svg.appendChild(el('circle', { cx: x.toFixed(1), cy: y.toFixed(1), r: 2, fill: colourFor(model) }));
        }
      });
      if (path) {
        svg.appendChild(el('path', {
          d: path, fill: 'none', stroke: colourFor(model), 'stroke-width': 2,
          'stroke-linejoin': 'round', 'stroke-linecap': 'round'
        }));
      }
    });

    return svg;
  }

  function legend(models) {
    if (!legendBox) return;
    legendBox.innerHTML = '';
    models.forEach(function (model) {
      var wrap = document.createElement('span');
      wrap.style.marginRight = '12px';
      wrap.style.fontSize = '11.5px';

      var dot = document.createElement('span');
      dot.style.display = 'inline-block';
      dot.style.width = '9px';
      dot.style.height = '9px';
      dot.style.borderRadius = '50%';
      dot.style.marginRight = '5px';
      dot.style.background = colourFor(model);

      wrap.appendChild(dot);
      wrap.appendChild(document.createTextNode(model));
      legendBox.appendChild(wrap);
    });
  }

  function load() {
    var url = '/api/speeds/series?window=' + encodeURIComponent(windowSel.value) +
              '&session=' + encodeURIComponent(session.value);

    fetch(url).then(function (r) { return r.json(); }).then(function (points) {
      var all = points || [];
      // TPS is only a measurement where the reply was seen streaming; the rest
      // are counted and reported rather than drawn as if they meant something.
      var unmeasured = all.filter(function (p) { return !p.streamed; }).length;

      points = all.map(function (p) {
        return {
          model: p.model,
          at: p.at,
          ttft: p.ttft,
          tps: p.streamed && p.tps > 0 ? p.tps : null
        };
      });

      if (!points.length) {
        box.innerHTML = '';
        note.textContent = 'Nothing measured in this stretch yet.';
        legend([]);
        return;
      }

      var models = [];
      points.forEach(function (p) { if (models.indexOf(p.model) < 0) models.push(p.model); });
      models.sort();

      var first = chart('TTFT', 'ms', 'ttft', points, models, 500, 0);
      if (!first) { setTimeout(load, 250); return; }

      box.innerHTML = '';
      box.appendChild(first);
      var second = chart('TPS', 'tokens/s', 'tps', points, models, 250, 0);
      if (second) box.appendChild(second);
      legend(models);

      note.textContent = points.length + ' repl' + (points.length === 1 ? 'y' : 'ies') +
        ' across ' + models.length + ' model' + (models.length === 1 ? '' : 's') +
        (unmeasured ? ' — ' + unmeasured + ' excluded from TPS as not streamed.' : '.');
    }).catch(function () {
      note.textContent = 'Could not read the measurements.';
    });
  }

  function loadSessions() {
    fetch('/api/speeds/sessions').then(function (r) { return r.json(); }).then(function (list) {
      (list || []).forEach(function (s) {
        var o = document.createElement('option');
        o.value = s.id;
        o.textContent = s.title || s.id;
        session.appendChild(o);
      });
    }).catch(function () {});
  }

  // The section is built when it is opened, so watch for it rather than drawing
  // once at boot and hoping.
  // Drawn once, when the section first appears, then the watch stops: an observer
  // that keeps firing during a scroll redraws the charts under the person's hand.
  var pending = null;
  var watcher = new MutationObserver(function () {
    var canvas = document.getElementById('speedsCharts');
    if (!canvas) return;
    if (pending) clearTimeout(pending);
    pending = setTimeout(function () {
      load();
      watcher.disconnect();
    }, 120);
  });
  watcher.observe(document.body, { childList: true, subtree: true });

  session.addEventListener('change', load);
  windowSel.addEventListener('change', load);
  // document.addEventListener('lola:reply', load);

  loadSessions();
  load();
})();
`
