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
  ...(window.DR_ADMIN_ROUTES || {}),
};

document.addEventListener('DOMContentLoaded', boot);

async function boot() {
  const hash = location.hash.replace('#', '') || '/today';
  if (hash === '/login' || hash === '/signup') {
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
        <p style="text-align:center"><button type="button" class="btn btn-ghost btn-sm" id="theme-toggle">Toggle theme</button></p>
      </div>
    </div>
  `;
  document.getElementById('theme-toggle').onclick = toggleTheme;
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
          <a href="#/reminders" class="${active === '/reminders' ? 'active' : ''}">All reminders</a>
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
          <button class="btn btn-primary btn-sm" id="add-reminder-top">+ Add</button>
        </header>
        <main id="main" class="page">${mainHTML}</main>
      </div>
    </div>
    <nav class="bottom-nav" aria-label="Mobile">
      <a href="#/today" class="${active === '/today' ? 'active' : ''}">Today</a>
      <a href="#/reminders" class="${active === '/reminders' ? 'active' : ''}">All</a>
      <a href="#/settings" class="${active.startsWith('/settings') ? 'active' : ''}">Settings</a>
      <a href="#/today" aria-label="Add" onclick="openCreateDrawer();return false;">Add</a>
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
  document.getElementById('resume-vacation')?.addEventListener('click', async () => {
    state.user = await API.post('/api/me/notifications/resume');
    renderApp(active);
    showToast('Notifications resumed');
  });
}

async function renderApp(route) {
  if (route.startsWith('/admin')) {
    const fn = routes[route] || routes['/admin'];
    await fn(route);
    return;
  }
  const fn = routes[route] || routes['/today'];
  if (!routes[route] && route.startsWith('/settings')) {
    await renderSettings(route);
    return;
  }
  await fn(route);
}

async function loadReminders(params = '') {
  const data = await API.get('/api/reminders' + params);
  state.reminders = data.reminders || [];
  return state.reminders;
}

function reminderActions(item) {
  const btns = [];
  if (item.status !== 'done') {
    btns.push(`<button class="btn btn-sm" data-action="done" data-id="${item.id}">Done</button>`);
  }
  if (item.status === 'paused') {
    btns.push(`<button class="btn btn-sm" data-action="resume" data-id="${item.id}">Resume</button>`);
  } else if (item.status === 'scheduled') {
    btns.push(`<button class="btn btn-sm" data-action="pause" data-id="${item.id}">Pause</button>`);
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
  const main = `
    <h1 class="page-title">Settings</h1>
    <p class="page-sub">Manage your account and preferences</p>
    <div class="settings-grid">
      <div class="card settings-card" data-href="#/settings/vacation">
        <h3>Vacation & Pause</h3>
        <p>Pause all notifications or review paused reminders.</p>
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
