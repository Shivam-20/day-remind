/* Feature routes: calendar, lists, settings sub-pages. */
window.DR_FEATURE_ROUTES = {
  '/calendar': renderCalendar,
  '/lists': renderLists,
  '/settings/wording': renderMessageWording,
};

const featureState = {
  calendarMonth: new Date(),
  selectedDay: null,
};

const LIST_PALETTE = ['#7cacf8', '#f5a623', '#6bcf7f', '#f07178', '#b48ef0', '#4ec3d4'];

function localDayKey(iso) {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '';
  const y = d.getFullYear();
  const m = String(d.getMonth() + 1).padStart(2, '0');
  const day = String(d.getDate()).padStart(2, '0');
  return `${y}-${m}-${day}`;
}

// The calendar API is optional; reminders are a complete fallback.
async function loadCalendarEvents(from, to) {
  try {
    const data = await API.get(`/api/calendar?from=${encodeURIComponent(from.toISOString())}&to=${encodeURIComponent(to.toISOString())}`);
    return data.events || [];
  } catch {
    await loadReminders();
    return (state.reminders || [])
      .filter((r) => r.status !== 'cancelled')
      .map((r) => ({
        id: r.id,
        title: r.title,
        start: r.due_at,
        status: r.status,
        priority: r.priority,
        timezone: r.timezone,
        pinned: r.pinned,
        recurrence: r.recurrence,
      }));
  }
}

// ---------------------------------------------------------------- calendar

function calDotClass(ev) {
  if (ev.status === 'done') return '';
  if (new Date(ev.start) < new Date()) return 'cal-dot-overdue';
  if (ev.priority === 'urgent' || ev.priority === 'high') return `cal-dot-${ev.priority}`;
  return '';
}

