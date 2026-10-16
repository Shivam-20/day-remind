function showToast(message, type = 'info') {
  const root = document.getElementById('toast-root');
  const el = document.createElement('div');
  el.className = 'toast' + (type === 'error' ? ' error' : '');
  el.textContent = message;
  root.appendChild(el);
  setTimeout(() => el.remove(), 4000);
}

function openDrawer(title, bodyHTML, footerHTML) {
  closeDrawer();
  const backdrop = document.createElement('div');
  backdrop.className = 'drawer-backdrop';
  backdrop.addEventListener('click', closeDrawer);
  const drawer = document.createElement('div');
  drawer.className = 'drawer';
  drawer.setAttribute('role', 'dialog');
  drawer.setAttribute('aria-modal', 'true');
  drawer.innerHTML = `
    <div class="drawer-header">
      <h2 style="margin:0;font-size:1.1rem">${title}</h2>
      <button type="button" class="btn btn-ghost btn-sm" data-close-drawer aria-label="Close">✕</button>
    </div>
    <div class="drawer-body">${bodyHTML}</div>
    <div class="drawer-footer">${footerHTML || ''}</div>
  `;
  document.body.appendChild(backdrop);
  document.body.appendChild(drawer);
  drawer.querySelectorAll('[data-close-drawer]').forEach((el) => el.addEventListener('click', closeDrawer));
  document.addEventListener('keydown', drawerEscHandler);
  drawer.querySelector('input, textarea, select, button')?.focus();
  return drawer;
}

function drawerEscHandler(e) {
  if (e.key === 'Escape') closeDrawer();
}

function closeDrawer() {
  document.removeEventListener('keydown', drawerEscHandler);
  document.querySelectorAll('.drawer-backdrop, .drawer').forEach((el) => el.remove());
}

function emptyState(title, body, ctaHTML = '') {
  return `<div class="empty-state"><h3>${title}</h3><p>${body}</p>${ctaHTML}</div>`;
}

function safeTimeZone(tz) {
  if (!tz) return null;
  try {
    new Intl.DateTimeFormat('en-CA', { timeZone: tz });
    return tz;
  } catch {
    return null;
  }
}

function dayKey(date, tz) {
  const opts = { year: 'numeric', month: '2-digit', day: '2-digit' };
  if (tz) opts.timeZone = tz;
  return new Intl.DateTimeFormat('en-CA', opts).format(date);
}

function formatDue(dueISO, tz) {
  const d = new Date(dueISO);
  const now = new Date();
  const zone = safeTimeZone(tz);
  const opts = { hour: '2-digit', minute: '2-digit' };
  if (zone) {
    opts.timeZone = zone;
    // A bare clock time is misleading when it is not the viewer's own zone.
    if (zone !== Intl.DateTimeFormat().resolvedOptions().timeZone) opts.timeZoneName = 'short';
  }
  if (dayKey(d, zone) === dayKey(now, zone)) return d.toLocaleTimeString(undefined, opts);
  return d.toLocaleString(undefined, { month: 'short', day: 'numeric', ...opts });
}

function isOverdue(item) {
  if (item.status === 'done' || item.status === 'cancelled') return false;
  return new Date(item.due_at) < new Date();
}

function statusBadge(item) {
  if (item.status === 'paused') return '<span class="badge badge-paused">Paused</span>';
  if (item.status === 'done') return '<span class="badge badge-done">Done</span>';
  if (isOverdue(item)) return '<span class="badge badge-overdue">Overdue</span>';
  return '';
}

function actionIconSVG(name) {
  const s = 'xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"';
  const icons = {
    info: `<svg ${s}><circle cx="12" cy="12" r="10"/><path d="M12 16v-4M12 8h.01"/></svg>`,
    check: `<svg ${s}><path d="M20 6 9 17l-5-5"/></svg>`,
    pause: `<svg ${s}><rect x="6" y="4" width="4" height="16"/><rect x="14" y="4" width="4" height="16"/></svg>`,
    play: `<svg ${s}><polygon points="5 3 19 12 5 21 5 3"/></svg>`,
    clock: `<svg ${s}><circle cx="12" cy="12" r="10"/><path d="M12 6v6l4 2"/></svg>`,
    sun: `<svg ${s}><circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M4.93 4.93l1.41 1.41M17.66 17.66l1.41 1.41M2 12h2M20 12h2M4.93 19.07l1.41-1.41M17.66 6.34l1.41-1.41"/></svg>`,
    trash: `<svg ${s}><path d="M3 6h18M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>`,
    menu: `<svg ${s}><path d="M4 6h16M4 12h16M4 18h16"/></svg>`,
    panel: `<svg ${s}><rect x="3" y="3" width="18" height="18" rx="2"/><path d="M9 3v18"/></svg>`,
  };
  return icons[name] || icons.info;
}

