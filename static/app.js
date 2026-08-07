/* ── Theme ─────────────────────────────────────────────────────── */
function toggleTheme() {
  var next = document.documentElement.getAttribute('data-theme') === 'light' ? 'dark' : 'light';
  document.documentElement.setAttribute('data-theme', next);
  document.cookie = 'chemres-theme=' + next + ';path=/;max-age=31536000;SameSite=Lax';

  document.querySelectorAll('.structure-wrap[data-sd-rendered]').forEach(function(el) {
    var col        = el.closest('.structure-col');
    var frame3d    = col && col.querySelector('.structure-3d-frame');
    var was3dActive = frame3d && frame3d.style.display !== 'none';
    el.removeAttribute('data-sd-rendered');
    el.innerHTML = '';
    drawStructure(el);
    // drawStructure() always shows the 2D panel — re-hide it if 3D was active
    // so the two views don't end up stacked and both visible at once, and
    // re-render the 3D canvas so its background picks up the new theme.
    if (was3dActive) {
      el.style.display = 'none';
      _inlineViewer = null;
      renderViewerInto(frame3d.querySelector('.structure-wrap-3d'), el.dataset.cid)
        .then(function(v) { _inlineViewer = v; })
        .catch(function() { _inlineViewer = null; });
    }
  });

  if (_modalSmiles && document.getElementById('structure-modal').classList.contains('is-open')) {
    openStructureModal(_modalSmiles, _modalName, _modalFormula, _modalCID);
  }
}

function currentTheme() {
  return document.documentElement.getAttribute('data-theme') === 'dark' ? 'dark' : 'light';
}

/* ── Structure utilities ────────────────────────────────────────── */
function copyAsPng(svgEl, btn) {
  var src = new XMLSerializer().serializeToString(svgEl);
  if (!src.match(/xmlns=/)) src = src.replace('<svg', '<svg xmlns="http://www.w3.org/2000/svg"');
  var blob = new Blob([src], {type: 'image/svg+xml;charset=utf-8'});
  var url  = URL.createObjectURL(blob);
  var img  = new Image();
  img.onload = function() {
    var canvas = document.createElement('canvas');
    canvas.width  = img.naturalWidth  || 420;
    canvas.height = img.naturalHeight || 360;
    var ctx = canvas.getContext('2d');
    ctx.fillStyle = '#ffffff';
    ctx.fillRect(0, 0, canvas.width, canvas.height);
    ctx.drawImage(img, 0, 0);
    URL.revokeObjectURL(url);
    canvas.toBlob(function(pngBlob) {
      function flash() {
        btn.classList.add('copy-btn--done');
        setTimeout(function() { btn.classList.remove('copy-btn--done'); }, 1500);
      }
      navigator.clipboard.write([new ClipboardItem({'image/png': pngBlob})])
        .then(flash).catch(flash);
    }, 'image/png');
  };
  img.src = url;
}

function downloadSvg(svgEl, name) {
  var src = new XMLSerializer().serializeToString(svgEl);
  if (!src.match(/xmlns=/)) src = src.replace('<svg', '<svg xmlns="http://www.w3.org/2000/svg"');
  var blob = new Blob([src], {type: 'image/svg+xml;charset=utf-8'});
  var url  = URL.createObjectURL(blob);
  var a    = document.createElement('a');
  a.href   = url;
  a.download = (name || 'structure').replace(/[^\w-]/g, '_').substring(0, 60) + '.svg';
  document.body.appendChild(a);
  a.click();
  document.body.removeChild(a);
  URL.revokeObjectURL(url);
}

function formatFormula(f) {
  return f ? f.replace(/(\d+)/g, '<sub>$1</sub>') : '';
}

function copyText(btn, text) {
  navigator.clipboard.writeText(text.trim()).then(function() {
    btn.classList.add('copy-btn--done');
    setTimeout(function() { btn.classList.remove('copy-btn--done'); }, 1500);
  });
}

/* ── Structure downloads ────────────────────────────────────────── */
function downloadPng(svgEl, name) {
  var src = new XMLSerializer().serializeToString(svgEl);
  if (!src.match(/xmlns=/)) src = src.replace('<svg', '<svg xmlns="http://www.w3.org/2000/svg"');
  var blob = new Blob([src], {type: 'image/svg+xml;charset=utf-8'});
  var url  = URL.createObjectURL(blob);
  var img  = new Image();
  img.onload = function() {
    var canvas = document.createElement('canvas');
    canvas.width  = img.naturalWidth  || 420;
    canvas.height = img.naturalHeight || 360;
    var ctx = canvas.getContext('2d');
    ctx.fillStyle = '#ffffff';
    ctx.fillRect(0, 0, canvas.width, canvas.height);
    ctx.drawImage(img, 0, 0);
    URL.revokeObjectURL(url);
    canvas.toBlob(function(pngBlob) {
      var dlUrl = URL.createObjectURL(pngBlob);
      var a = document.createElement('a');
      a.href = dlUrl;
      a.download = (name || 'structure').replace(/[^\w-]/g, '_').substring(0, 60) + '.png';
      document.body.appendChild(a); a.click(); document.body.removeChild(a);
      URL.revokeObjectURL(dlUrl);
    }, 'image/png');
  };
  img.src = url;
}

