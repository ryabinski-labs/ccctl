import { terminalBus } from './terminalBus';
import { useSessions } from '../stores/sessions';

/** Ctrl+Shift+1..4 focus pane, Ctrl+Shift+0 leave terminals, Ctrl+Shift+M maximize. Returns true if handled. */
export function handleShortcut(e: KeyboardEvent): boolean {
  if (!(e.ctrlKey && e.shiftKey)) return false;
  const store = useSessions();
  const m = /^Digit([0-4])$/.exec(e.code);
  if (m) {
    const n = Number(m[1]);
    if (n === 0) {
      document.querySelector<HTMLElement>('[data-header-tools] button:not([disabled])')?.focus();
    } else {
      if (store.maximized !== null && store.maximized !== n) store.maximized = null;
      if (!terminalBus.focus(n)) document.querySelector<HTMLElement>(`[data-slot="${n}"] button`)?.focus();
    }
    return true;
  }
  if (e.code === 'KeyM') {
    const slot = store.focusedSlot ?? store.maximized;
    if (slot && store.slots[slot - 1]) store.toggleMaximize(slot);
    requestAnimationFrame(() => slot && terminalBus.focus(slot));
    return true;
  }
  return false;
}
