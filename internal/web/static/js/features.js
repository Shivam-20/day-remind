/* Phases 4–12: calendar, lists, settings sub-pages, enhanced drawers */
window.DR_FEATURE_ROUTES = {
  '/today': renderTodayEnhanced,
  '/reminders': renderRemindersEnhanced,
  '/calendar': renderCalendar,
  '/lists': renderLists,
  '/settings': renderSettingsEnhanced,
  '/settings/data': renderDataSettings,
  '/settings/templates': renderTemplateGallery,
  '/settings/wording': renderMessageWording,
  '/settings/google': renderGoogleSettings,
  '/settings/account': renderAccountSettings,
};

const featureState = {
  lists: [],
  tags: [],
  searchQuery: '',
  listFilter: '',
  tagFilter: '',
  priorityFilter: '',
  calendarMonth: new Date(),
  selectedDay: null,
};

const OFFSET_PRESETS = [
  { label: '15m', minutes: 15 },
  { label: '1h', minutes: 60 },
  { label: '1d', minutes: 1440 },
  { label: '1w', minutes: 10080 },
];

const RECURRENCE_PRESETS = [
  { value: '', label: 'None' },
  { value: 'daily', label: 'Daily' },
  { value: 'weekly', label: 'Weekly' },
  { value: 'monthly', label: 'Monthly' },
  { value: 'yearly', label: 'Yearly' },
];

function buildRecurrenceRule(preset, dueLocal) {
  if (!preset) return '';
  const freq = { daily: 'DAILY', weekly: 'WEEKLY', monthly: 'MONTHLY', yearly: 'YEARLY' }[preset];
  if (!freq) return '';
  const dt = new Date(dueLocal);
  const pad = (n) => String(n).padStart(2, '0');
  const dtstart = `${dt.getUTCFullYear()}${pad(dt.getUTCMonth() + 1)}${pad(dt.getUTCDate())}T${pad(dt.getUTCHours())}${pad(dt.getUTCMinutes())}${pad(dt.getUTCSeconds())}Z`;
  return `DTSTART:${dtstart}\nRRULE:FREQ=${freq}`;
}

async function loadListsAndTags() {
  const [listsData, tagsData] = await Promise.all([
    API.get('/api/lists'),
    API.get('/api/tags'),
  ]);
  featureState.lists = listsData.lists || [];
  featureState.tags = tagsData.tags || [];
  return featureState;
}

async function renderCalendar() {
  const year = featureState.calendarMonth.getFullYear();
  const month = featureState.calendarMonth.getMonth();
  const from = new Date(year, month, 1);
  const to = new Date(year, month + 1, 0, 23, 59, 59);
  const data = await API.get(`/api/calendar?from=${encodeURIComponent(from.toISOString())}&to=${encodeURIComponent(to.toISOString())}`);
  const events = data.events || [];
  const byDay = {};
  events.forEach((ev) => {
    const key = ev.start.slice(0, 10);
    if (!byDay[key]) byDay[key] = [];
    byDay[key].push(ev);
  });

  const firstDow = new Date(year, month, 1).getDay();
  const daysInMonth = new Date(year, month + 1, 0).getDate();
  const monthLabel = featureState.calendarMonth.toLocaleDateString(undefined, { month: 'long', year: 'numeric' });
  let cells = '';
  for (let i = 0; i < firstDow; i++) {
    cells += '<div class="cal-cell cal-empty"></div>';
  }
  const todayKey = new Date().toISOString().slice(0, 10);
  for (let d = 1; d <= daysInMonth; d++) {
    const key = `${year}-${String(month + 1).padStart(2, '0')}-${String(d).padStart(2, '0')}`;
    const count = (byDay[key] || []).length;
    const isToday = key === todayKey;
    const isSelected = featureState.selectedDay === key;
    cells += `<button type="button" class="cal-cell${isToday ? ' cal-today' : ''}${isSelected ? ' cal-selected' : ''}" data-day="${key}">
      <span class="cal-day-num">${d}</span>
      ${count ? `<span class="cal-dot" title="${count} reminder(s)"></span>` : ''}
    </button>`;
  }

  const selectedEvents = featureState.selectedDay ? (byDay[featureState.selectedDay] || []) : [];
  const sidePanel = featureState.selectedDay
    ? `<div class="cal-side card">
        <h3>${new Date(featureState.selectedDay + 'T12:00:00').toLocaleDateString(undefined, { weekday: 'long', month: 'short', day: 'numeric' })}</h3>
        <div class="reminder-list">
          ${selectedEvents.length
            ? selectedEvents.map((ev) => reminderCardHTML(
                { id: ev.id, title: ev.title, due_at: ev.start, status: ev.status, priority: ev.priority },
                reminderActionBtn({ action: 'detail', id: ev.id, label: 'Details', icon: 'info' })
              )).join('')
            : emptyState('No reminders', 'Nothing scheduled this day.')}
        </div>
      </div>`
    : '<div class="cal-side card"><p class="page-sub">Click a day to see reminders</p></div>';

  const main = `
    <h1 class="page-title">Calendar</h1>
    <div class="cal-toolbar">
      <button class="btn btn-sm" id="cal-prev" type="button">←</button>
      <strong>${monthLabel}</strong>
      <button class="btn btn-sm" id="cal-next" type="button">→</button>
      <button class="btn btn-sm" id="cal-today" type="button">Today</button>
    </div>
    <div class="cal-layout">
      <div class="cal-grid-wrap card">
        <div class="cal-weekdays">
          <span>Sun</span><span>Mon</span><span>Tue</span><span>Wed</span><span>Thu</span><span>Fri</span><span>Sat</span>
        </div>
        <div class="cal-grid">${cells}</div>
      </div>
      ${sidePanel}
    </div>
  `;
  document.getElementById('app').innerHTML = shellHTML('/calendar', main);
  bindShell('/calendar');
  document.getElementById('cal-prev')?.addEventListener('click', () => {
    featureState.calendarMonth = new Date(year, month - 1, 1);
    featureState.selectedDay = null;
    renderCalendar();
  });
  document.getElementById('cal-next')?.addEventListener('click', () => {
    featureState.calendarMonth = new Date(year, month + 1, 1);
    featureState.selectedDay = null;
    renderCalendar();
  });
  document.getElementById('cal-today')?.addEventListener('click', () => {
    featureState.calendarMonth = new Date();
    featureState.selectedDay = new Date().toISOString().slice(0, 10);
    renderCalendar();
  });
  document.querySelectorAll('.cal-cell[data-day]').forEach((btn) => {
    btn.addEventListener('click', () => {
      featureState.selectedDay = btn.dataset.day;
      renderCalendar();
    });
  });
  bindReminderActions(document.getElementById('app'), () => renderCalendar());
}

