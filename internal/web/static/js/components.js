function showToast(message, type = 'info') {
  const root = document.getElementById('toast-root');
  const el = document.createElement('div');
  el.className = 'toast' + (type === 'error' ? ' error' : '');
  const icon = type === 'error' ? 'trash' : 'check';
  el.innerHTML = `
    <span class="btn-icon" aria-hidden="true">${actionIconSVG(type === 'error' ? 'warn' : icon)}</span>
    <span class="toast-msg"></span>
    <button type="button" class="toast-close" aria-label="Dismiss">${actionIconSVG('x')}</button>
  `;
  el.querySelector('.toast-msg').textContent = message;
  el.querySelector('.toast-close').addEventListener('click', () => el.remove());
  root.appendChild(el);
  setTimeout(() => el.remove(), 5000);
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
      <button type="button" class="icon-btn" data-close-drawer aria-label="Close">${actionIconSVG('x')}</button>
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
  closeAllMenus();
  document.querySelectorAll('.drawer-backdrop, .drawer').forEach((el) => el.remove());
}

function emptyState(title, body, ctaHTML = '', icon = 'inbox') {
  return `<div class="empty-state">
    <span class="btn-icon" aria-hidden="true">${actionIconSVG(icon)}</span>
    <h3>${title}</h3><p>${body}</p>${ctaHTML}
  </div>`;
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

// Both sides go through Intl so legacy aliases (Asia/Calcutta vs Asia/Kolkata)
// compare equal instead of tripping the "different zone" suffix.
function canonicalTimeZone(tz) {
  if (!tz) return null;
  try {
    return new Intl.DateTimeFormat('en-US', { timeZone: tz }).resolvedOptions().timeZone;
  } catch {
    return null;
  }
}

function formatDue(dueISO, tz) {
  const d = new Date(dueISO);
  const now = new Date();
  const zone = safeTimeZone(tz);
  const opts = { hour: '2-digit', minute: '2-digit' };
  if (zone) {
    opts.timeZone = zone;
    // A bare clock time is misleading when it is not the viewer's own zone.
    const viewerZone = canonicalTimeZone(Intl.DateTimeFormat().resolvedOptions().timeZone);
    if (canonicalTimeZone(zone) !== viewerZone) opts.timeZoneName = 'short';
  }
  if (dayKey(d, zone) === dayKey(now, zone)) return d.toLocaleTimeString(undefined, opts);
  return d.toLocaleString(undefined, { month: 'short', day: 'numeric', ...opts });
}

function isOverdue(item) {
  if (item.status === 'done' || item.status === 'cancelled') return false;
  return new Date(item.due_at) < new Date();
}

function overdueAge(item) {
  const diffMs = Date.now() - new Date(item.due_at).getTime();
  const mins = Math.floor(diffMs / 60000);
  if (mins < 60) return `${mins}m late`;
  const hrs = Math.floor(mins / 60);
  if (hrs < 24) return `${hrs}h late`;
  const days = Math.floor(hrs / 24);
  return `${days}d late`;
}

function tagHTML(cls, label) {
  return `<span class="tag tag-${cls}">${escapeHTML(label)}</span>`;
}

function reminderTags(item) {
  const tags = [];
  if (isOverdue(item)) tags.push(tagHTML('overdue', overdueAge(item)));
  if (item.status === 'paused') tags.push(tagHTML('paused', 'paused'));
  if (item.status === 'done') tags.push(tagHTML('done', 'done'));
  const pri = item.priority || 'normal';
  if (pri !== 'normal') tags.push(tagHTML(pri, pri));
  return tags.join('');
}

// ---------------------------------------------------------------- icons

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
    pin: `<svg ${s}><path d="M12 17v5M8 3h8l1 6-5 3-5-3Z"/></svg>`,
    'pin-off': `<svg ${s}><path d="M12 17v5M8 3h8l1 6-5 3-5-3Z"/><path d="m2 2 20 20"/></svg>`,
    search: `<svg ${s}><circle cx="11" cy="11" r="8"/><path d="m21 21-4.3-4.3"/></svg>`,
    plus: `<svg ${s}><path d="M5 12h14M12 5v14"/></svg>`,
    x: `<svg ${s}><path d="M18 6 6 18M6 6l12 12"/></svg>`,
    warn: `<svg ${s}><path d="m21.73 18-8-14a2 2 0 0 0-3.46 0l-8 14A2 2 0 0 0 4 20h16a2 2 0 0 0 1.73-2Z"/><path d="M12 9v4M12 17h.01"/></svg>`,
    'chevron-left': `<svg ${s}><path d="m15 18-6-6 6-6"/></svg>`,
    'chevron-right': `<svg ${s}><path d="m9 18 6-6-6-6"/></svg>`,
    'chevron-down': `<svg ${s}><path d="m6 9 6 6 6-6"/></svg>`,
    'more-horizontal': `<svg ${s}><circle cx="12" cy="12" r="1"/><circle cx="19" cy="12" r="1"/><circle cx="5" cy="12" r="1"/></svg>`,
    layers: `<svg ${s}><polygon points="12 2 2 7 12 12 22 7 12 2"/><polyline points="2 17 12 22 22 17"/><polyline points="2 12 12 17 22 12"/></svg>`,
    list: `<svg ${s}><path d="M8 6h13M8 12h13M8 18h13M3 6h.01M3 12h.01M3 18h.01"/></svg>`,
    calendar: `<svg ${s}><rect x="3" y="4" width="18" height="18" rx="2"/><path d="M16 2v4M8 2v4M3 10h18"/></svg>`,
    'calendar-plus': `<svg ${s}><rect x="3" y="4" width="18" height="18" rx="2"/><path d="M16 2v4M8 2v4M3 10h10"/><path d="M15 15v6M12 18h6"/></svg>`,
    moon: `<svg ${s}><path d="M12 3a6 6 0 0 0 9 9 9 9 0 1 1-9-9Z"/></svg>`,
    'log-out': `<svg ${s}><path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/><polyline points="16 17 21 12 16 7"/><path d="M21 12H9"/></svg>`,
    settings: `<svg ${s}><path d="M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.08a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.09a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2z"/><circle cx="12" cy="12" r="3"/></svg>`,
    shield: `<svg ${s}><path d="M20 13c0 5-3.5 7.5-7.66 8.95a1 1 0 0 1-.67-.01C7.5 20.5 4 18 4 13V6a1 1 0 0 1 1-1c2 0 4.5-1.2 6.24-2.72a1.17 1.17 0 0 1 1.52 0C14.51 3.81 17 5 19 5a1 1 0 0 1 1 1z"/></svg>`,
    mail: `<svg ${s}><rect x="2" y="4" width="20" height="16" rx="2"/><path d="m22 7-8.97 5.7a1.94 1.94 0 0 1-2.06 0L2 7"/></svg>`,
    send: `<svg ${s}><path d="m22 2-7 20-4-9-9-4Z"/><path d="M22 2 11 13"/></svg>`,
    flag: `<svg ${s}><path d="M4 15s1-1 4-1 5 2 8 2 4-1 4-1V3s-1 1-4 1-5-2-8-2-4 1-4 1z"/><path d="M4 22v-7"/></svg>`,
    repeat: `<svg ${s}><path d="m17 2 4 4-4 4"/><path d="M3 11v-1a4 4 0 0 1 4-4h14"/><path d="m7 22-4-4 4-4"/><path d="M21 13v1a4 4 0 0 1-4 4H3"/></svg>`,
    pencil: `<svg ${s}><path d="M17 3a2.85 2.83 0 1 1 4 4L7.5 20.5 2 22l1.5-5.5Z"/></svg>`,
    server: `<svg ${s}><rect x="2" y="2" width="20" height="8" rx="2"/><rect x="2" y="14" width="20" height="8" rx="2"/><path d="M6 6h.01M6 18h.01"/></svg>`,
    bot: `<svg ${s}><rect x="3" y="8" width="18" height="12" rx="2"/><path d="M12 8V4M8 4h8M8 14h.01M16 14h.01"/></svg>`,
    'arrow-right': `<svg ${s}><path d="M5 12h14M12 5l7 7-7 7"/></svg>`,
    'arrow-up-down': `<svg ${s}><path d="m3 16 4 4 4-4M7 20V4M21 8l-4-4-4 4M17 4v16"/></svg>`,
    type: `<svg ${s}><path d="M4 7V4h16v3M9 20h6M12 4v16"/></svg>`,
    'align-left': `<svg ${s}><path d="M3 5h18M3 12h12M3 19h15"/></svg>`,
    'alarm-clock': `<svg ${s}><circle cx="12" cy="13" r="8"/><path d="M12 9v4l2 2M5 3 2 6M19 3l3 3"/></svg>`,
    inbox: `<svg ${s}><polyline points="22 12 16 12 14 15 10 15 8 12 2 12"/><path d="M5.45 5.11 2 12v6a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2v-6l-3.45-6.89A2 2 0 0 0 16.76 4H7.24a2 2 0 0 0-1.79 1.11z"/></svg>`,
    'bell-off': `<svg ${s}><path d="M8.7 3A6 6 0 0 1 18 8c0 4.5 2 6 2 6H8"/><path d="M17 17H4s2-1.5 2-6c0-.35 0-.68.05-1M6.28 3.7 22 19"/></svg>`,
    bell: `<svg ${s}><path d="M6 8a6 6 0 0 1 12 0c0 7 3 9 3 9H3s3-2 3-9"/><path d="M10.3 21a1.94 1.94 0 0 0 3.4 0"/></svg>`,
    download: `<svg ${s}><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><path d="M12 15V3"/></svg>`,
    copy: `<svg ${s}><rect x="8" y="8" width="14" height="14" rx="2"/><path d="M4 16c-1.1 0-2-.9-2-2V4c0-1.1.9-2 2-2h10c1.1 0 2 .9 2 2"/></svg>`,
    clipboard: `<svg ${s}><rect x="8" y="2" width="8" height="4" rx="1"/><path d="M16 4h2a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2h2"/></svg>`,
  };
  return icons[name] || icons.info;
}

