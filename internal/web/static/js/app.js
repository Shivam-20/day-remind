const state = {
  user: null,
  reminders: [],
  filter: 'all',
};

const routes = {
  '/today': renderToday,
  '/reminders': renderReminders,
  '/settings': renderSettings,
  '/settings/vacation': renderVacationSettings,
  '/settings/alerts': renderAlertSettings,
  ...(window.DR_FEATURE_ROUTES || {}),
  ...(window.DR_ADMIN_ROUTES || {}),
};

document.addEventListener('DOMContentLoaded', boot);

async function boot() {
  const hash = location.hash.replace('#', '') || '/today';
  if (hash === '/login' || hash === '/signup' || hash.startsWith('/reset-password')) {
    if (hash.startsWith('/reset-password')) {
      renderResetPassword(new URLSearchParams(location.hash.split('?')[1] || '').get('token') || new URLSearchParams(location.search).get('token'));
      return;
    }
    renderAuth(hash === '/signup');
    return;
  }
  try {
    state.user = await API.get('/api/me');
    if (hash.startsWith('/admin') && !state.user.is_admin) {
      showToast('Admin access required', 'error');
      location.hash = '#/today';
      renderApp('/today');
      return;
    }
    renderApp(hash);
  } catch {
    location.hash = '#/login';
    renderAuth(false);
  }
}

window.addEventListener('hashchange', () => {
  const hash = location.hash.replace('#', '') || '/today';
  if (!state.user && hash !== '/login' && hash !== '/signup') {
    renderAuth(false);
    return;
  }
  if (state.user) {
    if (hash.startsWith('/admin') && !state.user.is_admin) {
      showToast('Admin access required', 'error');
      location.hash = '#/today';
      renderToday();
      return;
    }
    renderApp(hash);
  } else renderAuth(hash === '/signup');
});

function renderAuth(signup) {
  const app = document.getElementById('app');
  app.innerHTML = `
    <div class="auth-wrap">
      <div class="auth-card card">
        <div class="auth-hero">
          <h1><span style="color:var(--accent)">Day</span> Remind</h1>
          <p>Never miss what matters.</p>
        </div>
        <form id="auth-form">
          ${signup ? `<div class="field"><label for="name">Name</label><input id="name" name="name" autocomplete="name"></div>` : ''}
          <div class="field"><label for="email">Email</label><input id="email" name="email" type="email" required autocomplete="email"></div>
          <div class="field"><label for="password">Password</label><input id="password" name="password" type="password" required minlength="8" autocomplete="${signup ? 'new-password' : 'current-password'}"></div>
          <button class="btn btn-primary" type="submit" style="width:100%">${signup ? 'Create account' : 'Sign in'}</button>
        </form>
        <p style="margin-top:16px;text-align:center;font-size:.9rem;color:var(--text-2)">
          ${signup ? 'Have an account?' : 'New here?'}
          <a href="#/${signup ? 'login' : 'signup'}">${signup ? 'Sign in' : 'Create account'}</a>
        </p>
        ${signup ? '' : `<p style="text-align:center;font-size:.85rem"><a href="#" id="forgot-password">Forgot password?</a></p>`}
        <p style="text-align:center"><button type="button" class="btn btn-ghost btn-sm" id="theme-toggle">Toggle theme</button></p>
      </div>
    </div>
  `;
  document.getElementById('theme-toggle').onclick = toggleTheme;
  document.getElementById('forgot-password')?.addEventListener('click', async (e) => {
    e.preventDefault();
    const email = prompt('Enter your account email');
    if (!email) return;
    try {
      await API.post('/api/auth/forgot-password', { email });
      showToast('If that email exists, a reset link was sent');
    } catch (err) {
      showToast(err.message, 'error');
    }
  });
  document.getElementById('auth-form').onsubmit = async (e) => {
    e.preventDefault();
    const fd = new FormData(e.target);
    try {
      if (signup) {
        state.user = await API.post('/api/auth/signup', {
          email: fd.get('email'),
          password: fd.get('password'),
          display_name: fd.get('name') || '',
          timezone: Intl.DateTimeFormat().resolvedOptions().timeZone,
        });
      } else {
        state.user = await API.post('/api/auth/login', {
          email: fd.get('email'),
          password: fd.get('password'),
        });
      }
      location.hash = '#/today';
      renderApp('/today');
    } catch (err) {
      showToast(err.message, 'error');
    }
  };
}