async function renderLists() {
  const data = await API.get('/api/lists');
  const lists = data.lists || [];
  const countLabel = lists.length === 1 ? '1 list' : `${lists.length} lists`;
  const cards = lists.map((l) => `
        <article class="list-card card" style="--list-color:${escapeHTML(l.color || '#3b82f6')}">
          <div class="list-card-stripe"></div>
          <div class="list-card-body">
            <h3>${escapeHTML(l.name)}</h3>
            <p class="page-sub">${l.count ?? 0} active · ${l.paused ? 'Paused' : 'Active'}</p>
          </div>
          <div class="list-card-actions">
            ${l.paused
              ? `<button class="btn btn-sm" data-resume="${l.id}">Resume</button>`
              : `<button class="btn btn-sm" data-pause="${l.id}">Pause</button>`}
          </div>
        </article>
      `).join('');
  const body = lists.length
    ? `<div class="list-cards">${cards}</div>
       <p class="list-page-footer"><a href="#/reminders" class="btn btn-ghost btn-sm">View all reminders</a></p>`
    : emptyState(
        'No lists yet',
        'Create a list to organize reminders.',
        '<button class="btn btn-primary" data-new-list>Create list</button><a href="#/reminders" class="btn btn-ghost btn-sm">View all reminders</a>'
      );

  const main = `
    <div class="page-header-row">
      <div class="page-header-text">
        <h1 class="page-title">Lists</h1>
        <p class="page-sub">${countLabel}</p>
      </div>
      <button class="btn btn-primary btn-sm" data-new-list>+ New list</button>
    </div>
    ${body}
  `;
  document.getElementById('app').innerHTML = shellHTML('/lists', main);
  bindShell('/lists');
  document.querySelectorAll('[data-new-list]').forEach((btn) => {
    btn.addEventListener('click', openCreateListModal);
  });
  document.querySelectorAll('[data-pause]').forEach((btn) => {
    btn.addEventListener('click', async () => {
      await API.post(`/api/lists/${btn.dataset.pause}/pause`);
      showToast('List paused');
      renderLists();
    });
  });
  document.querySelectorAll('[data-resume]').forEach((btn) => {
    btn.addEventListener('click', async () => {
      await API.post(`/api/lists/${btn.dataset.resume}/resume`);
      showToast('List resumed');
      renderLists();
    });
  });
}

function openCreateListModal() {
  const drawer = openDrawer('New list', `
    <div class="field"><label for="list-name">Name</label><input id="list-name" required></div>
    <div class="field"><label for="list-color">Color</label><input id="list-color" type="color" value="#3b82f6"></div>
  `, `<button class="btn" type="button" data-close-drawer>Cancel</button><button class="btn btn-primary" id="save-list">Create</button>`);
  drawer.querySelector('[data-close-drawer]').addEventListener('click', closeDrawer);
  drawer.querySelector('#save-list').addEventListener('click', async () => {
    const name = document.getElementById('list-name').value.trim();
    if (!name) return showToast('Name required', 'error');
    try {
      await API.post('/api/lists', { name, color: document.getElementById('list-color').value });
      closeDrawer();
      showToast('List created');
      renderLists();
    } catch (err) {
      showToast(err.message, 'error');
    }
  });
}

