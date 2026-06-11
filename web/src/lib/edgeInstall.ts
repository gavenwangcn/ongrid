/** Edge curl-pipe install command scheme/origin helpers. */

export type EdgeInstallScheme = 'http' | 'https';

export type EdgeInstallOrigin = {
  origin: string;
  scheme: EdgeInstallScheme;
  /** curl -k prefix for self-signed HTTPS; empty for HTTP. */
  curlInsecure: string;
};

/**
 * Resolve the manager origin embedded in the Devices install one-liner.
 *
 * Priority:
 *  1. VITE_EDGE_INSTALL_SCHEME build-time override (deploy/.env)
 *  2. Current browser page protocol (http: → http, https: → https)
 */
export function edgeInstallOrigin(host: string): EdgeInstallOrigin {
  const forced = import.meta.env.VITE_EDGE_INSTALL_SCHEME as string | undefined;
  let scheme: EdgeInstallScheme;
  if (forced === 'http' || forced === 'https') {
    scheme = forced;
  } else if (typeof window !== 'undefined' && window.location.protocol === 'https:') {
    scheme = 'https';
  } else {
    scheme = 'http';
  }
  return {
    origin: `${scheme}://${host}`,
    scheme,
    curlInsecure: scheme === 'https' ? '-k ' : '',
  };
}