function dataUriToBlob(dataUri) {
  var parts     = dataUri.split(',');
  var mimeMatch = parts[0].match(/:(.*?);/);
  var mime      = mimeMatch ? mimeMatch[1] : 'image/png';
  var bytes     = atob(parts[1]);
  var arr       = new Uint8Array(bytes.length);
  for (var i = 0; i < bytes.length; i++) arr[i] = bytes.charCodeAt(i);
  return new Blob([arr], { type: mime });
}

function copyPngDataUri(dataUri, btn) {
  // Decode locally instead of fetch(dataUri) — CSP's connect-src falls back
  // to default-src 'self', which does not permit fetching data: URIs.
  var blob = dataUriToBlob(dataUri);
  function flash() {
    btn.classList.add('copy-btn--done');
    setTimeout(function() { btn.classList.remove('copy-btn--done'); }, 1500);
  }
  navigator.clipboard.write([new ClipboardItem({'image/png': blob})]).then(flash).catch(flash);
}

function downloadPngDataUri(dataUri, name) {
  var a = document.createElement('a');
  a.href = dataUri;
  a.download = (name || 'structure').replace(/[^\w-]/g, '_').substring(0, 60) + '_3d.png';
  document.body.appendChild(a); a.click(); document.body.removeChild(a);
}

function sdfToXyz(sdf, name) {
  var lines = sdf.split('\n');
  if (lines.length < 4) return null;
  var atomCount = parseInt(lines[3].substring(0, 3).trim(), 10);
  if (isNaN(atomCount) || atomCount <= 0) return null;
  var atoms = [];
  for (var i = 0; i < atomCount; i++) {
    var line = lines[4 + i] || '';
    var x = parseFloat(line.substring(0, 10));
    var y = parseFloat(line.substring(10, 20));
    var z = parseFloat(line.substring(20, 30));
    var elem = line.substring(31, 34).trim();
    if (!elem || isNaN(x)) break;
    atoms.push(elem + '  ' + x.toFixed(4) + '  ' + y.toFixed(4) + '  ' + z.toFixed(4));
  }
  if (!atoms.length) return null;
  return atoms.length + '\n' + (name || 'compound') + '\n' + atoms.join('\n');
}

function downloadFromCid(cid, name, format) {
  if (!cid) { alert('No CID available for this compound.'); return; }
  var url = '/api/v1/conformer?cid=' + encodeURIComponent(cid) + '&format=' + format + '&name=' + encodeURIComponent(name || 'compound');
  var a = document.createElement('a');
  a.href = url;
  document.body.appendChild(a); a.click(); document.body.removeChild(a);
}

var _openDlMenu = null;
function toggleDlMenu(menu) {
  if (_openDlMenu && _openDlMenu !== menu) { _openDlMenu.style.display = 'none'; }
  if (menu.style.display === 'none') { menu.style.display = 'block'; _openDlMenu = menu; }
  else { menu.style.display = 'none'; _openDlMenu = null; }
}

/* ── Structure modal ────────────────────────────────────────────── */
var _modalSmiles = null, _modalName = null, _modalFormula = null, _modalCID = null;

var _modalTrigger = null;

function openStructureModal(smiles, name, formula, cid) {
  _modalSmiles = smiles; _modalName = name; _modalFormula = formula; _modalCID = cid || null;
  _modalViewer = null;
  _modalTrigger = document.activeElement;
  var modal  = document.getElementById('structure-modal');
  var cont   = document.getElementById('modal-structure');
  var title  = (name || smiles).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;');
  if (formula) title += ' · ' + formatFormula(formula);
  document.getElementById('modal-name').innerHTML = title;
  cont.innerHTML = '';
  modal.classList.add("is-open");
  document.body.style.overflow = 'hidden';
  document.querySelector('.modal-close').focus();

  var toggle = document.getElementById('modal-view-toggle');
  toggle.style.display = _modalCID ? 'flex' : 'none';
  setModalView('2d');
  if (_modalCID) prefetchConformer(_modalCID);

  var drawer  = new SmilesDrawer.SmiDrawer({ width: 420, height: 360, padding: 24, bondThickness: 1.6, isomeric: true, explicitHydrogens: false });
  drawer.draw(smiles, null, currentTheme(), function(svgEl) {
    cont.appendChild(svgEl);
  }, function() {
    cont.innerHTML = '<p style="color:var(--error-text);padding:1rem;font-family:var(--font-mono);font-size:0.8rem">Could not render structure.</p>';
  });
}

