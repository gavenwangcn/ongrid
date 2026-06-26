// Pages — operations view for hosted "artifacts": the HTML pages the agent /
// workflows generate via serve_page. List / open / preview / delete. Page
// content is served publicly by token at /api/pages/<id>; previews render in a
// sandboxed iframe (scripts disabled, opaque origin) so an LLM-generated page
// can never touch the SPA's session.
import { useCallback, useEffect, useState } from 'react';
import { ExternalLink, Eye, FileText, Loader2, Search, Trash2 } from 'lucide-react';

import { deletePage, listPages, type HostedPage } from '@/api/pages';
import { useI18n } from '@/i18n/locale';
import { useAuth } from '@/store/auth';
import { PageHeader, Button, EmptyState } from '@/components/ui';
import { Modal } from '@/components/Modal';

export default function PagesPage() {
  const { tr } = useI18n();
  const role = useAuth((s) => s.role);
  const canWrite = role !== 'viewer';

  const [items, setItems] = useState<HostedPage[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [search, setSearch] = useState('');
  const [busyId, setBusyId] = useState<string | null>(null);
  const [preview, setPreview] = useState<HostedPage | null>(null);

  const refresh = useCallback(async () => {
    try {
      const r = await listPages();
      setItems(r.items ?? []);
      setError('');
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const onDelete = async (p: HostedPage) => {
    if (!window.confirm(tr(`删除页面「${p.title || p.id}」？链接将立即失效。`, `Delete page "${p.title || p.id}"? Its link dies immediately.`))) return;
    setBusyId(p.id);
    try {
      await deletePage(p.id);
      await refresh();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusyId(null);
    }
  };

  const relTime = (iso: string) => {
    if (!iso) return '—';
    const sec = Math.max(0, (Date.now() - new Date(iso).getTime()) / 1000);
    if (sec < 60) return tr('刚刚', 'just now');
    const m = Math.floor(sec / 60);
    if (m < 60) return tr(`${m} 分钟前`, `${m}m ago`);
    const h = Math.floor(m / 60);
    if (h < 24) return tr(`${h} 小时前`, `${h}h ago`);
    const d = Math.floor(h / 24);
    if (d < 30) return tr(`${d} 天前`, `${d}d ago`);
    return new Date(iso).toLocaleDateString();
  };

  const shown = items.filter((p) => {
    const q = search.trim().toLowerCase();
    if (!q) return true;
    return (p.title ?? '').toLowerCase().includes(q) || p.id.includes(q);
  });

  return (
    <main className="anim-fade flex flex-1 flex-col overflow-hidden">
      <PageHeader
        title={tr('产物', 'Artifacts')}
        subtitle={tr(
          `Agent 与工作流通过 serve_page 生成的网页 · 共 ${items.length} 个`,
          `Web pages the agent & workflows generate via serve_page · ${items.length} total`,
        )}
      />
      {items.length > 0 && (
        <div className="border-b border-zinc-800 px-6 py-3">
          <div className="flex flex-wrap items-center gap-3">
            <label className="relative block w-64">
              <span className="sr-only">{tr('搜索', 'Search')}</span>
              <Search size={12} className="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-zinc-500" />
              <input
                type="search"
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                placeholder={tr('搜索页面…', 'Search pages…')}
                className="w-full rounded-md border border-zinc-800 bg-zinc-950/40 py-1.5 pl-8 pr-2 text-xs text-zinc-200 placeholder:text-zinc-500 focus:border-zinc-600 focus:outline-none"
              />
            </label>
            <span className="ml-auto text-xs text-zinc-500">
              {tr(`${items.length} 个 · 匹配 ${shown.length}`, `${items.length} total · ${shown.length} matched`)}
            </span>
          </div>
        </div>
      )}

      <div className="flex-1 overflow-y-auto px-6 py-4">
        {error && (
          <div className="mb-4 rounded-md border border-red-900/50 bg-red-950/30 px-3 py-2 text-xs text-red-400">{error}</div>
        )}
        {loading ? (
          <div className="py-16 text-center text-xs text-zinc-500">{tr('加载中…', 'Loading…')}</div>
        ) : items.length === 0 ? (
          <EmptyState
            title={tr('还没有生成的页面', 'No generated pages yet')}
            hint={tr('让工作流或助理用 serve_page 生成一个网页报告，它会出现在这里。', 'Have a workflow or the assistant generate a web report via serve_page — it shows up here.')}
            className="flex flex-col items-center gap-2 py-20 text-center"
          />
        ) : shown.length === 0 ? (
          <div className="py-16 text-center text-xs text-zinc-500">{tr('无匹配的页面', 'No matching pages')}</div>
        ) : (
          <div className="space-y-2">
            {shown.map((p) => (
              <div
                key={p.id}
                className="flex items-center gap-3 rounded-lg border border-zinc-800 bg-zinc-900/40 px-4 py-3 transition-colors hover:border-zinc-700"
              >
                <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg border border-indigo-500/30 bg-indigo-500/10 text-indigo-400">
                  <FileText size={15} />
                </div>
                <div className="min-w-0 flex-1">
                  <div className="truncate text-[13px] font-medium text-zinc-200">{p.title || tr('（未命名页面）', '(untitled page)')}</div>
                  <div className="mt-0.5 flex items-center gap-2 text-[11px] text-zinc-500">
                    <span>{relTime(p.created_at)}</span>
                    <span className="text-zinc-700">·</span>
                    <span className="truncate font-mono">{p.url}</span>
                  </div>
                </div>
                <div className="flex shrink-0 items-center gap-1.5">
                  <Button variant="ghost" onClick={() => setPreview(p)} className="whitespace-nowrap">
                    <Eye size={13} /> {tr('预览', 'Preview')}
                  </Button>
                  <a
                    href={p.url}
                    target="_blank"
                    rel="noreferrer"
                    className="inline-flex items-center gap-1 rounded-md px-2.5 py-1.5 text-xs text-zinc-400 transition-colors hover:bg-zinc-800 hover:text-zinc-200"
                  >
                    <ExternalLink size={13} /> {tr('打开', 'Open')}
                  </a>
                  {canWrite && (
                    <Button variant="danger" onClick={() => void onDelete(p)} disabled={busyId === p.id} className="whitespace-nowrap">
                      {busyId === p.id ? <Loader2 size={13} className="animate-spin" /> : <Trash2 size={13} />}
                    </Button>
                  )}
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {preview && (
        <Modal open onClose={() => setPreview(null)} size="lg" title={preview.title || tr('页面预览', 'Page preview')}>
          <div className="space-y-2">
            <div className="flex items-center gap-2 text-[11px] text-zinc-500">
              <span className="truncate font-mono">{preview.url}</span>
              <a href={preview.url} target="_blank" rel="noreferrer" className="ml-auto inline-flex shrink-0 items-center gap-1 text-indigo-400 hover:text-indigo-300">
                <ExternalLink size={11} /> {tr('新标签打开', 'Open in new tab')}
              </a>
            </div>
            {/* Sandboxed: empty sandbox = no scripts, opaque origin. Belt to the
                server-side CSP sandbox so an LLM page can't touch the session. */}
            <iframe
              title={preview.title || 'page'}
              src={preview.url}
              sandbox=""
              className="h-[60vh] w-full rounded-md border border-zinc-800 bg-white"
            />
          </div>
        </Modal>
      )}
    </main>
  );
}
