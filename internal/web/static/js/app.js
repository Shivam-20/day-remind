const state = {
  user: null,
  reminders: [],
  filter: 'all',
  search: '',
  listFilter: '',
  searchFocus: false,
};

function allRoutes() {
  return {
    '/today': renderToday,
    '/reminders': renderReminders,
    '/settings': renderSettings,
    '/settings/vacation': renderVacationSettings,
    '/settings/notifications': renderNotificationSettings,
    // The old duplicate wording page now lands on the single wording screen.
    '/settings/templates': () => renderMessageWording(),
    '/admin': renderAdmin,
    ...(window.DR_FEATURE_ROUTES || {}),
  };
}

function isAdmin() {
  return state.user?.role === 'admin';
}

if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', boot);
} else {
  boot();
}

async function boot() {
  const hash = location.hash.replace('#', '') || '/today';
  if (hash === '/login' || hash === '/signup') {
    renderAuth(hash === '/signup');
    return;
  }
  try {
    state.user = await API.get('/api/me');
    await renderApp(hash);
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
          <div class="brand"><img class="brand-logo" src="static/icons/logo.svg" alt="" aria-hidden="true"><span class="brand-name">Day Remind</span></div>
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
        <p style="text-align:center;margin-bottom:0"><button type="button" class="icon-btn" id="theme-toggle" aria-label="Toggle theme">${actionIconSVG('moon')}</button></p>
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

function showKeyboardShortcutsDrawer() {
  openDrawer('Keyboard shortcuts', `
    <dl class="detail-list">
      <dt><kbd>N</kbd></dt><dd>New reminder</dd>
      <dt><kbd>⌘K</kbd> / <kbd>/</kbd></dt><dd>Search reminders</dd>
      <dt><kbd>?</kbd></dt><dd>Show keyboard shortcuts</dd>
      <dt><kbd>Esc</kbd></dt><dd>Close drawer or menu</dd>
    </dl>
  `, '<button class="btn" type="button" data-close-drawer>Close</button>');
}

// ---------------------------------------------------------------- shell

function navCounts() {
  const rs = state.reminders || [];
  if (!rs.length) return {};
  const now = new Date();
  const todayN = rs.filter((r) => {
    if (r.status === 'done' || r.status === 'cancelled') return false;
    const d = new Date(r.due_at);
    return d.toDateString() === now.toDateString() || d < now;
  }).length;
  const overdueN = rs.filter((r) => isOverdue(r)).length;
  const listsN = new Set(rs.filter((r) => r.list_id).map((r) => r.list_id)).size;
  return { today: todayN, overdue: overdueN, all: rs.length, lists: listsN };
}

function shellNavHTML(active) {
  const c = navCounts();
  const links = [
    { href: '#/today', path: '/today', label: 'Today', icon: 'sun', count: c.today, hot: c.overdue },
    { href: '#/calendar', path: '/calendar', label: 'Calendar', icon: 'calendar' },
    { href: '#/lists', path: '/lists', label: 'Lists', icon: 'list', count: c.lists },
    { href: '#/reminders', path: '/reminders', label: 'All reminders', icon: 'layers', count: c.all },
  ];
  const general = [
    ...(isAdmin() ? [{ href: '#/admin', path: '/admin', label: 'Admin', icon: 'shield' }] : []),
    { href: '#/settings', path: '/settings', label: 'Settings', icon: 'settings', match: (p) => p.startsWith('/settings') },
  ];
  const linkHTML = (l) => {
    const on = l.match ? l.match(active) : active === l.path;
    return `<a href="${l.href}" class="${on ? 'active' : ''}">${iconHTML(l.icon)}${l.label}${l.count != null ? ` <span class="nav-count${l.hot && l.hot > 0 && l.path === '/today' ? ' hot' : ''}">${l.count}</span>` : ''}</a>`;
  };
  return `
    <div class="nav-section">Menu</div>
    ${links.map(linkHTML).join('')}
    <div class="nav-section">General</div>
    ${general.map(linkHTML).join('')}`;
}

function shellHTML(active, mainHTML, crumbLabel) {
  const vacation = state.user?.notifications_paused
    ? `<div class="vacation-banner"><span>${iconHTML('bell-off')}Notifications paused${state.user.notifications_paused_until ? ' until ' + new Date(state.user.notifications_paused_until).toLocaleString() : ''}</span><button class="btn btn-sm" id="resume-vacation">Resume</button></div>`
    : '';
  const sidebarHidden = isSidebarHidden();
  const menuLabel = sidebarHidden ? 'Show menu' : 'Hide menu';
  const crumb = crumbLabel || (active === '/today' ? 'Today' : active.split('/').filter(Boolean).map((s) => s[0].toUpperCase() + s.slice(1)).join(' · '));
  const who = state.user?.display_name || state.user?.email || 'Day Remind';
  const initial = (who[0] || 'D').toUpperCase();
  return `
    <div class="layout${sidebarHidden ? ' sidebar-hidden' : ''}">
      <aside class="sidebar" aria-label="Main navigation">
        <div class="brand"><img class="brand-logo" src="static/icons/logo.svg" alt="" aria-hidden="true"><span class="brand-name">Day Remind</span></div>
        <nav class="nav">
          ${shellNavHTML(active)}
        </nav>
        <div class="sidebar-footer">
          <span class="user-chip" aria-hidden="true">${escapeHTML(initial)}</span>
          <span class="user-chip-meta"><strong>${escapeHTML(state.user?.display_name || '')}</strong><span>${escapeHTML(state.user?.email || '')}</span></span>
          <button class="icon-btn" id="theme-toggle" aria-label="Toggle theme">${actionIconSVG('moon')}</button>
          <button class="icon-btn" id="logout-btn" aria-label="Log out">${actionIconSVG('log-out')}</button>
        </div>
      </aside>
      <div class="main">
        ${vacation}
        <header class="topbar">
          <div class="topbar-start">
            <button type="button" class="icon-btn sidebar-top-toggle" data-sidebar-toggle aria-expanded="${sidebarHidden ? 'false' : 'true'}" aria-label="${menuLabel}">${actionIconSVG('menu')}</button>
            <span class="topbar-crumb">${escapeHTML(crumb)} <span>· ${new Date().toLocaleDateString(undefined, { weekday: 'short', month: 'short', day: 'numeric' })}</span></span>
            ${isAdmin() ? '<span class="badge-admin">Admin</span>' : ''}
          </div>
          <button type="button" class="topbar-search" id="topbar-search">
            ${iconHTML('search')}
            Search reminders…
            <kbd>⌘K</kbd>
          </button>
          <button class="btn btn-primary btn-sm btn-with-icon" id="add-reminder-top">${iconHTML('plus')}<span class="btn-label">New reminder</span></button>
        </header>
        <main id="main" class="page">${mainHTML}</main>
      </div>
    </div>
    <nav class="bottom-nav" aria-label="Mobile">
      <a href="#/today" class="${active === '/today' ? 'active' : ''}">${iconHTML('sun')}Today</a>
      <a href="#/calendar" class="${active === '/calendar' ? 'active' : ''}">${iconHTML('calendar')}Calendar</a>
      <a href="#/lists" class="${active === '/lists' ? 'active' : ''}">${iconHTML('list')}Lists</a>
      <a href="#/reminders" class="${active === '/reminders' ? 'active' : ''}">${iconHTML('layers')}All</a>
      <a href="#/settings" class="${active.startsWith('/settings') ? 'active' : ''}">${iconHTML('settings')}Settings</a>
    </nav>
    <button class="fab" id="fab-add" aria-label="New reminder">${actionIconSVG('plus')}</button>
  `;
}

function focusGlobalSearch() {
  if ((location.hash.replace('#', '') || '/today') !== '/reminders') {
    state.searchFocus = true;
    location.hash = '#/reminders';
    // hashchange handler re-renders; nothing else to do.
  } else {
    document.getElementById('list-search')?.focus();
    document.getElementById('list-search')?.select();
  }
}

function bindShell(active) {
  document.querySelectorAll('[data-sidebar-toggle]').forEach((btn) => {
    btn.addEventListener('click', toggleSidebarHidden);
  });
  document.querySelectorAll('.layout .nav a').forEach((link) => {
    link.addEventListener('click', () => {
      document.querySelector('.layout')?.classList.remove('sidebar-visible-mobile');
    });
  });
  setSidebarHidden(isSidebarHidden());
  document.getElementById('theme-toggle')?.addEventListener('click', toggleTheme);
  document.getElementById('topbar-search')?.addEventListener('click', focusGlobalSearch);
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
  const _openCreate = typeof openCreateDrawerEnhanced === 'function' ? openCreateDrawerEnhanced : openCreateDrawer;
  document.getElementById('add-reminder-top')?.addEventListener('click', () => _openCreate());
  document.getElementById('fab-add')?.addEventListener('click', () => _openCreate());
  if (window._shellKbdHandler) document.removeEventListener('keydown', window._shellKbdHandler);
  window._shellKbdHandler = (e) => {
    if (e.target.tagName === 'INPUT' || e.target.tagName === 'TEXTAREA' || e.target.tagName === 'SELECT') return;
    if (document.querySelector('.drawer')) return;
    if ((e.key === 'n' || e.key === 'N') && !e.metaKey && !e.ctrlKey) { e.preventDefault(); _openCreate(); }
    if (e.key === '?') showKeyboardShortcutsDrawer();
    if ((e.metaKey || e.ctrlKey) && (e.key === 'k' || e.key === 'K')) { e.preventDefault(); focusGlobalSearch(); }
    if (e.key === '/' && !e.metaKey && !e.ctrlKey) { e.preventDefault(); focusGlobalSearch(); }
  };
  document.addEventListener('keydown', window._shellKbdHandler);
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
  if (route.startsWith('/settings')) {
    const feature = (window.DR_FEATURE_ROUTES || {})[route];
    if (feature && route !== '/settings') {
      await feature(route);
      return;
    }
    const table = allRoutes();
    if (table[route]) {
      await table[route](route);
      return;
    }
    await renderSettings(route);
    return;
  }
  const table = allRoutes();
  const fn = table[route] || table['/today'];
  await fn(route);
}

async function loadReminders(params = '') {
  const data = await API.get('/api/reminders' + params);
  state.reminders = data.reminders || [];
  return state.reminders;
}

// ---------------------------------------------------------------- reminder actions

function snoozeUntilTomorrow9am() {
  const d = new Date();
  d.setDate(d.getDate() + 1);
  d.setHours(9, 0, 0, 0);
  return d;
}

function confirmDeleteReminder(id, onConfirm) {
  const drawer = openDrawer('Delete reminder', `
    <p style="margin-top:0">This reminder will be permanently deleted.</p>
    <p style="color:var(--danger);margin-bottom:0">This action cannot be undone.</p>
  `, `<button class="btn" type="button" data-close-drawer>Cancel</button><button class="btn btn-danger" id="confirm-delete-go">Delete permanently</button>`);
  drawer.querySelector('#confirm-delete-go')?.addEventListener('click', async () => {
    try {
      await API.delete(`/api/reminders/${id}`);
      closeDrawer();
      showToast('Reminder deleted');
      await onConfirm();
    } catch (err) {
      showToast(err.message, 'error');
    }
  });
}

async function runReminderAction(btn, rerender) {
  const id = btn.dataset.id;
  const action = btn.dataset.action;
  if (action === 'detail') {
    const open = window.openReminderDrawer;
    if (typeof open === 'function') await open(id);
    else showToast('Details not available', 'error');
    return;
  }
  if (action === 'pin') await API.patch(`/api/reminders/${id}`, { pinned: true });
  if (action === 'unpin') await API.patch(`/api/reminders/${id}`, { pinned: false });
  if (action === 'done') await API.post(`/api/reminders/${id}/done`);
  if (action === 'pause') await API.post(`/api/reminders/${id}/pause`);
  if (action === 'resume') await API.post(`/api/reminders/${id}/resume`);
  if (action === 'snooze') {
    let until;
    if (btn.dataset.snooze === 'tomorrow9') {
      until = snoozeUntilTomorrow9am();
    } else {
      const mins = parseInt(btn.dataset.snoozeMinutes, 10);
      if (!mins) {
        showToast('Invalid snooze', 'error');
        return;
      }
      until = new Date(Date.now() + mins * 60 * 1000);
    }
    await API.post(`/api/reminders/${id}/snooze`, { until: until.toISOString() });
  }
  if (action === 'delete') {
    confirmDeleteReminder(id, rerender);
    return;
  }
  showToast(action === 'done' ? 'Nice — done' : 'Updated');
  await rerender();
}

function bindReminderActions(container, rerender) {
  container.querySelectorAll('[data-action]').forEach((btn) => {
    if (btn.classList.contains('row-check')) {
      btn.addEventListener('change', async () => {
        try {
          await runReminderAction(btn, rerender);
        } catch (err) {
          btn.checked = false;
          showToast(err.message, 'error');
        }
      });
      return;
    }
    btn.addEventListener('click', async () => {
      try {
        closeAllMenus();
        await runReminderAction(btn, rerender);
      } catch (err) {
        showToast(err.message, 'error');
      }
    });
  });
}

// ---------------------------------------------------------------- today

function statTilesHTML(m) {
  const tile = (pill, cls, dotColor, label, num, ctx) => `
    <button class="stat-card stat-tile ${cls}" type="button" data-pill="${pill}">
      <span class="stat-lbl"><i style="background:${dotColor}" aria-hidden="true"></i>${label}</span>
      <strong>${num}</strong>
      <span class="stat-ctx">${ctx}</span>
    </button>`;
  return `
    <div class="stats-row" role="group" aria-label="Summary">
      ${tile('today', 'accent', 'var(--accent)', 'Due today', m.today, m.todayCtx)}
      ${tile('overdue', 'danger', 'var(--danger)', 'Overdue', m.overdue, m.overdueCtx)}
      ${tile('upcoming', '', 'var(--accent-2)', 'Upcoming', m.upcoming, m.upcomingCtx)}
      ${tile('paused', '', 'var(--text-3)', 'Paused', m.paused, 'resume anytime')}
      ${tile('done', 'done', 'var(--success)', 'Completed', m.doneWeek, 'this week')}
    </div>`;
}

function bindStatTiles() {
  document.querySelectorAll('.stat-tile[data-pill]').forEach((tile) => {
    tile.addEventListener('click', () => {
      const which = tile.dataset.pill;
      if (which === 'today') document.getElementById('today-list')?.scrollIntoView({ behavior: 'smooth' });
      else if (which === 'upcoming') document.getElementById('upcoming-groups')?.scrollIntoView({ behavior: 'smooth' });
      else {
        state.filter = which;
        location.hash = '#/reminders';
        renderReminders();
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
    return d > now
      && (r.status === 'scheduled' || r.status === 'paused')
      && d.toDateString() !== now.toDateString();
  });
  const paused = state.reminders.filter((r) => r.status === 'paused').length;
  const upcomingAll = state.reminders.filter((r) => {
    const d = new Date(r.due_at);
    return d > now && r.status === 'scheduled';
  });
  const upcomingCount = upcomingAll.length;
  const doneToday = state.reminders.filter((r) => {
    const d = new Date(r.due_at);
    return r.status === 'done' && d.toDateString() === now.toDateString();
  }).length;
  const doneWeek = state.reminders.filter((r) => {
    if (r.status !== 'done' || !r.completed_at) return false;
    return (now - new Date(r.completed_at)) / 86400000 <= 7;
  }).length;
  const oldestOverdue = overdue.reduce((a, r) => (new Date(r.due_at) < new Date(a.due_at) ? r : a), overdue[0]);
  const nextUp = upcomingAll.slice().sort((a, b) => new Date(a.due_at) - new Date(b.due_at))[0];
  const sortByPinned = (a, b) => (b.pinned ? 1 : 0) - (a.pinned ? 1 : 0);
  today.sort(sortByPinned);
  overdue.sort(sortByPinned);
  upcoming.sort(sortByPinned);

  const main = `
    <div class="page-header-row">
      <div>
        <h1 class="page-title">Today</h1>
        <p class="page-sub">${now.toLocaleDateString(undefined, { weekday: 'long', month: 'long', day: 'numeric' })}</p>
      </div>
    </div>
    ${statTilesHTML({
      today: today.length,
      todayCtx: today.length || doneToday ? `${doneToday} done of ${today.length + doneToday}` : 'nothing scheduled',
      overdue: overdue.length,
      overdueCtx: overdue.length ? `oldest ${overdueAge(oldestOverdue)}` : 'all clear',
      upcoming: upcomingCount,
      upcomingCtx: nextUp ? `next: ${escapeHTML(nextUp.title.length > 26 ? nextUp.title.slice(0, 26) + '…' : nextUp.title)}` : 'nothing ahead',
      paused,
      doneWeek,
    })}
    ${overdue.length ? `
      <div class="sec-head overdue"><h2>Overdue</h2><span class="sec-count">${overdue.length}</span></div>
      <div class="reminder-list">${overdue.map((r) => reminderRowHTML(r)).join('')}</div>` : ''}
    ${today.length ? `
      <div class="sec-head"><h2>Today</h2><span class="sec-count">${today.length}</span></div>
      <div class="reminder-list" id="today-list">${today.map((r) => reminderRowHTML(r)).join('')}</div>` : ''}
    ${upcomingGroupsHTML(upcoming, now)}
  `;
  document.getElementById('app').innerHTML = shellHTML('/today', main);
  bindShell('/today');
  bindStatTiles();
  bindReminderActions(document.getElementById('app'), () => renderToday());
  bindUpcomingShowMore();
}

function upcomingDayLabel(date, now) {
  const tomorrow = new Date(now);
  tomorrow.setDate(now.getDate() + 1);
  const in7 = new Date(now);
  in7.setDate(now.getDate() + 7);
  if (date.toDateString() === tomorrow.toDateString()) return 'Tomorrow';
  if (date <= in7) return date.toLocaleDateString(undefined, { weekday: 'long', month: 'short', day: 'numeric' });
  return date.toLocaleDateString(undefined, { month: 'long', day: 'numeric', year: date.getFullYear() !== now.getFullYear() ? 'numeric' : undefined });
}

function upcomingGroupsHTML(upcoming, now) {
  if (!upcoming.length) return '';
  const groups = [];
  const seen = new Map();
  for (const r of upcoming) {
    const d = new Date(r.due_at);
    const key = d.toDateString();
    if (!seen.has(key)) {
      seen.set(key, groups.length);
      groups.push({ label: upcomingDayLabel(d, now), items: [] });
    }
    groups[seen.get(key)].items.push(r);
  }

  const INITIAL_GROUPS = 3;
  const visibleGroups = groups.slice(0, INITIAL_GROUPS);
  const hiddenGroups = groups.slice(INITIAL_GROUPS);
  const hasMore = hiddenGroups.length > 0;

  function groupHTML(g) {
    return `
      <div class="upcoming-group">
        <div class="sec-head"><h2>${escapeHTML(g.label)}</h2><span class="sec-count">${g.items.length}</span></div>
        <div class="reminder-list">${g.items.map((r) => reminderRowHTML(r)).join('')}</div>
      </div>`;
  }

  const hiddenHTML = hiddenGroups.map(groupHTML).join('');
  const totalHidden = hiddenGroups.reduce((n, g) => n + g.items.length, 0);

  return `
    <div class="sec-head" style="margin-top:28px">
      <h2>Upcoming</h2><span class="sec-count">${upcoming.length}</span>
      <a href="#/reminders" class="sec-link">View all →</a>
    </div>
    <div id="upcoming-groups">
      ${visibleGroups.map(groupHTML).join('')}
      ${hasMore ? `
        <div id="upcoming-hidden" hidden>${hiddenHTML}</div>
        <button class="btn btn-ghost btn-sm" id="upcoming-show-more">Show ${totalHidden} more reminder${totalHidden !== 1 ? 's' : ''}</button>
      ` : ''}
    </div>
  `;
}

function bindUpcomingShowMore() {
  document.getElementById('upcoming-show-more')?.addEventListener('click', (e) => {
    const hidden = document.getElementById('upcoming-hidden');
    const btn = e.currentTarget;
    if (hidden) hidden.hidden = false;
    btn.remove();
  });
}

// ---------------------------------------------------------------- all reminders

function dateGroupLabel(d, now) {
  const tomorrow = new Date(now);
  tomorrow.setDate(now.getDate() + 1);
  if (d.toDateString() === now.toDateString()) return 'Today';
  if (d.toDateString() === tomorrow.toDateString()) return 'Tomorrow';
  return d.toLocaleDateString(undefined, { weekday: 'long', month: 'short', day: 'numeric' });
}

// Groups a flat reminder list into display sections; done items sink to the end.
function groupReminders(items, now) {
  const groups = [];
  const seen = new Map();
  const doneItems = [];
  const push = (key, label, cls, item) => {
    if (!seen.has(key)) {
      seen.set(key, groups.length);
      groups.push({ key, label, cls, items: [] });
    }
    groups[seen.get(key)].items.push(item);
  };
  for (const r of items) {
    if (r.status === 'done') { doneItems.push(r); continue; }
    if (isOverdue(r)) { push('overdue', 'Overdue', 'overdue', r); continue; }
    const d = new Date(r.due_at);
    push(`d-${d.toDateString()}`, dateGroupLabel(d, now), '', r);
  }
  if (doneItems.length) push('done', 'Completed', '', ...doneItems);
  return groups;
}

async function renderReminders() {
  const q = state.search.trim();
  const params = new URLSearchParams();
  if (q) params.set('q', q);
  await loadReminders(params.toString() ? `?${params}` : '');

  let items = state.reminders.slice();
  if (state.filter !== 'all') items = items.filter((r) => r.status === state.filter);
  if (state.listFilter) items = items.filter((r) => r.list_id === state.listFilter);
  items.sort((a, b) => new Date(a.due_at) - new Date(b.due_at));

  const now = new Date();
  const counts = {
    all: state.reminders.length,
    scheduled: state.reminders.filter((r) => r.status === 'scheduled').length,
    paused: state.reminders.filter((r) => r.status === 'paused').length,
    done: state.reminders.filter((r) => r.status === 'done').length,
  };
  const chips = ['all', 'scheduled', 'paused', 'done'].map((f) =>
    `<button class="filter-chip ${state.filter === f ? 'active' : ''}" data-filter="${f}"><b>${f === 'all' ? 'All' : f}</b><span class="chip-count">${counts[f]}</span></button>`
  ).join('');

  const listFilterName = state.listFilter
    ? (state.listFilterName || state.listFilter.slice(0, 8))
    : '';

  const groups = groupReminders(items, now);
  const groupedHTML = groups.length
    ? groups.map((g) => `
        <div class="sec-head ${g.cls}"><h2>${escapeHTML(g.label)}</h2><span class="sec-count">${g.items.length}</span></div>
        <div class="reminder-list">${g.items.map((r) => reminderRowHTML(r)).join('')}</div>`).join('')
    : emptyState(q ? 'No matches' : 'No reminders', q ? `Nothing matches “${escapeHTML(q)}”.` : 'Create one to get started.', '', q ? 'search' : 'inbox');

  const main = `
    <div class="page-header-row">
      <div>
        <h1 class="page-title">All reminders</h1>
        <p class="page-sub">${items.length} shown · ${state.reminders.length} total</p>
      </div>
    </div>
    <div class="list-tools">
      <span class="list-tools-search">${actionIconSVG('search')}<input id="list-search" type="search" placeholder="Search by title…" value="${escapeHTML(state.search)}" aria-label="Search reminders"></span>
    </div>
    ${state.listFilter ? `<div class="pill-row"><span class="pill-note">${iconHTML('list')}List: ${escapeHTML(listFilterName)}<button id="clear-list-filter" aria-label="Clear list filter">${actionIconSVG('x')}</button></span></div>` : ''}
    <div class="filter-row">${chips}</div>
    ${groupedHTML}
  `;
  document.getElementById('app').innerHTML = shellHTML('/reminders', main);
  bindShell('/reminders');
  bindReminderActions(document.getElementById('app'), () => renderReminders());

  const searchInput = document.getElementById('list-search');
  if (state.searchFocus) {
    state.searchFocus = false;
    searchInput?.focus();
    searchInput?.select();
  }
  let debounce = null;
  searchInput?.addEventListener('input', () => {
    clearTimeout(debounce);
    debounce = setTimeout(() => {
      state.search = searchInput.value;
      renderReminders();
    }, 300);
  });
  document.querySelectorAll('[data-filter]').forEach((chip) => {
    chip.addEventListener('click', () => {
      state.filter = chip.dataset.filter;
      renderReminders();
    });
  });
  document.getElementById('clear-list-filter')?.addEventListener('click', () => {
    state.listFilter = '';
    state.listFilterName = '';
    renderReminders();
  });
}

// ---------------------------------------------------------------- settings

async function renderSettings(route = '/settings') {
  if (route === '/settings/vacation') {
    await renderVacationSettings();
    return;
  }
  if (route === '/settings/notifications') {
    await renderNotificationSettings();
    return;
  }
  let notifyCfg = {};
  try { notifyCfg = await API.get('/api/me/notify'); } catch {}
  const emailStatus = notifyCfg.email_enabled ? 'Email on' : (notifyCfg.channels?.email_configured ? 'Email off' : 'Not configured');
  const tgStatus = notifyCfg.telegram_linked ? 'Telegram linked' : 'Not linked';
  const main = `
    <h1 class="page-title">Settings</h1>
    <p class="page-sub">Manage your account and preferences</p>
    <div class="settings-grid">
      <div class="card settings-card" data-href="#/settings/notifications">
        <h3>${iconHTML('bell')}Notifications</h3>
        <p>Email and Telegram delivery, plus how far ahead to warn you.</p>
        <p class="settings-card-status">${escapeHTML(emailStatus)} · ${escapeHTML(tgStatus)}</p>
      </div>
      <div class="card settings-card" data-href="#/settings/wording">
        <h3>${iconHTML('mail')}Message wording</h3>
        <p>Customise how your reminders read, or keep the defaults.</p>
      </div>
      <div class="card settings-card" data-href="#/settings/vacation">
        <h3>${iconHTML('moon')}Vacation &amp; pause</h3>
        <p>Pause all notifications or review paused reminders.</p>
        ${state.user?.notifications_paused ? '<p class="settings-card-status" style="color:var(--danger)">Notifications paused</p>' : ''}
      </div>
      <div class="card settings-card" id="open-profile">
        <h3>${iconHTML('panel')}Profile</h3>
        <p>${escapeHTML(state.user.email)} · ${escapeHTML(state.user.timezone)}</p>
        <p class="settings-card-status">Name, timezone, password</p>
      </div>
      <div class="card settings-card">
        <h3>${iconHTML('download')}Data export</h3>
        <p>Download a backup of all your reminders.</p>
        <div class="settings-actions" style="margin-top:10px">
          <a class="btn btn-sm" href="${API.url('/api/export/json')}" download>JSON</a>
          <a class="btn btn-sm" href="${API.url('/api/export/ical')}" download>iCal</a>
        </div>
      </div>
      <div class="card settings-card is-dead">
        <h3>${iconHTML('mail')}Google / Gmail sync</h3>
        <p>Connect Google for calendar sync.</p>
        <span class="soon">Coming soon</span>
      </div>
    </div>
  `;
  document.getElementById('app').innerHTML = shellHTML('/settings', main);
  bindShell('/settings');
  document.querySelectorAll('[data-href]').forEach((card) => {
    card.addEventListener('click', () => { location.hash = card.dataset.href; });
  });
  document.getElementById('open-profile')?.addEventListener('click', openProfileDrawer);
}

function openProfileDrawer() {
  const drawer = openDrawer('Profile', `
    <div class="field">
      <label for="p-name">Display name</label>
      <input id="p-name" value="${escapeHTML(state.user.display_name || '')}" maxlength="80">
    </div>
    <div class="field">
      <label for="p-tz">Timezone</label>
      <input id="p-tz" value="${escapeHTML(state.user.timezone || '')}" placeholder="Asia/Kolkata" list="p-tz-list">
      <datalist id="p-tz-list">
        ${['Asia/Kolkata', 'Asia/Dubai', 'Asia/Singapore', 'Europe/London', 'Europe/Berlin', 'America/New_York', 'America/Chicago', 'America/Los_Angeles', 'Australia/Sydney', 'UTC'].map((z) => `<option value="${z}"></option>`).join('')}
      </datalist>
      <p class="channel-sub" style="margin:6px 0 0">Times are shown in this zone. Pick from the list or type any IANA zone.</p>
    </div>
    <div class="settings-actions" style="margin-top:4px">
      <button class="btn btn-primary btn-sm" id="p-save">Save profile</button>
    </div>

    <div class="sec-head" style="margin-top:24px"><h2>Change password</h2></div>
    <div class="field"><label for="p-cur">Current password</label><input id="p-cur" type="password" autocomplete="current-password"></div>
    <div class="field"><label for="p-new">New password</label><input id="p-new" type="password" autocomplete="new-password" minlength="8" placeholder="At least 8 characters"></div>
    <div class="field"><label for="p-new2">Repeat new password</label><input id="p-new2" type="password" autocomplete="new-password"></div>
    <p class="channel-sub" style="margin:0 0 12px">Changing it signs you out everywhere else.</p>
    <button class="btn btn-sm" id="p-pass">Change password</button>
  `, '<button class="btn" type="button" data-close-drawer>Close</button>');

  drawer.querySelector('#p-save').addEventListener('click', async () => {
    const body = {};
    const name = drawer.querySelector('#p-name').value.trim();
    const tz = drawer.querySelector('#p-tz').value.trim();
    if (name !== (state.user.display_name || '')) body.display_name = name;
    if (tz && tz !== state.user.timezone) body.timezone = tz;
    if (!Object.keys(body).length) {
      showToast('Nothing to save');
      return;
    }
    try {
      state.user = await API.patch('/api/me', body);
      showToast('Profile saved');
      renderSettings('/settings');
      closeDrawer();
    } catch (err) {
      showToast(err.message, 'error');
    }
  });

  drawer.querySelector('#p-pass').addEventListener('click', async (e) => {
    const btn = e.currentTarget;
    const cur = drawer.querySelector('#p-cur').value;
    const nw = drawer.querySelector('#p-new').value;
    const nw2 = drawer.querySelector('#p-new2').value;
    if (!cur || !nw) return showToast('Fill in both password fields', 'error');
    if (nw !== nw2) return showToast('New passwords do not match', 'error');
    btn.disabled = true;
    try {
      await API.post('/api/me/password', { current_password: cur, new_password: nw });
      closeDrawer();
      showToast('Password changed');
      renderSettings('/settings');
    } catch (err) {
      showToast(err.message, 'error');
      btn.disabled = false;
    }
  });
}

async function renderVacationSettings() {
  const main = `
    <h1 class="page-title">Vacation &amp; pause</h1>
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

// ---------------------------------------------------------------- reminder drawers

function listPickerHTML(lists, selectedID) {
  if (!lists.length) return '';
  return `
    <div class="field">
      <span class="form-label-icon" style="margin-bottom:8px">${actionIconSVG('list')}List</span>
      ${chipGroupHTML('r-list', [['', 'None'], ...lists.map((l) => [l.id, l.name])], selectedID || '', 'due-chips')}
    </div>`;
}

async function fetchUserLists() {
  try {
    const data = await API.get('/api/lists');
    return data.lists || [];
  } catch {
    return [];
  }
}

function subtasksSectionHTML(subtasks) {
  const rows = subtasks.map((st) => `
    <label class="subtask-row">
      <input type="checkbox" data-subtask="${escapeHTML(st.id)}" ${st.done ? 'checked' : ''}>
      <span class="${st.done ? 'is-done' : ''}">${escapeHTML(st.title)}</span>
    </label>`).join('');
  return `
    <div class="sec-head" style="margin-top:20px"><h2>Subtasks</h2><span class="sec-count">${subtasks.filter((s) => s.done).length}/${subtasks.length}</span></div>
    ${rows || '<p class="page-sub" style="margin:0 0 8px">No subtasks yet.</p>'}
    <div class="list-tools-search" style="margin-bottom:0">
      ${actionIconSVG('plus')}
      <input id="st-new" type="text" placeholder="Add a subtask…" aria-label="New subtask">
      <button class="btn btn-sm" type="button" id="st-add">Add</button>
    </div>`;
}

function commentsSectionHTML(comments) {
  const rows = comments.map((c) => `
    <div class="comment-row">
      <div class="comment-body">${escapeHTML(c.body)}</div>
      <div class="comment-when">${escapeHTML(new Date(c.created_at).toLocaleString())}</div>
    </div>`).join('');
  return `
    <div class="sec-head" style="margin-top:20px"><h2>Notes</h2><span class="sec-count">${comments.length}</span></div>
    ${rows || '<p class="page-sub" style="margin:0 0 8px">No notes yet.</p>'}
    <div class="list-tools-search" style="margin-bottom:0">
      ${actionIconSVG('align-left')}
      <input id="cm-new" type="text" placeholder="Add a note…" aria-label="New note">
      <button class="btn btn-sm" type="button" id="cm-add">Post</button>
    </div>`;
}

// Wire the subtask/comment sections inside an open edit drawer.
function bindDrawerExtras(drawer, id) {
  drawer.querySelectorAll('[data-subtask]').forEach((cb) => {
    cb.addEventListener('change', async () => {
      try {
        await API.patch(`/api/reminders/${id}/subtasks/${cb.dataset.subtask}`, { done: cb.checked });
        cb.closest('label').querySelector('span').classList.toggle('is-done', cb.checked);
      } catch (err) {
        cb.checked = !cb.checked;
        showToast(err.message, 'error');
      }
    });
  });
  const addSubtask = async () => {
    const input = drawer.querySelector('#st-new');
    const title = input?.value.trim();
    if (!title) return;
    try {
      await API.post(`/api/reminders/${id}/subtasks`, { title });
      showToast('Subtask added');
      openReminderDrawer(id);
    } catch (err) {
      showToast(err.message, 'error');
    }
  };
  drawer.querySelector('#st-add')?.addEventListener('click', addSubtask);
  drawer.querySelector('#st-new')?.addEventListener('keydown', (e) => { if (e.key === 'Enter') { e.preventDefault(); addSubtask(); } });

  const addComment = async () => {
    const input = drawer.querySelector('#cm-new');
    const body = input?.value.trim();
    if (!body) return;
    try {
      await API.post(`/api/reminders/${id}/comments`, { body });
      showToast('Note added');
      openReminderDrawer(id);
    } catch (err) {
      showToast(err.message, 'error');
    }
  };
  drawer.querySelector('#cm-add')?.addEventListener('click', addComment);
  drawer.querySelector('#cm-new')?.addEventListener('keydown', (e) => { if (e.key === 'Enter') { e.preventDefault(); addComment(); } });
}

function reminderFormFields({ title, description, due, recurrence, priority }) {
  return `
    <div class="field">
      <label for="r-title"><span class="form-label-icon">${actionIconSVG('type')}Title</span></label>
      <input id="r-title" value="${escapeHTML(title || '')}" required>
    </div>
    <div class="field">
      <label for="r-desc"><span class="form-label-icon">${actionIconSVG('align-left')}Notes <span style="text-transform:none;font-weight:400;letter-spacing:0">(optional)</span></span></label>
      <textarea id="r-desc" rows="2" placeholder="Add details…">${escapeHTML(description || '')}</textarea>
    </div>
    ${dueFieldsHTML(due)}
    <div class="two-col">
      ${repeatFieldsHTML(recurrence || '')}
      ${priorityFieldsHTML(priority || 'normal')}
    </div>`;
}

function bindReminderForm(drawer) {
  bindDueFields(drawer);
  bindChoiceControls(drawer);
}

async function openCreateDrawer(defaultDate) {
  const defaultDue = defaultDate
    ? new Date(defaultDate)
    : new Date();
  if (!defaultDate) defaultDue.setHours(defaultDue.getHours() + 1, 0, 0, 0);
  else defaultDue.setHours(9, 0, 0, 0);
  const lists = await fetchUserLists();
  const drawer = openDrawer('New reminder', `
    ${reminderFormFields({ due: defaultDue })}
    ${listPickerHTML(lists, '')}
  `, `<button class="btn" type="button" data-close-drawer>Cancel</button><button class="btn btn-primary" id="save-reminder">Save reminder</button>`);
  bindReminderForm(drawer);
  drawer.querySelector('#save-reminder').addEventListener('click', async () => {
    const title = document.getElementById('r-title').value.trim();
    const dueLocal = readDueLocal(drawer);
    if (!title || !dueLocal) {
      showToast('Title and due date required', 'error');
      return;
    }
    const body = {
      title,
      description: document.getElementById('r-desc').value,
      due_at: new Date(dueLocal).toISOString(),
      timezone: Intl.DateTimeFormat().resolvedOptions().timeZone,
      priority: document.getElementById('r-priority').value,
      recurrence: readRepeat(drawer),
    };
    const listId = document.getElementById('r-list')?.value;
    if (listId) body.list_id = listId;
    try {
      await API.post('/api/reminders', body);
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
  let lists = [];
  let subtasks = [];
  let comments = [];
  try {
    [reminder, lists, subtasks, comments] = await Promise.all([
      API.get(`/api/reminders/${id}`),
      fetchUserLists(),
      API.get(`/api/reminders/${id}/subtasks`).then((d) => d.subtasks || []).catch(() => []),
      API.get(`/api/reminders/${id}/comments`).then((d) => d.comments || []).catch(() => []),
    ]);
  } catch (err) {
    showToast(err.message, 'error');
    return;
  }
  const due = new Date(reminder.due_at);
  const drawer = openDrawer('Edit reminder', `
    ${reminderFormFields({
      title: reminder.title,
      description: reminder.description,
      due,
      recurrence: reminder.recurrence,
      priority: reminder.priority,
    })}
    ${listPickerHTML(lists, reminder.list_id)}
    <dl class="detail-list">
      <dt>Status</dt><dd>${escapeHTML(reminder.status)}</dd>
      <dt>Created</dt><dd>${escapeHTML(new Date(reminder.created_at).toLocaleDateString())}</dd>
    </dl>
    ${subtasksSectionHTML(subtasks)}
    ${commentsSectionHTML(comments)}
  `, `<button class="btn" type="button" data-close-drawer>Cancel</button><button class="btn btn-primary" id="save-reminder">Save changes</button>`);
  bindReminderForm(drawer);
  bindDrawerExtras(drawer, id);
  drawer.querySelector('#save-reminder').addEventListener('click', async () => {
    const title = document.getElementById('r-title').value.trim();
    const dueLocal = readDueLocal(drawer);
    if (!title || !dueLocal) {
      showToast('Title and due date required', 'error');
      return;
    }
    const body = {
      title,
      description: document.getElementById('r-desc').value,
      due_at: new Date(dueLocal).toISOString(),
      timezone: reminder.timezone || Intl.DateTimeFormat().resolvedOptions().timeZone,
      priority: document.getElementById('r-priority').value,
      recurrence: readRepeat(drawer),
    };
    const listId = document.getElementById('r-list')?.value;
    if (listId) body.list_id = listId;
    try {
      await API.patch(`/api/reminders/${id}`, body);
      closeDrawer();
      showToast('Reminder updated');
      const route = location.hash.replace('#', '') || '/today';
      renderApp(route);
    } catch (err) {
      showToast(err.message, 'error');
    }
  });
}

window.openReminderDrawer = openReminderDrawer;

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
      `<h1 class="page-title">Admin</h1>${emptyState('Could not load the panel', escapeHTML(err.message), '', 'warn')}`);
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
    </div>` : emptyState('No matching users', 'Try a different search.', '', 'search')}
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
      `<h1 class="page-title">Notifications</h1>${emptyState('Could not load settings', escapeHTML(err.message), '', 'warn')}`);
    bindShell('/settings');
    return;
  }

  const emailReady = cfg.channels.email_configured;
  const tgReady = cfg.channels.telegram_configured;

  const main = `
    <h1 class="page-title">Notifications</h1>
    <p class="page-sub">Choose how Day Remind reaches you.</p>

    <div class="card channel-card" style="margin-top:16px">
      <div class="channel-head">
        <div class="channel-title">
          <span class="channel-ico ico-mail">${actionIconSVG('mail')}</span>
          <div>
            <h3>Email</h3>
            <p>Delivered over SMTP · instance server</p>
          </div>
        </div>
        ${emailReady ? '<span class="badge badge-ok">Configured</span>' : ''}
        ${switchHTML('email-enabled', cfg.email_enabled, 'Email enabled', !emailReady)}
      </div>
      ${emailReady ? `
        <div class="channel-body">
          <span class="channel-lbl">Send to</span>
          <input id="email-address" type="email" style="flex:1 1 200px" value="${escapeHTML(cfg.email_address || '')}" autocomplete="email" aria-label="Email address">
        </div>` : `<p class="channel-warn" style="margin-top:12px">Not configured on this server — set <code>SMTP_HOST</code> and <code>SMTP_FROM</code>.</p>`}
    </div>

    <div class="card channel-card">
      <div class="channel-head">
        <div class="channel-title">
          <span class="channel-ico ico-tg">${actionIconSVG('send')}</span>
          <div>
            <h3>Telegram</h3>
            <p>${cfg.telegram_linked ? 'This account is linked to a chat.' : 'Link a chat to receive reminders there.'}</p>
          </div>
        </div>
        ${cfg.telegram_linked ? '<span class="badge badge-ok">Linked</span>' : ''}
        ${switchHTML('tg-enabled', cfg.telegram_enabled, 'Telegram enabled', !(tgReady && cfg.telegram_linked))}
      </div>
      ${!tgReady
        ? `<p class="channel-warn" style="margin-top:12px">Not configured on this server — set <code>TELEGRAM_BOT_TOKEN</code>.</p>`
        : cfg.telegram_linked
          ? `<div class="channel-actions" style="margin-top:12px">
               <button class="btn btn-sm" id="tg-unlink">Unlink this chat</button>
             </div>`
          : `<div class="channel-actions" style="margin-top:12px">
               <button class="btn btn-sm btn-primary" id="tg-link">Get link code</button>
             </div>
             <div id="tg-link-out"></div>`}
    </div>

    <div class="card channel-card">
      <div class="channel-head">
        <div class="channel-title">
          <span class="channel-ico ico-clock">${actionIconSVG('alarm-clock')}</span>
          <div>
            <h3>Advance warning</h3>
            <p>An extra heads-up before each reminder is due.</p>
          </div>
        </div>
      </div>
      <div class="channel-body">
        <span class="channel-lbl">Warn me</span>
        <select id="advance-minutes" aria-label="Advance warning">
          ${ADVANCE_CHOICES.map(([v, label]) =>
            `<option value="${v}" ${cfg.advance_minutes === v ? 'selected' : ''}>${label}</option>`).join('')}
        </select>
      </div>
    </div>

    <details class="card channel-card own-server" ${cfg.own_smtp.enabled ? 'open' : ''}>
      <summary><h3>Use my own mail server</h3></summary>
      <p class="channel-sub">Send your reminders through your own SMTP server instead of the instance's.
        Your password is encrypted before it is stored, but whoever runs this server can decrypt it.</p>
      <label class="channel-lbl" style="display:flex;align-items:center;gap:10px;margin-bottom:12px">
        ${switchHTML('u-smtp-enabled', cfg.own_smtp.enabled, 'Use my server')}
        Use my server
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
      <label class="channel-lbl" style="display:flex;align-items:center;gap:10px;margin-bottom:12px">
        ${switchHTML('u-tg-enabled', cfg.own_telegram.enabled, 'Use my bot')}
        Use my bot
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
      <a href="#/settings/wording" class="btn btn-ghost btn-sm">Message wording →</a>
      <a href="#/settings" class="btn btn-ghost btn-sm">← Back</a>
    </div>
    <div id="notify-result"></div>
  `;

  document.getElementById('app').innerHTML = shellHTML('/settings/notifications', main, 'Settings · Notifications');
  bindShell('/settings/notifications');
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
    adminShell(emptyState('Could not load settings', escapeHTML(err.message), '', 'warn'), 'Delivery configuration');
    return;
  }
  adminState.settings = cfg;
  const smtp = cfg.smtp;
  const tg = cfg.telegram;

  const body = `
    <div class="card channel-card">
      <div class="channel-head">
        <div class="channel-title">
          <span class="channel-ico ico-mail">${actionIconSVG('mail')}</span>
          <div>
            <h3>Email (SMTP)</h3>
            <p>Used for everyone who has not configured their own mail server.</p>
          </div>
        </div>
        ${switchHTML('a-smtp-enabled', smtp.enabled, 'SMTP enabled')}
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
        <div class="channel-title">
          <span class="channel-ico ico-tg">${actionIconSVG('send')}</span>
          <div>
            <h3>Telegram bot</h3>
            <p>The shared bot. It serves commands and delivers to anyone who has linked a chat.</p>
          </div>
        </div>
        ${switchHTML('a-tg-enabled', tg.enabled, 'Telegram enabled')}
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
  ['email_css', 'Email CSS', 'code'],
  ['email_text', 'Email body (plain text)', 'code'],
  ['telegram_text', 'Telegram message', 'code'],
];

function templateFieldHTML(key, label, kind, values) {
  const rows = key === 'email_html' || key === 'email_css' ? 14 : 7;
  return `
    <div class="field">
      <label for="t-${key}">${escapeHTML(label)}</label>
      ${kind === 'input'
        ? `<input id="t-${key}" value="${escapeHTML(values[key] || '')}">`
        : `<textarea id="t-${key}" rows="${rows}" spellcheck="false">${escapeHTML(values[key] || '')}</textarea>`}
    </div>`;
}

function templateEditorHTML(values, opts = {}) {
  const hint = opts.inheritHint || 'Leave a field blank to use the default.';
  const name = opts.nameField ? `
    <div class="field">
      <label for="t-name">Name</label>
      <input id="t-name" value="${escapeHTML(values.name || '')}" maxlength="80">
    </div>` : '';
  const rest = TEMPLATE_FIELDS
    .filter(([key]) => key !== 'email_subject' && key !== 'email_html' && key !== 'email_css')
    .map(([key, label, kind]) => templateFieldHTML(key, label, kind, values)).join('');
  return `
    <p class="channel-sub">${escapeHTML(hint)}</p>
    <details class="var-ref">
      <summary>Available variables</summary>
      <ul>${(opts.variables || []).map((v) =>
        `<li><code>{{${escapeHTML(v.Name)}}}</code> — ${escapeHTML(v.Description)}</li>`).join('')}</ul>
    </details>
    ${name}
    ${templateFieldHTML('email_subject', 'Email subject', 'input', values)}
    <div class="mail-design">
      ${templateFieldHTML('email_html', 'Email body (HTML)', 'code', values)}
      ${templateFieldHTML('email_css', 'Email CSS', 'code', values)}
    </div>
    ${rest}
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
  const name = document.getElementById('t-name');
  if (name) out.name = name.value;
  return out;
}

let mailEditor = null;

function mailAPI(scope) {
  return scope === 'admin' ? '/api/admin/mail-templates' : '/api/me/mail-templates';
}

function mailPreviewAPI(scope) {
  return scope === 'admin' ? '/api/admin/templates/preview' : '/api/me/templates/preview';
}

// MAIL_AI_PROMPT, parseMailAIResult, copyMailAIPrompt and the inbox-style
// preview live in wording-ai.js so the wording page and this panel share them.

let mailPasteOpen = false;

function mailPayloadFromTemplate(t) {
  return {
    email_subject: t.email_subject || '',
    email_html: t.email_html || '',
    email_css: t.email_css || '',
    email_text: t.email_text || '',
    telegram_text: t.telegram_text || '',
    telegram_parse_mode: t.telegram_parse_mode || 'markdownv2',
  };
}

function mailTemplateCards(templates, editorOpen) {
  if (!templates.length) {
    if (editorOpen) return '';
    return emptyState('No mail templates yet', 'Add one to design the email with HTML and CSS, and to write the message.', '', 'mail');
  }
  return `<div class="template-grid">${templates.map((t) => `
    <article class="card template-card">
      <h3>${escapeHTML(t.name)}${t.active ? ' <span class="badge badge-ok">In use</span>' : ''}</h3>
      <p class="page-sub">${escapeHTML(t.email_subject || 'Default subject')}</p>
      <div class="settings-actions">
        <button class="btn btn-sm btn-primary" data-use-mail="${t.id}"${t.active ? ' disabled' : ''}>Use</button>
        <button class="btn btn-sm" data-view-mail="${t.id}">View as mail</button>
        <button class="btn btn-sm" data-edit-mail="${t.id}">Edit</button>
        <button class="btn btn-sm btn-danger" data-delete-mail="${t.id}">Delete</button>
      </div>
    </article>`).join('')}</div>`;
}

async function renderAdminTemplates() {
  let cfg;
  try {
    cfg = await API.get('/api/admin/settings');
  } catch (err) {
    adminShell(emptyState('Could not load templates', escapeHTML(err.message), '', 'warn'), 'Notification templates');
    return;
  }
  adminState.settings = cfg;
  let lib;
  try {
    lib = await API.get('/api/admin/mail-templates');
  } catch (err) {
    adminShell(emptyState('Could not load templates', escapeHTML(err.message), '', 'warn'), 'Notification templates');
    return;
  }
  adminShell(mailLibraryHTML(lib, cfg.variables, 'admin'), 'Notification templates');
  bindMailLibrary('admin', renderAdminTemplates);
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

function mailLibraryHTML(lib, variables, scope) {
  const templates = lib.templates || [];
  const editing = mailEditor && mailEditor.scope === scope;
  const hint = scope === 'admin'
    ? 'These designs are for the whole instance. Use one to make it the wording everyone inherits. Leave a field blank to keep the built-in default.'
    : 'Leave a field blank to keep what the instance already provides. Use a template to send your reminders with it.';
  const form = editing ? `
    <div class="card channel-card">
      <h3>${mailEditor.mode === 'add' ? 'New mail template' : 'Edit mail template'}</h3>
      ${templateEditorHTML(mailEditor.values, { variables, nameField: true, inheritHint: hint })}
      <div class="settings-actions">
        <button class="btn btn-primary" id="t-save">${mailEditor.mode === 'add' ? 'Add template' : 'Save template'}</button>
        <button class="btn" id="t-view-mail">View as mail</button>
        <button class="btn" id="t-preview">Preview</button>
        <button class="btn" id="t-cancel">Cancel</button>
      </div>
    </div>
    <div id="t-preview-out"></div>` : '';
  const pasteBox = mailPasteOpen ? `
    <div class="card channel-card" id="mail-paste-box">
      <h3>Paste AI result</h3>
      <p class="channel-sub">Paste the JSON from your AI assistant, then fill the form.</p>
      <div class="field"><textarea id="mail-paste-json" rows="10" spellcheck="false" placeholder='{"name":"…","email_subject":"…",…}'></textarea></div>
      <div class="settings-actions">
        <button class="btn btn-primary" id="mail-paste-apply">Fill form</button>
        <button class="btn" id="mail-paste-cancel">Cancel</button>
      </div>
    </div>` : '';
  return `
    <div class="settings-actions" style="margin-bottom:16px;flex-wrap:wrap">
      <button class="btn btn-primary" id="mail-add">Add template</button>
      <button class="btn" id="mail-copy-prompt" type="button">Copy AI prompt</button>
      <button class="btn" id="mail-paste-open" type="button">Paste AI result</button>
    </div>
    ${pasteBox}
    ${mailTemplateCards(templates, !!editing || mailPasteOpen)}
    ${form}
    <div id="mail-view-out"></div>
  `;
}

function bindMailLibrary(scope, after) {
  const base = mailAPI(scope);
  const previewAPI = mailPreviewAPI(scope);

  async function showViewAsMail(payload) {
    const out = document.getElementById('mail-view-out') || document.getElementById('t-preview-out');
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

  document.getElementById('mail-copy-prompt')?.addEventListener('click', () => copyMailAIPrompt());
  document.getElementById('mail-paste-open')?.addEventListener('click', () => {
    mailPasteOpen = true;
    if (!mailEditor) mailEditor = { scope, mode: 'add', values: { telegram_parse_mode: 'markdownv2' } };
    after();
  });
  document.getElementById('mail-paste-cancel')?.addEventListener('click', () => {
    mailPasteOpen = false;
    after();
  });
  document.getElementById('mail-paste-apply')?.addEventListener('click', () => {
    const raw = document.getElementById('mail-paste-json')?.value || '';
    try {
      const vals = parseMailAIResult(raw);
      mailEditor = { scope, mode: 'add', values: vals };
      mailPasteOpen = false;
      showToast('Form filled — review and save');
      after();
    } catch {
      showToast('Invalid JSON — check the AI response', 'error');
    }
  });

  document.getElementById('mail-add')?.addEventListener('click', () => {
    mailEditor = { scope, mode: 'add', values: { telegram_parse_mode: 'markdownv2' } };
    mailPasteOpen = false;
    after();
  });
  document.querySelectorAll('[data-edit-mail]').forEach((btn) => {
    btn.addEventListener('click', async () => {
      try {
        const lib = await API.get(base);
        const found = (lib.templates || []).find((t) => t.id === btn.dataset.editMail);
        if (!found) {
          showToast('That template is gone', 'error');
          return;
        }
        mailEditor = { scope, mode: 'edit', id: found.id, values: found };
        after();
      } catch (err) {
        showToast(err.message, 'error');
      }
    });
  });
  document.querySelectorAll('[data-use-mail]').forEach((btn) => {
    btn.addEventListener('click', async () => {
      try {
        await API.post(`${base}/${btn.dataset.useMail}/use`);
        showToast('Template in use');
        mailEditor = null;
        after();
      } catch (err) {
        showToast(err.message, 'error');
      }
    });
  });
  document.querySelectorAll('[data-view-mail]').forEach((btn) => {
    btn.addEventListener('click', async () => {
      try {
        const lib = await API.get(base);
        const found = (lib.templates || []).find((t) => t.id === btn.dataset.viewMail);
        if (!found) {
          showToast('That template is gone', 'error');
          return;
        }
        await showViewAsMail(mailPayloadFromTemplate(found));
      } catch (err) {
        showToast(err.message, 'error');
      }
    });
  });
  document.querySelectorAll('[data-delete-mail]').forEach((btn) => {
    btn.addEventListener('click', async () => {
      if (!confirm('Delete this mail template?')) return;
      try {
        await API.delete(`${base}/${btn.dataset.deleteMail}`);
        showToast('Template deleted');
        if (mailEditor && mailEditor.id === btn.dataset.deleteMail) mailEditor = null;
        after();
      } catch (err) {
        showToast(err.message, 'error');
      }
    });
  });
  document.getElementById('t-cancel')?.addEventListener('click', () => {
    mailEditor = null;
    after();
  });
  document.getElementById('t-save')?.addEventListener('click', async () => {
    const vals = readTemplateEditor();
    try {
      if (mailEditor.mode === 'add') {
        await API.post(base, vals);
        showToast('Template added');
      } else {
        await API.request(`${base}/${mailEditor.id}`, { method: 'PUT', body: JSON.stringify(vals) });
        showToast('Template saved');
      }
      mailEditor = null;
      after();
    } catch (err) {
      showToast(err.message, 'error');
    }
  });
  document.getElementById('t-view-mail')?.addEventListener('click', () => {
    showViewAsMail(readTemplateEditor());
  });
  document.getElementById('t-preview')?.addEventListener('click', async () => {
    const out = document.getElementById('t-preview-out');
    try {
      const res = await API.post(previewAPI, readTemplateEditor());
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
}