function closeStructureModal() {
  document.getElementById('structure-modal').classList.remove('is-open');
  document.body.style.overflow = '';
  _modalSmiles = null; _modalName = null; _modalFormula = null; _modalCID = null;
  _modalViewer = null;
  if (_modalTrigger) { _modalTrigger.focus(); _modalTrigger = null; }
}

/* ── 3D structure viewer (shared: inline card + modal) ─────────────── */
var _3dmolPromise   = null;
var _conformerCache = {};   // cid -> Promise<SDF text>
var _modalViewer    = null;
var _inlineViewer   = null;

function load3Dmol() {
  if (typeof $3Dmol !== 'undefined') return Promise.resolve();
  if (_3dmolPromise) return _3dmolPromise;
  _3dmolPromise = new Promise(function(resolve, reject) {
    var s = document.createElement('script');
    s.src = 'https://unpkg.com/3dmol/build/3Dmol-min.js';
    s.onload  = resolve;
    s.onerror = reject;
    document.head.appendChild(s);
  });
  return _3dmolPromise;
}

function fetchConformer(cid) {
  if (!_conformerCache[cid]) {
    _conformerCache[cid] = fetch('/api/v1/conformer?cid=' + encodeURIComponent(cid) + '&format=sdf')
      .then(function(r) { if (!r.ok) throw new Error('no 3D conformer'); return r.text(); })
      .catch(function(err) { delete _conformerCache[cid]; throw err; });
  }
  return _conformerCache[cid];
}

// Fires as soon as a CID is known, so the 3D data is already cached by the
// time the user clicks the 3D toggle — avoids a visible loading state.
function prefetchConformer(cid) {
  if (!cid) return;
  load3Dmol().catch(function() {});
  fetchConformer(cid).catch(function() {});
}

function renderViewerInto(container, cid) {
  container.innerHTML = '<p class="modal-3d-status">Loading 3D structure…</p>';
  return load3Dmol().then(function() {
    return fetchConformer(cid);
  }).then(function(sdf) {
    container.innerHTML = '';
    var bgColor = getComputedStyle(document.documentElement).getPropertyValue('--structure-bg').trim();
    var viewer = $3Dmol.createViewer(container, { backgroundColor: bgColor || '#ffffff' });
    viewer.addModel(sdf, 'sdf');
    viewer.setStyle({}, { stick: { radius: 0.15 }, sphere: { scale: 0.25 } });
    viewer.zoomTo();
    viewer.render();
    return viewer;
  }).catch(function(err) {
    container.innerHTML = '<p class="modal-3d-status" style="color:var(--error-text)">No 3D conformer available for this compound.</p>';
    throw err;
  });
}

function setModalView(view) {
  var is3d = view === '3d';
  document.querySelectorAll('#modal-view-toggle .view-toggle-btn').forEach(function(btn) {
    btn.classList.toggle('is-active', btn.dataset.view === view);
  });
  document.getElementById('modal-structure').style.display    = is3d ? 'none' : '';
  document.getElementById('modal-structure-3d').style.display = is3d ? '' : 'none';
  if (is3d && !_modalViewer) {
    renderViewerInto(document.getElementById('modal-structure-3d'), _modalCID)
      .then(function(v) { _modalViewer = v; })
      .catch(function() { _modalViewer = null; });
  }
}

function setInlineView(col, view) {
  var is3d = view === '3d';
  col.querySelectorAll('.inline-view-toggle .view-toggle-btn').forEach(function(btn) {
    btn.classList.toggle('is-active', btn.dataset.view === view);
  });
  var wrap2d = col.querySelector('.structure-wrap');
  var frame3d = col.querySelector('.structure-3d-frame');
  var wrap3d = frame3d.querySelector('.structure-wrap-3d');
  wrap2d.style.display  = is3d ? 'none' : '';
  frame3d.style.display = is3d ? '' : 'none';
  if (is3d && !_inlineViewer) {
    renderViewerInto(wrap3d, wrap2d.dataset.cid)
      .then(function(v) { _inlineViewer = v; })
      .catch(function() { _inlineViewer = null; });
  }
}

function trapModalTab(e) {
  if (e.key !== 'Tab') return;
  if (!document.getElementById('structure-modal').classList.contains('is-open')) return;
  var box = document.querySelector('#structure-modal .modal-box');
  var focusable = box.querySelectorAll('button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])');
  if (!focusable.length) return;
  var first = focusable[0], last = focusable[focusable.length - 1];
  if (e.shiftKey && document.activeElement === first) {
    e.preventDefault(); last.focus();
  } else if (!e.shiftKey && document.activeElement === last) {
    e.preventDefault(); first.focus();
  }
}

