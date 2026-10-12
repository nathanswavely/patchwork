/**
 * Liquid Textile — Behaviors
 *
 * The few interactions every app ends up writing, done once, without a
 * framework: theme selection, platform-aware key labels, a combobox, a
 * dialog (drawer, sheet), toasts, an action menu, tabs, a segmented
 * control, a two-step confirm, an inline editor, a fold, copying to the
 * clipboard, the shared media queries (media, viewport) and an edge fade
 * for sideways scrollers (scrollFade). Each is a plain function over
 * plain elements, styled by foundation.css, returning a small handle. Only
 * inlineEdit and copy use another (toast, to report), and tabs uses
 * scrollFade.
 *
 *   import { initTheme, keys, combobox, dialog, toast } from './behaviors.js';
 *   import { menu, tabs, segmented, confirmAction, inlineEdit, fold, copy } from './behaviors.js';
 *   import { media, viewport, scrollFade } from './behaviors.js';
 */

// ------------------------------------------------------------------ theme ---

/**
 * Set data-theme on <html> from a saved choice, else the OS setting, and
 * keep following the OS until the person chooses. Call it before first
 * paint (a module script in <head>) to avoid a flash of the wrong theme.
 *
 * Returns { get, set, toggle, preference }. get() is what is showing,
 * 'light' or 'dark'; preference() is what the person chose, 'light',
 * 'dark' or 'system', so a Display menu can keep "System" lit while the
 * OS swings it. set(null) and set('system') both forget the choice.
 *
 * To stop the flash before this module loads, put the same logic inline
 * in <head> (see README), reading the same key.
 */
export function initTheme({ key = 'lt-theme', root = document.documentElement } = {}) {
  const media = matchMedia('(prefers-color-scheme: dark)');
  // Anything but light or dark (an app that stored 'system', a stale value)
  // means no choice, never a data-theme the stylesheets do not know.
  const saved = () => {
    try { const v = localStorage.getItem(key); return v === 'light' || v === 'dark' ? v : null; } catch { return null; }
  };
  const apply = (t) => root.setAttribute('data-theme', t);
  const current = () => saved() || (media.matches ? 'dark' : 'light');

  apply(current());
  media.addEventListener('change', () => { if (!saved()) apply(current()); });

  return {
    get: () => root.getAttribute('data-theme'),
    preference: () => saved() || 'system',
    set(theme) {
      const choice = theme === 'light' || theme === 'dark' ? theme : null;
      try { choice ? localStorage.setItem(key, choice) : localStorage.removeItem(key); } catch { /* a preference */ }
      apply(choice || current());
    },
    toggle() { this.set(this.get() === 'dark' ? 'light' : 'dark'); },
  };
}

// ------------------------------------------------------------------- keys ---

const MAC = typeof navigator !== 'undefined' &&
  /Mac|iPhone|iPad/.test(navigator.userAgentData?.platform || navigator.platform || '');

/**
 * A shortcut as this platform writes it. "Mod" is Cmd on a Mac and Ctrl
 * elsewhere; handlers should accept either (ev.metaKey || ev.ctrlKey).
 *
 *   keys('Mod+Enter') → '⌘↵' on a Mac, 'Ctrl+Enter' elsewhere
 */
export function keys(combo) {
  const parts = combo.split('+');
  if (!MAC) return parts.map((p) => (p === 'Mod' ? 'Ctrl' : p)).join('+');
  const mac = { Mod: '⌘', Shift: '⇧', Alt: '⌥', Ctrl: '⌃', Enter: '↵' };
  return parts.map((p) => mac[p] ?? p).join('');
}
keys.mac = MAC;

/** True when a key press is meant as typing, not as a shortcut. */
export function isTyping(el) {
  if (!el) return false;
  if (el.tagName === 'TEXTAREA' || el.tagName === 'SELECT' || el.isContentEditable) return true;
  return el.tagName === 'INPUT' && !['checkbox', 'radio', 'button', 'submit', 'reset'].includes(el.type);
}

// --------------------------------------------------------------- viewport ---

/**
 * The media conditions Liquid Textile's CSS is written against, as
 * strings, so script decides "narrow" exactly where the stylesheet does.
 * (Patchwork kept one such string in a store and a different one in four
 * components.)
 *
 *   narrow         a phone layout: stack, drill down, tables to lists
 *   handheld       the app's chrome goes mobile: bottom tab bar
 *   coarse         a finger: --lt-target is 44px
 *   hover          a real hover: a pointer that can rest without pressing
 *   reducedMotion  the person asked for less movement
 */
