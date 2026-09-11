window.DR_ADMIN_ROUTES = {
  '/admin': renderAdminDashboard,
  '/admin/users': renderAdminUsers,
  '/admin/reminders': renderAdminReminders,
  '/admin/settings': renderAdminDeliverySettings,
};

const adminState = {
  stats: null,
  users: [],
  reminders: [],
  userQuery: '',
  reminderQuery: '',
  reminderStatus: 'all',
  reminderUserId: '',
};

function adminNavHTML(active) {
  const tabs = [
    ['/admin', 'Dashboard'],
    ['/admin/users', 'Users'],
    ['/admin/reminders', 'Reminders'],
    ['/admin/settings', 'Delivery'],
  ];
  return `
    <nav class="admin-nav" aria-label="Admin">
      ${tabs.map(([path, label]) =>
        `<a href="#${path}" class="${active === path ? 'active' : ''}">${label}</a>`
      ).join('')}
    </nav>
  `;
}

function adminShellHTML(active, mainHTML) {
  return `
    <div class="layout">
      <aside class="sidebar">
        <div class="brand"><span>Day</span> Remind</div>
        <div class="admin-badge">Admin panel</div>
        ${adminNavHTML(active)}
        <div class="sidebar-footer">
          <a href="#/today" class="btn btn-sm" style="width:100%;text-align:center">← Back to app</a>
          <button class="btn btn-sm" id="theme-toggle">Theme</button>
          <button class="btn btn-sm" id="logout-btn">Logout</button>
        </div>
      </aside>
      <div class="main">
        <header class="topbar">
          <strong>Administration</strong>
          <span class="admin-user-label">${escapeHTML(state.user?.email || '')}</span>
        </header>
        <main id="main" class="page admin-page">${mainHTML}</main>
      </div>
    </div>
  `;
}

function bindAdminShell() {
  document.getElementById('theme-toggle')?.addEventListener('click', toggleTheme);
  document.getElementById('logout-btn')?.addEventListener('click', async () => {
    await API.post('/api/auth/logout');
    state.user = null;
    location.hash = '#/login';
    renderAuth(false);
  });
}

async function loadAdminStats() {
  adminState.stats = await API.get('/api/admin/stats');
  return adminState.stats;
}

async function loadAdminUsers(query = adminState.userQuery) {
  const params = new URLSearchParams();
  if (query) params.set('q', query);
  params.set('limit', '200');
  const data = await API.get('/api/admin/users?' + params.toString());
  adminState.users = data.users || [];
  return adminState.users;
}

async function loadAdminReminders() {
  const params = new URLSearchParams();
  if (adminState.reminderQuery) params.set('q', adminState.reminderQuery);
  if (adminState.reminderStatus && adminState.reminderStatus !== 'all') {
    params.set('status', adminState.reminderStatus);
  }
  if (adminState.reminderUserId) params.set('user_id', adminState.reminderUserId);
  params.set('limit', '200');
  const data = await API.get('/api/admin/reminders?' + params.toString());
  adminState.reminders = data.reminders || [];
  return adminState.reminders;
}

async function renderAdminDashboard() {
  try {
    const stats = await loadAdminStats();
    const main = `
      <h1 class="page-title">Dashboard</h1>
      <p class="page-sub">System overview</p>
      <div class="stats-row">
        <div class="stat-card"><strong>${stats.users}</strong><span>Users</span></div>
        <div class="stat-card"><strong>${stats.reminders}</strong><span>Reminders</span></div>
        <div class="stat-card"><strong>${stats.sessions}</strong><span>Active sessions</span></div>
      </div>
      <div class="admin-quick-grid">
        <a href="#/admin/users" class="card settings-card">
          <h3>Manage users</h3>
          <p>Create, edit, revoke sessions, or remove accounts.</p>
        </a>
        <a href="#/admin/reminders" class="card settings-card">
          <h3>Manage reminders</h3>
          <p>View and act on reminders across all users.</p>
        </a>
        <a href="#/admin/settings" class="card settings-card">
          <h3>Delivery settings</h3>
          <p>Configure email (Resend) and Telegram bot for alerts.</p>
        </a>
      </div>
    `;
    document.getElementById('app').innerHTML = adminShellHTML('/admin', main);
    bindAdminShell();
  } catch (err) {
    showToast(err.message, 'error');
  }
}

