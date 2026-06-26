/* ── Theme ─────────────────────────────────────────────────────── */
function toggleTheme() {
  var next = document.documentElement.getAttribute('data-theme') === 'light' ? 'dark' : 'light';
  document.documentElement.setAttribute('data-theme', next);
  document.cookie = 'chemres-theme=' + next + ';path=/;max-age=31536000;SameSite=Lax';

  document.querySelectorAll('.structure-wrap[data-sd-rendered]').forEach(function(el) {
    el.removeAttribute('data-sd-rendered');
    el.innerHTML = '';
    drawStructure(el);
  });

  if (_modalSmiles && document.getElementById('structure-modal').classList.contains('is-open')) {
    openStructureModal(_modalSmiles, _modalName, _modalFormula);
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
      var icon  = btn.querySelector('.png-icon,.structure-png-icon');
      var check = btn.querySelector('.png-check,.structure-png-check');
      function flash() {
        if (icon)  icon.style.display  = 'none';
        if (check) check.style.display = 'block';
        setTimeout(function() {
          if (icon)  icon.style.display  = '';
          if (check) check.style.display = 'none';
        }, 1500);
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

/* ── Structure modal ────────────────────────────────────────────── */
var _modalSmiles = null, _modalName = null, _modalFormula = null;

function openStructureModal(smiles, name, formula) {
  _modalSmiles = smiles; _modalName = name; _modalFormula = formula;
  var modal  = document.getElementById('structure-modal');
  var cont   = document.getElementById('modal-structure');
  var title  = (name || smiles).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;');
  if (formula) title += ' · ' + formatFormula(formula);
  document.getElementById('modal-name').innerHTML = title;
  cont.innerHTML = '';
  modal.classList.add("is-open");
  document.body.style.overflow = 'hidden';

  var largest = smiles.split('.').reduce(function(a, b) { return a.length >= b.length ? a : b; });
  var drawer  = new SmilesDrawer.SmiDrawer({ width: 420, height: 360, padding: 24, bondThickness: 1.6, isomeric: true, explicitHydrogens: false });
  drawer.draw(largest, null, currentTheme(), function(svgEl) {
    cont.appendChild(svgEl);
    var pngBtn = document.getElementById('modal-png-btn');
    document.getElementById('modal-dl-btn').onclick = function() { downloadSvg(svgEl, name); };
    pngBtn.onclick = function() { copyAsPng(svgEl, pngBtn); };
  }, function() {
    cont.innerHTML = '<p style="color:var(--error-text);padding:1rem;font-family:var(--font-mono);font-size:0.8rem">Could not render structure.</p>';
  });
}

function closeStructureModal() {
  document.getElementById('structure-modal').classList.remove('is-open');
  document.body.style.overflow = '';
  _modalSmiles = null; _modalName = null; _modalFormula = null;
}

function drawStructure(el) {
  if (!el || typeof SmilesDrawer === 'undefined') return;
  var smiles  = el.getAttribute('data-smiles');
  var name    = el.getAttribute('data-name') || smiles;
  var formula = el.getAttribute('data-formula') || '';
  var cid     = el.getAttribute('data-cid');
  var dlName  = cid ? name + '_CID' + cid : name;
  var largest = smiles.split('.').reduce(function(a, b) { return a.length >= b.length ? a : b; });
  var drawer  = new SmilesDrawer.SmiDrawer({ width: 160, height: 140, padding: 10, bondThickness: 1.2, isomeric: true, explicitHydrogens: false });
  drawer.draw(largest, null, currentTheme(), function(svgEl) {
    el.innerHTML = '';
    el.appendChild(svgEl);

    var dlBtn = document.createElement('button');
    dlBtn.className = 'structure-dl';
    dlBtn.title = 'Download SVG';
    dlBtn.innerHTML = '<svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M8 2v8M5 7l3 3 3-3"/><line x1="2" y1="13" x2="14" y2="13"/></svg>';
    dlBtn.addEventListener('click', function(e) { e.stopPropagation(); downloadSvg(svgEl, dlName); });
    el.appendChild(dlBtn);

    var cpBtn = document.createElement('button');
    cpBtn.className = 'structure-dl structure-dl--png';
    cpBtn.title = 'Copy PNG';
    cpBtn.innerHTML = '<svg class="structure-png-icon" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="2" y="4" width="9" height="11" rx="1"/><path d="M5 4V2.5A1.5 1.5 0 0 1 6.5 1h7A1.5 1.5 0 0 1 15 2.5v9A1.5 1.5 0 0 1 13.5 13H12"/></svg><svg class="structure-png-check" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" style="display:none"><polyline points="2.5,8.5 6,12 13.5,4.5"/></svg>';
    cpBtn.addEventListener('click', function(e) { e.stopPropagation(); copyAsPng(svgEl, cpBtn); });
    el.appendChild(cpBtn);

    el.setAttribute('data-sd-rendered', '1');
    el.style.display = '';
    el.style.cursor  = 'zoom-in';
    el.onclick = function() { openStructureModal(smiles, name, formula); };
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
    btn.classList.add('btn--pressed');
    setTimeout(function() { btn.classList.remove('btn--pressed'); }, 150);
  });

  // Modal overlay click
  document.getElementById('structure-modal').addEventListener('click', function(e) {
    if (e.target === this) closeStructureModal();
  });

  // Modal close button
  document.querySelector('.modal-close').addEventListener('click', closeStructureModal);

  // Keyboard: Escape closes modal; Arrow keys / Enter for suggestions
  document.addEventListener('keydown', function(e) {
    if (e.key === 'Escape') {
      closeStructureModal();
      _sugClose();
      return;
    }
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

    // Modal overlay
    if (e.target === document.getElementById('structure-modal')) { closeStructureModal(); return; }
  });

  // Delegation: copy buttons in lookup result
  document.getElementById('lookup-result').addEventListener('click', function(e) {
    var btn = e.target.closest('.copy-btn');
    if (btn) {
      var sib = btn.previousElementSibling;
      if (sib) copyText(btn, sib.textContent);
    }
  });

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