function drawStructure(el) {
  if (!el || typeof SmilesDrawer === 'undefined') return;
  var smiles  = el.getAttribute('data-smiles');
  var name    = el.getAttribute('data-name') || smiles;
  var formula = el.getAttribute('data-formula') || '';
  var cid     = el.getAttribute('data-cid');
  var dlName  = cid ? name + '_CID' + cid : name;
  var drawer  = new SmilesDrawer.SmiDrawer({ width: 160, height: 140, padding: 10, bondThickness: 1.2, isomeric: true, explicitHydrogens: false });
  drawer.draw(smiles, null, currentTheme(), function(svgEl) {
    el.innerHTML = '';
    el.appendChild(svgEl);
    el.setAttribute('data-sd-rendered', '1');
    el.style.display = '';
    el.style.cursor  = 'zoom-in';
    el.onclick = function() { openStructureModal(smiles, name, formula, cid); };
    // Single lookup only: prefetch the 3D conformer so the inline 2D/3D
    // toggle switches instantly. Batch rows skip this (only fetched on modal open).
    if (cid && el.closest('#lookup-result')) {
      _inlineViewer = null;
      prefetchConformer(cid);
    }
  }, function() { /* keep hidden */ });
}

/* ── Suggestion keyboard navigation ────────────────────────────── */
var _sugIdx = -1;
var _sugOriginal = '';
var _sugBlocked = false;

function _sugItems() {
  return Array.from(document.querySelectorAll('#lookup-suggestions .suggest-item'));
}

function _sugHighlight(items, idx) {
  var input = document.getElementById('lookup-input');
  items.forEach(function(el, i) {
    var active = i === idx;
    el.classList.toggle('suggest-item--active', active);
    el.setAttribute('aria-selected', active ? 'true' : 'false');
  });
  if (idx >= 0 && items[idx]) {
    items[idx].scrollIntoView({block: 'nearest'});
    input.value = items[idx].querySelector('.suggest-name').textContent;
    input.setAttribute('aria-activedescendant', items[idx].id);
  } else {
    input.value = _sugOriginal;
    input.removeAttribute('aria-activedescendant');
  }
}

function _sugClose() {
  var input = document.getElementById('lookup-input');
  var drop  = document.getElementById('lookup-suggestions');
  if (_sugIdx >= 0) input.value = _sugOriginal;
  drop.innerHTML = '';
  input.setAttribute('aria-expanded', 'false');
  input.removeAttribute('aria-activedescendant');
  _sugIdx = -1;
}

function _sugAfterSwap() {
  var drop  = document.getElementById('lookup-suggestions');
  var input = document.getElementById('lookup-input');
  if (_sugBlocked || !drop.textContent.trim()) { drop.innerHTML = ''; return; }
  input.setAttribute('aria-expanded', drop.children.length > 0 ? 'true' : 'false');
  _sugIdx = -1;
  _sugOriginal = input.value;
}

/* ── Batch progress ─────────────────────────────────────────────── */
var _batchES = null;

function cancelBatch() {
  if (_batchES) { _batchES.close(); _batchES = null; }
  hideBatchProgress();
}

function startBatch(e) {
  e.preventDefault();
  var form      = document.getElementById('batch-form');
  var submitBtn = form.querySelector('.btn[type="submit"]');
  if (submitBtn) submitBtn.disabled = true;
  var data = new FormData(form);
  var synonymsCb = document.getElementById('opt-synonyms');
  var ghsCb      = document.getElementById('opt-ghs');
  data.set('opt_synonyms', synonymsCb && !synonymsCb.checked ? '0' : '1');
  data.set('opt_ghs',      ghsCb && ghsCb.checked ? '1' : '0');

  document.getElementById('batch-result').innerHTML = '';
  document.getElementById('batch-reset').style.display = 'flex';
  showBatchProgress(0, 'Starting…');

  fetch('/batch/start', { method: 'POST', body: data })
    .then(function(r) { return r.json(); })
    .then(function(res) {
      if (res.error) { hideBatchProgress(); showBatchError(res.error); return; }
      var es = new EventSource('/batch/stream?job=' + res.job);
      _batchES = es;

      es.addEventListener('progress', function(ev) {
        var parts = ev.data.split('/');
        var done = parseInt(parts[0], 10);
        var tot  = parseInt(parts[1], 10);
        showBatchProgress(done / tot, done + ' / ' + tot);
      });

      es.addEventListener('done', function(ev) {
        es.close(); _batchES = null;
        hideBatchProgress();
        document.getElementById('batch-result').innerHTML = ev.data;
        document.querySelectorAll('.batch-table .structure-wrap[data-smiles]:not([data-sd-rendered])').forEach(function(el) {
          if (!el.closest('.expand-row')) drawStructure(el);
        });
      });

      es.addEventListener('error', function(ev) {
        es.close(); _batchES = null;
        hideBatchProgress();
        showBatchError(ev.data || 'Could not reach PubChem — please try again.');
      });

      es.onerror = function() {
        es.close(); _batchES = null;
        hideBatchProgress();
        showBatchError('Connection lost — please try again.');
      };
    })
    .catch(function() {
      hideBatchProgress();
      showBatchError('Could not reach server.');
    });
}