function iconHTML(name, cls = '') {
  return `<span class="btn-icon${cls ? ` ${cls}` : ''}" aria-hidden="true">${actionIconSVG(name)}</span>`;
}

// ---------------------------------------------------------------- reminder rows

const PRIORITIES = ['low', 'normal', 'high', 'urgent'];

function rowHoverActions(item) {
  const btns = [];
  const active = item.status !== 'done' && item.status !== 'cancelled';
  btns.push(`<button type="button" class="icon-btn" data-action="detail" data-id="${escapeHTML(item.id)}" aria-label="Details">${actionIconSVG('info')}</button>`);
  if (active) {
    btns.push(`<button type="button" class="icon-btn" data-action="${item.pinned ? 'unpin' : 'pin'}" data-id="${escapeHTML(item.id)}" aria-label="${item.pinned ? 'Unpin' : 'Pin'}">${actionIconSVG(item.pinned ? 'pin-off' : 'pin')}</button>`);
  }
  return btns.join('');
}

function rowOverflowMenuHTML(item) {
  const active = item.status !== 'done' && item.status !== 'cancelled';
  const items = [];
  if (active) {
    items.push(`<button type="button" data-action="snooze" data-id="${escapeHTML(item.id)}" data-snooze-minutes="15">${iconHTML('clock')}Snooze 15 min</button>`);
    items.push(`<button type="button" data-action="snooze" data-id="${escapeHTML(item.id)}" data-snooze-minutes="60">${iconHTML('clock')}Snooze 1 hour</button>`);
    items.push(`<button type="button" data-action="snooze" data-id="${escapeHTML(item.id)}" data-snooze="tomorrow9">${iconHTML('sun')}Tomorrow, 9:00</button>`);
    items.push('<div class="menu-sep" role="separator"></div>');
    items.push(item.status === 'paused'
      ? `<button type="button" data-action="resume" data-id="${escapeHTML(item.id)}">${iconHTML('play')}Resume</button>`
      : `<button type="button" data-action="pause" data-id="${escapeHTML(item.id)}">${iconHTML('pause')}Pause</button>`);
    items.push('<div class="menu-sep" role="separator"></div>');
  }
  items.push(`<button type="button" class="danger" data-action="delete" data-id="${escapeHTML(item.id)}">${iconHTML('trash')}Delete</button>`);
  return `
    <div class="menu-anchor">
      <button type="button" class="icon-btn" data-menu-toggle aria-label="More actions" aria-haspopup="menu">${actionIconSVG('more-horizontal')}</button>
      <div class="menu" role="menu">${items.join('')}</div>
    </div>`;
}