async function renderAdminUsers() {
  try {
    await loadAdminUsers();
    const rows = adminState.users.map((user) => `
      <tr>
        <td>
          <div class="cell-primary">${escapeHTML(user.email)}</div>
          <div class="cell-secondary">${escapeHTML(user.display_name || '—')}</div>
        </td>
        <td>${user.is_admin ? '<span class="badge badge-admin">Admin</span>' : '—'}</td>
        <td>${user.notifications_paused ? '<span class="badge badge-paused">Paused</span>' : 'Active'}</td>
        <td class="mono-cell">${formatAdminDate(user.created_at)}</td>
        <td class="table-actions">
          <button class="btn btn-sm" data-user-edit="${user.id}">Edit</button>
          <button class="btn btn-sm" data-user-sessions="${user.id}">Revoke</button>
          <button class="btn btn-sm btn-danger" data-user-delete="${user.id}">Delete</button>
        </td>
      </tr>
    `).join('');

    const main = `
      <div class="admin-toolbar">
        <div>
          <h1 class="page-title">Users</h1>
          <p class="page-sub">${adminState.users.length} shown</p>
        </div>
        <button class="btn btn-primary btn-sm" id="admin-create-user">+ New user</button>
      </div>
      <div class="admin-filters card">
        <div class="field" style="margin:0">
          <label for="user-search">Search email</label>
          <input id="user-search" type="search" value="${escapeHTML(adminState.userQuery)}" placeholder="Search by email">
        </div>
      </div>
      <div class="table-wrap card">
        <table class="data-table">
          <thead>
            <tr>
              <th>User</th>
              <th>Role</th>
              <th>Notifications</th>
              <th>Created</th>
              <th>Actions</th>
            </tr>
          </thead>
          <tbody>
            ${rows || '<tr><td colspan="5" class="table-empty">No users found</td></tr>'}
          </tbody>
        </table>
      </div>
    `;
    document.getElementById('app').innerHTML = adminShellHTML('/admin/users', main);
    bindAdminShell();
    bindAdminUserEvents();
  } catch (err) {
    showToast(err.message, 'error');
  }
}

function bindAdminUserEvents() {
  document.getElementById('admin-create-user')?.addEventListener('click', openCreateUserDrawer);
  const search = document.getElementById('user-search');
  let searchTimer;
  search?.addEventListener('input', () => {
    clearTimeout(searchTimer);
    searchTimer = setTimeout(() => {
      adminState.userQuery = search.value.trim();
      renderAdminUsers();
    }, 300);
  });

  document.querySelectorAll('[data-user-edit]').forEach((btn) => {
    btn.addEventListener('click', () => {
      const user = adminState.users.find((u) => u.id === btn.dataset.userEdit);
      if (user) openEditUserDrawer(user);
    });
  });
  document.querySelectorAll('[data-user-sessions]').forEach((btn) => {
    btn.addEventListener('click', async () => {
      if (!confirm('Revoke all sessions for this user?')) return;
      try {
        const data = await API.delete(`/api/admin/users/${btn.dataset.userSessions}/sessions`);
        showToast(`Revoked ${data.revoked} session(s)`);
      } catch (err) {
        showToast(err.message, 'error');
      }
    });
  });
  document.querySelectorAll('[data-user-delete]').forEach((btn) => {
    btn.addEventListener('click', async () => {
      if (!confirm('Delete this user and all their reminders?')) return;
      try {
        await API.delete(`/api/admin/users/${btn.dataset.userDelete}`);
        showToast('User deleted');
        renderAdminUsers();
      } catch (err) {
        showToast(err.message, 'error');
      }
    });
  });
}