function showBatchProgress(fraction, label) {
  var wrap   = document.getElementById('batch-progress-wrap');
  var fill   = document.getElementById('batch-progress-fill');
  var ind    = document.getElementById('batch-indicator');
  var cancel = document.getElementById('batch-cancel');
  wrap.style.display   = 'flex';
  fill.style.width     = (fraction * 100) + '%';
  ind.textContent      = label || 'Processing';
  ind.style.display    = 'inline-flex';
  cancel.style.display = 'inline-flex';
}

function hideBatchProgress() {
  var wrap      = document.getElementById('batch-progress-wrap');
  var fill      = document.getElementById('batch-progress-fill');
  var cancel    = document.getElementById('batch-cancel');
  var submitBtn = document.querySelector('#batch-form .btn[type="submit"]');
  wrap.style.display   = 'none';
  fill.style.width     = '0%';
  cancel.style.display = 'none';
  if (submitBtn) submitBtn.disabled = false;
}

function updateSummaryPills() {
  var rows = document.querySelectorAll('.batch-table tbody tr[data-status]');
  var found = 0, notFound = 0, errors = 0;
  rows.forEach(function(r) {
    var s = r.dataset.status;
    if (s === 'found') found++;
    else if (s === 'notfound') notFound++;
    else errors++;
  });
  var vp = document.querySelector('.summary-pill.valid');
  var wp = document.querySelector('.summary-pill.warn');
  var ip = document.querySelector('.summary-pill.invalid');
  if (vp) vp.textContent = found + ' found';
  if (wp) wp.textContent = notFound + ' not found';
  if (ip) { ip.textContent = errors + ' errors'; ip.style.display = errors > 0 ? '' : 'none'; }
}

function toggleBatchRow(row) {
  var expandRow = row.nextElementSibling;
  if (!expandRow || !expandRow.classList.contains('expand-row')) return;
  var isOpen = expandRow.style.display !== 'none';
  document.querySelectorAll('.expand-row').forEach(function(r) { r.style.display = 'none'; });
  document.querySelectorAll('.expandable-row').forEach(function(r) { r.classList.remove('row-expanded'); });
  if (!isOpen) {
    expandRow.style.display = '';
    row.classList.add('row-expanded');
    var wrap = expandRow.querySelector('.structure-wrap[data-smiles]:not([data-sd-rendered])');
    if (wrap) drawStructure(wrap);
  }
}

function retryRow(btn) {
  var input = btn.dataset.input;
  var row   = btn.closest('tr');
  btn.disabled    = true;
  btn.textContent = '…';

  var data = new FormData();
  data.append('inputs', input);

  fetch('/batch/start', { method: 'POST', body: data })
    .then(function(r) { return r.json(); })
    .then(function(res) {
      if (res.error) { btn.disabled = false; btn.textContent = '↺'; return; }
      var es = new EventSource('/batch/stream?job=' + res.job);
      es.addEventListener('done', function(ev) {
        es.close();
        var doc    = new DOMParser().parseFromString(ev.data, 'text/html');
        var newTrs = Array.from(doc.querySelectorAll('tbody tr'));
        if (!newTrs.length) { btn.disabled = false; btn.textContent = '↺'; return; }
        var next = row.nextElementSibling;
        if (next && next.classList.contains('expand-row')) next.remove();
        var frag = document.createDocumentFragment();
        newTrs.forEach(function(tr) { frag.appendChild(tr); });
        row.replaceWith(frag);
        updateSummaryPills();
      });
      es.addEventListener('error', function() { es.close(); btn.disabled = false; btn.textContent = '↺'; });
      es.onerror = function() { es.close(); btn.disabled = false; btn.textContent = '↺'; };
    })
    .catch(function() { btn.disabled = false; btn.textContent = '↺'; });
}