function reminderRowHTML(item, opts = {}) {
  const cls = [
    'row-card',
    isOverdue(item) ? 'overdue' : '',
    item.status === 'paused' ? 'is-paused' : '',
    item.status === 'done' ? 'is-done' : '',
  ].filter(Boolean).join(' ');
  const done = item.status === 'done';
  const check = done
    ? `<input class="row-check" type="checkbox" checked disabled aria-label="Completed">`
    : `<input class="row-check" type="checkbox" data-action="done" data-id="${escapeHTML(item.id)}" aria-label="Mark ${escapeHTML(item.title)} as done">`;
  const hover = rowHoverActions(item);
  const menu = opts.menu === false ? '' : rowOverflowMenuHTML(item);
  const metaParts = [];
  metaParts.push(`<span class="row-when">${escapeHTML(formatDue(item.due_at, item.timezone))}</span>`);
  if (item.recurrence) metaParts.push(`<span>↻ ${escapeHTML(item.recurrence)}</span>`);
  if (done && item.completed_at) metaParts.push(`<span>done ${escapeHTML(new Date(item.completed_at).toLocaleDateString())}</span>`);
  return `
    <article class="${cls}" data-id="${escapeHTML(item.id)}">
      ${check}
      <div class="row-main">
        <div class="row-title">${item.pinned ? iconHTML('pin') : ''}${escapeHTML(item.title)}</div>
        <div class="row-meta">${metaParts.join('')} ${reminderTags(item)}</div>
      </div>
      <div class="row-actions">${hover}${menu}</div>
    </article>`;
}