function openCreateUserDrawer() {
  const drawer = openDrawer('New user', `
    <div class="field"><label for="u-email">Email</label><input id="u-email" type="email" required></div>
    <div class="field"><label for="u-password">Password</label><input id="u-password" type="password" minlength="8" required></div>
    <div class="field"><label for="u-name">Display name</label><input id="u-name"></div>
    <div class="field"><label for="u-tz">Timezone</label><input id="u-tz" value="${escapeHTML(Intl.DateTimeFormat().resolvedOptions().timeZone)}"></div>
    <div class="field"><label class="checkbox-label"><input id="u-admin" type="checkbox"> Grant admin access</label></div>
  `, `<button class="btn" type="button" data-close-drawer>Cancel</button><button class="btn btn-primary" id="save-user">Create</button>`);
  drawer.querySelector('#save-user').addEventListener('click', async () => {
    const email = document.getElementById('u-email').value.trim();
    const password = document.getElementById('u-password').value;
    if (!email || password.length < 8) {
      showToast('Email and password (8+) required', 'error');
      return;
    }
    try {
      await API.post('/api/admin/users', {
        email,
        password,
        display_name: document.getElementById('u-name').value,
        timezone: document.getElementById('u-tz').value || Intl.DateTimeFormat().resolvedOptions().timeZone,
        is_admin: document.getElementById('u-admin').checked,
      });
      closeDrawer();
      showToast('User created');
      renderAdminUsers();
    } catch (err) {
      showToast(err.message, 'error');
    }
  });
}

function openEditUserDrawer(user) {
  const drawer = openDrawer('Edit user', `
    <div class="field"><label for="u-email">Email</label><input id="u-email" type="email" value="${escapeHTML(user.email)}" required></div>
    <div class="field"><label for="u-name">Display name</label><input id="u-name" value="${escapeHTML(user.display_name || '')}"></div>
    <div class="field"><label for="u-tz">Timezone</label><input id="u-tz" value="${escapeHTML(user.timezone || '')}"></div>
    <div class="field"><label for="u-password">New password (optional)</label><input id="u-password" type="password" minlength="8" autocomplete="new-password"></div>
    <div class="field"><label class="checkbox-label"><input id="u-admin" type="checkbox"${user.is_admin ? ' checked' : ''}> Admin access</label></div>
    <div class="field"><label class="checkbox-label"><input id="u-paused" type="checkbox"${user.notifications_paused ? ' checked' : ''}> Notifications paused</label></div>
  `, `<button class="btn" type="button" data-close-drawer>Cancel</button><button class="btn btn-primary" id="save-user">Save</button>`);
  drawer.querySelector('#save-user').addEventListener('click', async () => {
    const body = {
      email: document.getElementById('u-email').value.trim(),
      display_name: document.getElementById('u-name').value,
      timezone: document.getElementById('u-tz').value,
      is_admin: document.getElementById('u-admin').checked,
      notifications_paused: document.getElementById('u-paused').checked,
    };
    const password = document.getElementById('u-password').value;
    if (password) body.password = password;
    try {
      await API.patch(`/api/admin/users/${user.id}`, body);
      closeDrawer();
      showToast('User updated');
      if (user.id === state.user?.id) {
        state.user = await API.get('/api/me');
      }
      renderAdminUsers();
    } catch (err) {
      showToast(err.message, 'error');
    }
  });
}

