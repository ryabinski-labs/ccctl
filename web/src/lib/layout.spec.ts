import { describe, expect, it } from 'vitest';
import { gridShape, visibleSlots } from './layout';
import type { SlotInfo } from './types';

const s = (n: number) => ({ slot: n, task: `t${n}`, state: 'running' }) as unknown as SlotInfo;
const slotsOf = (...open: number[]) => Array.from({ length: 6 }, (_, i) => (open.includes(i + 1) ? s(i + 1) : null));

describe('gridShape', () => {
  it.each([
    [0, 1, 1],
    [1, 1, 1],
    [2, 2, 1],
    [3, 2, 2],
    [4, 2, 2],
    [5, 3, 2],
    [6, 3, 2],
  ])('%i open panes -> %i cols x %i rows', (n, cols, rows) => {
    expect(gridShape(n)).toEqual({ cols, rows });
  });
});

describe('visibleSlots', () => {
  it('shows one start card when nothing is open', () => {
    expect(visibleSlots(slotsOf(), null, true)).toEqual([1]);
  });
  it('shows open panes only when the grid is full', () => {
    expect(visibleSlots(slotsOf(1), null, true)).toEqual([1]);
    expect(visibleSlots(slotsOf(1, 2), null, true)).toEqual([1, 2]);
    expect(visibleSlots(slotsOf(1, 2, 3, 4), null, true)).toEqual([1, 2, 3, 4]);
    expect(visibleSlots(slotsOf(1, 2, 3, 4, 5, 6), null, true)).toEqual([1, 2, 3, 4, 5, 6]);
  });
  it('fills the spare cell with the start card', () => {
    expect(visibleSlots(slotsOf(1, 2, 3), null, true)).toEqual([1, 2, 3, 4]);
    expect(visibleSlots(slotsOf(1, 2, 3, 4, 5), null, true)).toEqual([1, 2, 3, 4, 5, 6]);
  });
  it('puts the start card at the lowest empty slot, between open panes', () => {
    expect(visibleSlots(slotsOf(1, 3, 4), null, true)).toEqual([1, 2, 3, 4]);
  });
  it('hides the start card when launching is not possible', () => {
    expect(visibleSlots(slotsOf(1, 2, 3), null, false)).toEqual([1, 2, 3]);
  });
  it('shows only the maximized pane', () => {
    expect(visibleSlots(slotsOf(1, 2, 3), 2, true)).toEqual([2]);
  });
});