function shellHTML(active, mainHTML) {
  const vacation = state.user?.notifications_paused
    ? `<div class="vacation-banner"><span>Notifications paused${state.user.notifications_paused_until ? ' until ' + new Date(state.user.notifications_paused_until).toLocaleString() : ''}</span><button class="btn btn-sm" id="resume-vacation">Resume</button></div>`
    : '';
  return `
    <div class="layout">
      <aside class="sidebar">
        <div class="brand"><span>Day</span> Remind</div>
        <nav class="nav">
          <a href="#/today" class="${active === '/today' ? 'active' : ''}">Today</a>
          <a href="#/calendar" class="${active === '/calendar' ? 'active' : ''}">Calendar</a>
          <a href="#/reminders" class="${active === '/reminders' ? 'active' : ''}">All reminders</a>
          <a href="#/lists" class="${active === '/lists' ? 'active' : ''}">Lists</a>
          <a href="#/settings" class="${active.startsWith('/settings') ? 'active' : ''}">Settings</a>
          ${state.user?.is_admin ? `<a href="#/admin" class="${active.startsWith('/admin') ? 'active' : ''}">Admin</a>` : ''}
        </nav>
        <div class="sidebar-footer">
          <button class="btn btn-sm" id="theme-toggle">Theme</button>
          <button class="btn btn-sm" id="logout-btn">Logout</button>
        </div>
      </aside>
      <div class="main">
        ${vacation}
        <header class="topbar">
          <strong>${escapeHTML(state.user?.display_name || state.user?.email || 'Day Remind')}</strong>
          <div class="topbar-actions">
            <button class="btn btn-sm" id="notif-bell" aria-label="Notifications">🔔</button>
            <button class="btn btn-primary btn-sm" id="add-reminder-top">+ Add</button>
          </div>
        </header>
        <main id="main" class="page">${mainHTML}</main>
      </div>
    </div>
    <nav class="bottom-nav" aria-label="Mobile">
      <a href="#/today" class="${active === '/today' ? 'active' : ''}">Today</a>
      <a href="#/calendar" class="${active === '/calendar' ? 'active' : ''}">Cal</a>
      <a href="#/reminders" class="${active === '/reminders' ? 'active' : ''}">All</a>
      <a href="#/lists" class="${active === '/lists' ? 'active' : ''}">Lists</a>
      <a href="#/settings" class="${active.startsWith('/settings') ? 'active' : ''}">Settings</a>
    </nav>
    <button class="fab" id="fab-add" aria-label="Add reminder">+</button>
  `;
}

function bindShell(active) {
  document.getElementById('theme-toggle')?.addEventListener('click', toggleTheme);
  document.getElementById('logout-btn')?.addEventListener('click', async () => {
    await API.post('/api/auth/logout');
    state.user = null;
    location.hash = '#/login';
    renderAuth(false);
  });
  document.getElementById('add-reminder-top')?.addEventListener('click', openCreateDrawer);
  document.getElementById('fab-add')?.addEventListener('click', openCreateDrawer);
  document.getElementById('notif-bell')?.addEventListener('click', openNotificationsDrawer);
  document.getElementById('resume-vacation')?.addEventListener('click', async () => {
    state.user = await API.post('/api/me/notifications/resume');
    renderApp(active);
    showToast('Notifications resumed');
  });
}

function resolveRoute(route) {
  const featureRoutes = window.DR_FEATURE_ROUTES || {};
  if (featureRoutes[route]) return featureRoutes[route];
  if (routes[route]) return routes[route];
  return null;
}

