let wordingEditor = null;

function renderMessageWording() {
  API.get('/api/me/mail-templates').then((lib) => {
    const templates = lib.templates || [];
    const editing = wordingEditor;
    const cards = templates.length
      ? `<div class="template-grid">${templates.map((t) => `
          <article class="card template-card">
            <h3>${escapeHTML(t.name)}${t.active ? ' <span class="badge">In use</span>' : ''}</h3>
            <p class="page-sub">${escapeHTML(t.email_subject || 'Default subject')}</p>
            <div class="settings-actions">
              <button class="btn btn-sm btn-primary" data-use-wording="${t.id}"${t.active ? ' disabled' : ''}>Use</button>
              <button class="btn btn-sm" data-view-wording="${t.id}">View as mail</button>
              <button class="btn btn-sm" data-edit-wording="${t.id}">Edit</button>
              <button class="btn btn-sm btn-danger" data-delete-wording="${t.id}">Delete</button>
            </div>
          </article>`).join('')}</div>`
      : (editing || wordingPasteOpen
        ? ''
        : emptyState('No mail templates yet', 'Add one to design the email and the message.', ''));
    const form = editing ? `
      <div class="card channel-card">
        <h3>${editing.mode === 'add' ? 'New mail template' : 'Edit mail template'}</h3>
        <p class="channel-sub">Use {{.Title}}, {{.Description}}, {{.DueAt}}, {{.Priority}}, {{.Advance}} and {{.LeadTime}}.</p>
        <div class="field"><label for="w-name">Name</label><input id="w-name" value="${escapeHTML(editing.values.name || '')}"></div>
        <div class="field"><label for="w-subject">Email subject</label><input id="w-subject" value="${escapeHTML(editing.values.email_subject || '')}"></div>
        <div class="mail-design">
          <div class="field"><label for="w-html">Email body (HTML)</label><textarea id="w-html" rows="12" spellcheck="false">${escapeHTML(editing.values.email_html || '')}</textarea></div>
          <div class="field"><label for="w-css">Email CSS</label><textarea id="w-css" rows="12" spellcheck="false">${escapeHTML(editing.values.email_css || '')}</textarea></div>
        </div>
        <div class="field"><label for="w-text">Email plain text</label><textarea id="w-text" rows="5" spellcheck="false">${escapeHTML(editing.values.email_text || '')}</textarea></div>
        <div class="field"><label for="w-tg">Telegram message</label><textarea id="w-tg" rows="5" spellcheck="false">${escapeHTML(editing.values.telegram_text || '')}</textarea></div>
        <div class="settings-actions">
          <button class="btn btn-primary" id="w-save">${editing.mode === 'add' ? 'Add template' : 'Save template'}</button>
          <button class="btn" id="w-view-mail">View as mail</button>
          <button class="btn" id="w-preview">Preview</button>
          <button class="btn" id="w-cancel">Cancel</button>
        </div>
      </div>
      <div id="w-preview-out"></div>` : '';
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
      <p class="page-sub">Design how reminder emails and messages read. Use a template to send with it.</p>
      <div class="settings-actions" style="margin-bottom:16px;flex-wrap:wrap">
        <button class="btn btn-primary" id="w-add">Add template</button>
        <button class="btn" id="wording-copy-prompt" type="button">Copy AI prompt</button>
        <button class="btn" id="wording-paste-open" type="button">Paste AI result</button>
      </div>
      ${pasteBox}
      ${cards}
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

function bindWordingEditor(templates) {
  const previewAPI = '/api/me/templates/preview';

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
          <iframe class="preview-frame" sandbox srcdoc="${escapeHTML(p.html || '')}" title="Email preview"></iframe>
          <pre class="preview-pre">${escapeHTML(p.telegram || p.text || '')}</pre></div>`;
      }).join('');
    } catch (err) {
      out.innerHTML = `<p class="test-result bad">${escapeHTML(err.message)}</p>`;
    }
  });
}