function sortBatchTable(th, colIdx, mode) {
  var table  = th.closest('table');
  var tbody  = table.querySelector('tbody');
  var allThs = th.closest('tr').querySelectorAll('th');

  var asc = !th.classList.contains('sort-asc');
  allThs.forEach(function(h) { h.classList.remove('sort-asc','sort-desc'); });
  th.classList.add(asc ? 'sort-asc' : 'sort-desc');
  var ind = th.querySelector('.sort-indicator');
  if (ind) ind.textContent = asc ? '↑' : '↓';
  allThs.forEach(function(h) {
    if (h !== th) { var i = h.querySelector('.sort-indicator'); if (i) i.textContent = '↕'; }
  });

  var pairs = [];
  var rows  = Array.from(tbody.children);
  var i = 0;
  while (i < rows.length) {
    var row = rows[i];
    if (row.classList.contains('expand-row')) { i++; continue; }
    var next      = rows[i + 1];
    var expandRow = (next && next.classList.contains('expand-row')) ? next : null;
    pairs.push([row, expandRow]);
    i += expandRow ? 2 : 1;
  }

  pairs.sort(function(a, b) {
    var av, bv;
    if (mode === 'status') {
      av = a[0].dataset.status || '';
      bv = b[0].dataset.status || '';
    } else {
      var ac = a[0].cells[colIdx], bc = b[0].cells[colIdx];
      av = ac ? ac.textContent.trim() : '';
      bv = bc ? bc.textContent.trim() : '';
    }
    if (mode === 'num') {
      av = parseFloat(av) || 0;
      bv = parseFloat(bv) || 0;
      return asc ? av - bv : bv - av;
    }
    return asc ? av.localeCompare(bv) : bv.localeCompare(av);
  });

  document.querySelectorAll('.expand-row').forEach(function(r) { r.style.display = 'none'; });
  document.querySelectorAll('.expandable-row').forEach(function(r) { r.classList.remove('row-expanded'); });
  pairs.forEach(function(pair) {
    tbody.appendChild(pair[0]);
    if (pair[1]) tbody.appendChild(pair[1]);
  });
}

function applyBatchFilter(term) {
  term = term.toLowerCase().trim();
  document.querySelectorAll('.batch-table tbody tr[data-status]').forEach(function(row) {
    var visible = !term || row.textContent.toLowerCase().includes(term);
    row.style.display = visible ? '' : 'none';
    var expandRow = row.nextElementSibling;
    if (expandRow && expandRow.classList.contains('expand-row')) {
      if (!visible) {
        expandRow.style.display = 'none';
      } else if (row.classList.contains('row-expanded')) {
        expandRow.style.display = '';
      }
    }
  });
}

function showBatchError(msg) {
  document.getElementById('batch-result').innerHTML =
    '<div class="result-box error mt-md"><span class="status-badge error-tag">Error</span><p>' +
    msg.replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;') + '</p></div>';
}