export const media = Object.freeze({
  narrow: '(max-width: 640px)',
  handheld: '(max-width: 768px)',
  coarse: '(pointer: coarse)',
  hover: '(hover: hover) and (pointer: fine)',
  reducedMotion: '(prefers-reduced-motion: reduce)',
});

/**
 * Live answers to `media`, for the layout decisions CSS cannot make:
 * whether a bare section URL should redirect (only on a wide screen),
 * whether to render a table or a list, whether a tap previews or opens.
 *
 *   const vp = viewport((now) => render(now));   // called on any change
 *   if (vp.narrow) ...
 *
 * Returns an object with a boolean per condition, plus destroy().
 */
export function viewport(onChange = () => {}) {
  const lists = Object.entries(media).map(([name, q]) => [name, matchMedia(q)]);
  const now = () => Object.fromEntries(lists.map(([name, m]) => [name, m.matches]));
  const fire = () => onChange(now());
  for (const [, m] of lists) m.addEventListener('change', fire);
  const handle = {
    destroy() { for (const [, m] of lists) m.removeEventListener('change', fire); },
  };
  for (const [name, m] of lists) Object.defineProperty(handle, name, { get: () => m.matches, enumerable: true });
  return handle;
}

// --------------------------------------------------------------- combobox ---

let comboCount = 0;

/**
 * Turn a text input into a combobox: a filtered list under it, arrow keys
 * and Enter to choose, Escape to close. The input's parent must be
 * positioned (position: relative); the list is appended to it.
 *
 *   combobox(input, {
 *     options: () => [{ value: 'a.md', meta: '5' }, ...],   // read on every open
 *     onChoose: (value) => ...,                             // '' when cleared
 *     onType: (text) => ...,                                // as the person types
 *   })
 *
 * An option may also carry `group` (options with the same group are
 * listed under it, in the order the groups first appear) and `disabled`
 * (shown, refused, skipped by the arrow keys: a choice that exists but is
 * not open to this person, with its reason in `meta`).
 *
 * Typed text filters by substring and highlights the match. Returns
 * { open, close, choose, destroy }.
 */
