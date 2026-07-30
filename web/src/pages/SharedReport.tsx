import { useCallback, useEffect, useState } from 'react';
import { useParams } from 'react-router-dom';
import { Download, RefreshCw } from 'lucide-react';
import { OngridLogo } from '@/components/OngridLogo';
import { ReportContentView } from '@/components/ReportContent';
import { useI18n } from '@/i18n/locale';
import { fullDateTime } from '@/lib/format';
import { getSharedReport, type ReportDetail } from '@/api/reports';

/** Public, login-free report viewer at /r/:token (share link). */
export default function SharedReportPage() {
  const { token = '' } = useParams();
  const { tr } = useI18n();
  const [report, setReport] = useState<ReportDetail | null>(null);
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState<string | null>(null);

  const load = useCallback(async () => {
    if (!token) {
      setErr(tr('链接无效', 'Invalid link'));
      setLoading(false);
      return;
    }
    try {
      setErr(null);
      const r = await getSharedReport(token);
      setReport(r);
    } catch (e) {
      setReport(null);
      setErr((e as Error)?.message || tr('报告不存在或链接已过期', 'Report not found or link expired'));
    } finally {
      setLoading(false);
    }
  }, [token, tr]);

  useEffect(() => {
    void load();
  }, [load]);

  return (
    <div className="report-print-area flex min-h-screen flex-col bg-zinc-950 text-zinc-100">
      <header className="border-b border-zinc-800/60 px-6 py-4 print:hidden">
        <div className="mx-auto flex max-w-4xl items-center gap-2">
          <OngridLogo className="h-5 w-5 shrink-0 opacity-80" />
          <span className="text-xs text-zinc-500">{tr('Ongrid 报告分享', 'Ongrid shared report')}</span>
        </div>
      </header>

      <main className="flex-1 px-6 py-5">
        <div className="mx-auto max-w-4xl">
          {loading ? (
            <div className="py-20 text-center text-sm text-zinc-500">
              <RefreshCw size={18} className="mx-auto mb-2 animate-spin" />
              {tr('加载中…', 'Loading…')}
            </div>
          ) : err ? (
            <div className="rounded-lg border border-zinc-800/60 bg-zinc-900/40 p-6 text-center text-sm text-zinc-400">
              {err}
            </div>
          ) : report ? (
            <>
              <div className="mb-6 flex items-start justify-between gap-4">
                <div className="min-w-0">
                  <h1 className="text-base font-semibold text-zinc-100">{report.title}</h1>
                  <div className="mt-1.5 text-[11px] text-zinc-500">
                    {report.generated_at
                      ? tr(`生成于 ${fullDateTime(report.generated_at)}`, `Generated ${fullDateTime(report.generated_at)}`)
                      : tr('生成中…', 'Generating…')}
                    {report.timezone ? ` · ${report.timezone}` : ''}
                  </div>
                </div>
                {report.status === 'ready' && (
                  <button
                    type="button"
                    onClick={() => window.print()}
                    className="inline-flex shrink-0 items-center gap-1.5 rounded-md border border-zinc-700 bg-zinc-900 px-2.5 py-1.5 text-xs text-zinc-300 hover:bg-zinc-800 print:hidden"
                  >
                    <Download size={12} /> {tr('导出 PDF', 'Export PDF')}
                  </button>
                )}
              </div>

              {report.status === 'failed' ? (
                <div className="rounded-lg border border-red-700/40 bg-red-900/20 p-4 text-sm text-red-200">
                  <div className="font-medium">{tr('报告生成失败', 'Report generation failed')}</div>
                  {report.error_msg && <div className="mt-1 text-xs text-red-300/80">{report.error_msg}</div>}
                </div>
              ) : report.status !== 'ready' || !report.content ? (
                <div className="py-12 text-center text-sm text-zinc-500">
                  <RefreshCw size={18} className="mx-auto mb-2 animate-spin" />
                  {tr('报告生成中，请稍候…', 'Report is being generated…')}
                </div>
              ) : (
                <ReportContentView content={report.content} />
              )}
            </>
          ) : null}
        </div>
      </main>
    </div>
  );
}