async function renderDataSettings() {
  const main = `
    <h1 class="page-title">Data</h1>
    <p class="page-sub">Export backups or import reminders</p>
    <div class="card" style="margin-bottom:16px">
      <h3>Export</h3>
      <p class="page-sub">Download your reminders and settings</p>
      <div class="inline-actions">
        <a class="btn btn-primary btn-sm" href="${API.url('/api/export/json')}" download>Export JSON</a>
        <a class="btn btn-sm" href="${API.url('/api/export/ical')}" download>Export iCal</a>
      </div>
    </div>
    <div class="card">
      <h3>Import</h3>
      <p class="page-sub">Upload a JSON or iCal backup</p>
      <div class="field">
        <label for="import-mode">Mode</label>
        <select id="import-mode">
          <option value="merge">Merge (skip existing)</option>
          <option value="merge_update">Merge & update</option>
          <option value="replace">Replace all</option>
        </select>
      </div>
      <div class="import-dropzone" id="import-dropzone">
        <p>Drop a file here or click to browse</p>
        <input type="file" id="import-file" accept=".json,.ics,.ical" hidden>
      </div>
      <pre id="import-preview" class="link-out" hidden></pre>
      <button class="btn btn-primary" id="import-run" disabled>Import</button>
    </div>
    <a href="#/settings" class="btn btn-ghost btn-sm" style="margin-top:16px">← Back to settings</a>
  `;
  document.getElementById('app').innerHTML = shellHTML('/settings/data', main);
  bindShell('/settings/data');
  let pendingFile = null;
  const dropzone = document.getElementById('import-dropzone');
  const fileInput = document.getElementById('import-file');
  dropzone.addEventListener('click', () => fileInput.click());
  dropzone.addEventListener('dragover', (e) => { e.preventDefault(); dropzone.classList.add('drag-over'); });
  dropzone.addEventListener('dragleave', () => dropzone.classList.remove('drag-over'));
  dropzone.addEventListener('drop', (e) => {
    e.preventDefault();
    dropzone.classList.remove('drag-over');
    if (e.dataTransfer.files[0]) handleImportFile(e.dataTransfer.files[0]);
  });
  fileInput.addEventListener('change', () => {
    if (fileInput.files[0]) handleImportFile(fileInput.files[0]);
  });

  async function handleImportFile(file) {
    pendingFile = file;
    document.getElementById('import-run').disabled = false;
    dropzone.querySelector('p').textContent = file.name;
    if (file.name.endsWith('.json')) {
      const text = await file.text();
      try {
        const preview = await API.post('/api/import/json/preview', JSON.parse(text));
        const pre = document.getElementById('import-preview');
        pre.hidden = false;
        pre.textContent = JSON.stringify(preview, null, 2);
      } catch (err) {
        showToast(err.message, 'error');
      }
    }
  }

  document.getElementById('import-run').addEventListener('click', async () => {
    if (!pendingFile) return;
    const mode = document.getElementById('import-mode').value;
    const form = new FormData();
    form.append('file', pendingFile);
    const isJson = pendingFile.name.endsWith('.json');
    const url = API.url(isJson ? `/api/import/json?mode=${encodeURIComponent(mode)}` : '/api/import/ical');
    try {
      const res = await fetch(url, { method: 'POST', credentials: 'include', body: isJson ? await pendingFile.text() : form });
      const data = await res.json();
      if (!res.ok) throw new Error(data.error || 'import failed');
      showToast('Import complete');
      document.getElementById('import-preview').hidden = false;
      document.getElementById('import-preview').textContent = JSON.stringify(data, null, 2);
    } catch (err) {
      showToast(err.message, 'error');
    }
  });
}

async function renderTemplateGallery() {
  const data = await API.get('/api/templates');
  const templates = data.templates || [];
  const cards = templates.map((t) => `
    <article class="card template-card">
      <h3>${escapeHTML(t.name)}${t.is_system ? ' <span class="badge">System</span>' : ''}</h3>
      <p class="page-sub">${escapeHTML(t.description || t.title)}</p>
      <p class="page-sub">Priority: ${escapeHTML(t.priority)} · Offsets: ${(t.offsets || []).join(', ') || 'none'}</p>
      <button class="btn btn-sm btn-primary" data-use-template="${t.id}">Use template</button>
    </article>
  `).join('');
  const main = `
    <h1 class="page-title">Templates</h1>
    <p class="page-sub">Start from a preset reminder pattern</p>
    <div class="template-grid">${cards || emptyState('No templates', 'Templates will appear here.')}</div>
    <a href="#/settings" class="btn btn-ghost btn-sm" style="margin-top:16px">← Back to settings</a>
  `;
  document.getElementById('app').innerHTML = shellHTML('/settings/templates', main);
  bindShell('/settings/templates');
  document.querySelectorAll('[data-use-template]').forEach((btn) => {
    btn.addEventListener('click', async () => {
      try {
        await API.post(`/api/templates/${btn.dataset.useTemplate}/use`, {});
        showToast('Reminder created from template');
        location.hash = '#/reminders';
        renderApp('/reminders');
      } catch (err) {
        showToast(err.message, 'error');
      }
    });
  });
}