export function combobox(input, { options, onChoose = () => {}, onType = () => {}, empty = 'No matches' } = {}) {
  const id = `lt-combo-${++comboCount}`;
  const menu = document.createElement('ul');
  menu.className = 'lt-popover lt-menu';
  menu.id = id;
  menu.setAttribute('role', 'listbox');
  menu.hidden = true;
  Object.assign(menu.style, {
    position: 'absolute', top: 'calc(100% + 4px)', left: '0', zIndex: 'var(--lt-z-menu, 30)',
    minWidth: '100%', width: 'max-content', maxWidth: 'min(560px, calc(100vw - 32px))',
  });
  input.parentElement.append(menu);
  input.setAttribute('role', 'combobox');
  input.setAttribute('aria-autocomplete', 'list');
  input.setAttribute('aria-expanded', 'false');
  input.setAttribute('aria-controls', id);
  input.autocomplete = 'off';

  let items = [];
  let rows = [];        // the <li> for each item, in the same order
  let active = -1;
  let picked = false;   // the box holds a chosen value, not a search

  const usable = (i) => i >= 0 && i < items.length && !items[i].disabled;

  function render() {
    const q = picked ? '' : input.value.trim().toLowerCase();
    const found = options().filter((o) => !q || String(o.label ?? o.value).toLowerCase().includes(q));
    // Group, keeping the order groups first appear in; ungrouped first.
    const groups = new Map([[undefined, []]]);
    for (const o of found) { if (!groups.has(o.group)) groups.set(o.group, []); groups.get(o.group).push(o); }
    items = [...groups.values()].flat();
    rows = [];
    active = items.findIndex((o) => !o.disabled);
    menu.textContent = '';
    if (!items.length) {
      const li = document.createElement('li');
      li.className = 'lt-menu-empty';
      li.textContent = empty;
      menu.append(li);
    }
    let lastGroup;
    items.forEach((o, i) => {
      if (o.group !== undefined && o.group !== lastGroup) {
        const head = document.createElement('li');
        head.className = 'lt-menu-label';
        head.setAttribute('role', 'presentation');
        head.textContent = o.group;
        menu.append(head);
      }
      lastGroup = o.group;
      const li = document.createElement('li');
      li.className = 'lt-menu-item';
      li.id = `${id}-${i}`;
      li.setAttribute('role', 'option');
      if (o.disabled) li.setAttribute('aria-disabled', 'true');
      const label = String(o.label ?? o.value);
      const name = document.createElement('span');
      const at = q ? label.toLowerCase().indexOf(q) : -1;
      if (at >= 0) {
        const m = document.createElement('mark');
        m.textContent = label.slice(at, at + q.length);
        name.append(label.slice(0, at), m, label.slice(at + q.length));
      } else {
        name.textContent = label;
      }
      li.append(name);
      if (o.meta !== undefined) {
        const meta = document.createElement('span');
        meta.className = 'lt-menu-meta';
        meta.textContent = o.meta;
        li.append(meta);
      }
      // mousedown, not click: a click blurs the input and closes the menu first.
      li.addEventListener('mousedown', (ev) => { ev.preventDefault(); if (!o.disabled) choose(o.value); });
      li.addEventListener('mousemove', () => { if (active !== i && !o.disabled) { active = i; mark(); } });
      menu.append(li);
      rows.push(li);
    });
    mark();
  }

  function mark() {
    rows.forEach((li, i) => li.setAttribute('aria-selected', String(i === active)));
    const li = rows[active];
    if (!li) { input.removeAttribute('aria-activedescendant'); return; }
    input.setAttribute('aria-activedescendant', li.id);
    // Scroll the list, never the page, to keep the active row in view.
    if (li.offsetTop < menu.scrollTop) menu.scrollTop = li.offsetTop;
    else if (li.offsetTop + li.offsetHeight > menu.scrollTop + menu.clientHeight) {
      menu.scrollTop = li.offsetTop + li.offsetHeight - menu.clientHeight;
    }
  }

  // The next usable item in a direction, wrapping; -1 when there is none.
  function step(dir) {
    for (let n = 1; n <= items.length; n++) {
      const i = (active + dir * n + items.length * 2) % items.length;
      if (usable(i)) return i;
    }
    return -1;
  }

  function open() {
    render();
    menu.hidden = false;
    input.setAttribute('aria-expanded', 'true');
  }

  function close() {
    menu.hidden = true;
    input.setAttribute('aria-expanded', 'false');
    input.removeAttribute('aria-activedescendant');
  }

  function choose(value) {
    input.value = value;
    picked = !!value;
    close();
    onChoose(value);
  }

  const onFocus = () => { input.select(); open(); };
  const onInput = () => { picked = false; open(); onType(input.value); };
  const onKey = (ev) => {
    const isOpen = !menu.hidden;
    if (ev.key === 'ArrowDown' || ev.key === 'ArrowUp') {
      ev.preventDefault();
      if (!isOpen) return open();
      const next = step(ev.key === 'ArrowDown' ? 1 : -1);
      if (next < 0) return;
      active = next;
      mark();
    } else if (ev.key === 'Enter') {
      ev.preventDefault();
      if (isOpen && usable(active)) choose(items[active].value);
      input.blur();
    } else if (ev.key === 'Escape' && isOpen) {
      // The first Escape closes the list; the next one is the page's.
      ev.preventDefault();
      ev.stopPropagation();
      close();
    }
  };

  input.addEventListener('focus', onFocus);
  input.addEventListener('blur', close);
  input.addEventListener('input', onInput);
  input.addEventListener('keydown', onKey);

  return {
    open,
    close,
    choose,
    /** True when the box holds a value picked from the list. */
    get picked() { return picked; },
    destroy() {
      input.removeEventListener('focus', onFocus);
      input.removeEventListener('blur', close);
      input.removeEventListener('input', onInput);
      input.removeEventListener('keydown', onKey);
      menu.remove();
    },
  };
}

// ----------------------------------------------------------------- dialog ---

/**
 * A modal over a native <dialog class="lt-dialog"> (or .lt-drawer, or
 * .lt-sheet: same behaviour, different edge). The browser does the hard
 * parts (focus held inside, Escape closes, the rest of the page inert);
 * this adds closing on a backdrop click, closing from any [data-close]
 * inside, and handing focus back afterwards.
 *
 * Returns { open, close, toggle, get isOpen }.
 */
