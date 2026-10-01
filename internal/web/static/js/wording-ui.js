let wordingEditor = null;
let wordingGenerateOpen = false;
let wordingLivePreview = true;
let wordingVariables = [];
let wordingPresets = [];
let wordingLastField = null;
let wordingLiveTimer = null;

function renderMessageWording() {
  API.get('/api/me/mail-templates').then((lib) => {
    const templates = lib.templates || [];
    wordingVariables = lib.variables || [];
    wordingPresets = lib.presets || [];
    const editing = wordingEditor;
    const cards = templates.length
      ? `<div class="template-grid">${templates.map((t) => `
          <article class="card template-card">
            <div class="template-card-body">
              <h3>${escapeHTML(t.name)}${t.active ? ' <span class="badge">In use</span>' : ''}</h3>
              <p class="template-subject">${escapeHTML(wordingSubjectPreview(t.email_subject))}</p>
            </div>
            <div class="settings-actions template-card-actions">
              <button class="btn btn-sm btn-primary" data-use-wording="${t.id}"${t.active ? ' disabled' : ''}>Use</button>
              <button class="btn btn-sm" data-view-wording="${t.id}">View as mail</button>
              <button class="btn btn-sm" data-edit-wording="${t.id}">Edit</button>
              <button class="btn btn-sm" data-duplicate-wording="${t.id}">Duplicate</button>
              <button class="btn btn-sm btn-danger" data-delete-wording="${t.id}">Delete</button>
            </div>
          </article>`).join('')}</div>`
      : (editing || wordingPasteOpen || wordingGenerateOpen
        ? ''
        : emptyState('No mail templates yet', 'Generate one with AI or start from a built-in design.', ''));
    const genBox = wordingGenerateOpen ? `
      <div class="card channel-card" id="wording-generate-box">
        <h3>Generate with AI</h3>
        <p class="channel-sub">Describe the design in a sentence or two — tone, colors, what it's for. You review the result before anything is saved.</p>
        <div class="field"><textarea id="w-ai-brief" rows="3" placeholder="e.g. A calm, minimal bill reminder in warm amber with a big pay-by date and a friendly tone"></textarea></div>
        <div class="settings-actions">
          <button class="btn btn-primary" id="w-ai-go">Generate template</button>
          <button class="btn" id="w-ai-cancel">Cancel</button>
        </div>
      </div>` : '';
    const presetsRow = (!editing && !wordingGenerateOpen && wordingPresets.length) ? `
      <div class="wording-presets">
        <p class="preview-label">Start from a design</p>
        <div class="chip-row">${wordingPresets.map((p, i) => `
          <button class="btn btn-sm preset-chip" data-preset-wording="${i}">${escapeHTML(p.name)}</button>`).join('')}</div>
      </div>` : '';
    const chips = editing ? `
      <div class="wording-chips">
        <p class="preview-label">Variables — click to insert</p>
        <div class="chip-row">${wordingVariables.map((v) => `
          <button class="chip" data-insert-var="${escapeHTML(v.Name)}" title="${escapeHTML(v.Description)}">${escapeHTML(v.Name)}</button>`).join('')}</div>
      </div>` : '';
    const form = editing ? `
      <div class="card channel-card">
        <h3>${editing.mode === 'add' ? 'New mail template' : 'Edit mail template'}</h3>
        <p class="channel-sub">Every field supports the variables above. {{if .Advance}}…{{else}}…{{end}} switches between the early warning and the due notice.</p>
        <div class="field"><label for="w-name">Name</label><input id="w-name" value="${escapeHTML(editing.values.name || '')}"></div>
        <div class="field"><label for="w-subject">Email subject</label><input id="w-subject" value="${escapeHTML(editing.values.email_subject || '')}"></div>
        <div class="mail-design">
          <div class="field"><label for="w-html">Email body (HTML)</label><textarea id="w-html" rows="12" spellcheck="false">${escapeHTML(editing.values.email_html || '')}</textarea></div>
          <div class="field"><label for="w-css">Email CSS</label><textarea id="w-css" rows="12" spellcheck="false">${escapeHTML(editing.values.email_css || '')}</textarea></div>
        </div>
        <div class="field"><label for="w-text">Email plain text</label><textarea id="w-text" rows="5" spellcheck="false">${escapeHTML(editing.values.email_text || '')}</textarea></div>
        <div class="field"><label for="w-tg">Telegram message</label><textarea id="w-tg" rows="5" spellcheck="false">${escapeHTML(editing.values.telegram_text || '')}</textarea></div>
        ${chips}
        <div class="settings-actions">
          <button class="btn btn-primary" id="w-save">${editing.mode === 'add' ? 'Add template' : 'Save template'}</button>
          <button class="btn" id="w-view-mail">View as mail</button>
          <button class="btn" id="w-preview">Preview</button>
          <button class="btn" id="w-cancel">Cancel</button>
          <label class="live-toggle"><input type="checkbox" id="w-live" ${wordingLivePreview ? 'checked' : ''}> Live preview</label>
        </div>
      </div>
      <div id="w-preview-out"></div>
      <div id="w-live-out"></div>` : '';
    const pasteBox = wordingPasteOpen ? `
      <div class="card channel-card" id="wording-paste-box">
        <h3>Paste AI result</h3>
        <p class="channel-sub">Paste the JSON from your AI assistant, then fill the form.</p>
        <div class="field"><textarea id="wording-paste-json" rows="10" spellcheck="false" placeholder='{"name":"…","email_subject":"…",…}'></textarea></div>
        <div class="settings-actions">
          <button class="btn btn-primary" id="wording-paste-apply">Fill form</button>
          <button class="btn" id="wording-paste-cancel">Cancel</button>
        </div>
      </div>` : '';
    const main = `
      <h1 class="page-title">Message wording</h1>
      <p class="page-sub">Design how reminder emails and messages read. The template in use is sent with every alert.</p>
      <div class="settings-actions" style="margin-bottom:16px;flex-wrap:wrap">
        <button class="btn btn-primary" id="w-generate-open" type="button">✨ Generate with AI</button>
        <button class="btn" id="w-add">Add template</button>
        <button class="btn" id="wording-copy-prompt" type="button">Copy AI prompt</button>
        <button class="btn" id="wording-paste-open" type="button">Paste AI result</button>
      </div>
      ${genBox}
      ${pasteBox}
      ${cards}
      ${presetsRow}
      ${form}
      <div id="wording-view-out"></div>
      <a href="#/settings" class="btn btn-ghost btn-sm" style="margin-top:16px">← Settings</a>
    `;
    document.getElementById('app').innerHTML = shellHTML('/settings', main);
    bindShell('/settings');
    bindWordingEditor(templates);
  }).catch((err) => {
    document.getElementById('app').innerHTML = shellHTML('/settings', `<h1 class="page-title">Message wording</h1>${emptyState('Could not load templates', escapeHTML(err.message))}`);
    bindShell('/settings');
  });
}