async function renderGoogleSettings() {
  let data = { connected: false };
  try {
    data = await API.get('/api/integrations/google');
  } catch (err) {
    showToast(err.message, 'error');
  }
  const main = data.connected ? `
    <h1 class="page-title">Google</h1>
    <p class="page-sub">Connected as ${escapeHTML(data.email || '')}</p>
    <div class="card" style="margin-bottom:16px">
      <label class="checkbox-label"><input type="checkbox" id="g-cal" ${data.calendar_enabled ? 'checked' : ''}> Calendar sync</label>
      <label class="checkbox-label"><input type="checkbox" id="g-cal-pause" ${data.calendar_paused ? 'checked' : ''}> Pause calendar</label>
      <label class="checkbox-label"><input type="checkbox" id="g-mail" ${data.gmail_enabled ? 'checked' : ''}> Gmail ingest</label>
      <label class="checkbox-label"><input type="checkbox" id="g-mail-pause" ${data.gmail_paused ? 'checked' : ''}> Pause Gmail</label>
      <div class="inline-actions" style="margin-top:12px">
        <button class="btn btn-primary btn-sm" id="g-save">Save</button>
        <button class="btn btn-sm" id="g-sync">Sync now</button>
        <button class="btn btn-danger btn-sm" id="g-disconnect">Disconnect</button>
      </div>
    </div>
    <a href="#/settings" class="btn btn-ghost btn-sm">← Back to settings</a>
  ` : `
    <h1 class="page-title">Google</h1>
    <p class="page-sub">Connect Google Calendar and Gmail</p>
    <div class="card">
      <button class="btn btn-primary" id="g-connect">Connect Google account</button>
    </div>
    <a href="#/settings" class="btn btn-ghost btn-sm" style="margin-top:16px">← Back to settings</a>
  `;
  document.getElementById('app').innerHTML = shellHTML('/settings/google', main);
  bindShell('/settings/google');
  document.getElementById('g-connect')?.addEventListener('click', async () => {
    try {
      const res = await API.post('/api/integrations/google/connect');
      if (res.redirect_url) window.location.href = res.redirect_url;
    } catch (err) {
      showToast(err.message, 'error');
    }
  });
  document.getElementById('g-save')?.addEventListener('click', async () => {
    try {
      if (document.getElementById('g-cal-pause').checked) {
        await API.post('/api/integrations/google/calendar/pause');
      } else {
        await API.post('/api/integrations/google/calendar/resume');
      }
      if (document.getElementById('g-mail-pause').checked) {
        await API.post('/api/integrations/google/gmail/pause');
      } else {
        await API.post('/api/integrations/google/gmail/resume');
      }
      showToast('Google settings saved');
    } catch (err) {
      showToast(err.message, 'error');
    }
  });
  document.getElementById('g-sync')?.addEventListener('click', async () => {
    try {
      await API.post('/api/integrations/google/sync');
      showToast('Sync started');
    } catch (err) {
      showToast(err.message, 'error');
    }
  });
  document.getElementById('g-disconnect')?.addEventListener('click', async () => {
    if (!confirm('Disconnect Google?')) return;
    try {
      await API.delete('/api/integrations/google');
      showToast('Disconnected');
      renderGoogleSettings();
    } catch (err) {
      showToast(err.message, 'error');
    }
  });
}

