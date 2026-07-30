import { cn } from '@/lib/cn';
import { useI18n } from '@/i18n/locale';
import {
  ALL_REPORT_SECTIONS,
  DEFAULT_DAILY_SECTIONS,
  parseReportScope,
  REPORT_SECTION_DEFS,
  type ReportSection,
} from '@/api/reports';

type Props = {
  value: ReportSection[];
  onChange(next: ReportSection[]): void;
};

export function ReportSectionsPicker({ value, onChange }: Props) {
  const { tr } = useI18n();
  const selected = new Set(value);

  const toggle = (key: ReportSection) => {
    if (selected.has(key)) {
      if (selected.size <= 1) return;
      onChange(value.filter((s) => s !== key));
      return;
    }
    onChange([...value, key]);
  };

  return (
    <div className="space-y-1.5">
      {REPORT_SECTION_DEFS.map((def) => {
        const on = selected.has(def.key);
        return (
          <label
            key={def.key}
            className={cn(
              'flex cursor-pointer items-start gap-2 rounded-md border px-2.5 py-2 transition-colors',
              on ? 'border-indigo-500/50 bg-indigo-500/10' : 'border-zinc-800 bg-zinc-950/40 hover:border-zinc-700',
            )}
          >
            <input
              type="checkbox"
              className="mt-0.5 accent-indigo-500"
              checked={on}
              onChange={() => toggle(def.key)}
            />
            <span className="min-w-0">
              <span className="block text-xs font-medium text-zinc-200">
                {tr(def.zh, def.en)}
              </span>
              <span className="block text-[11px] text-zinc-500">
                {tr(def.descZh, def.descEn)}
              </span>
            </span>
          </label>
        );
      })}
      {value.length < ALL_REPORT_SECTIONS.length && (
        <p className="text-[11px] text-zinc-600">
          {tr('未选模块不会采集数据、也不会出现在报告中。', 'Unselected modules are skipped during generation and hidden in the report.')}
        </p>
      )}
    </div>
  );
}

export function initialDailySections(scopeJson?: string): ReportSection[] {
  const parsed = parseReportScope(scopeJson);
  if (parsed.sections?.length) return parsed.sections;
  return [...DEFAULT_DAILY_SECTIONS];
}
