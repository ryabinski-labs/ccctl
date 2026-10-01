import { afterEach, describe, expect, it, vi } from 'vitest';
import { copyText, parseOsc52 } from './clipboard';

const b64 = (s: string) => btoa(String.fromCharCode(...new TextEncoder().encode(s)));

describe('parseOsc52', () => {
  it('decodes utf-8 text', () => {
    expect(parseOsc52(`c;${b64('héllo\nworld')}`)).toBe('héllo\nworld');
  });
  it('accepts an empty target list', () => {
    expect(parseOsc52(`;${b64('x')}`)).toBe('x');
  });
  it('ignores queries, empty payloads and garbage', () => {
    expect(parseOsc52('c;?')).toBeNull();
    expect(parseOsc52('c;')).toBeNull();
    expect(parseOsc52('nonsense')).toBeNull();
    expect(parseOsc52('c;!!!')).toBeNull();
  });
});

describe('copyText', () => {
  afterEach(() => vi.restoreAllMocks());

  it('uses the async clipboard API in a secure context', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    vi.stubGlobal('isSecureContext', true);
    vi.stubGlobal('navigator', { clipboard: { writeText } });
    expect(await copyText('hi')).toBe(true);
    expect(writeText).toHaveBeenCalledWith('hi');
  });

  it('falls back to execCommand over plain http', async () => {
    vi.stubGlobal('isSecureContext', false);
    vi.stubGlobal('navigator', {});
    const exec = vi.fn().mockReturnValue(true);
    (document as unknown as { execCommand: typeof exec }).execCommand = exec;
    expect(await copyText('hi')).toBe(true);
    expect(exec).toHaveBeenCalledWith('copy');
    expect(document.querySelector('textarea')).toBeNull();
  });

  it('reports failure when the browser refuses', async () => {
    vi.stubGlobal('isSecureContext', false);
    vi.stubGlobal('navigator', {});
    (document as unknown as { execCommand: () => boolean }).execCommand = () => false;
    expect(await copyText('hi')).toBe(false);
  });
});