async function renderAccountSettings() {
  let keys = [];
  let totp = { enabled: false };
  try {
    const [keysData, setupData] = await Promise.all([
      API.get('/api/api-keys'),
      fetch(API.url('/api/me/2fa/setup'), { method: 'POST', credentials: 'include' }).catch(() => null),
    ]);
    keys = keysData.api_keys || [];
    if (setupData?.ok) {
      totp = await setupData.json().catch(() => ({ enabled: false }));
    }
  } catch {
    /* keys may still load */
  }
  const keyRows = keys.length
    ? keys.map((k) => `
        <tr>
          <td>${escapeHTML(k.name)}</td>
          <td class="mono-cell">${escapeHTML(k.prefix)}…</td>
          <td class="mono-cell">${k.last_used_at ? new Date(k.last_used_at).toLocaleString() : '—'}</td>
          <td><button class="btn btn-sm btn-danger" data-revoke-key="${k.id}">Revoke</button></td>
        </tr>
      `).join('')
    : '<tr><td colspan="4" class="table-empty">No API keys</td></tr>';

  const main = `
    <h1 class="page-title">Account</h1>
    <p class="page-sub">API keys and two-factor authentication</p>
    <div class="card" style="margin-bottom:16px">
      <h3>API keys</h3>
      <div class="inline-actions">
        <input id="key-name" placeholder="Key name" aria-label="Key name">
        <button class="btn btn-primary btn-sm" id="create-key">Create key</button>
      </div>
      <pre id="new-key-token" class="link-out" hidden></pre>
      <div class="table-wrap" style="margin-top:12px">
        <table class="data-table">
          <thead><tr><th>Name</th><th>Prefix</th><th>Last used</th><th></th></tr></thead>
          <tbody>${keyRows}</tbody>
        </table>
      </div>
    </div>
    <div class="card">
      <h3>Two-factor authentication</h3>
      <p class="page-sub">Protect your account with an authenticator app</p>
      <div id="totp-setup">
        <button class="btn btn-sm" id="totp-begin">Set up 2FA</button>
        <div id="totp-enroll" hidden>
          <pre id="totp-uri" class="link-out"></pre>
          <div class="field"><label for="totp-code">Verification code</label><input id="totp-code" inputmode="numeric" autocomplete="one-time-code"></div>
          <button class="btn btn-primary btn-sm" id="totp-enable">Enable 2FA</button>
        </div>
        <div id="totp-disable" hidden>
          <div class="field"><label for="totp-disable-code">Code to disable</label><input id="totp-disable-code" inputmode="numeric"></div>
          <button class="btn btn-danger btn-sm" id="totp-disable-btn">Disable 2FA</button>
        </div>
      </div>
    </div>
    <a href="#/settings" class="btn btn-ghost btn-sm" style="margin-top:16px">← Back to settings</a>
  `;
  document.getElementById('app').innerHTML = shellHTML('/settings/account', main);
  bindShell('/settings/account');

  document.getElementById('create-key')?.addEventListener('click', async () => {
    const name = document.getElementById('key-name').value.trim() || 'API key';
    try {
      const res = await API.post('/api/api-keys', { name });
      const pre = document.getElementById('new-key-token');
      pre.hidden = false;
      pre.textContent = `Copy this token now — it won't be shown again:\n${res.token}`;
      renderAccountSettings();
    } catch (err) {
      showToast(err.message, 'error');
    }
  });
  document.querySelectorAll('[data-revoke-key]').forEach((btn) => {
    btn.addEventListener('click', async () => {
      if (!confirm('Revoke this API key?')) return;
      try {
        await API.delete(`/api/api-keys/${btn.dataset.revokeKey}`);
        showToast('Key revoked');
        renderAccountSettings();
      } catch (err) {
        showToast(err.message, 'error');
      }
    });
  });
  document.getElementById('totp-begin')?.addEventListener('click', async () => {
    try {
      const res = await API.post('/api/me/2fa/setup');
      document.getElementById('totp-enroll').hidden = false;
      document.getElementById('totp-uri').textContent = res.provisioning_uri || res.uri || res.secret;
    } catch (err) {
      showToast(err.message, 'error');
    }
  });
  document.getElementById('totp-enable')?.addEventListener('click', async () => {
    try {
      await API.post('/api/me/2fa/enable', { code: document.getElementById('totp-code').value.trim() });
      showToast('2FA enabled');
      renderAccountSettings();
    } catch (err) {
      showToast(err.message, 'error');
    }
  });
  document.getElementById('totp-disable-btn')?.addEventListener('click', async () => {
    try {
      await API.post('/api/me/2fa/disable', { code: document.getElementById('totp-disable-code').value.trim() });
      showToast('2FA disabled');
      renderAccountSettings();
    } catch (err) {
      showToast(err.message, 'error');
    }
  });
}

/* Enhanced Today page using dashboard stats */
async function renderTodayEnhanced() {
  let stats = { overdue: 0, today: 0, upcoming_7d: 0, done_count: 0, next_five: [] };
  try {
    stats = await API.get('/api/stats/dashboard');
  } catch {
    return renderTodayLegacy();
  }
  await loadReminders();
  const now = new Date();
  const today = state.reminders.filter((r) => {
    const d = new Date(r.due_at);
    return d.toDateString() === now.toDateString() && r.status !== 'done' && r.status !== 'cancelled';
  });
  const overdue = state.reminders.filter((r) => isOverdue(r));
  const upcoming = (stats.next_five || []).slice(0, 5);

  const main = `
    <h1 class="page-title">Today</h1>
    <p class="page-sub">${now.toLocaleDateString(undefined, { weekday: 'long', month: 'long', day: 'numeric' })}</p>
    <div class="stats-row">
      <div class="stat-card"><strong>${stats.today ?? today.length}</strong><span>Due today</span></div>
      <div class="stat-card"><strong>${stats.overdue ?? overdue.length}</strong><span>Overdue</span></div>
      <div class="stat-card"><strong>${stats.upcoming_7d ?? 0}</strong><span>Next 7 days</span></div>
      <div class="stat-card"><strong>${stats.done_count ?? 0}</strong><span>Done</span></div>
    </div>
    ${overdue.length ? `<h2 style="font-size:1rem;color:var(--danger)">Overdue</h2><div class="reminder-list">${overdue.map((r) => reminderCardHTML(r, reminderActions(r))).join('')}</div>` : ''}
    <h2 style="font-size:1rem;margin-top:24px">Today</h2>
    <div class="reminder-list" id="today-list">
      ${today.length ? today.map((r) => reminderCardHTML(r, reminderActions(r))).join('') : ''}
    </div>
    ${upcoming.length ? `<h2 style="font-size:1rem;margin-top:24px">Upcoming</h2><div class="reminder-list">${upcoming.map((r) => reminderCardHTML(r, reminderActions(r))).join('')}</div>` : ''}
  `;
  document.getElementById('app').innerHTML = shellHTML('/today', main);
  bindShell('/today');
  bindReminderActions(document.getElementById('app'), () => renderTodayEnhanced());
}