async function renderAdminReminders() {
  try {
    await loadAdminUsers();
    await loadAdminReminders();
    const userOptions = adminState.users.map((u) =>
      `<option value="${u.id}"${adminState.reminderUserId === u.id ? ' selected' : ''}>${escapeHTML(u.email)}</option>`
    ).join('');
    const statusChips = ['all', 'scheduled', 'paused', 'done', 'cancelled'].map((f) =>
      `<button class="filter-chip ${adminState.reminderStatus === f ? 'active' : ''}" data-status="${f}">${f}</button>`
    ).join('');

    const rows = adminState.reminders.map((item) => {
      const user = adminState.users.find((u) => u.id === item.user_id);
      return `
        <tr>
          <td>
            <div class="cell-primary">${escapeHTML(item.title)}</div>
            <div class="cell-secondary">${escapeHTML(item.description || '—')}</div>
          </td>
          <td>${escapeHTML(user?.email || item.user_id)}</td>
          <td class="mono-cell">${formatDue(item.due_at)}</td>
          <td>${statusBadge(item)} ${escapeHTML(item.status)}</td>
          <td class="table-actions">
            ${item.status !== 'done' ? `<button class="btn btn-sm" data-r-done="${item.id}">Done</button>` : ''}
            ${item.status === 'paused'
              ? `<button class="btn btn-sm" data-r-resume="${item.id}">Resume</button>`
              : item.status === 'scheduled'
                ? `<button class="btn btn-sm" data-r-pause="${item.id}">Pause</button>`
                : ''}
            <button class="btn btn-sm btn-danger" data-r-delete="${item.id}">Delete</button>
          </td>
        </tr>
      `;
    }).join('');

    const main = `
      <div class="admin-toolbar">
        <div>
          <h1 class="page-title">Reminders</h1>
          <p class="page-sub">${adminState.reminders.length} shown</p>
        </div>
        <button class="btn btn-primary btn-sm" id="admin-create-reminder">+ New reminder</button>
      </div>
      <div class="admin-filters card">
        <div class="admin-filter-grid">
          <div class="field" style="margin:0">
            <label for="reminder-search">Search</label>
            <input id="reminder-search" type="search" value="${escapeHTML(adminState.reminderQuery)}" placeholder="Title or description">
          </div>
          <div class="field" style="margin:0">
            <label for="reminder-user">User</label>
            <select id="reminder-user">
              <option value="">All users</option>
              ${userOptions}
            </select>
          </div>
        </div>
        <div class="filter-row" style="margin-top:12px;margin-bottom:0">${statusChips}</div>
      </div>
      <div class="table-wrap card">
        <table class="data-table">
          <thead>
            <tr>
              <th>Reminder</th>
              <th>User</th>
              <th>Due</th>
              <th>Status</th>
              <th>Actions</th>
            </tr>
          </thead>
          <tbody>
            ${rows || '<tr><td colspan="5" class="table-empty">No reminders found</td></tr>'}
          </tbody>
        </table>
      </div>
    `;
    document.getElementById('app').innerHTML = adminShellHTML('/admin/reminders', main);
    bindAdminShell();
    bindAdminReminderEvents();
  } catch (err) {
    showToast(err.message, 'error');
  }
}

