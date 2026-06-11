/** Edge curl-pipe install command scheme/origin helpers. */

export type EdgeInstallScheme = 'http' | 'https';

export type EdgeInstallOrigin = {
  origin: string;
  scheme: EdgeInstallScheme;
  /** curl -k prefix for self-signed HTTPS; empty for HTTP. */
  curlInsecure: string;
};

/**
 * Pick http vs https for the Devices install one-liner.
 *
 * Priority:
 *  1. forced build-time override (VITE_EDGE_INSTALL_SCHEME)
 *  2. browser page protocol
 *  3. http
 */
export function resolveEdgeInstallScheme(
  forced?: string,
  pageProtocol?: string,
): EdgeInstallScheme {
  if (forced === 'http' || forced === 'https') {
    return forced;
  }
  if (pageProtocol === 'https:') {
    return 'https';
  }
  return 'http';
}

/**
 * Resolve the manager origin embedded in the Devices install one-liner.
 */
export function edgeInstallOrigin(host: string): EdgeInstallOrigin {
  const forced = import.meta.env.VITE_EDGE_INSTALL_SCHEME as string | undefined;
  const pageProtocol =
    typeof window !== 'undefined' ? window.location.protocol : undefined;
  const scheme = resolveEdgeInstallScheme(forced, pageProtocol);
  return {
    origin: `${scheme}://${host}`,
    scheme,
    curlInsecure: scheme === 'https' ? '-k ' : '',
  };
}
