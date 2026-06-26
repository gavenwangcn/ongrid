// Hosted-page (serve_page artifact) client. The agent / workflows host
// generated HTML at /api/pages/<token>; these authed routes list + delete them
// for the operations UI. Page CONTENT is served publicly by token, not here.
import { request } from './client';

export type HostedPage = {
  id: string;
  title: string;
  created_at: string;
  url: string; // /api/pages/<id>
  size_bytes?: number;
};

export function listPages() {
  return request<{ items: HostedPage[]; total: number }>('GET', '/pages');
}

export function deletePage(id: string) {
  return request<void>('DELETE', `/pages/${encodeURIComponent(id)}`);
}
