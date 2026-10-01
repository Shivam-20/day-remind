/* Shared helpers for Message wording: AI prompt, paste JSON, inbox-style mail preview */
const MAIL_AI_PROMPT = `You are designing a reminder notification template for Day Remind.
Return ONLY valid JSON (no markdown code fences, no commentary) with exactly these keys:
"name", "email_subject", "email_html", "email_css", "email_text", "telegram_text"

Use Go text/template actions for dynamic content (not your own placeholder syntax):
{{.Title}}, {{.Description}}, {{.DueAt}}, {{.Timezone}}, {{.Priority}}, {{.Advance}}, {{.LeadTime}},
{{.UserName}}, {{.ListName}}, {{.Tags}}, {{.ReminderURL}}, {{.Today}}
UserName/ListName/Tags/ReminderURL may be empty — guard with {{if .UserName}}…{{end}}.
Use {{if .Advance}}...{{else}}...{{end}} where the wording should differ for an early warning vs the due notice.

Rules:
- email_html: complete mobile-friendly HTML email, table-based layout, INLINE styles only, max-width 560px, no <script>.
- email_css: CSS rules only — no <style> wrapper, no <script>.
- email_text: plain-text alternative for mail clients that skip HTML.
- telegram_text: Telegram message (short, only <b>/<i>/<a> tags, 1-2 emoji).
- name: short label for the template, e.g. "Friendly bill reminder".

Sample data: Title "Pay electricity bill", Description "Account 12345", Priority "high", DueAt like "Fri, 25 Dec 2026 09:00 IST", UserName "Alex", ListName "Bills".`;

let wordingPasteOpen = false;

function wordingPayloadFromTemplate(t) {
  return {
    name: t.name || '',
    email_subject: t.email_subject || '',
    email_html: t.email_html || '',
    email_css: t.email_css || '',
    email_text: t.email_text || '',
    telegram_text: t.telegram_text || '',
  };
}

// Fragments have no document of their own. srcdoc must go through escapeAttr so quotes in the HTML do not truncate the attribute.
function mailPreviewDocument(html) {
  const trimmed = String(html || '').trim();
  if (/^<!doctype/i.test(trimmed) || /^<html[\s>]/i.test(trimmed)) return trimmed;
  return `<!DOCTYPE html><html><head><meta charset="utf-8"><style>html,body{margin:0;padding:12px;background:transparent;}</style></head><body>${trimmed}</body></html>`;
}

/** Plain-text card subtitle from a Go text/template subject (no server round-trip). */
function wordingSubjectPreview(subject) {
  let s = String(subject || '').trim();
  if (!s) return 'Default subject';

  const ifElse = /\{\{if\s+[^}]+\}\}([\s\S]*?)\{\{else\}\}([\s\S]*?)\{\{end\}\}/;
  const ifOnly = /\{\{if\s+[^}]+\}\}([\s\S]*?)\{\{end\}\}/;
  for (let i = 0; i < 20; i++) {
    const m = ifElse.exec(s);
    if (m) {
      const a = m[1].trim();
      const b = m[2].trim();
      s = s.slice(0, m.index) + `${a} / ${b}` + s.slice(m.index + m[0].length);
      continue;
    }
    const m2 = ifOnly.exec(s);
    if (m2) {
      s = s.slice(0, m2.index) + m2[1].trim() + s.slice(m2.index + m2[0].length);
      continue;
    }
    break;
  }

  s = s.replace(/\{\{\.([^}]+)\}\}/g, (_, name) => name.trim());
  s = s.replace(/\{\{[^}]+\}\}/g, '');
  s = s.replace(/\s+/g, ' ').trim();
  return s || 'Default subject';
}

function mailPreviewAsInboxHTML(res) {
  return ['due', 'advance'].map((kind) => {
    const p = res[kind] || {};
    const label = kind === 'due' ? 'At the due time' : 'Advance notice';
    return `
      <div class="card channel-card mail-preview-card">
        <h3>View as mail — ${label}</h3>
        ${(p.warnings || []).map((w) => `<p class="test-result bad">${escapeHTML(w)}</p>`).join('')}
        <div class="mail-preview-inbox">
          <span class="mail-preview-from">Day Remind</span>
          <p class="mail-preview-subject">${escapeHTML(p.subject || '(no subject)')}</p>
        </div>
        <iframe class="preview-frame mail-preview-frame" sandbox srcdoc="${escapeAttr(mailPreviewDocument(p.html || ''))}" title="Email preview"></iframe>
        <p class="preview-label">Plain text alternative</p>
        <pre class="preview-pre">${escapeHTML(p.text || '')}</pre>
      </div>`;
  }).join('');
}

async function copyMailAIPrompt() {
  try {
    await navigator.clipboard.writeText(MAIL_AI_PROMPT);
    showToast('AI prompt copied');
  } catch {
    showToast('Could not copy — check browser permissions', 'error');
  }
}

function parseMailAIResult(raw) {
  let text = raw.trim();
  const fence = text.match(/^```(?:json)?\s*([\s\S]*?)```$/i);
  if (fence) text = fence[1].trim();
  const data = JSON.parse(text);
  const keys = ['name', 'email_subject', 'email_html', 'email_css', 'email_text', 'telegram_text'];
  const out = {};
  for (const k of keys) {
    if (data[k] != null) out[k] = String(data[k]);
  }
  if (!out.name) out.name = 'AI template';
  return out;
}
