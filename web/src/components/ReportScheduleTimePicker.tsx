import { useI18n } from '@/i18n/locale';
import { cn } from '@/lib/cn';
import {
  buildScheduleCron,
  type ReportKind,
  type ScheduleTimeConfig,
} from '@/api/reports';

const WEEKDAYS: { cron: number; zh: string; en: string }[] = [
  { cron: 1, zh: '周一', en: 'Mon' },
  { cron: 2, zh: '周二', en: 'Tue' },
  { cron: 3, zh: '周三', en: 'Wed' },
  { cron: 4, zh: '周四', en: 'Thu' },
  { cron: 5, zh: '周五', en: 'Fri' },
  { cron: 6, zh: '周六', en: 'Sat' },
  { cron: 0, zh: '周日', en: 'Sun' },
];

const inputCls =
  'rounded-md border border-zinc-700 bg-zinc-950 px-2 py-1.5 text-xs text-zinc-100 focus:border-zinc-600 focus:outline-none';

type Props = {
  kind: ReportKind;
  value: ScheduleTimeConfig;
  onChange(next: ScheduleTimeConfig): void;
  customCron?: string;
  onCustomCronChange?(next: string): void;
};

function toTimeValue(hour: number, minute: number): string {
  return `${String(hour).padStart(2, '0')}:${String(minute).padStart(2, '0')}`;
}

function fromTimeValue(raw: string, prev: ScheduleTimeConfig): ScheduleTimeConfig {
  const [h, m] = raw.split(':');
  const hour = Number(h);
  const minute = Number(m);
  return {
    ...prev,
    hour: Number.isFinite(hour) ? hour : prev.hour,
    minute: Number.isFinite(minute) ? minute : prev.minute,
  };
}

/** Fire time picker for scheduled reports (daily / weekly / monthly / custom cron). */
export function ReportScheduleTimePicker({ kind, value, onChange, customCron = '', onCustomCronChange }: Props) {
  const { tr } = useI18n();

  if (kind === 'custom') {
    return (
      <div className="space-y-1">
        <input
          value={customCron}
          onChange={(e) => onCustomCronChange?.(e.target.value)}
          placeholder="0 9 * * 1"
          className={cn(inputCls, 'w-full font-mono')}
        />
        <p className="text-[11px] text-zinc-600">
          {tr('5 段 cron：分 时 日 月 周', '5-field cron: minute hour day month weekday')}
        </p>
      </div>
    );
  }

  const preview = buildScheduleCron(kind, value);

  return (
    <div className="space-y-2">
      <div className="flex flex-wrap items-center gap-2">
        <label className="flex items-center gap-1.5 text-xs text-zinc-400">
          <span>{tr('执行时间', 'Run at')}</span>
          <input
            type="time"
            value={toTimeValue(value.hour, value.minute)}
            onChange={(e) => onChange(fromTimeValue(e.target.value, value))}
            className={cn(inputCls, 'font-mono')}
          />
        </label>

        {kind === 'weekly' && (
          <label className="flex items-center gap-1.5 text-xs text-zinc-400">
            <span>{tr('每周', 'On')}</span>
            <select
              value={value.weekday}
              onChange={(e) => onChange({ ...value, weekday: Number(e.target.value) })}
              className={cn(inputCls, 'text-sm')}
            >
              {WEEKDAYS.map((d) => (
                <option key={d.cron} value={d.cron}>
                  {tr(d.zh, d.en)}
                </option>
              ))}
            </select>
          </label>
        )}

        {kind === 'monthly' && (
          <label className="flex items-center gap-1.5 text-xs text-zinc-400">
            <span>{tr('每月', 'On day')}</span>
            <select
              value={value.dayOfMonth}
              onChange={(e) => onChange({ ...value, dayOfMonth: Number(e.target.value) })}
              className={cn(inputCls, 'text-sm')}
            >
              {Array.from({ length: 28 }, (_, i) => i + 1).map((d) => (
                <option key={d} value={d}>
                  {tr(`${d} 日`, `Day ${d}`)}
                </option>
              ))}
            </select>
          </label>
        )}
      </div>
      <p className="font-mono text-[11px] text-zinc-600">{preview}</p>
    </div>
  );
}