async function renderTodayLegacy() {
  await loadReminders();
  const now = new Date();
  const today = state.reminders.filter((r) => {
    const d = new Date(r.due_at);
    return d.toDateString() === now.toDateString() && r.status !== 'done' && r.status !== 'cancelled';
  });
  const overdue = state.reminders.filter((r) => isOverdue(r));
  const main = `
    <h1 class="page-title">Today</h1>
    <p class="page-sub">${now.toLocaleDateString(undefined, { weekday: 'long', month: 'long', day: 'numeric' })}</p>
    <div class="reminder-list">${today.map((r) => reminderCardHTML(r, reminderActions(r))).join('')}</div>
  `;
  document.getElementById('app').innerHTML = shellHTML('/today', main);
  bindShell('/today');
  bindReminderActions(document.getElementById('app'), () => renderTodayLegacy());
}

async function renderRemindersEnhanced() {
  const params = new URLSearchParams();
  if (state.filter !== 'all') params.set('status', state.filter);
  if (featureState.searchQuery) params.set('q', featureState.searchQuery);
  if (featureState.listFilter) params.set('list_id', featureState.listFilter);
  if (featureState.tagFilter) params.set('tag', featureState.tagFilter);
  if (featureState.priorityFilter) params.set('priority', featureState.priorityFilter);
  const qs = params.toString() ? `?${params}` : '';
  await loadReminders(qs);
  await loadListsAndTags().catch(() => {});

  const chips = ['all', 'scheduled', 'paused', 'done'].map((f) =>
    `<button class="filter-chip ${state.filter === f ? 'active' : ''}" data-filter="${f}">${f}</button>`
  ).join('');
  const listOpts = featureState.lists.map((l) =>
    `<option value="${l.id}" ${featureState.listFilter === l.id ? 'selected' : ''}>${escapeHTML(l.name)}</option>`
  ).join('');
  const tagOpts = featureState.tags.map((t) =>
    `<option value="${t.id}" ${featureState.tagFilter === t.id ? 'selected' : ''}>${escapeHTML(t.name)}</option>`
  ).join('');

  const main = `
    <h1 class="page-title">All reminders</h1>
    <p class="page-sub">${state.reminders.length} items</p>
    <div class="field" style="margin-bottom:12px">
      <input id="reminder-search" type="search" placeholder="Search reminders…" value="${escapeHTML(featureState.searchQuery)}">
    </div>
    <div class="filter-row">${chips}</div>
    <div class="admin-filter-grid" style="margin-bottom:16px">
      <div class="field" style="margin:0"><label for="filter-list">List</label><select id="filter-list"><option value="">All lists</option>${listOpts}</select></div>
      <div class="field" style="margin:0"><label for="filter-tag">Tag</label><select id="filter-tag"><option value="">All tags</option>${tagOpts}</select></div>
      <div class="field" style="margin:0"><label for="filter-priority">Priority</label>
        <select id="filter-priority">
          <option value="">Any</option>
          <option value="low" ${featureState.priorityFilter === 'low' ? 'selected' : ''}>Low</option>
          <option value="normal" ${featureState.priorityFilter === 'normal' ? 'selected' : ''}>Normal</option>
          <option value="high" ${featureState.priorityFilter === 'high' ? 'selected' : ''}>High</option>
          <option value="urgent" ${featureState.priorityFilter === 'urgent' ? 'selected' : ''}>Urgent</option>
        </select>
      </div>
    </div>
    <div class="reminder-list" id="all-list">
      ${state.reminders.length ? state.reminders.map((r) => reminderCardHTML(r, reminderActions(r))).join('') : emptyState('No reminders', 'Create one to get started.')}
    </div>
  `;
  document.getElementById('app').innerHTML = shellHTML('/reminders', main);
  bindShell('/reminders');
  bindReminderActions(document.getElementById('app'), () => renderRemindersEnhanced());
  document.querySelectorAll('[data-filter]').forEach((chip) => {
    chip.addEventListener('click', () => {
      state.filter = chip.dataset.filter;
      renderRemindersEnhanced();
    });
  });
  let searchTimer;
  document.getElementById('reminder-search')?.addEventListener('input', (e) => {
    clearTimeout(searchTimer);
    searchTimer = setTimeout(() => {
      featureState.searchQuery = e.target.value.trim();
      renderRemindersEnhanced();
    }, 300);
  });
  document.getElementById('filter-list')?.addEventListener('change', (e) => {
    featureState.listFilter = e.target.value;
    renderRemindersEnhanced();
  });
  document.getElementById('filter-tag')?.addEventListener('change', (e) => {
    featureState.tagFilter = e.target.value;
    renderRemindersEnhanced();
  });
  document.getElementById('filter-priority')?.addEventListener('change', (e) => {
    featureState.priorityFilter = e.target.value;
    renderRemindersEnhanced();
  });
}