export function dialog(el, { onClose = () => {} } = {}) {
  let returnTo = null;
  el.addEventListener('click', (ev) => {
    const closer = ev.target.closest?.('[data-close]');
    if (closer && el.contains(closer)) { el.close(); return; }
    // A click on the dialog element itself, outside its content box, is the backdrop.
    if (ev.target !== el) return;
    const r = el.getBoundingClientRect();
    const inside = ev.clientX >= r.left && ev.clientX <= r.right && ev.clientY >= r.top && ev.clientY <= r.bottom;
    if (!inside) el.close();
  });
  el.addEventListener('close', () => { returnTo?.focus?.(); returnTo = null; onClose(); });
  return {
    open() { if (!el.open) { returnTo = document.activeElement; el.showModal(); } },
    close() { if (el.open) el.close(); },
    toggle() { el.open ? el.close() : this.open(); },
    get isOpen() { return el.open; },
  };
}

// ------------------------------------------------------------------ toast ---

let region = null;

/**
 * A short message near the foot of the screen, announced to screen readers
 * (role=status, polite). Returns a function that dismisses it early.
 * Set --lt-toast-lift on <body> to clear anything else floating down there.
 */
export function toast(message, { timeout = 2400, tone = '' } = {}) {
  if (!region || !region.isConnected) {
    region = document.createElement('div');
    region.className = 'lt-toast-region';
    region.setAttribute('role', 'status');
    region.setAttribute('aria-live', 'polite');
    document.body.append(region);
  }
  const el = document.createElement('div');
  el.className = `lt-toast lt-popover${tone ? ` lt-tone-${tone}` : ''}`;
  el.textContent = message;
  // One at a time reads better than a stack for short confirmations.
  region.textContent = '';
  region.append(el);
  let timer = setTimeout(dismiss, timeout);
  function dismiss() {
    clearTimeout(timer);
    el.classList.add('is-leaving');
    setTimeout(() => el.remove(), 220);
  }
  return dismiss;
}

// ------------------------------------------------------------------- menu ---

let menuCount = 0;

/**
 * A menu of actions under a button: the overflow "⋯", an account menu, a
 * "New" menu. The list is any element of .lt-menu-item buttons and links
 * (put .lt-popover .lt-menu on it for the look). Its parent must be
 * positioned, as with combobox; the list opens under the trigger and flips
 * above it when there is no room below.
 *
 *   menu(button, list, { onSelect: (item) => ..., align: 'end' })
 *
 * Click, or ArrowDown on the trigger, opens it on the first item; ArrowUp
 * on the last. Arrows, Home and End move; Escape closes and hands focus
 * back to the trigger; Tab or a click elsewhere closes it. Choosing an item
 * closes it too. Items with aria-disabled="true" are skipped and refused.
 *
 * Returns { open, close, toggle, get isOpen, destroy }.
 */