function readWordingForm() {
  return {
    name: document.getElementById('w-name').value,
    email_subject: document.getElementById('w-subject').value,
    email_html: document.getElementById('w-html').value,
    email_css: document.getElementById('w-css').value,
    email_text: document.getElementById('w-text').value,
    telegram_text: document.getElementById('w-tg').value,
  };
}

function insertAtCursor(el, text) {
  if (!el) return;
  const start = el.selectionStart ?? el.value.length;
  const end = el.selectionEnd ?? el.value.length;
  el.value = el.value.slice(0, start) + text + el.value.slice(end);
  el.selectionStart = el.selectionEnd = start + text.length;
  el.focus();
  el.dispatchEvent(new Event('input', { bubbles: true }));
}

function scheduleLiveWordingPreview(previewAPI) {
  if (!wordingLivePreview) return;
  clearTimeout(wordingLiveTimer);
  wordingLiveTimer = setTimeout(async () => {
    const out = document.getElementById('w-live-out');
    if (!out) return;
    try {
      const res = await API.post(previewAPI, readWordingForm());
      const p = res.due || {};
      out.innerHTML = `
        <div class="card channel-card mail-preview-card">
          <h3>Live preview — at the due time</h3>
          ${(p.warnings || []).map((w) => `<p class="test-result bad">${escapeHTML(w)}</p>`).join('')}
          <div class="mail-preview-inbox">
            <span class="mail-preview-from">Day Remind</span>
            <p class="mail-preview-subject">${escapeHTML(p.subject || '(no subject)')}</p>
          </div>
          <iframe class="preview-frame mail-preview-frame" sandbox srcdoc="${escapeAttr(mailPreviewDocument(p.html || ''))}" title="Live email preview"></iframe>
        </div>`;
    } catch { /* keep the last good preview; the Preview button reports errors */ }
  }, 700);
}

