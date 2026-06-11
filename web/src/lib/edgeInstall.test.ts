import { describe, expect, it } from 'vitest';
import { edgeInstallOrigin } from './edgeInstall';

describe('edgeInstallOrigin', () => {
  it('uses forced http scheme from env', () => {
    const prev = import.meta.env.VITE_EDGE_INSTALL_SCHEME;
    import.meta.env.VITE_EDGE_INSTALL_SCHEME = 'http';
    try {
      const got = edgeInstallOrigin('156.254.6.224');
      expect(got.origin).toBe('http://156.254.6.224');
      expect(got.scheme).toBe('http');
      expect(got.curlInsecure).toBe('');
    } finally {
      import.meta.env.VITE_EDGE_INSTALL_SCHEME = prev;
    }
  });

  it('uses forced https scheme with curl -k', () => {
    const prev = import.meta.env.VITE_EDGE_INSTALL_SCHEME;
    import.meta.env.VITE_EDGE_INSTALL_SCHEME = 'https';
    try {
      const got = edgeInstallOrigin('ops.example.com:8443');
      expect(got.origin).toBe('https://ops.example.com:8443');
      expect(got.curlInsecure).toBe('-k ');
    } finally {
      import.meta.env.VITE_EDGE_INSTALL_SCHEME = prev;
    }
  });
});