export function menu(trigger, list, { onSelect = () => {}, align = 'start' } = {}) {
  list.id ||= `lt-menu-${++menuCount}`;
  list.setAttribute('role', 'menu');
  list.hidden = true;
  Object.assign(list.style, {
    position: 'absolute', zIndex: 'var(--lt-z-menu, 30)', minWidth: 'max(10rem, 100%)',
    [align === 'end' ? 'right' : 'left']: '0',
  });
  trigger.setAttribute('aria-haspopup', 'menu');
  trigger.setAttribute('aria-expanded', 'false');
  trigger.setAttribute('aria-controls', list.id);
  for (const el of list.querySelectorAll('.lt-menu-item')) {
    el.setAttribute('role', 'menuitem');
    el.tabIndex = -1;
  }
  const items = () => [...list.querySelectorAll('[role="menuitem"]')]
    .filter((el) => el.getAttribute('aria-disabled') !== 'true' && !el.disabled);

  function place() {
    // Below by default; above when the bottom would leave the viewport and
    // there is room over the trigger (a menu near the foot of a sheet).
    Object.assign(list.style, { top: 'calc(100% + 4px)', bottom: 'auto' });
    const r = list.getBoundingClientRect();
    if (r.bottom > innerHeight && trigger.getBoundingClientRect().top > r.height + 8) {
      Object.assign(list.style, { top: 'auto', bottom: 'calc(100% + 4px)' });
    }
  }

  function open(which = 'first') {
    list.hidden = false;
    trigger.setAttribute('aria-expanded', 'true');
    place();
    const all = items();
    (which === 'last' ? all[all.length - 1] : all[0])?.focus();
    document.addEventListener('pointerdown', onOutside, true);
  }

  function close(returnFocus = false) {
    if (list.hidden) return;
    list.hidden = true;
    trigger.setAttribute('aria-expanded', 'false');
    document.removeEventListener('pointerdown', onOutside, true);
    if (returnFocus) trigger.focus();
  }

  const onOutside = (ev) => {
    if (!list.contains(ev.target) && !trigger.contains(ev.target)) close();
  };
  const onTriggerClick = () => (list.hidden ? open() : close());
  const onTriggerKey = (ev) => {
    if (ev.key === 'ArrowDown' || ev.key === 'ArrowUp') {
      ev.preventDefault();
      open(ev.key === 'ArrowUp' ? 'last' : 'first');
    }
  };
  const onListKey = (ev) => {
    const all = items();
    const at = all.indexOf(document.activeElement);
    const go = (i) => all[(i + all.length) % all.length]?.focus();
    if (ev.key === 'ArrowDown') { ev.preventDefault(); go(at + 1); }
    else if (ev.key === 'ArrowUp') { ev.preventDefault(); go(at - 1); }
    else if (ev.key === 'Home') { ev.preventDefault(); go(0); }
    else if (ev.key === 'End') { ev.preventDefault(); go(all.length - 1); }
    else if (ev.key === 'Escape') { ev.preventDefault(); ev.stopPropagation(); close(true); }
    else if (ev.key === 'Tab') close();
  };
  const onListClick = (ev) => {
    const item = ev.target.closest('[role="menuitem"]');
    if (!item || !list.contains(item)) return;
    if (item.getAttribute('aria-disabled') === 'true') { ev.preventDefault(); return; }
    onSelect(item);
    close(item.tagName !== 'A');
  };

  trigger.addEventListener('click', onTriggerClick);
  trigger.addEventListener('keydown', onTriggerKey);
  list.addEventListener('keydown', onListKey);
  list.addEventListener('click', onListClick);

  return {
    open,
    close,
    toggle() { if (list.hidden) open(); else close(); },
    get isOpen() { return !list.hidden; },
    destroy() {
      close();
      trigger.removeEventListener('click', onTriggerClick);
      trigger.removeEventListener('keydown', onTriggerKey);
      list.removeEventListener('keydown', onListKey);
      list.removeEventListener('click', onListClick);
    },
  };
}

// ------------------------------------------------------------------- tabs ---

/**
 * A row of .lt-tab inside .lt-tabs. Two kinds:
 *
 * - Tabs that switch a panel in place: role="tab" buttons, each with
 *   aria-controls naming its panel. This sets aria-selected, hides the
 *   other panels, and gives the row one tab stop with Left/Right/Home/End
 *   (choosing as it moves, as tabs do).
 * - Tabs that are links (aria-current="page" on the one you are on): only
 *   the overflow part below applies.
 *
 * Either way it scrolls the current tab into view and keeps data-overflow
 * ("start", "end", "both") on the row, so the CSS fades whichever edge
 * has more beyond it.
 *
 *   tabs(row, { onSelect: (tab) => ... })
 *
 * Returns { select(tabOrIndex), destroy }.
 */