// ---------------------------------------------------------------- menus

function closeAllMenus() {
  document.querySelectorAll('.menu-anchor.open').forEach((m) => m.classList.remove('open'));
}

// One delegated listener drives every overflow menu on the page.
document.addEventListener('click', (e) => {
  const toggle = e.target.closest('[data-menu-toggle]');
  if (toggle) {
    e.stopPropagation();
    const anchor = toggle.closest('.menu-anchor');
    const wasOpen = anchor.classList.contains('open');
    closeAllMenus();
    anchor.classList.toggle('open', !wasOpen);
    return;
  }
  if (!e.target.closest('.menu')) closeAllMenus();
});

document.addEventListener('keydown', (e) => {
  if (e.key === 'Escape') closeAllMenus();
});

// ---------------------------------------------------------------- choice controls

function chipGroupHTML(targetID, options, selected, groupClass = '') {
  const chips = options.map(([v, label]) =>
    `<button type="button" class="chip${v === selected ? ' on' : ''}" data-value="${escapeHTML(String(v))}">${escapeHTML(label)}</button>`).join('');
  return `<div class="${groupClass}" data-choice-group="${escapeHTML(targetID)}" role="group">${chips}</div>
  <input type="hidden" id="${escapeHTML(targetID)}" value="${escapeHTML(String(selected ?? ''))}">`;
}

function segControlHTML(targetID, options, selected) {
  const btns = options.map(([v, label]) =>
    `<button type="button" class="${v === selected ? 'on' : ''}" data-value="${escapeHTML(String(v))}">${escapeHTML(label)}</button>`).join('');
  return `<div class="seg" data-choice-group="${escapeHTML(targetID)}" role="group">${btns}</div>
  <input type="hidden" id="${escapeHTML(targetID)}" value="${escapeHTML(String(selected ?? ''))}">`;
}

// Binds every [data-choice-group] inside root: clicking a chip/segment writes
// the value into the hidden input so existing read paths keep working.
function bindChoiceControls(root) {
  root.querySelectorAll('[data-choice-group]').forEach((group) => {
    const input = document.getElementById(group.dataset.choiceGroup);
    if (!input) return;
    group.addEventListener('click', (e) => {
      const chip = e.target.closest('[data-value]');
      if (!chip) return;
      input.value = chip.dataset.value;
      group.querySelectorAll('[data-value]').forEach((c) => c.classList.toggle('on', c === chip));
    });
  });
}

