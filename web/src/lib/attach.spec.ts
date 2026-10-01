import { afterEach, describe, expect, it, vi } from 'vitest';
import { api } from './api';
import { filesOf, termPath, uploadForPaste } from './attach';

afterEach(() => vi.restoreAllMocks());

describe('termPath', () => {
  it('leaves plain paths alone', () => {
    expect(termPath('/Users/me/.ccctl/uploads/20261001-120000-ab12cd-shot.png')).toBe(
      '/Users/me/.ccctl/uploads/20261001-120000-ab12cd-shot.png',
    );
  });
  it('escapes spaces and shell characters', () => {
    expect(termPath("/Users/First Last/a (1)'.png")).toBe("/Users/First\\ Last/a\\ \\(1\\)\\'.png");
  });
});

describe('filesOf', () => {
  it('is empty for text-only data and for null', () => {
    expect(filesOf(null)).toEqual([]);
    expect(filesOf({ files: [] } as unknown as DataTransfer)).toEqual([]);
  });
  it('lists the files', () => {
    const f = new File(['x'], 'a.png');
    expect(filesOf({ files: [f] } as unknown as DataTransfer)).toEqual([f]);
  });
});

describe('uploadForPaste', () => {
  it('uploads in order and joins the escaped paths', async () => {
    const up = vi.spyOn(api, 'upload').mockResolvedValueOnce('/h/a b.png').mockResolvedValueOnce('/h/c.txt');
    const a = new File(['1'], 'a b.png');
    const c = new File(['2'], 'c.txt');
    expect(await uploadForPaste(2, [a, c])).toBe('/h/a\\ b.png /h/c.txt ');
    expect(up.mock.calls).toEqual([
      [2, a],
      [2, c],
    ]);
  });
  it('rejects when an upload fails, so nothing is pasted', async () => {
    vi.spyOn(api, 'upload').mockRejectedValue(new Error('nope'));
    await expect(uploadForPaste(1, [new File(['x'], 'a')])).rejects.toThrow('nope');
  });
});
