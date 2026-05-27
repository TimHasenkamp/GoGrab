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
  pending: 'bg-warning/10 text-warning ring-warning/30',
  submitted: 'bg-success/10 text-success ring-success/30',
  retrieved: 'bg-muted text-muted-foreground ring-border',
  expired: 'bg-danger/10 text-danger ring-danger/30'
};

export const statusDot: Record<Status, string> = {
  pending: 'bg-warning',
  submitted: 'bg-success',
  retrieved: 'bg-muted-foreground',
  expired: 'bg-danger'
};
