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
        <button type="button" class="btn btn-sm" data-due-month-shift="-1" aria-label="Previous month">‹</button>
        <span class="due-cal-label">${monthLabel}</span>
        <button type="button" class="btn btn-sm" data-due-month-shift="1" aria-label="Next month">›</button>
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
      <legend>Due</legend>
      ${dueCalendarHTML(parts.date, fromDate)}
      <div class="due-chips" role="group" aria-label="Quick dates">
        <button type="button" class="btn btn-sm" data-due-date="0">Today</button>
        <button type="button" class="btn btn-sm" data-due-date="1">Tomorrow</button>
        <button type="button" class="btn btn-sm" data-due-date="7">In a week</button>
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
          <button type="button" class="btn btn-sm" data-due-time="09:00">9:00</button>
          <button type="button" class="btn btn-sm" data-due-time="12:00">12:00</button>
          <button type="button" class="btn btn-sm" data-due-time="18:00">18:00</button>
          <button type="button" class="btn btn-sm" data-due-time="21:00">21:00</button>
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
