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
  '/settings/notifications': renderNotificationSettings,
  '/settings/templates': renderMyTemplates,
  '/admin': renderAdmin,
};

function isAdmin() {
  return state.user?.role === 'admin';
}

document.addEventListener('DOMContentLoaded', boot);

async function boot() {
  const hash = location.hash.replace('#', '') || '/today';
  if (hash === '/login' || hash === '/signup') {
    renderAuth(hash === '/signup');
    return;
  }
  try {
    state.user = await API.get('/api/me');
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
  if (state.user) renderApp(hash);
  else renderAuth(hash === '/signup');
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
          ${isAdmin() ? `<a href="#/admin" class="${active === '/admin' ? 'active' : ''}">Admin</a>` : ''}
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
          ${isAdmin() ? '<span class="badge badge-admin">Admin</span>' : ''}
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
    try {
      await API.post('/api/auth/logout');
    } catch {
      // Drop the local session regardless; the cookie may already be gone.
    }
    state.user = null;
    location.hash = '#/login';
    renderAuth(false);
  });
  document.getElementById('add-reminder-top')?.addEventListener('click', openCreateDrawer);
  document.getElementById('fab-add')?.addEventListener('click', openCreateDrawer);
  document.getElementById('resume-vacation')?.addEventListener('click', async () => {
    try {
      state.user = await API.post('/api/me/notifications/resume');
      renderApp(active);
      showToast('Notifications resumed');
    } catch (err) {
      showToast(err.message, 'error');
    }
  });
}