function bindAdminReminderEvents() {
  document.getElementById('admin-create-reminder')?.addEventListener('click', openCreateAdminReminderDrawer);

  const search = document.getElementById('reminder-search');
  let searchTimer;
  search?.addEventListener('input', () => {
    clearTimeout(searchTimer);
    searchTimer = setTimeout(() => {
      adminState.reminderQuery = search.value.trim();
      renderAdminReminders();
    }, 300);
  });

  document.getElementById('reminder-user')?.addEventListener('change', (e) => {
    adminState.reminderUserId = e.target.value;
    renderAdminReminders();
  });

  document.querySelectorAll('[data-status]').forEach((chip) => {
    chip.addEventListener('click', () => {
      adminState.reminderStatus = chip.dataset.status;
      renderAdminReminders();
    });
  });

  document.querySelectorAll('[data-r-done]').forEach((btn) => {
    btn.addEventListener('click', async () => {
      try {
        await API.post(`/api/admin/reminders/${btn.dataset.rDone}/done`);
        showToast('Marked done');
        renderAdminReminders();
      } catch (err) {
        showToast(err.message, 'error');
      }
    });
  });
  document.querySelectorAll('[data-r-pause]').forEach((btn) => {
    btn.addEventListener('click', async () => {
      try {
        await API.post(`/api/admin/reminders/${btn.dataset.rPause}/pause`);
        showToast('Paused');
        renderAdminReminders();
      } catch (err) {
        showToast(err.message, 'error');
      }
    });
  });
  document.querySelectorAll('[data-r-resume]').forEach((btn) => {
    btn.addEventListener('click', async () => {
      try {
        await API.post(`/api/admin/reminders/${btn.dataset.rResume}/resume`);
        showToast('Resumed');
        renderAdminReminders();
      } catch (err) {
        showToast(err.message, 'error');
      }
    });
  });
  document.querySelectorAll('[data-r-delete]').forEach((btn) => {
    btn.addEventListener('click', async () => {
      if (!confirm('Delete this reminder?')) return;
      try {
        await API.delete(`/api/admin/reminders/${btn.dataset.rDelete}`);
        showToast('Reminder deleted');
        renderAdminReminders();
      } catch (err) {
        showToast(err.message, 'error');
      }
    });
  });
}

function openCreateAdminReminderDrawer() {
  const userOptions = adminState.users.map((u) =>
    `<option value="${u.id}">${escapeHTML(u.email)}</option>`
  ).join('');
  const defaultDue = new Date();
  defaultDue.setHours(defaultDue.getHours() + 1, 0, 0, 0);
  const local = new Date(defaultDue.getTime() - defaultDue.getTimezoneOffset() * 60000).toISOString().slice(0, 16);

  const drawer = openDrawer('New reminder', `
    <div class="field"><label for="r-user">User</label><select id="r-user" required>${userOptions}</select></div>
    <div class="field"><label for="r-title">Title</label><input id="r-title" required></div>
    <div class="field"><label for="r-desc">Description</label><textarea id="r-desc" rows="3"></textarea></div>
    <div class="field"><label for="r-due">Due</label><input id="r-due" type="datetime-local" value="${local}" required></div>
    <div class="field"><label for="r-priority">Priority</label>
      <select id="r-priority"><option value="normal">Normal</option><option value="high">High</option><option value="urgent">Urgent</option><option value="low">Low</option></select>
    </div>
  `, `<button class="btn" type="button" data-close-drawer>Cancel</button><button class="btn btn-primary" id="save-reminder">Create</button>`);
  drawer.querySelector('#save-reminder').addEventListener('click', async () => {
    const title = document.getElementById('r-title').value.trim();
    const dueLocal = document.getElementById('r-due').value;
    const userId = document.getElementById('r-user').value;
    if (!title || !dueLocal || !userId) {
      showToast('User, title, and due date required', 'error');
      return;
    }
    try {
      await API.post('/api/admin/reminders', {
        user_id: userId,
        title,
        description: document.getElementById('r-desc').value,
        due_at: new Date(dueLocal).toISOString(),
        timezone: Intl.DateTimeFormat().resolvedOptions().timeZone,
        priority: document.getElementById('r-priority').value,
      });
      closeDrawer();
      showToast('Reminder created');
      renderAdminReminders();
    } catch (err) {
      showToast(err.message, 'error');
    }
  });
}

function formatAdminDate(iso) {
  if (!iso) return '—';
  return new Date(iso).toLocaleString(undefined, {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
  });
}