async function renderApp(route) {
  if (route.startsWith('/admin')) {
    const fn = resolveRoute(route) || routes['/admin'];
    await fn(route);
    return;
  }
  const fn = resolveRoute(route);
  if (fn) {
    await fn(route);
    return;
  }
  if (route.startsWith('/settings')) {
    if (route === '/settings/alerts') {
      await renderAlertSettings();
      return;
    }
    await renderSettings(route);
    return;
  }
  await routes['/today'](route);
}

async function loadReminders(params = '') {
  const data = await API.get('/api/reminders' + params);
  state.reminders = data.reminders || [];
  return state.reminders;
}

function reminderActions(item) {
  const btns = [];
  btns.push(`<button class="btn btn-sm" data-action="detail" data-id="${item.id}">Details</button>`);
  if (item.status !== 'done') {
    btns.push(`<button class="btn btn-sm" data-action="done" data-id="${item.id}">Done</button>`);
  }
  if (item.status === 'paused') {
    btns.push(`<button class="btn btn-sm" data-action="resume" data-id="${item.id}">Resume</button>`);
  } else if (item.status === 'scheduled') {
    btns.push(`<button class="btn btn-sm" data-action="pause" data-id="${item.id}">Pause</button>`);
  }
  if (item.status !== 'done' && item.status !== 'cancelled') {
    btns.push(`<button class="btn btn-sm" data-action="snooze" data-id="${item.id}" data-preset="+15m">+15m</button>`);
    btns.push(`<button class="btn btn-sm" data-action="snooze" data-id="${item.id}" data-preset="+1h">+1h</button>`);
    btns.push(`<button class="btn btn-sm" data-action="snooze" data-id="${item.id}" data-preset="tomorrow_9am">Tomorrow</button>`);
  }
  btns.push(`<button class="btn btn-sm btn-danger" data-action="delete" data-id="${item.id}">Delete</button>`);
  return btns.join('');
}

function bindReminderActions(container, rerender) {
  container.querySelectorAll('[data-action]').forEach((btn) => {
    btn.addEventListener('click', async () => {
      const id = btn.dataset.id;
      const action = btn.dataset.action;
      try {
        if (action === 'done') await API.post(`/api/reminders/${id}/done`);
        if (action === 'pause') await API.post(`/api/reminders/${id}/pause`);
        if (action === 'resume') await API.post(`/api/reminders/${id}/resume`);
        if (action === 'snooze') await API.post(`/api/reminders/${id}/snooze`, { preset: btn.dataset.preset });
        if (action === 'detail') { openReminderDrawer(id); return; }
        if (action === 'delete') {
          if (!confirm('Delete this reminder?')) return;
          await API.delete(`/api/reminders/${id}`);
        }
        showToast('Updated');
        await rerender();
      } catch (err) {
        showToast(err.message, 'error');
      }
    });
  });
}

async function renderToday() {
  await loadReminders();
  const now = new Date();
  const today = state.reminders.filter((r) => {
    const d = new Date(r.due_at);
    return d.toDateString() === now.toDateString() && r.status !== 'done' && r.status !== 'cancelled';
  });
  const overdue = state.reminders.filter((r) => isOverdue(r));
  const upcoming = state.reminders.filter((r) => {
    const d = new Date(r.due_at);
    return d > now && r.status === 'scheduled' && d.toDateString() !== now.toDateString();
  }).slice(0, 5);
  const paused = state.reminders.filter((r) => r.status === 'paused').length;

  const main = `
    <h1 class="page-title">Today</h1>
    <p class="page-sub">${now.toLocaleDateString(undefined, { weekday: 'long', month: 'long', day: 'numeric' })}</p>
    <div class="stats-row">
      <div class="stat-card"><strong>${today.length}</strong><span>Due today</span></div>
      <div class="stat-card"><strong>${overdue.length}</strong><span>Overdue</span></div>
      <div class="stat-card"><strong>${paused}</strong><span>Paused</span></div>
      <div class="stat-card"><strong>${state.reminders.filter((r) => r.status === 'done').length}</strong><span>Done</span></div>
    </div>
    ${overdue.length ? `<h2 style="font-size:1rem;color:var(--danger)">Overdue</h2><div class="reminder-list">${overdue.map((r) => reminderCardHTML(r, reminderActions(r))).join('')}</div>` : ''}
    <h2 style="font-size:1rem;margin-top:24px">Today</h2>
    <div class="reminder-list" id="today-list">
      ${today.length ? today.map((r) => reminderCardHTML(r, reminderActions(r))).join('') : emptyState('Nothing scheduled today', 'Tap + to add a reminder.', '<button class="btn btn-primary" id="empty-add">Add reminder</button>')}
    </div>
    ${upcoming.length ? `<h2 style="font-size:1rem;margin-top:24px">Upcoming</h2><div class="reminder-list">${upcoming.map((r) => reminderCardHTML(r, reminderActions(r))).join('')}</div>` : ''}
  `;
  document.getElementById('app').innerHTML = shellHTML('/today', main);
  bindShell('/today');
  bindReminderActions(document.getElementById('app'), () => renderToday());
  document.getElementById('empty-add')?.addEventListener('click', openCreateDrawer);
}

