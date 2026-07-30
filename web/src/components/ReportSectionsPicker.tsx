import { cn } from '@/lib/cn';
import { useI18n } from '@/i18n/locale';
import {
  ALL_REPORT_SECTIONS,
  DEFAULT_DAILY_SECTIONS,
  initialSectionsForKind,
  parseReportScope,
  REPORT_SECTION_DEFS,
  type ReportKind,
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
    <div>
      <div className="flex flex-wrap gap-2">
        {REPORT_SECTION_DEFS.map((def) => {
          const on = selected.has(def.key);
          return (
            <label
              key={def.key}
              title={tr(def.descZh, def.descEn)}
              className={cn(
                'inline-flex cursor-pointer items-center gap-1.5 rounded-md border px-2.5 py-1.5 text-xs transition-colors',
                on ? 'border-indigo-500/50 bg-indigo-500/10 text-indigo-200' : 'border-zinc-800 bg-zinc-950/40 text-zinc-300 hover:border-zinc-700',
              )}
            >
              <input
                type="checkbox"
                className="accent-indigo-500"
                checked={on}
                onChange={() => toggle(def.key)}
              />
              <span className="whitespace-nowrap font-medium">{tr(def.zh, def.en)}</span>
            </label>
          );
        })}
      </div>
      {value.length < ALL_REPORT_SECTIONS.length && (
        <p className="mt-1.5 text-[11px] text-zinc-600">
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

export function initialSectionsForScope(scopeJson?: string, kind: ReportKind = 'daily'): ReportSection[] {
  const parsed = parseReportScope(scopeJson);
  if (parsed.sections?.length) return parsed.sections;
  return initialSectionsForKind(kind);
}