export function tabs(row, { onSelect = () => {} } = {}) {
  const all = () => [...row.querySelectorAll('[role="tab"]')];
  const panel = (t) => document.getElementById(t.getAttribute('aria-controls'));

  // Scroll the row, never the page, to bring a tab into view.
  function reveal(t) {
    if (!t) return;
    const r = row.getBoundingClientRect();
    const b = t.getBoundingClientRect();
    if (b.left < r.left) row.scrollLeft -= r.left - b.left;
    else if (b.right > r.right) row.scrollLeft += b.right - r.right;
  }

  function select(t, { focus = false, quiet = false } = {}) {
    if (typeof t === 'number') t = all()[t];
    if (!t) return;
    for (const x of all()) {
      const on = x === t;
      x.setAttribute('aria-selected', String(on));
      x.tabIndex = on ? 0 : -1;
      const p = panel(x);
      if (p) p.hidden = !on;
    }
    if (focus) t.focus();
    reveal(t);
    if (!quiet) onSelect(t);
  }

  const onClick = (ev) => {
    const t = ev.target.closest('[role="tab"]');
    if (t && row.contains(t)) select(t);
  };
  const onKey = (ev) => {
    const list = all();
    const at = list.indexOf(document.activeElement);
    if (at < 0) return;
    const to = { ArrowRight: at + 1, ArrowLeft: at - 1, Home: 0, End: list.length - 1 }[ev.key];
    if (to === undefined) return;
    ev.preventDefault();
    select(list[(to + list.length) % list.length], { focus: true });
  };

  row.addEventListener('click', onClick);
  row.addEventListener('keydown', onKey);

  const first = all();
  if (first.length) select(first.find((t) => t.getAttribute('aria-selected') === 'true') || first[0], { quiet: true });
  else reveal(row.querySelector('[aria-current="page"]'));
  const fade = scrollFade(row);

  return {
    select: (t) => select(t),
    destroy() {
      row.removeEventListener('click', onClick);
      row.removeEventListener('keydown', onKey);
      fade.destroy();
    },
  };
}

// ------------------------------------------------------------- scrollFade ---

/**
 * Keep data-overflow ("start", "end", "both", or none) on a sideways
 * scroller, so the CSS can fade whichever edge has more beyond it: a tab
 * row, a .lt-chips-scroll row. A fade over nothing is its own small lie,
 * so there is none while nothing is hidden (Patchwork, f2660b9: a member
 * of a year never found the Noticeboard tab past the edge).
 *
 * Returns { update, destroy }.
 */
export function scrollFade(el) {
  function update() {
    const start = el.scrollLeft > 1;
    const end = el.scrollLeft + el.clientWidth < el.scrollWidth - 1;
    if (start || end) el.dataset.overflow = start && end ? 'both' : start ? 'start' : 'end';
    else delete el.dataset.overflow;
  }
  el.addEventListener('scroll', update, { passive: true });
  const ro = typeof ResizeObserver !== 'undefined' ? new ResizeObserver(update) : null;
  ro?.observe(el);
  update();
  return {
    update,
    destroy() {
      el.removeEventListener('scroll', update);
      ro?.disconnect();
      delete el.dataset.overflow;
    },
  };
}

// -------------------------------------------------------------- segmented ---

/**
 * Make .lt-segmented a radio group of its .lt-segment buttons: one tab
 * stop, arrows move and choose, aria-checked marks the choice. A segment's
 * value is its data-value, else its text.
 *
 *   const theme = segmented(el, { value: 'system', onChange: (v) => ... })
 *
 * Returns { get value, set value }. Setting it does not call onChange.
 */
export function segmented(group, { value, onChange = () => {} } = {}) {
  group.setAttribute('role', 'radiogroup');
  const segs = () => [...group.querySelectorAll('.lt-segment')];
  const valueOf = (s) => s.dataset.value ?? s.textContent.trim();
  for (const s of segs()) { s.setAttribute('role', 'radio'); s.type = 'button'; }

  let current;
  function set(v) {
    const list = segs();
    const on = list.find((s) => valueOf(s) === v) || list[0];
    for (const s of list) {
      s.setAttribute('aria-checked', String(s === on));
      s.tabIndex = s === on ? 0 : -1;
    }
    current = on ? valueOf(on) : v;
  }
  function choose(s, focus) {
    if (focus) s.focus();
    if (valueOf(s) === current) return;
    set(valueOf(s));
    onChange(current);
  }

  group.addEventListener('click', (ev) => {
    const s = ev.target.closest('.lt-segment');
    if (s && group.contains(s) && !s.disabled) choose(s);
  });
  group.addEventListener('keydown', (ev) => {
    const list = segs().filter((s) => !s.disabled);
    const at = list.indexOf(document.activeElement);
    if (at < 0) return;
    const d = { ArrowRight: 1, ArrowDown: 1, ArrowLeft: -1, ArrowUp: -1 }[ev.key];
    if (!d) return;
    ev.preventDefault();
    choose(list[(at + d + list.length) % list.length], true);
  });

  const marked = segs().find((s) => s.getAttribute('aria-checked') === 'true' || s.getAttribute('aria-pressed') === 'true');
  set(value ?? (marked ? valueOf(marked) : undefined));
  return {
    get value() { return current; },
    set value(v) { set(v); },
  };
}

