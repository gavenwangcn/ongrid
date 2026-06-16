import {
  ENVIRONMENT_TAGS,
  ENVIRONMENT_TAG_LABELS,
  ENVIRONMENT_TAG_LABELS_EN,
  type EnvironmentTag,
} from '@/api/environment';
import { useI18n } from '@/i18n/locale';

export type EnvironmentFilterValue = '' | EnvironmentTag;

type Variant = 'chip' | 'block';

export function EnvironmentSelect({
  value,
  onChange,
  className,
  showLabel = true,
  variant = 'chip',
}: {
  value: EnvironmentFilterValue;
  onChange(value: EnvironmentFilterValue): void;
  className?: string;
  showLabel?: boolean;
  variant?: Variant;
}) {
  const { tr } = useI18n();
  const options: { value: EnvironmentFilterValue; label: string }[] = [
    { value: '', label: tr('全部环境', 'All environments') },
    ...ENVIRONMENT_TAGS.map((tag) => ({
      value: tag,
      label: tr(ENVIRONMENT_TAG_LABELS[tag], ENVIRONMENT_TAG_LABELS_EN[tag]),
    })),
  ];

  if (variant === 'block') {
    return (
      <label className={'block ' + (className ?? '')}>
        {showLabel && <span className="mb-1 block text-[11px] text-zinc-500">{tr('环境', 'Environment')}</span>}
        <select
          value={value}
          onChange={(e) => onChange(e.target.value as EnvironmentFilterValue)}
          className="w-full rounded-md border border-zinc-800 bg-zinc-950 px-2 py-1.5 text-xs text-zinc-100 focus:border-zinc-600 focus:outline-none"
        >
          {options.map((o) => (
            <option key={o.value || 'all'} value={o.value} className="bg-zinc-900">
              {o.label}
            </option>
          ))}
        </select>
      </label>
    );
  }

  return (
    <label
      className={
        'inline-flex items-center gap-1 rounded-md border border-zinc-800/60 bg-zinc-950/40 pl-2 pr-1 py-1 text-zinc-300 hover:border-zinc-700 ' +
        (className ?? '')
      }
    >
      {showLabel && <span className="text-[11px] text-zinc-500">{tr('环境', 'Environment')}</span>}
      <select
        value={value}
        onChange={(e) => onChange(e.target.value as EnvironmentFilterValue)}
        className="appearance-none border-none bg-transparent pl-1 pr-4 text-[12px] text-zinc-100 focus:outline-none"
      >
        {options.map((o) => (
          <option key={o.value || 'all'} value={o.value} className="bg-zinc-900">
            {o.label}
          </option>
        ))}
      </select>
    </label>
  );
}
