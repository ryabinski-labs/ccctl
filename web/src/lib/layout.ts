import type { SlotInfo } from './types';

export interface GridShape {
  cols: number;
  rows: number;
}

/** Grid for this many open panes: 1 full, 2 side by side, 3-4 as 2x2, 5-6 as 3x2. */
export function gridShape(open: number): GridShape {
  if (open <= 1) return { cols: 1, rows: 1 };
  if (open === 2) return { cols: 2, rows: 1 };
  if (open <= 4) return { cols: 2, rows: 2 };
  return { cols: 3, rows: 2 };
}

/**
 * Slot numbers (1-based) to show. Every open pane shows, and the lowest empty slot
 * shows as a "Start a session" card only when the grid has a spare cell for it, so
 * four sessions stay a 2x2 instead of growing a fifth, empty cell.
 */
export function visibleSlots(slots: (SlotInfo | null)[], maximized: number | null, canLaunch: boolean): number[] {
  if (maximized !== null) return slots[maximized - 1] ? [maximized] : [];
  const open = slots.filter(Boolean).length;
  const { cols, rows } = gridShape(open);
  const emptyAt = slots.findIndex((s) => !s);
  const out: number[] = [];
  slots.forEach((s, i) => {
    if (s || (i === emptyAt && canLaunch && open < cols * rows)) out.push(i + 1);
  });
  return out;
}