// ---------------------------------------------------------- confirmAction ---

let confirmCount = 0;

/**
 * A two-step button for an act you cannot take back. The first click swaps
 * the button for "Are you sure?", a confirm and a cancel, with focus on the
 * confirm; it swaps back after `timeout` ms, on Cancel, or on Escape, and
 * focus returns to the button. The confirm runs onConfirm (it may return a
 * promise; both buttons wait for it).
 *
 *   confirmAction(deleteButton, {
 *     onConfirm: () => api.delete(id),
 *     confirmLabel: 'Delete', tone: 'danger',   // 'danger' | 'warning' | 'primary'
 *   })
 *
 * Returns { arm, disarm, destroy }.
 */
export function confirmAction(button, {
  onConfirm = () => {}, prompt = 'Are you sure?', confirmLabel = 'Confirm',
  cancelLabel = 'Cancel', tone = 'danger', timeout = 5000,
} = {}) {
  const id = `lt-confirm-${++confirmCount}`;
  const group = document.createElement('span');
  group.className = 'lt-confirm';
  group.hidden = true;
  const ask = document.createElement('span');
  ask.className = 'lt-confirm-prompt';
  ask.id = `${id}-prompt`;
  ask.textContent = prompt;
  const yes = document.createElement('button');
  yes.type = 'button';
  yes.className = `lt-btn lt-btn-sm lt-btn-${tone}`;
  yes.textContent = confirmLabel;
  yes.setAttribute('aria-describedby', ask.id);
  const no = document.createElement('button');
  no.type = 'button';
  no.className = 'lt-btn lt-btn-sm lt-btn-secondary';
  no.textContent = cancelLabel;
  group.append(ask, yes, no);
  button.after(group);

  let timer = 0;
  function arm() {
    button.hidden = true;
    group.hidden = false;
    yes.focus();
    clearTimeout(timer);
    timer = setTimeout(disarm, timeout);
  }
  function disarm() {
    clearTimeout(timer);
    const hadFocus = group.contains(document.activeElement);
    group.hidden = true;
    button.hidden = false;
    if (hadFocus) button.focus();
  }
  async function run() {
    clearTimeout(timer);
    yes.disabled = no.disabled = true;
    yes.textContent = '…';
    try { await onConfirm(); } finally {
      yes.disabled = no.disabled = false;
      yes.textContent = confirmLabel;
      disarm();
    }
  }
  const onKey = (ev) => { if (ev.key === 'Escape') { ev.stopPropagation(); disarm(); } };

  button.addEventListener('click', arm);
  yes.addEventListener('click', run);
  no.addEventListener('click', disarm);
  group.addEventListener('keydown', onKey);

  return {
    arm,
    disarm,
    destroy() {
      clearTimeout(timer);
      button.removeEventListener('click', arm);
      group.remove();
      button.hidden = false;
    },
  };
}

// ------------------------------------------------------------- inlineEdit ---

/**
 * A value you can edit where it stands. The root holds a [data-edit]
 * button and a [data-value] element that shows the value; this builds the
 * field (text, textarea, or select) and its Save and Cancel, swaps them in
 * on Edit with focus in the field, and back on save or cancel with focus
 * on Edit. Enter saves a text field, Mod+Enter a textarea, Escape cancels.
 * Saving an unchanged value just closes.
 *
 *   inlineEdit(root, {
 *     value: 'Lancaster', type: 'text',                 // 'text' | 'textarea' | 'select'
 *     options: [{ value, label }],                      // for a select
 *     onSave: async (v) => api.save(v),                 // throw to stay open
 *     onError: (err) => toast(err.message, { tone: 'error' }),
 *     empty: 'Not set', label: 'Name',                  // label names the field
 *     words: { save: 'Save', cancel: 'Cancel', saving: 'Saving…' },
 *   })
 *
 * Returns { edit, cancel, get value }.
 */
