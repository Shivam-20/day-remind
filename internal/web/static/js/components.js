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
  drawer.querySelector('[data-close-drawer]').addEventListener('click', closeDrawer);
  return drawer;
}

function closeDrawer() {
  document.querySelectorAll('.drawer-backdrop, .drawer').forEach((el) => el.remove());
}

function emptyState(title, body, ctaHTML = '') {
  return `<div class="empty-state"><h3>${title}</h3><p>${body}</p>${ctaHTML}</div>`;
}

function formatDue(dueISO, tz) {
  const d = new Date(dueISO);
  const now = new Date();
  const opts = { hour: '2-digit', minute: '2-digit' };
  const sameDay = d.toDateString() === now.toDateString();
  if (sameDay) return d.toLocaleTimeString(undefined, opts);
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

function reminderCardHTML(item, actionsHTML) {
  const paused = item.status === 'paused' ? ' is-paused' : '';
  return `
    <article class="reminder-card${paused}" data-id="${item.id}">
      <div class="reminder-stripe"></div>
      <div>
        <div class="reminder-time">${formatDue(item.due_at)} ${statusBadge(item)}</div>
        <div class="reminder-title">${escapeHTML(item.title)}</div>
        <div class="reminder-meta">${escapeHTML(item.priority)} · ${escapeHTML(item.status)}</div>
      </div>
      <div class="reminder-actions">${actionsHTML}</div>
    </article>
  `;
}

function escapeHTML(s) {
  return String(s || '').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;');
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