async function renderAdminDeliverySettings() {
  let settings;
  try {
    settings = await API.get('/api/admin/settings/delivery');
  } catch (err) {
    showToast(err.message, 'error');
    return;
  }
  const email = settings.email || {};
  const telegram = settings.telegram || {};

  const main = `
    <div class="admin-toolbar">
      <div>
        <h1 class="page-title">Delivery settings</h1>
        <p class="page-sub">Configure how the app sends email and Telegram alerts.</p>
      </div>
    </div>
    <div class="card alert-channel-card">
      <h3>Email (Resend)</h3>
      <p class="channel-help">Users can enable email alerts once Resend is configured. Env vars <code>RESEND_API_KEY</code> and <code>EMAIL_FROM</code> are used as fallback.</p>
      <label class="checkbox-label"><input id="email-enabled" type="checkbox"${email.enabled ? ' checked' : ''}> Enable email delivery</label>
      <div class="field"><label for="email-from">From address</label><input id="email-from" type="email" value="${escapeHTML(email.from || '')}" placeholder="reminders@yourdomain.com"></div>
      <div class="field"><label for="resend-key">Resend API key</label><input id="resend-key" type="password" autocomplete="new-password" placeholder="${email.api_key_hint ? 'Saved ' + escapeHTML(email.api_key_hint) + ' — leave blank to keep' : 're_...'}"></div>
      <div class="inline-actions">
        <input id="test-email-to" type="email" placeholder="Test recipient email" style="flex:1">
        <button class="btn btn-sm" id="test-email">Send test</button>
      </div>
    </div>
    <div class="card alert-channel-card">
      <h3>Telegram bot</h3>
      <p class="channel-help">Create a bot via <a href="https://t.me/BotFather" target="_blank" rel="noopener">@BotFather</a>. Users add their chat ID in Settings → Alert channels.</p>
      <label class="checkbox-label"><input id="telegram-enabled" type="checkbox"${telegram.enabled ? ' checked' : ''}> Enable Telegram delivery</label>
      <div class="field"><label for="telegram-token">Bot token</label><input id="telegram-token" type="password" autocomplete="new-password" placeholder="${telegram.bot_token_hint ? 'Saved ' + escapeHTML(telegram.bot_token_hint) + ' — leave blank to keep' : '123456:ABC...'}"></div>
      <div class="inline-actions">
        <input id="test-telegram-to" inputmode="numeric" placeholder="Test chat ID" style="flex:1">
        <button class="btn btn-sm" id="test-telegram">Send test</button>
      </div>
    </div>
    <button class="btn btn-primary" id="save-delivery">Save delivery settings</button>
  `;
  document.getElementById('app').innerHTML = adminShellHTML('/admin/settings', main);
  bindAdminShell();

  document.getElementById('save-delivery')?.addEventListener('click', async () => {
    const body = {
      email_enabled: document.getElementById('email-enabled').checked,
      email_from: document.getElementById('email-from').value.trim(),
      telegram_enabled: document.getElementById('telegram-enabled').checked,
    };
    const key = document.getElementById('resend-key').value.trim();
    const token = document.getElementById('telegram-token').value.trim();
    if (key) body.resend_api_key = key;
    if (token) body.telegram_bot_token = token;
    try {
      await API.patch('/api/admin/settings/delivery', body);
      showToast('Delivery settings saved');
      renderAdminDeliverySettings();
    } catch (err) {
      showToast(err.message, 'error');
    }
  });

  document.getElementById('test-email')?.addEventListener('click', async () => {
    const to = document.getElementById('test-email-to').value.trim();
    if (!to) {
      showToast('Enter a test email address', 'error');
      return;
    }
    try {
      await API.post('/api/admin/settings/delivery/test', { channel: 'email', to });
      showToast('Test email sent');
    } catch (err) {
      showToast(err.message, 'error');
    }
  });

  document.getElementById('test-telegram')?.addEventListener('click', async () => {
    const to = document.getElementById('test-telegram-to').value.trim();
    if (!to) {
      showToast('Enter a test chat ID', 'error');
      return;
    }
    try {
      await API.post('/api/admin/settings/delivery/test', { channel: 'telegram', to });
      showToast('Test Telegram message sent');
    } catch (err) {
      showToast(err.message, 'error');
    }
  });
}