function bindWordingEditor(templates) {
  const previewAPI = '/api/me/templates/preview';
  const formFields = ['w-name', 'w-subject', 'w-html', 'w-css', 'w-text', 'w-tg'];

  async function showViewAsMail(payload) {
    const out = document.getElementById('wording-view-out') || document.getElementById('w-preview-out');
    if (!out) return;
    out.innerHTML = '<p class="page-sub">Loading preview…</p>';
    try {
      const res = await API.post(previewAPI, payload);
      out.innerHTML = mailPreviewAsInboxHTML(res);
      out.scrollIntoView({ behavior: 'smooth', block: 'start' });
    } catch (err) {
      out.innerHTML = `<p class="test-result bad">${escapeHTML(err.message)}</p>`;
    }
  }

  document.getElementById('w-generate-open')?.addEventListener('click', () => {
    wordingGenerateOpen = true;
    wordingPasteOpen = false;
    if (!wordingEditor) wordingEditor = { mode: 'add', values: {} };
    renderMessageWording();
    document.getElementById('w-ai-brief')?.focus();
  });
  document.getElementById('w-ai-cancel')?.addEventListener('click', () => {
    wordingGenerateOpen = false;
    if (!wordingEditor.values || (!wordingEditor.values.name && !wordingEditor.values.email_html)) wordingEditor = null;
    renderMessageWording();
  });
  document.getElementById('w-ai-go')?.addEventListener('click', async () => {
    const brief = document.getElementById('w-ai-brief')?.value || '';
    if (!brief.trim()) {
      showToast('Describe the design first', 'error');
      return;
    }
    const btn = document.getElementById('w-ai-go');
    btn.disabled = true;
    btn.textContent = 'Generating…';
    try {
      const result = await API.post('/api/me/mail-wordings/generate', { brief });
      wordingEditor = { mode: 'add', values: result };
      wordingGenerateOpen = false;
      showToast('Draft ready — review and save');
      renderMessageWording();
    } catch (err) {
      showToast(err.message, 'error');
      btn.disabled = false;
      btn.textContent = 'Generate template';
    }
  });

  document.querySelectorAll('[data-preset-wording]').forEach((btn) => {
    btn.addEventListener('click', () => {
      const preset = wordingPresets[Number(btn.dataset.presetWording)];
      if (!preset) return;
      wordingEditor = { mode: 'add', values: { ...preset } };
      renderMessageWording();
    });
  });

  document.querySelectorAll('[data-insert-var]').forEach((btn) => {
    btn.addEventListener('click', () => {
      insertAtCursor(wordingLastField || document.getElementById('w-text'), btn.dataset.insertVar);
    });
  });
  formFields.forEach((id) => {
    const el = document.getElementById(id);
    if (!el) return;
    el.addEventListener('focusin', () => { wordingLastField = el; });
    el.addEventListener('input', () => scheduleLiveWordingPreview(previewAPI));
  });
  document.getElementById('w-live')?.addEventListener('change', (e) => {
    wordingLivePreview = e.target.checked;
    if (!wordingLivePreview) {
      clearTimeout(wordingLiveTimer);
      const out = document.getElementById('w-live-out');
      if (out) out.innerHTML = '';
    } else {
      scheduleLiveWordingPreview(previewAPI);
    }
  });

  document.getElementById('wording-copy-prompt')?.addEventListener('click', () => copyMailAIPrompt());
  document.getElementById('wording-paste-open')?.addEventListener('click', () => {
    wordingPasteOpen = true;
    if (!wordingEditor) wordingEditor = { mode: 'add', values: {} };
    renderMessageWording();
  });
  document.getElementById('wording-paste-cancel')?.addEventListener('click', () => {
    wordingPasteOpen = false;
    renderMessageWording();
  });
  document.getElementById('wording-paste-apply')?.addEventListener('click', () => {
    const raw = document.getElementById('wording-paste-json')?.value || '';
    try {
      const vals = parseMailAIResult(raw);
      wordingEditor = { mode: 'add', values: vals };
      wordingPasteOpen = false;
      showToast('Form filled — review and save');
      renderMessageWording();
    } catch {
      showToast('Invalid JSON — check the AI response', 'error');
    }
  });

  document.getElementById('w-add')?.addEventListener('click', () => {
    wordingEditor = { mode: 'add', values: {} };
    wordingPasteOpen = false;
    wordingGenerateOpen = false;
    renderMessageWording();
  });
  document.getElementById('w-cancel')?.addEventListener('click', () => {
    wordingEditor = null;
    renderMessageWording();
  });
  document.querySelectorAll('[data-edit-wording]').forEach((btn) => {
    btn.addEventListener('click', () => {
      const found = templates.find((t) => t.id === btn.dataset.editWording);
      if (!found) return;
      wordingEditor = { mode: 'edit', id: found.id, values: found };
      renderMessageWording();
    });
  });
  document.querySelectorAll('[data-duplicate-wording]').forEach((btn) => {
    btn.addEventListener('click', () => {
      const found = templates.find((t) => t.id === btn.dataset.duplicateWording);
      if (!found) return;
      wordingEditor = { mode: 'add', values: { ...wordingPayloadFromTemplate(found), name: (found.name || 'Template') + ' copy' } };
      renderMessageWording();
    });
  });
  document.querySelectorAll('[data-view-wording]').forEach((btn) => {
    btn.addEventListener('click', async () => {
      const found = templates.find((t) => t.id === btn.dataset.viewWording);
      if (!found) {
        showToast('That template is gone', 'error');
        return;
      }
      await showViewAsMail(wordingPayloadFromTemplate(found));
    });
  });
  document.querySelectorAll('[data-use-wording]').forEach((btn) => {
    btn.addEventListener('click', async () => {
      try {
        await API.post(`/api/me/mail-templates/${btn.dataset.useWording}/use`);
        showToast('Template in use');
        wordingEditor = null;
        renderMessageWording();
      } catch (err) {
        showToast(err.message, 'error');
      }
    });
  });
  document.querySelectorAll('[data-delete-wording]').forEach((btn) => {
    btn.addEventListener('click', async () => {
      if (!confirm('Delete this mail template?')) return;
      try {
        await API.delete(`/api/me/mail-templates/${btn.dataset.deleteWording}`);
        showToast('Template deleted');
        if (wordingEditor && wordingEditor.id === btn.dataset.deleteWording) wordingEditor = null;
        renderMessageWording();
      } catch (err) {
        showToast(err.message, 'error');
      }
    });
  });
  document.getElementById('w-save')?.addEventListener('click', async () => {
    const vals = readWordingForm();
    try {
      if (wordingEditor.mode === 'add') await API.post('/api/me/mail-templates', vals);
      else await API.request(`/api/me/mail-templates/${wordingEditor.id}`, { method: 'PUT', body: JSON.stringify(vals) });
      showToast(wordingEditor.mode === 'add' ? 'Template added' : 'Template saved');
      wordingEditor = null;
      renderMessageWording();
    } catch (err) {
      showToast(err.message, 'error');
    }
  });
  document.getElementById('w-view-mail')?.addEventListener('click', () => {
    showViewAsMail(readWordingForm());
  });
  document.getElementById('w-preview')?.addEventListener('click', async () => {
    const out = document.getElementById('w-preview-out');
    try {
      const res = await API.post(previewAPI, readWordingForm());
      out.innerHTML = ['due', 'advance'].map((kind) => {
        const p = res[kind];
        return `<div class="card preview-card"><h3>${kind === 'due' ? 'At the due time' : 'Advance notice'}</h3>
          <p class="preview-subject">${escapeHTML(p.subject || '')}</p>
          <iframe class="preview-frame" sandbox srcdoc="${escapeAttr(mailPreviewDocument(p.html || ''))}" title="Email preview"></iframe>
          <pre class="preview-pre">${escapeHTML(p.telegram || p.text || '')}</pre></div>`;
      }).join('');
      out.scrollIntoView({ behavior: 'smooth', block: 'start' });
    } catch (err) {
      out.innerHTML = `<p class="test-result bad">${escapeHTML(err.message)}</p>`;
    }
  });
}