async function renderCalendar() {
  const year = featureState.calendarMonth.getFullYear();
  const month = featureState.calendarMonth.getMonth();
  const from = new Date(year, month, 1);
  const to = new Date(year, month + 1, 0, 23, 59, 59);
  const events = await loadCalendarEvents(from, to);
  const byDay = {};
  events.forEach((ev) => {
    const key = localDayKey(ev.start);
    if (!key) return;
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
  const todayKey = localDayKey(new Date().toISOString());
  for (let d = 1; d <= daysInMonth; d++) {
    const key = `${year}-${String(month + 1).padStart(2, '0')}-${String(d).padStart(2, '0')}`;
    const dayEvents = byDay[key] || [];
    const dots = dayEvents
      .filter((ev) => ev.status !== 'done' && ev.status !== 'cancelled')
      .slice(0, 3)
      .map((ev) => `<i class="cal-dot ${calDotClass(ev)}" aria-hidden="true"></i>`)
      .join('');
    const isToday = key === todayKey;
    const isSelected = featureState.selectedDay === key;
    const ariaLabel = `${d} ${monthLabel}${dayEvents.length ? `, ${dayEvents.length} reminder${dayEvents.length !== 1 ? 's' : ''}` : ''}`;
    cells += `<button type="button" class="cal-cell${isToday ? ' cal-today' : ''}${isSelected ? ' cal-selected' : ''}" data-day="${key}" aria-label="${escapeHTML(ariaLabel)}">
      <span class="cal-day-num">${d}</span>
      <span class="cal-dots">${dots}</span>
    </button>`;
  }

  const selectedEvents = featureState.selectedDay ? (byDay[featureState.selectedDay] || []) : [];
  const sidePanel = featureState.selectedDay
    ? `<div class="card">
        <div class="cal-side-title">
          <h3>${new Date(featureState.selectedDay + 'T12:00:00').toLocaleDateString(undefined, { weekday: 'long', month: 'short', day: 'numeric' })}</h3>
          <span>${selectedEvents.length} reminder${selectedEvents.length !== 1 ? 's' : ''}</span>
        </div>
        ${selectedEvents.length
          ? selectedEvents.map((ev) => `
            <div class="cal-srow${isOverdue(ev) ? ' late' : ''}">
              <span class="cal-srow-when">${escapeHTML(formatDue(ev.start, ev.timezone))}</span>
              <span class="cal-srow-title">${ev.pinned ? iconHTML('pin') : ''}${escapeHTML(ev.title)}</span>
              <button type="button" class="icon-btn" data-action="detail" data-id="${escapeHTML(ev.id)}" aria-label="Details for ${escapeHTML(ev.title)}">${actionIconSVG('info')}</button>
            </div>`).join('')
          : emptyState('Nothing scheduled', 'Enjoy the empty day.', '', 'sun')}
        <button class="cal-add" type="button" id="cal-add-day">${actionIconSVG('calendar-plus')}Add for this day</button>
      </div>`
    : '<div class="card"><p class="page-sub" style="margin:0">Click a day to see its reminders.</p></div>';

  const main = `
    <h1 class="page-title">Calendar</h1>
    <div class="cal-headrow">
      <div class="cal-month">
        <button class="icon-btn" id="cal-prev" type="button" aria-label="Previous month">${actionIconSVG('chevron-left')}</button>
        <strong>${monthLabel}</strong>
        <button class="icon-btn" id="cal-next" type="button" aria-label="Next month">${actionIconSVG('chevron-right')}</button>
        <button class="cal-today-btn" id="cal-today" type="button">Today</button>
      </div>
      <div class="cal-legend" aria-hidden="true">
        <span><i style="background:var(--danger)"></i>overdue</span>
        <span><i style="background:var(--warning)"></i>high</span>
        <span><i style="background:var(--border-strong)"></i>normal</span>
      </div>
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
  document.getElementById('app').innerHTML = shellHTML('/calendar', main, 'Calendar');
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
    featureState.selectedDay = localDayKey(new Date().toISOString());
    renderCalendar();
  });
  document.querySelectorAll('.cal-cell[data-day]').forEach((btn) => {
    btn.addEventListener('click', () => {
      featureState.selectedDay = btn.dataset.day;
      renderCalendar();
    });
  });
  document.getElementById('cal-add-day')?.addEventListener('click', () => {
    const day = featureState.selectedDay;
    closeAllMenus();
    if (day) openCreateDrawer(new Date(day + 'T00:00:00'));
  });
  bindReminderActions(document.getElementById('app'), () => renderCalendar());
}

// ---------------------------------------------------------------- lists

// The lists API is optional; without it, lists are derived from reminders.
async function loadListsOrFallback() {
  try {
    const data = await API.get('/api/lists');
    return { lists: data.lists || [], canManage: true };
  } catch {
    await loadReminders();
    const groups = {};
    (state.reminders || []).forEach((r) => {
      const id = r.list_id || 'unlisted';
      if (!groups[id]) {
        groups[id] = {
          id,
          name: id === 'unlisted' ? 'Unlisted' : 'List',
          count: 0,
          done: 0,
          overdue: 0,
          paused: false,
        };
      }
      if (r.status === 'done') groups[id].done += 1;
      else {
        groups[id].count += 1;
        if (isOverdue(r)) groups[id].overdue += 1;
      }
    });
    return { lists: Object.values(groups), canManage: false };
  }
}

async function renderLists() {
  const { lists, canManage } = await loadListsOrFallback();
  const countLabel = lists.length === 1 ? '1 list' : `${lists.length} lists`;

  const cards = lists.map((l, i) => {
    const color = l.color || LIST_PALETTE[i % LIST_PALETTE.length];
    const total = l.done != null ? l.count + l.done : (l.count ?? 0);
    const active = l.count ?? 0;
    const paused = l.paused;
    return `
      <article class="list-card${paused ? ' is-paused' : ''}" style="--list-color:${escapeHTML(color)}" data-list="${escapeHTML(l.id || '')}" tabindex="0">
        <h3>${escapeHTML(l.name)}
          ${paused ? '<span class="badge badge-idle">paused</span>' : `<span class="badge badge-ok">${active} active</span>`}
        </h3>
        <p class="list-card-sub"><b>${total} total</b>${l.done ? ` · ${l.done} done` : ''}${l.overdue ? ` · <span style="color:var(--danger)">${l.overdue} overdue</span>` : ''}</p>
        <div class="row-actions">
          <button type="button" class="icon-btn" data-open-list aria-label="Open ${escapeHTML(l.name)}">${actionIconSVG('arrow-right')}</button>
          ${canManage ? `
            <button type="button" class="icon-btn" data-rename-list aria-label="Rename ${escapeHTML(l.name)}">${actionIconSVG('pencil')}</button>
            ${paused
              ? `<button type="button" class="icon-btn" data-resume="${escapeHTML(l.id)}" aria-label="Resume ${escapeHTML(l.name)}">${actionIconSVG('play')}</button>`
              : `<button type="button" class="icon-btn" data-pause="${escapeHTML(l.id)}" aria-label="Pause ${escapeHTML(l.name)}">${actionIconSVG('pause')}</button>`}
          ` : ''}
        </div>
      </article>`;
  }).join('');

  const body = lists.length
    ? `<div class="list-cards">${cards}</div>
       <p class="list-page-footer"><a href="#/reminders" class="btn btn-ghost btn-sm">View all reminders →</a></p>`
    : emptyState(
        'No lists yet',
        canManage ? 'Create a list to organize reminders.' : 'Add a reminder with a list to see groups here. Reminders always show under All reminders.',
        canManage ? '<button class="btn btn-primary" data-new-list>Create list</button>' : '<a href="#/reminders" class="btn btn-primary btn-sm">View all reminders</a>',
        'list');

  const main = `
    <div class="page-header-row">
      <div>
        <h1 class="page-title">Lists</h1>
        <p class="page-sub">${countLabel}</p>
      </div>
      ${canManage ? '<button class="btn btn-primary btn-sm btn-with-icon" data-new-list>' + iconHTML('plus') + '<span class="btn-label">New list</span></button>' : ''}
    </div>
    ${body}
  `;
  document.getElementById('app').innerHTML = shellHTML('/lists', main, 'Lists');
  bindShell('/lists');

  document.querySelectorAll('.list-card').forEach((card) => {
    const openCard = (e) => {
      if (e.target.closest('button')) return;
      const id = card.dataset.list;
      if (id && id !== 'unlisted') {
        state.listFilter = id;
        state.listFilterName = card.querySelector('h3')?.textContent.split('\n')[0]?.trim() || id;
      } else {
        state.listFilter = '';
      }
      location.hash = '#/reminders';
      renderReminders();
    };
    card.addEventListener('click', openCard);
    card.addEventListener('keydown', (e) => {
      if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); openCard(e); }
    });
  });
  document.querySelectorAll('[data-open-list]').forEach((btn) => {
    btn.addEventListener('click', () => btn.closest('.list-card').click());
  });
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
