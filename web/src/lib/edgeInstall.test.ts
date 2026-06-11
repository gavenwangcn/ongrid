import { describe, expect, it } from 'vitest';
import { edgeInstallOrigin, resolveEdgeInstallScheme } from './edgeInstall';

describe('resolveEdgeInstallScheme', () => {
  it('honours forced http', () => {
    expect(resolveEdgeInstallScheme('http', 'https:')).toBe('http');
  });

  it('honours forced https', () => {
    expect(resolveEdgeInstallScheme('https', 'http:')).toBe('https');
  });

  it('falls back to page protocol then http', () => {
    expect(resolveEdgeInstallScheme(undefined, 'https:')).toBe('https');
    expect(resolveEdgeInstallScheme(undefined, 'http:')).toBe('http');
    expect(resolveEdgeInstallScheme(undefined, undefined)).toBe('http');
  });
});

describe('edgeInstallOrigin', () => {
  it('builds origin without curl -k for http scheme', () => {
    const got = edgeInstallOrigin('156.254.6.224');
    expect(got.origin).toMatch(/^https?:\/\/156\.254\.6\.224$/);
    if (got.scheme === 'http') {
      expect(got.curlInsecure).toBe('');
    }
  });
});