export function inlineEdit(root, {
  value = '', type = 'text', options = [], onSave = () => {},
  onError = (err) => toast(err?.message || 'Could not save', { tone: 'error' }),
  empty = 'Not set', label, words = {},
} = {}) {
  const say = { save: 'Save', cancel: 'Cancel', saving: 'Saving…', ...words };
  const editBtn = root.querySelector('[data-edit]');
  const view = root.querySelector('[data-value]');
  let current = value;

  const form = document.createElement('form');
  form.className = 'lt-inline-edit-form';
  form.hidden = true;
  let field;
  if (type === 'select') {
    field = document.createElement('select');
    for (const o of options) field.append(new Option(o.label ?? o.value, o.value));
  } else if (type === 'textarea') {
    field = document.createElement('textarea');
    field.rows = 3;
  } else {
    field = document.createElement('input');
    field.type = 'text';
  }
  field.className = 'lt-field';
  if (label) field.setAttribute('aria-label', label);
  const actions = document.createElement('div');
  actions.className = 'lt-actions';
  const save = document.createElement('button');
  save.type = 'submit';
  save.className = 'lt-btn lt-btn-sm lt-btn-primary';
  save.textContent = say.save;
  const cancelBtn = document.createElement('button');
  cancelBtn.type = 'button';
  cancelBtn.className = 'lt-btn lt-btn-sm lt-btn-secondary';
  cancelBtn.textContent = say.cancel;
  actions.append(save, cancelBtn);
  form.append(field, actions);
  view.after(form);

  function show() {
    const shown = type === 'select' ? (options.find((o) => o.value === current)?.label ?? current) : current;
    view.textContent = '';
    if (shown === '' || shown == null) {
      const none = document.createElement('span');
      none.className = 'lt-muted';
      none.textContent = empty;
      view.append(none);
    } else {
      view.textContent = shown;
    }
  }
  function edit() {
    field.value = current;
    view.hidden = true;
    editBtn.hidden = true;
    form.hidden = false;
    field.focus();
    if (type !== 'select') field.select();
  }
  function cancel() {
    form.hidden = true;
    view.hidden = false;
    editBtn.hidden = false;
    editBtn.focus();
  }
  async function submit(ev) {
    ev?.preventDefault();
    const next = field.value;
    if (next === current) { cancel(); return; }
    save.disabled = cancelBtn.disabled = true;
    save.textContent = say.saving;
    try {
      await onSave(next);
      current = next;
      show();
      cancel();
    } catch (err) {
      onError(err);
    } finally {
      save.disabled = cancelBtn.disabled = false;
      save.textContent = say.save;
    }
  }

  editBtn.addEventListener('click', edit);
  cancelBtn.addEventListener('click', cancel);
  form.addEventListener('submit', submit);
  form.addEventListener('keydown', (ev) => {
    if (ev.key === 'Escape') { ev.preventDefault(); ev.stopPropagation(); cancel(); }
    else if (ev.key === 'Enter' && type === 'textarea' && (ev.metaKey || ev.ctrlKey)) submit(ev);
  });
  show();

  return { edit, cancel, get value() { return current; } };
}

// ------------------------------------------------------------------- fold ---

/**
 * Hold a <details class="lt-fold"> open at and above a width, and let it
 * fold below it: a long settings page that folds on a phone and lies flat
 * on a desktop. While held open its summary does nothing. Below the width
 * it starts folded.
 *
 *   fold(details, { above: 641 })
 *
 * Returns { destroy }.
 */
export function fold(details, { above = 641 } = {}) {
  const wide = matchMedia(`(min-width: ${above}px)`);
  const summary = details.querySelector(':scope > summary');
  const sync = () => {
    details.open = wide.matches;
    details.toggleAttribute('data-held', wide.matches);
    if (summary) summary.tabIndex = wide.matches ? -1 : 0;
  };
  const hold = (ev) => { if (wide.matches) ev.preventDefault(); };
  summary?.addEventListener('click', hold);
  wide.addEventListener('change', sync);
  sync();
  return {
    destroy() {
      summary?.removeEventListener('click', hold);
      wide.removeEventListener('change', sync);
    },
  };
}

// ------------------------------------------------------------------- copy ---

/**
 * Put text on the clipboard and say so. Resolves true when it worked; on
 * failure (no permission, an old browser) the toast asks the person to
 * select it instead. Pass { toast: false } to say it yourself.
 */
export async function copy(text, { toast: announce = true, done = 'Copied', failed = 'Could not copy. Select it instead.' } = {}) {
  try {
    await navigator.clipboard.writeText(text);
    if (announce) toast(done);
    return true;
  } catch {
    if (announce) toast(failed, { tone: 'error' });
    return false;
  }
}