function switchHTML(id, checked, label, disabled = false) {
  return `<label class="switch">
    <input type="checkbox" id="${escapeHTML(id)}" ${checked ? 'checked' : ''} ${disabled ? 'disabled' : ''} aria-label="${escapeHTML(label)}">
    <span class="switch-track" aria-hidden="true"></span>
  </label>`;
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
    btn.setAttribute('aria-label', hide ? 'Show menu' : 'Hide menu');
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

function pad2(n) {
  return String(n).padStart(2, '0');
}

function localDateParts(d) {
  return {
    date: `${d.getFullYear()}-${pad2(d.getMonth() + 1)}-${pad2(d.getDate())}`,
    time: `${pad2(d.getHours())}:${pad2(d.getMinutes())}`,
  };
}

function parseYMD(ymd) {
  const [y, m, day] = (ymd || '').split('-').map(Number);
  if (!y || !m || !day) return new Date();
  return new Date(y, m - 1, day);
}

function dueCalendarHTML(selectedYMD, monthDate) {
  const y = monthDate.getFullYear();
  const m = monthDate.getMonth();
  const first = new Date(y, m, 1);
  const startPad = first.getDay();
  const daysInMonth = new Date(y, m + 1, 0).getDate();
  const today = localDateParts(new Date()).date;
  const monthLabel = first.toLocaleString(undefined, { month: 'long', year: 'numeric' });
  const weekdays = ['S', 'M', 'T', 'W', 'T', 'F', 'S'].map((d) => `<span>${d}</span>`).join('');
  let cells = '';
  for (let i = 0; i < startPad; i++) cells += '<span class="due-cal-cell due-cal-empty"></span>';
  for (let d = 1; d <= daysInMonth; d++) {
    const ymd = `${y}-${pad2(m + 1)}-${pad2(d)}`;
    const cls = ['due-cal-cell'];
    if (ymd === selectedYMD) cls.push('due-cal-selected');
    if (ymd === today) cls.push('due-cal-today');
    const aria = ymd === selectedYMD ? ' aria-current="date"' : '';
    cells += `<button type="button" class="${cls.join(' ')}" data-due-pick="${ymd}"${aria}>${d}</button>`;
  }
  return `
    <div class="due-cal" data-due-month="${y}-${pad2(m + 1)}">
      <div class="due-cal-head">
        <button type="button" class="icon-btn" data-due-month-shift="-1" aria-label="Previous month">${actionIconSVG('chevron-left')}</button>
        <span class="due-cal-label">${monthLabel}</span>
        <button type="button" class="icon-btn" data-due-month-shift="1" aria-label="Next month">${actionIconSVG('chevron-right')}</button>
      </div>
      <div class="due-cal-weekdays">${weekdays}</div>
      <div class="due-cal-grid">${cells}</div>
    </div>
  `;
}

function dueFieldsHTML(fromDate) {
  const parts = localDateParts(fromDate);
  return `
    <fieldset class="field due-fields">
      <legend><span class="form-label-icon">${actionIconSVG('calendar')}When</span></legend>
      ${dueCalendarHTML(parts.date, fromDate)}
      <div class="due-chips" role="group" aria-label="Quick dates">
        <button type="button" class="chip" data-due-date="0">Today</button>
        <button type="button" class="chip" data-due-date="1">Tomorrow</button>
        <button type="button" class="chip" data-due-date="7">In a week</button>
      </div>
      <input id="r-due-date" type="date" value="${parts.date}" required>
      <div class="due-time-block">
        <label for="r-due-time">Time</label>
        <div class="due-time-row">
          <button type="button" class="btn btn-sm due-step" data-due-time-shift="-15" aria-label="15 minutes earlier">−15</button>
          <input id="r-due-time" type="time" value="${parts.time}" required step="60">
          <button type="button" class="btn btn-sm due-step" data-due-time-shift="15" aria-label="15 minutes later">+15</button>
        </div>
        <div class="due-chips" role="group" aria-label="Quick times">
          <button type="button" class="chip" data-due-time="09:00">09:00</button>
          <button type="button" class="chip" data-due-time="12:00">12:00</button>
          <button type="button" class="chip" data-due-time="18:00">18:00</button>
          <button type="button" class="chip" data-due-time="21:00">21:00</button>
        </div>
      </div>
    </fieldset>
  `;
}

function refreshDueCalendar(wrap) {
  const dateEl = wrap.querySelector('#r-due-date');
  const cal = wrap.querySelector('.due-cal');
  if (!dateEl || !cal) return;
  const selected = dateEl.value;
  const monthStr = cal.dataset.dueMonth || selected;
  const [y, m] = monthStr.split('-').map(Number);
  const monthDate = selected && selected.startsWith(monthStr)
    ? parseYMD(selected)
    : new Date(y, (m || 1) - 1, 1);
  cal.outerHTML = dueCalendarHTML(selected, monthDate);
}

function readDueLocal(root) {
  const scope = root || document;
  const date = scope.querySelector('#r-due-date')?.value;
  const timeRaw = scope.querySelector('#r-due-time')?.value || '';
  if (!date || !timeRaw) return '';
  const time = timeRaw.slice(0, 5);
  return `${date}T${time}`;
}

function shiftDueTime(timeEl, minutes) {
  const [h, m] = (timeEl.value || '00:00').slice(0, 5).split(':').map(Number);
  const d = new Date();
  d.setHours(h || 0, (m || 0) + minutes, 0, 0);
  timeEl.value = localDateParts(d).time;
}

const REPEAT_OPTIONS = [
  ['', 'None'],
  ['daily', 'Daily'],
  ['weekly', 'Weekly'],
  ['monthly', 'Monthly'],
  ['yearly', 'Yearly'],
];

function repeatFieldsHTML(selected) {
  return `
    <div class="field">
      <span class="form-label-icon" style="margin-bottom:8px">${actionIconSVG('repeat')}Repeat</span>
      ${chipGroupHTML('r-repeat', REPEAT_OPTIONS, selected, 'due-chips')}
    </div>`;
}

function readRepeat(root) {
  return (root.querySelector('#r-repeat')?.value || '').trim();
}

function priorityFieldsHTML(selected) {
  const opts = PRIORITIES.map((p) => [p, p.charAt(0).toUpperCase() + p.slice(1)]);
  return `
    <div class="field">
      <span class="form-label-icon" style="margin-bottom:8px">${actionIconSVG('flag')}Priority</span>
      ${segControlHTML('r-priority', opts, selected || 'normal')}
    </div>`;
}

function bindDueFields(root) {
  const wrap = root.querySelector('.due-fields');
  if (!wrap) return;
  wrap.addEventListener('click', (e) => {
    const dateEl = wrap.querySelector('#r-due-date');
    const timeEl = wrap.querySelector('#r-due-time');
    const pick = e.target.closest('[data-due-pick]');
    if (pick) {
      dateEl.value = pick.dataset.duePick;
      refreshDueCalendar(wrap);
      return;
    }
    const monthShift = e.target.closest('[data-due-month-shift]');
    if (monthShift) {
      const cal = wrap.querySelector('.due-cal');
      const [y, m] = (cal.dataset.dueMonth || dateEl.value).split('-').map(Number);
      const next = new Date(y, m - 1 + Number(monthShift.dataset.dueMonthShift), 1);
      cal.dataset.dueMonth = `${next.getFullYear()}-${pad2(next.getMonth() + 1)}`;
      refreshDueCalendar(wrap);
      return;
    }
    const quickDate = e.target.closest('[data-due-date]');
    if (quickDate) {
      const d = new Date();
      d.setDate(d.getDate() + Number(quickDate.dataset.dueDate));
      dateEl.value = localDateParts(d).date;
      const cal = wrap.querySelector('.due-cal');
      cal.dataset.dueMonth = dateEl.value.slice(0, 7);
      refreshDueCalendar(wrap);
      return;
    }
    const quickTime = e.target.closest('[data-due-time]');
    if (quickTime) {
      timeEl.value = quickTime.dataset.dueTime;
      return;
    }
    const timeShift = e.target.closest('[data-due-time-shift]');
    if (timeShift) shiftDueTime(timeEl, Number(timeShift.dataset.dueTimeShift));
  });
  wrap.querySelector('#r-due-date').addEventListener('change', () => {
    const cal = wrap.querySelector('.due-cal');
    const v = wrap.querySelector('#r-due-date').value;
    if (v) cal.dataset.dueMonth = v.slice(0, 7);
    refreshDueCalendar(wrap);
  });
}

initTheme();