async function renderReminders() {
  const status = state.filter === 'all' ? '' : state.filter;
  await loadReminders(status ? `?status=${status}` : '');
  const chips = ['all', 'scheduled', 'paused', 'done'].map((f) =>
    `<button class="filter-chip ${state.filter === f ? 'active' : ''}" data-filter="${f}">${f}</button>`
  ).join('');
  const main = `
    <h1 class="page-title">All reminders</h1>
    <p class="page-sub">${state.reminders.length} items</p>
    <div class="filter-row">${chips}</div>
    <div class="reminder-list" id="all-list">
      ${state.reminders.length ? state.reminders.map((r) => reminderCardHTML(r, reminderActions(r))).join('') : emptyState('No reminders', 'Create one to get started.')}
    </div>
  `;
  document.getElementById('app').innerHTML = shellHTML('/reminders', main);
  bindShell('/reminders');
  bindReminderActions(document.getElementById('app'), () => renderReminders());
  document.querySelectorAll('[data-filter]').forEach((chip) => {
    chip.addEventListener('click', () => {
      state.filter = chip.dataset.filter;
      renderReminders();
    });
  });
}

async function renderSettings(route = '/settings') {
  if (route === '/settings/vacation') {
    await renderVacationSettings();
    return;
  }
  if (route === '/settings/alerts') {
    await renderAlertSettings();
    return;
  }
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
      <div class="card settings-card">
        <h3>Profile</h3>
        <p>${escapeHTML(state.user.email)} · ${escapeHTML(state.user.timezone)}${state.user.is_admin ? ' · <span class="badge badge-admin">Admin</span>' : ''}</p>
      </div>
      ${state.user.is_admin ? `
      <div class="card settings-card" data-href="#/admin">
        <h3>Admin panel</h3>
        <p>Manage users, reminders, and sessions.</p>
      </div>` : ''}
      <div class="card settings-card">
        <h3>Data export / import</h3>
        <p>JSON backup — coming in next release.</p>
      </div>
      <div class="card settings-card">
        <h3>Google / Gmail sync</h3>
        <p>Connect Google — coming in next release.</p>
      </div>
    </div>
  `;
  document.getElementById('app').innerHTML = shellHTML('/settings', main);
  bindShell('/settings');
  document.querySelectorAll('[data-href]').forEach((el) => {
    el.addEventListener('click', () => {
      location.hash = el.dataset.href;
    });
  });
}

async function renderAlertSettings() {
  let data;
  try {
    data = await API.get('/api/me/notification-channels');
  } catch (err) {
    showToast(err.message, 'error');
    return;
  }
  const channels = data.channels || {};
  const email = channels.email || {};
  const telegram = channels.telegram || {};
  const emailUnavailable = email.available === false;
  const tgUnavailable = telegram.available === false;

  const main = `
    <h1 class="page-title">Alert channels</h1>
    <p class="page-sub">Choose where Day Remind sends reminder alerts.</p>
    <div class="card alert-channel-card">
      <h3>Email alerts</h3>
      ${emailUnavailable ? '<p class="channel-unavailable">Email alerts are not configured yet. Ask an admin to set up Resend.</p>' : ''}
      <label class="checkbox-label"><input id="email-enabled" type="checkbox"${email.enabled ? ' checked' : ''}${emailUnavailable ? ' disabled' : ''}> Enable email alerts</label>
      <div class="field"><label for="email-address">Alert email</label><input id="email-address" type="email" value="${escapeHTML(email.address || state.user.email || '')}"${emailUnavailable ? ' disabled' : ''}></div>
    </div>
    <div class="card alert-channel-card">
      <h3>Telegram alerts</h3>
      ${tgUnavailable ? '<p class="channel-unavailable">Telegram is not configured yet. Ask an admin to add the bot token.</p>' : '<p class="channel-help">Link your bot account or paste chat ID from <a href="https://t.me/userinfobot" target="_blank" rel="noopener">@userinfobot</a>.</p>'}
      <label class="checkbox-label"><input id="telegram-enabled" type="checkbox"${telegram.enabled ? ' checked' : ''}${tgUnavailable ? ' disabled' : ''}> Enable Telegram alerts</label>
      <div class="field"><label for="telegram-chat-id">Telegram chat ID</label><input id="telegram-chat-id" inputmode="numeric" value="${escapeHTML(telegram.address || '')}" placeholder="e.g. 123456789"${tgUnavailable ? ' disabled' : ''}></div>
      ${tgUnavailable ? '' : '<button class="btn btn-sm" id="telegram-link" type="button">Generate link command</button><pre id="telegram-link-out" class="link-out" hidden></pre>'}
    </div>
    <div class="card alert-channel-card">
      <h3>Quiet hours</h3>
      <p class="channel-help">Non-urgent alerts are held during quiet hours (urgent reminders still send).</p>
      <div class="admin-filter-grid">
        <div class="field" style="margin:0"><label for="quiet-start">Start (HH:MM)</label><input id="quiet-start" placeholder="22:00" value="${escapeHTML(data.quiet_start || '')}"></div>
        <div class="field" style="margin:0"><label for="quiet-end">End (HH:MM)</label><input id="quiet-end" placeholder="08:00" value="${escapeHTML(data.quiet_end || '')}"></div>
      </div>
    </div>
    <div class="alert-actions">
      <button class="btn btn-primary" id="save-alerts">Save channels</button>
      <a href="#/settings" class="btn btn-ghost btn-sm">← Back to settings</a>
    </div>
  `;
  document.getElementById('app').innerHTML = shellHTML('/settings/alerts', main);
  bindShell('/settings/alerts');
  document.getElementById('telegram-link')?.addEventListener('click', async () => {
    try {
      const link = await API.post('/api/me/telegram/link');
      const out = document.getElementById('telegram-link-out');
      out.hidden = false;
      out.textContent = link.bot_hint ? `Open @${link.bot_hint} and send:\n${link.command}` : link.command;
    } catch (err) {
      showToast(err.message, 'error');
    }
  });
  document.getElementById('save-alerts')?.addEventListener('click', async () => {
    try {
      await API.patch('/api/me/notification-channels', {
        email: {
          enabled: document.getElementById('email-enabled').checked,
          address: document.getElementById('email-address').value.trim(),
        },
        telegram: {
          enabled: document.getElementById('telegram-enabled').checked,
          address: document.getElementById('telegram-chat-id').value.trim(),
        },
        quiet_start: document.getElementById('quiet-start').value.trim(),
        quiet_end: document.getElementById('quiet-end').value.trim(),
      });
      showToast('Alert channels saved');
    } catch (err) {
      showToast(err.message, 'error');
    }
  });
}

async function renderVacationSettings() {
  const main = `
    <h1 class="page-title">Vacation & Pause</h1>
    <p class="page-sub">Pause all outbound notifications while you're away.</p>
    <div class="card" style="margin-bottom:16px">
      <p style="margin:0 0 12px">Status: <strong>${state.user.notifications_paused ? 'Paused' : 'Active'}</strong></p>
      ${state.user.notifications_paused
        ? `<button class="btn btn-primary" id="resume-all">Resume notifications</button>`
        : `<div class="field"><label for="pause-until">Resume on (optional)</label><input id="pause-until" type="datetime-local"></div><button class="btn btn-primary" id="pause-all">Pause notifications</button>`}
    </div>
    <a href="#/settings" class="btn btn-ghost btn-sm">← Back to settings</a>
  `;
  document.getElementById('app').innerHTML = shellHTML('/settings/vacation', main);
  bindShell('/settings/vacation');
  document.getElementById('resume-all')?.addEventListener('click', async () => {
    state.user = await API.post('/api/me/notifications/resume');
    showToast('Notifications resumed');
    renderVacationSettings();
  });
  document.getElementById('pause-all')?.addEventListener('click', async () => {
    const raw = document.getElementById('pause-until').value;
    const body = {};
    if (raw) body.until = new Date(raw).toISOString();
    state.user = await API.post('/api/me/notifications/pause', body);
    showToast('Notifications paused');
    renderVacationSettings();
  });
}

function openCreateDrawer() {
  const defaultDue = new Date();
  defaultDue.setHours(defaultDue.getHours() + 1, 0, 0, 0);
  const local = new Date(defaultDue.getTime() - defaultDue.getTimezoneOffset() * 60000).toISOString().slice(0, 16);
  const drawer = openDrawer('New reminder', `
    <div class="field"><label for="r-title">Title</label><input id="r-title" required></div>
    <div class="field"><label for="r-desc">Description</label><textarea id="r-desc" rows="3"></textarea></div>
    <div class="field"><label for="r-due">Due</label><input id="r-due" type="datetime-local" value="${local}" required></div>
    <div class="field"><label for="r-priority">Priority</label>
      <select id="r-priority"><option value="normal">Normal</option><option value="high">High</option><option value="urgent">Urgent</option><option value="low">Low</option></select>
    </div>
  `, `<button class="btn" type="button" data-close-drawer>Cancel</button><button class="btn btn-primary" id="save-reminder">Save</button>`);
  drawer.querySelector('[data-close-drawer]').addEventListener('click', closeDrawer);
  drawer.querySelector('#save-reminder').addEventListener('click', async () => {
    const title = document.getElementById('r-title').value.trim();
    const dueLocal = document.getElementById('r-due').value;
    if (!title || !dueLocal) {
      showToast('Title and due date required', 'error');
      return;
    }
    try {
      await API.post('/api/reminders', {
        title,
        description: document.getElementById('r-desc').value,
        due_at: new Date(dueLocal).toISOString(),
        timezone: Intl.DateTimeFormat().resolvedOptions().timeZone,
        priority: document.getElementById('r-priority').value,
      });
      closeDrawer();
      showToast('Reminder created');
      const route = location.hash.replace('#', '') || '/today';
      renderApp(route);
    } catch (err) {
      showToast(err.message, 'error');
    }
  });
}

async function openReminderDrawer(id) {
  let reminder;
  let deliveries = [];
  try {
    reminder = await API.get(`/api/reminders/${id}`);
    const data = await API.get(`/api/reminders/${id}/deliveries`);
    deliveries = data.deliveries || [];
  } catch (err) {
    showToast(err.message, 'error');
    return;
  }
  const deliveryRows = deliveries.length
    ? deliveries.map((d) => `
        <tr>
          <td>${escapeHTML(d.channel)}</td>
          <td><span class="badge badge-${d.status === 'sent' ? 'done' : d.status === 'failed' ? 'overdue' : 'paused'}">${escapeHTML(d.status)}</span></td>
          <td class="mono-cell">${d.sent_at ? new Date(d.sent_at).toLocaleString() : new Date(d.created_at).toLocaleString()}</td>
          <td class="mono-cell">${d.last_error ? escapeHTML(d.last_error) : '—'}</td>
        </tr>
      `).join('')
    : '<tr><td colspan="4" class="table-empty">No delivery attempts yet</td></tr>';
  const drawer = openDrawer(escapeHTML(reminder.title), `
    <p class="page-sub" style="margin-top:0">${escapeHTML(reminder.description || 'No description')}</p>
    <dl class="detail-list">
      <dt>Due</dt><dd>${new Date(reminder.due_at).toLocaleString()}</dd>
      <dt>Status</dt><dd>${escapeHTML(reminder.status)}</dd>
      <dt>Priority</dt><dd>${escapeHTML(reminder.priority)}</dd>
      ${reminder.snoozed_until ? `<dt>Snoozed until</dt><dd>${new Date(reminder.snoozed_until).toLocaleString()}</dd>` : ''}
    </dl>
    <h3 style="font-size:1rem;margin:20px 0 8px">Delivery log</h3>
    <div class="table-wrap card" style="padding:0">
      <table class="data-table">
        <thead><tr><th>Channel</th><th>Status</th><th>When</th><th>Error</th></tr></thead>
        <tbody>${deliveryRows}</tbody>
      </table>
    </div>
  `, `<button class="btn" type="button" data-close-drawer>Close</button>`);
  drawer.querySelector('[data-close-drawer]').addEventListener('click', closeDrawer);
}

async function openNotificationsDrawer() {
  let items = [];
  try {
    const data = await API.get('/api/notifications');
    items = data.notifications || [];
  } catch (err) {
    showToast(err.message, 'error');
    return;
  }
  const body = items.length
    ? `<div class="notification-list">${items.map((n) => `
        <div class="notification-item">
          <div class="notification-meta">${escapeHTML(n.channel)} · ${escapeHTML(n.status)}</div>
          <div class="notification-time">${n.sent_at ? new Date(n.sent_at).toLocaleString() : new Date(n.created_at).toLocaleString()}</div>
          ${n.last_error ? `<div class="notification-error">${escapeHTML(n.last_error)}</div>` : ''}
        </div>
      `).join('')}</div>`
    : emptyState('No notifications yet', 'Delivery attempts appear here after reminders are due.');
  const drawer = openDrawer('Recent notifications', body, `<button class="btn" type="button" data-close-drawer>Close</button>`);
  drawer.querySelector('[data-close-drawer]').addEventListener('click', closeDrawer);
}

function renderResetPassword(token) {
  const app = document.getElementById('app');
  if (!token) {
    app.innerHTML = `
      <div class="auth-wrap">
        <div class="auth-card card">
          <h1>Reset password</h1>
          <p class="page-sub">Invalid or missing reset link.</p>
          <a href="#/login">Back to sign in</a>
        </div>
      </div>
    `;
    return;
  }
  app.innerHTML = `
    <div class="auth-wrap">
      <div class="auth-card card">
        <h1>Reset password</h1>
        <form id="reset-form">
          <div class="field"><label for="new-password">New password</label><input id="new-password" type="password" minlength="8" required autocomplete="new-password"></div>
          <button class="btn btn-primary" type="submit" style="width:100%">Update password</button>
        </form>
        <p style="margin-top:16px;text-align:center"><a href="#/login">Back to sign in</a></p>
      </div>
    </div>
  `;
  document.getElementById('reset-form').onsubmit = async (e) => {
    e.preventDefault();
    try {
      await API.post('/api/auth/reset-password', {
        token,
        password: document.getElementById('new-password').value,
      });
      showToast('Password updated — sign in with your new password');
      location.hash = '#/login';
      renderAuth(false);
    } catch (err) {
      showToast(err.message, 'error');
    }
  };
}

if ('serviceWorker' in navigator) {
  window.addEventListener('load', () => {
    // Relative to <base> so /remind/static/sw.js works behind the OCI path proxy.
    // Skip when base path is set — SW scope under /static/ is wrong for subpath deploys.
    const base = typeof API !== 'undefined' ? API.base() : '';
    if (!base) {
      navigator.serviceWorker.register('/static/sw.js').catch(() => {});
    }
  });
}
