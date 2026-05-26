// German-locale relative time + status helpers used by the admin UI.

const rtf = new Intl.RelativeTimeFormat('de', { numeric: 'auto' });

const UNITS: { unit: Intl.RelativeTimeFormatUnit; ms: number }[] = [
  { unit: 'year', ms: 365 * 24 * 60 * 60 * 1000 },
  { unit: 'month', ms: 30 * 24 * 60 * 60 * 1000 },
  { unit: 'day', ms: 24 * 60 * 60 * 1000 },
  { unit: 'hour', ms: 60 * 60 * 1000 },
  { unit: 'minute', ms: 60 * 1000 }
];

export function relativeTime(iso: string): string {
  const diff = new Date(iso).getTime() - Date.now();
  for (const { unit, ms } of UNITS) {
    if (Math.abs(diff) >= ms) {
      return rtf.format(Math.round(diff / ms), unit);
    }
  }
  return rtf.format(Math.round(diff / 1000), 'second');
}

export function absoluteTime(iso: string): string {
  return new Date(iso).toLocaleString('de-DE', {
    dateStyle: 'medium',
    timeStyle: 'short'
  });
}

export type Status = 'pending' | 'submitted' | 'retrieved' | 'expired';

export const statusLabel: Record<Status, string> = {
  pending: 'Wartet',
  submitted: 'Eingereicht',
  retrieved: 'Abgerufen',
  expired: 'Abgelaufen'
};

export const statusBadge: Record<Status, string> = {
  pending: 'bg-amber-50 text-amber-800 ring-amber-200',
  submitted: 'bg-emerald-50 text-emerald-800 ring-emerald-200',
  retrieved: 'bg-slate-100 text-slate-700 ring-slate-200',
  expired: 'bg-rose-50 text-rose-800 ring-rose-200'
};

export const statusDot: Record<Status, string> = {
  pending: 'bg-amber-500',
  submitted: 'bg-emerald-500',
  retrieved: 'bg-slate-400',
  expired: 'bg-rose-500'
};