async function openCreateDrawerEnhanced() {
  await loadListsAndTags().catch(() => {});
  const defaultDue = new Date();
  defaultDue.setHours(defaultDue.getHours() + 1, 0, 0, 0);
  const local = new Date(defaultDue.getTime() - defaultDue.getTimezoneOffset() * 60000).toISOString().slice(0, 16);
  const listOpts = featureState.lists.map((l) => `<option value="${l.id}">${escapeHTML(l.name)}</option>`).join('');
  const tagChips = featureState.tags.map((t) =>
    `<label class="tag-chip"><input type="checkbox" name="tags" value="${t.id}"> ${escapeHTML(t.name)}</label>`
  ).join('');
  const offsetChips = OFFSET_PRESETS.map((p) =>
    `<label class="tag-chip"><input type="checkbox" name="offsets" value="${p.minutes}"> ${p.label} before</label>`
  ).join('');
  const recurOpts = RECURRENCE_PRESETS.map((p) =>
    `<option value="${p.value}">${p.label}</option>`
  ).join('');

  const drawer = openDrawer('New reminder', `
    <div class="field"><label for="r-title">Title</label><input id="r-title" required></div>
    <div class="field"><label for="r-desc">Description</label><textarea id="r-desc" rows="3"></textarea></div>
    <div class="field"><label for="r-due">Due</label><input id="r-due" type="datetime-local" value="${local}" required></div>
    <div class="field"><label for="r-priority">Priority</label>
      <select id="r-priority"><option value="normal">Normal</option><option value="high">High</option><option value="urgent">Urgent</option><option value="low">Low</option></select>
    </div>
    <div class="field"><label for="r-list">List</label><select id="r-list"><option value="">No list</option>${listOpts}</select></div>
    ${tagChips ? `<div class="field"><label>Tags</label><div class="chip-row">${tagChips}</div></div>` : ''}
    <div class="field"><label for="r-recur">Recurrence</label><select id="r-recur">${recurOpts}</select></div>
    <div class="field"><label>Remind before</label><div class="chip-row">${offsetChips}</div></div>
  `, `<button class="btn" type="button" data-close-drawer>Cancel</button><button class="btn btn-primary" id="save-reminder">Save</button>`);
  drawer.querySelector('[data-close-drawer]').addEventListener('click', closeDrawer);
  drawer.querySelector('#save-reminder').addEventListener('click', async () => {
    const title = document.getElementById('r-title').value.trim();
    const dueLocal = document.getElementById('r-due').value;
    if (!title || !dueLocal) {
      showToast('Title and due date required', 'error');
      return;
    }
    const tagIDs = [...drawer.querySelectorAll('input[name="tags"]:checked')].map((el) => el.value);
    const offsets = [...drawer.querySelectorAll('input[name="offsets"]:checked')].map((el) => ({
      offset_minutes: parseInt(el.value, 10),
    }));
    const recurPreset = document.getElementById('r-recur').value;
    const listId = document.getElementById('r-list').value;
    const body = {
      title,
      description: document.getElementById('r-desc').value,
      due_at: new Date(dueLocal).toISOString(),
      timezone: Intl.DateTimeFormat().resolvedOptions().timeZone,
      priority: document.getElementById('r-priority').value,
      tag_ids: tagIDs,
      recurrence_rule: buildRecurrenceRule(recurPreset, dueLocal),
      offsets,
    };
    if (listId) body.list_id = listId;
    try {
      await API.post('/api/reminders', body);
      closeDrawer();
      showToast('Reminder created');
      renderApp(location.hash.replace('#', '') || '/today');
    } catch (err) {
      showToast(err.message, 'error');
    }
  });
}