async function renderApp(route) {
  if (route === '/admin' && !isAdmin()) {
    location.hash = '#/today';
    return renderToday();
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
  const overdue = state.reminders.filter((r) => isOverdue(r));
  const today = state.reminders.filter((r) => {
    const d = new Date(r.due_at);
    return d.toDateString() === now.toDateString()
      && r.status !== 'done' && r.status !== 'cancelled'
      && !isOverdue(r);
  });
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
  await loadReminders(status ? `?status=${encodeURIComponent(status)}` : '');
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
  if (route === '/settings/notifications') {
    await renderNotificationSettings();
    return;
  }
  if (route === '/settings/templates') {
    await renderMyTemplates();
    return;
  }
  const main = `
    <h1 class="page-title">Settings</h1>
    <p class="page-sub">Manage your account and preferences</p>
    <div class="settings-grid">
      <div class="card settings-card" data-href="#/settings/notifications">
        <h3>Notifications</h3>
        <p>Email and Telegram delivery, plus how far ahead to warn you.</p>
      </div>
      <div class="card settings-card" data-href="#/settings/templates">
        <h3>Message wording</h3>
        <p>Customise how your reminders read, or keep the defaults.</p>
      </div>
      <div class="card settings-card" data-href="#/settings/vacation">
        <h3>Vacation & Pause</h3>
        <p>Pause all notifications or review paused reminders.</p>
      </div>
      <div class="card settings-card">
        <h3>Profile</h3>
        <p>${escapeHTML(state.user.email)} · ${escapeHTML(state.user.timezone)}</p>
      </div>
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
  document.querySelectorAll('[data-href]').forEach((card) => {
    card.addEventListener('click', () => { location.hash = card.dataset.href; });
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
    try {
      state.user = await API.post('/api/me/notifications/resume');
      showToast('Notifications resumed');
      renderVacationSettings();
    } catch (err) {
      showToast(err.message, 'error');
    }
  });
  document.getElementById('pause-all')?.addEventListener('click', async () => {
    const raw = document.getElementById('pause-until').value;
    const body = {};
    if (raw) body.until = new Date(raw).toISOString();
    try {
      state.user = await API.post('/api/me/notifications/pause', body);
      showToast('Notifications paused');
      renderVacationSettings();
    } catch (err) {
      showToast(err.message, 'error');
    }
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

// ---------------------------------------------------------------- admin panel

const adminState = { tab: 'users', users: [], stats: null, settings: null, query: '', sort: 'created' };

async function loadAdmin() {
  const [stats, users] = await Promise.all([
    API.get('/api/admin/stats'),
    API.get('/api/admin/users'),
  ]);
  adminState.stats = stats;
  adminState.users = users.users || [];
}

function adminVisibleUsers() {
  const q = adminState.query.trim().toLowerCase();
  const rows = q
    ? adminState.users.filter((u) =>
        (u.email || '').toLowerCase().includes(q) || (u.display_name || '').toLowerCase().includes(q))
    : adminState.users.slice();
  const by = {
    created: (a, b) => new Date(a.created_at) - new Date(b.created_at),
    email: (a, b) => (a.email || '').localeCompare(b.email || ''),
    reminders: (a, b) => b.stats.total - a.stats.total,
    overdue: (a, b) => b.stats.overdue - a.stats.overdue,
  };
  return rows.sort(by[adminState.sort] || by.created);
}

function adminUserRow(u) {
  const self = u.id === state.user.id;
  const admin = u.role === 'admin';
  const actions = [];
  if (admin) {
    actions.push(`<button class="btn btn-sm" data-admin="demote" data-id="${u.id}"${u.pinned_admin ? ' disabled title="Pinned by ADMIN_EMAILS"' : ''}>Demote</button>`);
  } else {
    actions.push(`<button class="btn btn-sm" data-admin="promote" data-id="${u.id}">Make admin</button>`);
  }
  actions.push(u.notifications_paused
    ? `<button class="btn btn-sm" data-admin="resume" data-id="${u.id}">Resume</button>`
    : `<button class="btn btn-sm" data-admin="pause" data-id="${u.id}">Pause</button>`);
  if (!self) {
    actions.push(`<button class="btn btn-sm btn-danger" data-admin="delete" data-id="${u.id}">Delete</button>`);
  }

  return `
    <tr>
      <td>
        <div class="admin-user">
          <strong>${escapeHTML(u.display_name || '—')}</strong>
          <span>${escapeHTML(u.email)}</span>
        </div>
        <div class="admin-tags">
          ${admin ? '<span class="badge badge-admin">Admin</span>' : ''}
          ${u.pinned_admin ? '<span class="badge badge-pinned">Pinned</span>' : ''}
          ${self ? '<span class="badge">You</span>' : ''}
          ${u.notifications_paused ? '<span class="badge badge-paused">Paused</span>' : ''}
        </div>
      </td>
      <td class="admin-num">${u.stats.total}</td>
      <td class="admin-num">${u.stats.scheduled}</td>
      <td class="admin-num${u.stats.overdue ? ' is-danger' : ''}">${u.stats.overdue}</td>
      <td class="admin-num">${u.stats.done}</td>
      <td class="admin-when">${u.next_due_at ? formatDue(u.next_due_at, u.timezone) : '—'}</td>
      <td class="admin-when">${new Date(u.created_at).toLocaleDateString()}</td>
      <td><div class="admin-actions">${actions.join('')}</div></td>
    </tr>
  `;
}

const ADMIN_TABS = [
  ['users', 'Users'],
  ['delivery', 'Delivery'],
  ['templates', 'Templates'],
];

function adminTabsHTML() {
  return `<div class="filter-row admin-tabs">${ADMIN_TABS.map(([id, label]) =>
    `<button class="filter-chip ${adminState.tab === id ? 'active' : ''}" data-admin-tab="${id}">${label}</button>`
  ).join('')}</div>`;
}

function bindAdminTabs() {
  document.querySelectorAll('[data-admin-tab]').forEach((b) => {
    b.addEventListener('click', () => {
      adminState.tab = b.dataset.adminTab;
      renderAdmin();
    });
  });
}

// adminShell renders the page frame so every tab shares one layout and one
// error path.
function adminShell(bodyHTML, sub) {
  document.getElementById('app').innerHTML = shellHTML('/admin', `
    <h1 class="page-title">Admin</h1>
    <p class="page-sub">${escapeHTML(sub)}</p>
    ${adminTabsHTML()}
    ${bodyHTML}
  `);
  bindShell('/admin');
  bindAdminTabs();
}

async function renderAdmin() {
  switch (adminState.tab) {
    case 'delivery': return renderAdminDelivery();
    case 'templates': return renderAdminTemplates();
    default: return renderAdminUsers();
  }
}

async function renderAdminUsers() {
  try {
    await loadAdmin();
  } catch (err) {
    document.getElementById('app').innerHTML = shellHTML('/admin',
      `<h1 class="page-title">Admin</h1>${emptyState('Could not load the panel', escapeHTML(err.message))}`);
    bindShell('/admin');
    return;
  }

  const s = adminState.stats;
  const rows = adminVisibleUsers();
  const main = `
    <div class="stats-row">
      <div class="stat-card"><strong>${s.users}</strong><span>Users</span></div>
      <div class="stat-card"><strong>${s.admins}</strong><span>Admins</span></div>
      <div class="stat-card"><strong>${s.reminders}</strong><span>Reminders</span></div>
      <div class="stat-card"><strong>${s.reminders_open}</strong><span>Open</span></div>
      <div class="stat-card"><strong>${s.reminders_overdue}</strong><span>Overdue</span></div>
      <div class="stat-card"><strong>${s.active_sessions}</strong><span>Sessions</span></div>
    </div>

    <div class="admin-toolbar">
      <input id="admin-search" type="search" placeholder="Search name or email"
             value="${escapeHTML(adminState.query)}" aria-label="Search users">
      <select id="admin-sort" aria-label="Sort users">
        <option value="created"${adminState.sort === 'created' ? ' selected' : ''}>Oldest first</option>
        <option value="email"${adminState.sort === 'email' ? ' selected' : ''}>Email A–Z</option>
        <option value="reminders"${adminState.sort === 'reminders' ? ' selected' : ''}>Most reminders</option>
        <option value="overdue"${adminState.sort === 'overdue' ? ' selected' : ''}>Most overdue</option>
      </select>
      <span class="admin-count">${rows.length} of ${adminState.users.length}</span>
    </div>

    <p class="admin-privacy">Reminder titles and descriptions are never shown here — counts only.</p>

    ${rows.length ? `
    <div class="admin-table-wrap">
      <table class="admin-table">
        <thead>
          <tr>
            <th>User</th><th class="admin-num">All</th><th class="admin-num">Open</th>
            <th class="admin-num">Overdue</th><th class="admin-num">Done</th>
            <th>Next due</th><th>Joined</th><th></th>
          </tr>
        </thead>
        <tbody>${rows.map(adminUserRow).join('')}</tbody>
      </table>
    </div>` : emptyState('No matching users', 'Try a different search.')}
  `;

  adminShell(main, 'Instance overview and user management');
  bindAdminActions();
}

function bindAdminActions() {
  const search = document.getElementById('admin-search');
  search?.addEventListener('input', () => {
    adminState.query = search.value;
    // Re-render from cached data; no refetch while typing.
    renderAdminRowsOnly();
  });
  document.getElementById('admin-sort')?.addEventListener('change', (e) => {
    adminState.sort = e.target.value;
    renderAdminRowsOnly();
  });

  document.querySelectorAll('[data-admin]').forEach((btn) => {
    btn.addEventListener('click', () => {
      const id = btn.dataset.id;
      const user = adminState.users.find((u) => u.id === id);
      if (!user) return;
      switch (btn.dataset.admin) {
        case 'promote': return adminSetRole(user, 'admin');
        case 'demote': return adminSetRole(user, 'user');
        case 'pause': return adminPause(user);
        case 'resume': return adminAct(user, () => API.post(`/api/admin/users/${user.id}/notifications/resume`), 'Notifications resumed');
        case 'delete': return adminConfirmDelete(user);
      }
    });
  });
}

// Redraw just the table body and count, keeping focus in the search box.
function renderAdminRowsOnly() {
  const rows = adminVisibleUsers();
  const body = document.querySelector('.admin-table tbody');
  const count = document.querySelector('.admin-count');
  if (count) count.textContent = `${rows.length} of ${adminState.users.length}`;
  if (body) {
    body.innerHTML = rows.map(adminUserRow).join('');
    bindAdminActions();
  } else {
    renderAdminUsers();
  }
}

async function adminAct(user, fn, successMessage) {
  try {
    await fn();
    showToast(successMessage);
    await renderAdminUsers();
  } catch (err) {
    showToast(err.message, 'error');
  }
}

async function adminSetRole(user, role) {
  try {
    const res = await API.post(`/api/admin/users/${user.id}/role`, { role });
    showToast(`${user.email} is now ${role}`);
    if (res.demoted_self) {
      // The panel is gone for this session; refresh identity and leave.
      state.user = await API.get('/api/me');
      location.hash = '#/today';
      return renderToday();
    }
    await renderAdminUsers();
  } catch (err) {
    showToast(err.message, 'error');
  }
}

function adminPause(user) {
  const drawer = openDrawer('Pause notifications', `
    <p style="margin-top:0;color:var(--text-2)">Pausing all outbound notifications for <strong>${escapeHTML(user.email)}</strong>.</p>
    <div class="field"><label for="admin-pause-until">Resume on (optional)</label><input id="admin-pause-until" type="datetime-local"></div>
  `, `<button class="btn" type="button" data-close-drawer>Cancel</button><button class="btn btn-primary" id="admin-pause-go">Pause</button>`);
  drawer.querySelector('#admin-pause-go').addEventListener('click', async () => {
    const raw = document.getElementById('admin-pause-until').value;
    const body = {};
    if (raw) body.until = new Date(raw).toISOString();
    try {
      await API.post(`/api/admin/users/${user.id}/notifications/pause`, body);
      closeDrawer();
      showToast('Notifications paused');
      await renderAdminUsers();
    } catch (err) {
      showToast(err.message, 'error');
    }
  });
}

// Deleting a user also deletes their reminders and cannot be undone, so this
// asks for the address to be typed rather than relying on a one-click confirm.
function adminConfirmDelete(user) {
  const drawer = openDrawer('Delete user', `
    <p style="margin-top:0">This permanently deletes <strong>${escapeHTML(user.email)}</strong>,
    their <strong>${user.stats.total}</strong> reminder(s) and all their sessions.</p>
    <p style="color:var(--danger)">This cannot be undone and there are no backups.</p>
    <div class="field">
      <label for="admin-delete-confirm">Type the email address to confirm</label>
      <input id="admin-delete-confirm" autocomplete="off" placeholder="${escapeHTML(user.email)}">
    </div>
  `, `<button class="btn" type="button" data-close-drawer>Cancel</button><button class="btn btn-danger" id="admin-delete-go" disabled>Delete permanently</button>`);

  const input = drawer.querySelector('#admin-delete-confirm');
  const go = drawer.querySelector('#admin-delete-go');
  input.addEventListener('input', () => {
    go.disabled = input.value.trim().toLowerCase() !== user.email.toLowerCase();
  });
  go.addEventListener('click', async () => {
    if (go.disabled) return;
    go.disabled = true;
    try {
      await API.delete(`/api/admin/users/${user.id}`);
      closeDrawer();
      showToast(`Deleted ${user.email}`);
      await renderAdminUsers();
    } catch (err) {
      go.disabled = false;
      showToast(err.message, 'error');
    }
  });
}

// -------------------------------------------------- notification settings

const ADVANCE_CHOICES = [
  [0, 'Only at the due time'],
  [15, '15 minutes before'],
  [60, '1 hour before'],
  [180, '3 hours before'],
  [720, '12 hours before'],
  [1440, '1 day before'],
  [10080, '1 week before'],
];

async function renderNotificationSettings() {
  let cfg;
  try {
    cfg = await API.get('/api/me/notify');
  } catch (err) {
    document.getElementById('app').innerHTML = shellHTML('/settings',
      `<h1 class="page-title">Notifications</h1>${emptyState('Could not load settings', escapeHTML(err.message))}`);
    bindShell('/settings');
    return;
  }

  const emailReady = cfg.channels.email_configured;
  const tgReady = cfg.channels.telegram_configured;

  const main = `
    <h1 class="page-title">Notifications</h1>
    <p class="page-sub">Choose how Day Remind reaches you.</p>

    <div class="card channel-card">
      <div class="channel-head">
        <div>
          <h3>Email</h3>
          <p class="channel-sub">Delivered over SMTP.</p>
        </div>
        <label class="switch">
          <input type="checkbox" id="email-enabled" ${cfg.email_enabled ? 'checked' : ''} ${emailReady ? '' : 'disabled'}>
          <span>Enabled</span>
        </label>
      </div>
      ${emailReady ? `
        <div class="field">
          <label for="email-address">Send to</label>
          <input id="email-address" type="email" value="${escapeHTML(cfg.email_address || '')}" autocomplete="email">
        </div>` : `<p class="channel-warn">Not configured on this server — set <code>SMTP_HOST</code> and <code>SMTP_FROM</code>.</p>`}
    </div>

    <div class="card channel-card">
      <div class="channel-head">
        <div>
          <h3>Telegram</h3>
          <p class="channel-sub">${cfg.telegram_linked ? 'This account is linked to a chat.' : 'Link a chat to receive reminders there.'}</p>
        </div>
        <label class="switch">
          <input type="checkbox" id="tg-enabled" ${cfg.telegram_enabled ? 'checked' : ''} ${tgReady && cfg.telegram_linked ? '' : 'disabled'}>
          <span>Enabled</span>
        </label>
      </div>
      ${!tgReady
        ? `<p class="channel-warn">Not configured on this server — set <code>TELEGRAM_BOT_TOKEN</code>.</p>`
        : cfg.telegram_linked
          ? `<div class="channel-actions">
               <span class="badge badge-linked">Linked</span>
               <button class="btn btn-sm" id="tg-unlink">Unlink this chat</button>
             </div>`
          : `<div class="channel-actions">
               <button class="btn btn-sm btn-primary" id="tg-link">Get link code</button>
             </div>
             <div id="tg-link-out"></div>`}
    </div>

    <div class="card channel-card">
      <h3>Advance warning</h3>
      <p class="channel-sub">An extra heads-up before each reminder is due.</p>
      <div class="field">
        <label for="advance-minutes">Warn me</label>
        <select id="advance-minutes">
          ${ADVANCE_CHOICES.map(([v, label]) =>
            `<option value="${v}" ${cfg.advance_minutes === v ? 'selected' : ''}>${label}</option>`).join('')}
        </select>
      </div>
    </div>

    <details class="card channel-card own-server" ${cfg.own_smtp.enabled ? 'open' : ''}>
      <summary><h3>Use my own mail server</h3></summary>
      <p class="channel-sub">Send your reminders through your own SMTP server instead of the instance's.
        Your password is encrypted before it is stored, but whoever runs this server can decrypt it.</p>
      <label class="switch" style="margin-bottom:12px">
        <input type="checkbox" id="u-smtp-enabled" ${cfg.own_smtp.enabled ? 'checked' : ''}>
        <span>Use my server</span>
      </label>
      <div class="form-grid">
        <div class="field"><label for="u-smtp-host">Host</label>
          <input id="u-smtp-host" value="${escapeHTML(cfg.own_smtp.host || '')}" placeholder="smtp.example.com"></div>
        <div class="field"><label for="u-smtp-port">Port</label>
          <input id="u-smtp-port" type="number" value="${cfg.own_smtp.port || 587}"></div>
        <div class="field"><label for="u-smtp-user">Username</label>
          <input id="u-smtp-user" value="${escapeHTML(cfg.own_smtp.username || '')}" autocomplete="off"></div>
        <div class="field">
          <label for="u-smtp-pass">Password ${cfg.own_smtp.password_set ? '<span class="cred-ok">stored</span>' : '<span class="cred-none">not set</span>'}</label>
          <input id="u-smtp-pass" type="password" autocomplete="new-password"
                 placeholder="${cfg.own_smtp.password_set ? 'Leave blank to keep it' : ''}"></div>
        <div class="field"><label for="u-smtp-from">From address</label>
          <input id="u-smtp-from" type="email" value="${escapeHTML(cfg.own_smtp.from || '')}"></div>
        <div class="field"><label for="u-smtp-tls">Encryption</label>
          <select id="u-smtp-tls">${TLS_MODES.map(([v, l]) =>
            `<option value="${v}"${(cfg.own_smtp.tls || 'starttls') === v ? ' selected' : ''}>${l}</option>`).join('')}</select></div>
      </div>
      <button class="btn btn-primary btn-sm" id="u-smtp-save">Save my mail server</button>
    </details>

    <details class="card channel-card own-server" ${cfg.own_telegram.enabled ? 'open' : ''}>
      <summary><h3>Use my own Telegram bot</h3></summary>
      <p class="channel-sub">Deliver to a bot you control. Your own bot only sends notifications —
        commands like /add still go to the shared bot.</p>
      <label class="switch" style="margin-bottom:12px">
        <input type="checkbox" id="u-tg-enabled" ${cfg.own_telegram.enabled ? 'checked' : ''}>
        <span>Use my bot</span>
      </label>
      <div class="field">
        <label for="u-tg-token">Bot token ${cfg.own_telegram.token_set ? '<span class="cred-ok">stored</span>' : '<span class="cred-none">not set</span>'}</label>
        <input id="u-tg-token" type="password" autocomplete="new-password"
               placeholder="${cfg.own_telegram.token_set ? 'Leave blank to keep it' : '123456:ABC-DEF...'}">
      </div>
      <p class="channel-sub">${cfg.own_telegram.chat_set
        ? 'Chat found: <strong>' + cfg.own_telegram.chat_id + '</strong>'
        : 'After saving the token, send <strong>/start</strong> to your bot, then press Find my chat.'}</p>
      <div class="channel-actions">
        <button class="btn btn-primary btn-sm" id="u-tg-save">Save my bot</button>
        <button class="btn btn-sm" id="u-tg-discover" ${cfg.own_telegram.token_set ? '' : 'disabled'}>Find my chat</button>
      </div>
    </details>

    <div class="settings-actions">
      <button class="btn btn-primary" id="notify-save">Save changes</button>
      <button class="btn" id="notify-test">Send test notification</button>
      <a href="#/settings/templates" class="btn btn-ghost btn-sm">Message wording →</a>
      <a href="#/settings" class="btn btn-ghost btn-sm">← Back</a>
    </div>
    <div id="notify-result"></div>
  `;

  document.getElementById('app').innerHTML = shellHTML('/settings', main);
  bindShell('/settings');
  bindNotificationSettings(cfg);
  bindOwnServerForms();
}

function bindNotificationSettings(cfg) {
  const out = document.getElementById('notify-result');

  document.getElementById('notify-save')?.addEventListener('click', async () => {
    const body = {
      email_enabled: !!document.getElementById('email-enabled')?.checked,
      telegram_enabled: !!document.getElementById('tg-enabled')?.checked,
      advance_minutes: Number(document.getElementById('advance-minutes').value),
    };
    const addr = document.getElementById('email-address');
    if (addr) body.email_address = addr.value.trim();
    try {
      await API.patch('/api/me/notify', body);
      showToast('Settings saved');
      await renderNotificationSettings();
    } catch (err) {
      showToast(err.message, 'error');
    }
  });

  document.getElementById('notify-test')?.addEventListener('click', async (e) => {
    const btn = e.currentTarget;
    btn.disabled = true;
    btn.textContent = 'Sending…';
    try {
      const res = await API.post('/api/me/notify/test');
      // Report per channel: one can succeed while the other fails.
      out.innerHTML = Object.entries(res.results).map(([channel, r]) =>
        `<p class="test-result ${r.ok ? 'ok' : 'bad'}">${escapeHTML(channel)}: ${r.ok ? 'delivered' : escapeHTML(r.error)}</p>`
      ).join('');
      showToast('Test finished');
    } catch (err) {
      out.innerHTML = `<p class="test-result bad">${escapeHTML(err.message)}</p>`;
      showToast(err.message, 'error');
    } finally {
      btn.disabled = false;
      btn.textContent = 'Send test notification';
    }
  });

  document.getElementById('tg-link')?.addEventListener('click', async () => {
    try {
      const res = await API.post('/api/me/telegram/link-code');
      const mins = Math.max(1, Math.round((new Date(res.expires_at) - new Date()) / 60000));
      document.getElementById('tg-link-out').innerHTML = `
        <div class="link-code">
          <p>Send this to the bot on Telegram:</p>
          <code>${escapeHTML(res.command)}</code>
          <p class="channel-sub">Expires in ${mins} minute${mins === 1 ? '' : 's'}. Come back and reload once it is linked.</p>
          <button class="btn btn-sm" id="tg-recheck">I've sent it — check now</button>
        </div>`;
      document.getElementById('tg-recheck').addEventListener('click', renderNotificationSettings);
    } catch (err) {
      showToast(err.message, 'error');
    }
  });

  document.getElementById('tg-unlink')?.addEventListener('click', async () => {
    try {
      await API.post('/api/me/telegram/unlink');
      showToast('Telegram unlinked');
      await renderNotificationSettings();
    } catch (err) {
      showToast(err.message, 'error');
    }
  });
}

// ------------------------------------------------- admin: delivery settings

const TLS_MODES = [
  ['starttls', 'STARTTLS (usually port 587)'],
  ['implicit', 'Implicit TLS / SMTPS (usually port 465)'],
  ['none', 'No encryption (local catcher only)'],
];

// secretField renders an input for a credential that is never sent back to the
// browser: blank means "leave the stored one alone".
function secretField(id, label, state) {
  const status = state.unreadable
    ? `<span class="cred-warn">stored, but no longer readable — SESSION_KEY changed, re-enter it</span>`
    : state.set
      ? `<span class="cred-ok">stored${state.hint ? ' (' + escapeHTML(state.hint) + ')' : ''}</span>`
      : `<span class="cred-none">not set</span>`;
  return `
    <div class="field">
      <label for="${id}">${escapeHTML(label)} ${status}</label>
      <input id="${id}" type="password" autocomplete="new-password"
             placeholder="${state.set ? 'Leave blank to keep the stored value' : ''}">
    </div>`;
}

async function renderAdminDelivery() {
  let cfg;
  try {
    cfg = await API.get('/api/admin/settings');
  } catch (err) {
    adminShell(emptyState('Could not load settings', escapeHTML(err.message)), 'Delivery configuration');
    return;
  }
  adminState.settings = cfg;
  const smtp = cfg.smtp;
  const tg = cfg.telegram;

  const body = `
    <div class="card channel-card">
      <div class="channel-head">
        <div>
          <h3>Email (SMTP)</h3>
          <p class="channel-sub">Used for everyone who has not configured their own mail server.</p>
        </div>
        <label class="switch">
          <input type="checkbox" id="a-smtp-enabled" ${smtp.enabled ? 'checked' : ''}>
          <span>Enabled</span>
        </label>
      </div>
      <div class="form-grid">
        <div class="field"><label for="a-smtp-host">Host</label>
          <input id="a-smtp-host" value="${escapeHTML(smtp.host || '')}" placeholder="smtp.example.com"></div>
        <div class="field"><label for="a-smtp-port">Port</label>
          <input id="a-smtp-port" type="number" value="${smtp.port || 587}"></div>
        <div class="field"><label for="a-smtp-user">Username</label>
          <input id="a-smtp-user" value="${escapeHTML(smtp.username || '')}" autocomplete="off"></div>
        ${secretField('a-smtp-pass', 'Password', { set: smtp.password_set, unreadable: smtp.unreadable })}
        <div class="field"><label for="a-smtp-from">From address</label>
          <input id="a-smtp-from" type="email" value="${escapeHTML(smtp.from || '')}" placeholder="reminders@example.com"></div>
        <div class="field"><label for="a-smtp-tls">Encryption</label>
          <select id="a-smtp-tls">${TLS_MODES.map(([v, l]) =>
            `<option value="${v}"${(smtp.tls || 'starttls') === v ? ' selected' : ''}>${l}</option>`).join('')}</select></div>
      </div>
    </div>

    <div class="card channel-card">
      <div class="channel-head">
        <div>
          <h3>Telegram bot</h3>
          <p class="channel-sub">The shared bot. It serves commands and delivers to anyone who has linked a chat.</p>
        </div>
        <label class="switch">
          <input type="checkbox" id="a-tg-enabled" ${tg.enabled ? 'checked' : ''}>
          <span>Enabled</span>
        </label>
      </div>
      ${secretField('a-tg-token', 'Bot token', { set: tg.token_set, hint: tg.token_hint, unreadable: tg.unreadable })}
      <p class="channel-sub">Create one with <strong>@BotFather</strong>. Changing the token takes effect within a few seconds — no restart needed.</p>
    </div>

    <div class="settings-actions">
      <button class="btn btn-primary" id="a-save">Save changes</button>
      <button class="btn" id="a-test">Send test to myself</button>
      <span class="admin-count">${cfg.updated_at ? 'Last changed ' + new Date(cfg.updated_at).toLocaleString() : 'Never configured'}</span>
    </div>
    <div id="a-result"></div>
    <div class="card channel-card" style="margin-top:20px">
      <h3>Recent delivery attempts</h3>
      <div id="a-log"><p class="channel-sub">Loading…</p></div>
    </div>
  `;

  adminShell(body, 'Delivery configuration');
  bindAdminDelivery();
  loadAdminDeliveryLog();
}

function bindAdminDelivery() {
  const out = document.getElementById('a-result');

  document.getElementById('a-save')?.addEventListener('click', async () => {
    const pass = document.getElementById('a-smtp-pass').value;
    const token = document.getElementById('a-tg-token').value;
    const body = {
      smtp: {
        enabled: document.getElementById('a-smtp-enabled').checked,
        host: document.getElementById('a-smtp-host').value.trim(),
        port: Number(document.getElementById('a-smtp-port').value) || 587,
        username: document.getElementById('a-smtp-user').value.trim(),
        from: document.getElementById('a-smtp-from').value.trim(),
        tls: document.getElementById('a-smtp-tls').value,
      },
      telegram: { enabled: document.getElementById('a-tg-enabled').checked },
    };
    // Only send a credential when one was typed: blank means "keep it".
    if (pass !== '') body.smtp.password = pass;
    if (token !== '') body.telegram.bot_token = token;

    try {
      await API.patch('/api/admin/settings', body);
      showToast('Delivery settings saved');
      await renderAdminDelivery();
    } catch (err) {
      showToast(err.message, 'error');
    }
  });

  document.getElementById('a-test')?.addEventListener('click', async (e) => {
    const btn = e.currentTarget;
    btn.disabled = true;
    btn.textContent = 'Sending…';
    try {
      const res = await API.post('/api/admin/settings/test');
      out.innerHTML = Object.entries(res.results).map(([channel, r]) =>
        `<p class="test-result ${r.ok ? 'ok' : 'bad'}">${escapeHTML(channel)}${r.to ? ' → ' + escapeHTML(r.to) : ''}: ${r.ok ? 'delivered' : escapeHTML(r.error)}</p>`
      ).join('');
    } catch (err) {
      out.innerHTML = `<p class="test-result bad">${escapeHTML(err.message)}</p>`;
    } finally {
      btn.disabled = false;
      btn.textContent = 'Send test to myself';
      loadAdminDeliveryLog();
    }
  });
}

async function loadAdminDeliveryLog() {
  const box = document.getElementById('a-log');
  if (!box) return;
  try {
    const res = await API.get('/api/admin/deliveries');
    const rows = res.deliveries || [];
    if (!rows.length) {
      box.innerHTML = `<p class="channel-sub">Nothing delivered yet.</p>`;
      return;
    }
    box.innerHTML = `
      <div class="admin-table-wrap">
        <table class="admin-table">
          <thead><tr><th>When</th><th>Channel</th><th>Kind</th><th>Status</th><th>Detail</th></tr></thead>
          <tbody>${rows.map((d) => `
            <tr>
              <td class="admin-when">${new Date(d.created_at).toLocaleString()}</td>
              <td>${escapeHTML(d.channel)}</td>
              <td>${escapeHTML(d.kind)}</td>
              <td class="${d.status === 'sent' ? '' : 'admin-num is-danger'}">${escapeHTML(d.status)}</td>
              <td class="admin-detail">${escapeHTML(d.detail || '')}</td>
            </tr>`).join('')}</tbody>
        </table>
      </div>`;
  } catch (err) {
    box.innerHTML = `<p class="test-result bad">${escapeHTML(err.message)}</p>`;
  }
}

// ------------------------------------------------------- template editing

const TEMPLATE_FIELDS = [
  ['email_subject', 'Email subject', 'input'],
  ['email_html', 'Email body (HTML)', 'code'],
  ['email_text', 'Email body (plain text)', 'code'],
  ['telegram_text', 'Telegram message', 'code'],
];

function templateEditorHTML(values, opts = {}) {
  const hint = opts.inheritHint || 'Leave a field blank to use the default.';
  return `
    <p class="channel-sub">${escapeHTML(hint)}</p>
    <details class="var-ref">
      <summary>Available variables</summary>
      <ul>${(opts.variables || []).map((v) =>
        `<li><code>{{${escapeHTML(v.Name)}}}</code> — ${escapeHTML(v.Description)}</li>`).join('')}</ul>
    </details>
    ${TEMPLATE_FIELDS.map(([key, label, kind]) => `
      <div class="field">
        <label for="t-${key}">${escapeHTML(label)}</label>
        ${kind === 'input'
          ? `<input id="t-${key}" value="${escapeHTML(values[key] || '')}">`
          : `<textarea id="t-${key}" rows="${key === 'email_html' ? 14 : 7}" spellcheck="false">${escapeHTML(values[key] || '')}</textarea>`}
      </div>`).join('')}
    <div class="field">
      <label for="t-telegram_parse_mode">Telegram formatting</label>
      <select id="t-telegram_parse_mode">
        <option value="markdownv2"${(values.telegram_parse_mode || 'markdownv2') === 'markdownv2' ? ' selected' : ''}>MarkdownV2 (bold, italics)</option>
        <option value="plain"${values.telegram_parse_mode === 'plain' ? ' selected' : ''}>Plain text (safest)</option>
      </select>
    </div>`;
}

function readTemplateEditor() {
  const out = {};
  for (const [key] of TEMPLATE_FIELDS) out[key] = document.getElementById('t-' + key).value;
  out.telegram_parse_mode = document.getElementById('t-telegram_parse_mode').value;
  return out;
}

async function renderAdminTemplates() {
  let cfg;
  try {
    cfg = await API.get('/api/admin/settings');
  } catch (err) {
    adminShell(emptyState('Could not load templates', escapeHTML(err.message)), 'Notification templates');
    return;
  }
  adminState.settings = cfg;

  const body = `
    <div class="card channel-card">
      <h3>Instance templates</h3>
      ${templateEditorHTML(cfg.templates, {
        variables: cfg.variables,
        inheritHint: 'These apply to everyone who has not written their own. Leave a field blank to use the built-in default.',
      })}
      <div class="settings-actions">
        <button class="btn btn-primary" id="t-save">Save templates</button>
        <button class="btn" id="t-preview">Preview</button>
        <button class="btn btn-danger" id="t-reset">Reset to defaults</button>
      </div>
    </div>
    <div id="t-preview-out"></div>
  `;
  adminShell(body, 'Notification templates');
  bindTemplateEditor({
    save: (vals) => API.request('/api/admin/templates', { method: 'PUT', body: JSON.stringify(vals) }),
    preview: (vals) => API.post('/api/admin/templates/preview', vals),
    reset: () => API.post('/api/admin/templates/reset'),
    after: renderAdminTemplates,
  });
}

// bindTemplateEditor is shared by the admin and per-user editors: the only
// difference between them is which endpoints they call.
function bindTemplateEditor(actions) {
  document.getElementById('t-save')?.addEventListener('click', async () => {
    try {
      await actions.save(readTemplateEditor());
      showToast('Templates saved');
      await actions.after();
    } catch (err) {
      showToast(err.message, 'error');
    }
  });

  document.getElementById('t-preview')?.addEventListener('click', async () => {
    const out = document.getElementById('t-preview-out');
    try {
      const res = await actions.preview(readTemplateEditor());
      out.innerHTML = ['due', 'advance'].map((kind) => {
        const p = res[kind];
        return `
          <div class="card channel-card preview-card">
            <h3>Preview — ${kind === 'due' ? 'at the due time' : 'advance notice'}</h3>
            ${(p.warnings || []).map((wmsg) =>
              `<p class="test-result bad">${escapeHTML(wmsg)}</p>`).join('')}
            <p class="preview-label">Subject</p>
            <p class="preview-subject">${escapeHTML(p.subject)}</p>
            <p class="preview-label">HTML</p>
            <iframe class="preview-frame" sandbox srcdoc="${escapeHTML(p.html)}" title="Email preview"></iframe>
            <p class="preview-label">Plain text</p>
            <pre class="preview-pre">${escapeHTML(p.text)}</pre>
            <p class="preview-label">Telegram</p>
            <pre class="preview-pre">${escapeHTML(p.telegram)}</pre>
          </div>`;
      }).join('');
      out.scrollIntoView({ behavior: 'smooth', block: 'start' });
    } catch (err) {
      out.innerHTML = `<p class="test-result bad">${escapeHTML(err.message)}</p>`;
    }
  });

  document.getElementById('t-reset')?.addEventListener('click', async () => {
    if (!confirm('Clear these templates and go back to the defaults?')) return;
    try {
      await actions.reset();
      showToast('Templates reset');
      await actions.after();
    } catch (err) {
      showToast(err.message, 'error');
    }
  });
}

// ------------------------------------------------- user: own servers + wording

function bindOwnServerForms() {
  document.getElementById('u-smtp-save')?.addEventListener('click', async () => {
    const pass = document.getElementById('u-smtp-pass').value;
    const body = {
      enabled: document.getElementById('u-smtp-enabled').checked,
      host: document.getElementById('u-smtp-host').value.trim(),
      port: Number(document.getElementById('u-smtp-port').value) || 587,
      username: document.getElementById('u-smtp-user').value.trim(),
      from: document.getElementById('u-smtp-from').value.trim(),
      tls: document.getElementById('u-smtp-tls').value,
    };
    if (pass !== '') body.password = pass;
    try {
      await API.patch('/api/me/notify/smtp', body);
      showToast('Your mail server was saved');
      await renderNotificationSettings();
    } catch (err) {
      showToast(err.message, 'error');
    }
  });

  document.getElementById('u-tg-save')?.addEventListener('click', async () => {
    const token = document.getElementById('u-tg-token').value.trim();
    const body = { enabled: document.getElementById('u-tg-enabled').checked };
    if (token !== '') body.bot_token = token;
    try {
      await API.patch('/api/me/notify/telegram', body);
      showToast('Your bot was saved');
      await renderNotificationSettings();
    } catch (err) {
      showToast(err.message, 'error');
    }
  });

  document.getElementById('u-tg-discover')?.addEventListener('click', async (e) => {
    const btn = e.currentTarget;
    btn.disabled = true;
    btn.textContent = 'Looking…';
    try {
      await API.post('/api/me/notify/telegram/discover');
      showToast('Chat found');
      await renderNotificationSettings();
    } catch (err) {
      showToast(err.message, 'error');
      btn.disabled = false;
      btn.textContent = 'Find my chat';
    }
  });
}

async function renderMyTemplates() {
  let cfg;
  try {
    cfg = await API.get('/api/me/templates');
  } catch (err) {
    document.getElementById('app').innerHTML = shellHTML('/settings',
      `<h1 class="page-title">Message wording</h1>${emptyState('Could not load templates', escapeHTML(err.message))}`);
    bindShell('/settings');
    return;
  }

  const main = `
    <h1 class="page-title">Message wording</h1>
    <p class="page-sub">Change how your own reminders read. Everyone else keeps the defaults.</p>
    <div class="card channel-card">
      ${templateEditorHTML(cfg.own, {
        variables: cfg.variables,
        inheritHint: 'Leave a field blank to use what the instance provides. Only the fields you fill in are yours.',
      })}
      <div class="settings-actions">
        <button class="btn btn-primary" id="t-save">Save my wording</button>
        <button class="btn" id="t-preview">Preview</button>
        <button class="btn btn-danger" id="t-reset">Use the defaults</button>
        <a href="#/settings/notifications" class="btn btn-ghost btn-sm">← Notifications</a>
      </div>
    </div>
    <div id="t-preview-out"></div>
  `;
  document.getElementById('app').innerHTML = shellHTML('/settings', main);
  bindShell('/settings');
  bindTemplateEditor({
    save: (vals) => API.request('/api/me/templates', { method: 'PUT', body: JSON.stringify(vals) }),
    preview: (vals) => API.post('/api/me/templates/preview', vals),
    // Clearing every field is how a user reverts: the instance wording applies again.
    reset: () => API.request('/api/me/templates', {
      method: 'PUT',
      body: JSON.stringify({
        email_subject: '', email_html: '', email_text: '',
        telegram_text: '', telegram_parse_mode: '',
      }),
    }),
    after: renderMyTemplates,
  });
}