function reminderActionBtn({ action, id, label, icon, className = '', attrs = {} }) {
  const dataAttrs = Object.entries(attrs)
    .map(([k, v]) => ` data-${k}="${escapeHTML(String(v))}"`)
    .join('');
  return `<button type="button" class="btn btn-sm btn-with-icon${className ? ` ${className}` : ''}" data-action="${escapeHTML(action)}" data-id="${escapeHTML(id)}"${dataAttrs}><span class="btn-icon" aria-hidden="true">${actionIconSVG(icon)}</span><span class="btn-label">${escapeHTML(label)}</span></button>`;
}

function snoozeUntilTomorrow9am() {
  const d = new Date();
  d.setDate(d.getDate() + 1);
  d.setHours(9, 0, 0, 0);
  return d;
}

function reminderCardHTML(item, actionsHTML) {
  const paused = item.status === 'paused' ? ' is-paused' : '';
  return `
    <article class="reminder-card${paused}" data-id="${item.id}">
      <div class="reminder-stripe"></div>
      <div>
        <div class="reminder-time">${formatDue(item.due_at, item.timezone)} ${statusBadge(item)}</div>
        <div class="reminder-title">${escapeHTML(item.title)}</div>
        <div class="reminder-meta">${escapeHTML(item.priority)} · ${escapeHTML(item.status)}</div>
        <div class="reminder-actions">${actionsHTML}</div>
      </div>
    </article>
  `;
}

function escapeHTML(s) {
  return String(s ?? '')
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

const SIDEBAR_HIDDEN_KEY = 'drSidebarHidden';

function isSidebarHidden() {
  return localStorage.getItem(SIDEBAR_HIDDEN_KEY) === '1';
}

function setSidebarHidden(hidden) {
  localStorage.setItem(SIDEBAR_HIDDEN_KEY, hidden ? '1' : '0');
  document.querySelectorAll('.layout').forEach((el) => {
    el.classList.toggle('sidebar-hidden', hidden);
  });
  document.querySelectorAll('[data-sidebar-toggle]').forEach((btn) => {
    const hide = isSidebarHidden();
    btn.setAttribute('aria-expanded', hide ? 'false' : 'true');
    const label = btn.querySelector('.btn-label');
    if (label) label.textContent = hide ? 'Show menu' : 'Hide menu';
    const icon = btn.querySelector('.btn-icon');
    if (icon && btn.closest('.sidebar-footer')) {
      icon.innerHTML = actionIconSVG(hide ? 'menu' : 'panel');
    }
  });
}

function toggleSidebarHidden() {
  const layout = document.querySelector('.layout');
  if (!layout) return;
  if (window.matchMedia('(max-width: 768px)').matches) {
    layout.classList.toggle('sidebar-visible-mobile');
    const open = layout.classList.contains('sidebar-visible-mobile');
    document.querySelectorAll('[data-sidebar-toggle]').forEach((btn) => {
      btn.setAttribute('aria-expanded', open ? 'true' : 'false');
    });
    return;
  }
  setSidebarHidden(!isSidebarHidden());
}

function initTheme() {
  const saved = localStorage.getItem('drTheme') || 'dark';
  document.documentElement.setAttribute('data-bs-theme', saved);
}

function toggleTheme() {
  const cur = document.documentElement.getAttribute('data-bs-theme') === 'light' ? 'light' : 'dark';
  const next = cur === 'dark' ? 'light' : 'dark';
  document.documentElement.setAttribute('data-bs-theme', next);
  localStorage.setItem('drTheme', next);
}

initTheme();