/* ── Event delegation & wiring ──────────────────────────────────── */
document.addEventListener('DOMContentLoaded', function() {

  // Theme toggle
  document.querySelector('.theme-toggle').addEventListener('click', toggleTheme);

  // Lookup reset
  document.getElementById('lookup-reset').addEventListener('click', function() {
    document.getElementById('lookup-form').reset();
    document.getElementById('lookup-result').innerHTML = '';
    document.getElementById('lookup-suggestions').innerHTML = '';
    this.style.display = 'none';
  });

  // Batch reset
  document.getElementById('batch-reset').addEventListener('click', function() {
    document.getElementById('batch-form').reset();
    document.getElementById('batch-result').innerHTML = '';
    hideBatchProgress();
    this.style.display = 'none';
  });

  // Batch form submit
  document.getElementById('batch-form').addEventListener('submit', startBatch);

  // Ctrl/Cmd+Enter submits the batch form from the textarea
  document.getElementById('batch-inputs').addEventListener('keydown', function(e) {
    if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
      e.preventDefault();
      document.getElementById('batch-form').requestSubmit();
    }
  });

  // Cancel batch
  document.getElementById('batch-cancel').addEventListener('click', cancelBatch);

  // Lookup input blur (close suggestions)
  document.getElementById('lookup-input').addEventListener('blur', function() {
    setTimeout(_sugClose, 200);
  });

  // Suggest item selection via mousedown (delegation on dropdown)
  document.getElementById('lookup-suggestions').addEventListener('mousedown', function(e) {
    var item = e.target.closest('.suggest-item');
    if (!item) return;
    e.preventDefault();
    var value = item.dataset.value;
    var input = document.getElementById('lookup-input');
    input.value = value;
    _sugIdx = -1;
    document.getElementById('lookup-suggestions').innerHTML = '';
    input.setAttribute('aria-expanded', 'false');
    document.getElementById('lookup-form').requestSubmit();
  });

  // Lookup form: htmx beforeRequest → button animation + suggestion block
  document.getElementById('lookup-form').addEventListener('htmx:beforeRequest', function(evt) {
    if (evt.detail.requestConfig.verb !== 'post') return;
    document.getElementById('lookup-reset').style.display = 'flex';
    _sugClose();
    _sugBlocked = true;
    setTimeout(function() { _sugBlocked = false; }, 600);
    var btn = this.querySelector('.btn');
    btn.disabled = true;
    btn.classList.add('btn--pressed');
    setTimeout(function() { btn.classList.remove('btn--pressed'); }, 150);
  });

  document.getElementById('lookup-form').addEventListener('htmx:afterRequest', function(evt) {
    if (evt.detail.requestConfig.verb !== 'post') return;
    this.querySelector('.btn').disabled = false;
  });

  // Modal overlay click — only close if BOTH the press and release landed
  // directly on the backdrop. Uses document-level capture-phase listeners
  // (fire before any stopPropagation() further down, e.g. in 3Dmol's own
  // canvas handlers) and checks mousedown/mouseup targets directly instead
  // of relying on synthesized click events, whose target can end up being
  // the backdrop (the common ancestor) even when a drag merely started on
  // the 3D viewer and released outside the modal box.
  var _overlayDragStartedOnBackdrop = false;
  var _structureModalEl = document.getElementById('structure-modal');
  document.addEventListener('mousedown', function(e) {
    _overlayDragStartedOnBackdrop = (e.target === _structureModalEl);
  }, true);
  document.addEventListener('mouseup', function(e) {
    if (_overlayDragStartedOnBackdrop && e.target === _structureModalEl && _structureModalEl.classList.contains('is-open')) {
      closeStructureModal();
    }
    _overlayDragStartedOnBackdrop = false;
  }, true);

  // Modal close button
  document.querySelector('.modal-close').addEventListener('click', closeStructureModal);

  // Modal 2D/3D view toggle
  document.getElementById('modal-view-toggle').addEventListener('click', function(e) {
    var btn = e.target.closest('.view-toggle-btn');
    if (btn) setModalView(btn.dataset.view);
  });

  // Keyboard: Escape closes modal; Arrow keys / Enter for suggestions
  document.addEventListener('keydown', function(e) {
    if (e.key === 'Escape') {
      closeStructureModal();
      _sugClose();
      return;
    }
    trapModalTab(e);
    if (document.activeElement !== document.getElementById('lookup-input')) return;
    var items = _sugItems();
    if (!items.length) { _sugIdx = -1; return; }
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      if (_sugIdx === -1) _sugOriginal = document.getElementById('lookup-input').value;
      _sugIdx = Math.min(_sugIdx + 1, items.length - 1);
      _sugHighlight(items, _sugIdx);
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      _sugIdx = Math.max(_sugIdx - 1, -1);
      _sugHighlight(items, _sugIdx);
    } else if (e.key === 'Enter' && _sugIdx >= 0) {
      e.preventDefault();
      var item = items[_sugIdx];
      _sugIdx = -1;
      item.dispatchEvent(new MouseEvent('mousedown', {bubbles: true}));
    }
  });

  // Delegation: all batch result interactions
  document.addEventListener('click', function(e) {
    // Stop propagation from expand card (prevents toggling row when clicking inside card)
    if (e.target.closest('.expand-card')) return;

    // Sort column header
    var th = e.target.closest('th.sortable');
    if (th) {
      sortBatchTable(th, parseInt(th.dataset.sortCol, 10), th.dataset.sortMode || 'text');
      return;
    }

    // Retry button
    var retryBtn = e.target.closest('.btn-retry');
    if (retryBtn) { e.stopPropagation(); retryRow(retryBtn); return; }

    // Expand row toggle
    var row = e.target.closest('.expandable-row');
    if (row) { toggleBatchRow(row); return; }
  });

  // "Did you mean?" suggestion clicks
  document.getElementById('lookup-result').addEventListener('click', function(e) {
    var sug = e.target.closest('.suggestion-btn');
    if (sug) {
      document.getElementById('lookup-input').value = sug.dataset.value;
      document.getElementById('lookup-form').requestSubmit();
      return;
    }
  });

  // Delegation: copy buttons + structure actions in lookup result
  document.getElementById('lookup-result').addEventListener('click', function(e) {
    var btn = e.target.closest('.copy-btn');
    if (btn) {
      var text = btn.dataset.copy || (btn.previousElementSibling && btn.previousElementSibling.textContent);
      if (text) copyText(btn, text.trim());
      return;
    }
    var viewToggleBtn = e.target.closest('.inline-view-toggle .view-toggle-btn');
    if (viewToggleBtn) {
      setInlineView(viewToggleBtn.closest('.structure-col'), viewToggleBtn.dataset.view);
      return;
    }
    var expand3dBtn = e.target.closest('.structure-3d-expand');
    if (expand3dBtn) {
      var wrap = expand3dBtn.closest('.structure-col').querySelector('.structure-wrap');
      openStructureModal(wrap.dataset.smiles, wrap.dataset.name, wrap.dataset.formula, wrap.dataset.cid);
      setModalView('3d');
      return;
    }
    var copyBtn = e.target.closest('.str-copy-btn');
    if (copyBtn) {
      var col = copyBtn.closest('.structure-col');
      var frame3d = col.querySelector('.structure-3d-frame');
      if (frame3d && frame3d.style.display !== 'none' && _inlineViewer) {
        copyPngDataUri(_inlineViewer.pngURI(), copyBtn);
      } else {
        var svgEl = col.querySelector('.structure-wrap svg');
        if (svgEl) copyAsPng(svgEl, copyBtn);
      }
      return;
    }
    var dlToggle = e.target.closest('.str-dl-btn');
    if (dlToggle) {
      var menu = dlToggle.nextElementSibling;
      var is3dActive = dlToggle.closest('.structure-col').querySelector('.structure-3d-frame').style.display !== 'none';
      var svgOpt = menu.querySelector('.dl-opt[data-dl="svg"]');
      if (svgOpt) svgOpt.style.display = is3dActive ? 'none' : '';
      toggleDlMenu(menu);
      return;
    }
    var dlOpt = e.target.closest('.dl-opt');
    if (dlOpt && dlOpt.closest('.str-dl-wrap')) {
      var col = dlOpt.closest('.structure-col');
      var wrap = col.querySelector('.structure-wrap');
      var frame3d = col.querySelector('.structure-3d-frame');
      var is3dActive = frame3d.style.display !== 'none';
      var svgEl = wrap && wrap.querySelector('svg');
      var n = wrap && (wrap.dataset.name || wrap.dataset.smiles);
      var c = wrap && wrap.dataset.cid;
      var dlName = c ? (n||'structure')+'_CID'+c : (n||'structure');
      var fmt = dlOpt.dataset.dl;
      if (fmt==='png' && is3dActive && _inlineViewer) downloadPngDataUri(_inlineViewer.pngURI(), dlName);
      else if (fmt==='svg')      downloadSvg(svgEl, dlName);
      else if (fmt==='png') downloadPng(svgEl, dlName);
      else if (fmt==='sdf') downloadFromCid(c, dlName, 'sdf');
      else if (fmt==='xyz') downloadFromCid(c, dlName, 'xyz');
      dlOpt.closest('.dl-menu').style.display = 'none'; _openDlMenu = null;
      return;
    }
  });

  // Modal copy + download
  function modalIs3DActive() {
    return document.getElementById('modal-structure-3d').style.display !== 'none';
  }
  document.getElementById('modal-copy-btn').addEventListener('click', function() {
    if (modalIs3DActive() && _modalViewer) {
      copyPngDataUri(_modalViewer.pngURI(), this);
    } else {
      var svgEl = document.querySelector('#modal-structure svg');
      if (svgEl) copyAsPng(svgEl, this);
    }
  });
  document.getElementById('modal-dl-btn').addEventListener('click', function() {
    var menu = document.getElementById('modal-dl-menu');
    var svgOpt = menu.querySelector('.dl-opt[data-dl="svg"]');
    if (svgOpt) svgOpt.style.display = modalIs3DActive() ? 'none' : '';
    toggleDlMenu(menu);
  });
  document.getElementById('modal-dl-menu').addEventListener('click', function(e) {
    var opt = e.target.closest('.dl-opt');
    if (!opt) return;
    var svgEl = document.querySelector('#modal-structure svg');
    var dlName = _modalCID ? (_modalName||'structure')+'_CID'+_modalCID : (_modalName||'structure');
    var fmt = opt.dataset.dl;
    if (fmt==='png' && modalIs3DActive() && _modalViewer) downloadPngDataUri(_modalViewer.pngURI(), dlName);
    else if (fmt==='svg')      downloadSvg(svgEl, dlName);
    else if (fmt==='png') downloadPng(svgEl, dlName);
    else if (fmt==='sdf') downloadFromCid(_modalCID, dlName, 'sdf');
    else if (fmt==='xyz') downloadFromCid(_modalCID, dlName, 'xyz');
    this.style.display = 'none'; _openDlMenu = null;
  });

  // Close any open dropdown on outside click
  document.addEventListener('click', function(e) {
    if (_openDlMenu && !e.target.closest('.str-dl-wrap') && !e.target.closest('#modal-dl-wrap')) {
      _openDlMenu.style.display = 'none'; _openDlMenu = null;
    }
  }, true);

  // Batch result filter
  document.getElementById('batch-result').addEventListener('input', function(e) {
    if (e.target.classList.contains('batch-filter')) applyBatchFilter(e.target.value);
  });

  // htmx: after suggestion swap
  document.addEventListener('htmx:afterSwap', function(evt) {
    if (evt.detail && evt.detail.target && evt.detail.target.id === 'lookup-suggestions') {
      _sugAfterSwap();
    }
  });

  // htmx: render structures after settle
  document.addEventListener('htmx:afterSettle', function() {
    document.querySelectorAll('.structure-wrap[data-smiles]:not([data-sd-rendered])').forEach(drawStructure);
  });
});