async function openReminderDrawerEnhanced(id) {
  let reminder;
  let deliveries = [];
  let subtasks = [];
  let comments = [];
  try {
    [reminder, deliveries, subtasks, comments] = await Promise.all([
      API.get(`/api/reminders/${id}`),
      API.get(`/api/reminders/${id}/deliveries`).then((d) => d.deliveries || []).catch(() => []),
      API.get(`/api/reminders/${id}/subtasks`).then((d) => d.subtasks || []).catch(() => []),
      API.get(`/api/reminders/${id}/comments`).then((d) => d.comments || []).catch(() => []),
    ]);
  } catch (err) {
    showToast(err.message, 'error');
    return;
  }
  const doneCount = subtasks.filter((s) => s.done).length;
  const subtaskHTML = subtasks.length
    ? `<div class="subtask-progress">${doneCount}/${subtasks.length} complete</div>
       <ul class="subtask-list">${subtasks.map((s) => `
         <li><label class="checkbox-label"><input type="checkbox" data-subtask="${s.id}" ${s.done ? 'checked' : ''}> ${escapeHTML(s.title)}</label></li>
       `).join('')}</ul>`
    : '<p class="page-sub">No subtasks</p>';
  const commentHTML = comments.length
    ? `<div class="comment-list">${comments.map((c) => `
        <div class="comment-item"><div class="comment-body">${escapeHTML(c.body)}</div>
        <div class="comment-time">${new Date(c.created_at).toLocaleString()}</div></div>
      `).join('')}</div>`
    : '<p class="page-sub">No comments yet</p>';
  const deliveryRows = deliveries.length
    ? deliveries.map((d) => `
        <tr><td>${escapeHTML(d.channel)}</td>
        <td><span class="badge badge-${d.status === 'sent' ? 'done' : d.status === 'failed' ? 'overdue' : 'paused'}">${escapeHTML(d.status)}</span></td>
        <td class="mono-cell">${d.sent_at ? new Date(d.sent_at).toLocaleString() : new Date(d.created_at).toLocaleString()}</td></tr>
      `).join('')
    : '<tr><td colspan="3" class="table-empty">No deliveries yet</td></tr>';

  const drawer = openDrawer(escapeHTML(reminder.title), `
    <p class="page-sub" style="margin-top:0">${escapeHTML(reminder.description || 'No description')}</p>
    <dl class="detail-list">
      <dt>Due</dt><dd>${new Date(reminder.due_at).toLocaleString()}</dd>
      <dt>Status</dt><dd>${escapeHTML(reminder.status)}</dd>
      <dt>Priority</dt><dd>${escapeHTML(reminder.priority)}</dd>
    </dl>
    <h3 style="font-size:1rem;margin:20px 0 8px">Subtasks</h3>
    ${subtaskHTML}
    <h3 style="font-size:1rem;margin:20px 0 8px">Comments</h3>
    ${commentHTML}
    <div class="field" style="margin-top:8px"><textarea id="new-comment" rows="2" placeholder="Add a comment…"></textarea></div>
    <button class="btn btn-sm" id="post-comment">Post comment</button>
    <h3 style="font-size:1rem;margin:20px 0 8px">Delivery log</h3>
    <div class="table-wrap card" style="padding:0">
      <table class="data-table"><thead><tr><th>Channel</th><th>Status</th><th>When</th></tr></thead><tbody>${deliveryRows}</tbody></table>
    </div>
  `, `<button class="btn" type="button" data-close-drawer>Close</button>`);
  drawer.querySelector('[data-close-drawer]').addEventListener('click', closeDrawer);
  drawer.querySelectorAll('[data-subtask]').forEach((input) => {
    input.addEventListener('change', async () => {
      try {
        await API.patch(`/api/reminders/${id}/subtasks/${input.dataset.subtask}`, { done: input.checked });
      } catch (err) {
        showToast(err.message, 'error');
        input.checked = !input.checked;
      }
    });
  });
  drawer.querySelector('#post-comment')?.addEventListener('click', async () => {
    const body = document.getElementById('new-comment').value.trim();
    if (!body) return;
    try {
      await API.post(`/api/reminders/${id}/comments`, { body });
      openReminderDrawerEnhanced(id);
    } catch (err) {
      showToast(err.message, 'error');
    }
  });
}

function renderSettingsEnhanced(route) {
  if (route === '/settings/vacation') return renderVacationSettings();
  if (route === '/settings/alerts') return renderAlertSettings();
  const main = `
    <h1 class="page-title">Settings</h1>
    <p class="page-sub">Manage your account and preferences</p>
    <div class="settings-grid">
      <div class="card settings-card" data-href="#/settings/vacation">
        <h3>Vacation & Pause</h3>
        <p>Pause all notifications or review paused reminders.</p>
      </div>
      <div class="card settings-card" data-href="#/settings/alerts">
        <h3>Alert channels</h3>
        <p>Email and Telegram addresses for reminder alerts.</p>
      </div>
      <div class="card settings-card" data-href="#/settings/wording">
        <h3>Message wording</h3>
        <p>Design the email with HTML and CSS, and customize the message.</p>
      </div>
      <div class="card settings-card" data-href="#/settings/data">
        <h3>Data</h3>
        <p>Export JSON/iCal backups and import reminders.</p>
      </div>
      <div class="card settings-card" data-href="#/settings/templates">
        <h3>Templates</h3>
        <p>Reminder templates for birthdays, bills, and more.</p>
      </div>
      <div class="card settings-card" data-href="#/settings/google">
        <h3>Google</h3>
        <p>Connect Calendar and Gmail sync.</p>
      </div>
      <div class="card settings-card" data-href="#/settings/account">
        <h3>Account</h3>
        <p>API keys and two-factor authentication.</p>
      </div>
      <div class="card settings-card">
        <h3>Profile</h3>
        <p>${escapeHTML(state.user.email)} · ${escapeHTML(state.user.timezone)}</p>
      </div>
      ${state.user.is_admin ? `
      <div class="card settings-card" data-href="#/admin">
        <h3>Admin panel</h3>
        <p>Manage users, reminders, and sessions.</p>
      </div>` : ''}
    </div>
  `;
  document.getElementById('app').innerHTML = shellHTML('/settings', main);
  bindShell('/settings');
  document.querySelectorAll('[data-href]').forEach((el) => {
    el.addEventListener('click', () => { location.hash = el.dataset.href; });
  });
}

/* Override drawers used from app.js shell */
window.openCreateDrawer = openCreateDrawerEnhanced;
window.openReminderDrawer = openReminderDrawerEnhanced;
