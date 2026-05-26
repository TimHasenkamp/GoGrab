// Shared types for operator-defined customer forms.

export type FieldType = 'text' | 'password' | 'textarea';

export interface FormField {
  id: string;
  label: string;
  type: FieldType;
  placeholder?: string;
}

export const fieldTypeLabel: Record<FieldType, string> = {
  text: 'Text',
  password: 'Passwort',
  textarea: 'Mehrzeilig'
};

export const MAX_FIELDS = 10;
export const MAX_LABEL = 80;
export const MAX_PLACEHOLDER = 120;

/** Derive a stable, lowercase field id from a free-form label. Falls back to
 * a numeric suffix to guarantee uniqueness against `existing`. */
export function deriveFieldId(label: string, existing: ReadonlySet<string>): string {
  let base = label
    .toLowerCase()
    .replace(/ä/g, 'ae').replace(/ö/g, 'oe').replace(/ü/g, 'ue').replace(/ß/g, 'ss')
    .replace(/[^a-z0-9]+/g, '_')
    .replace(/^_+|_+$/g, '')
    .slice(0, 24);
  if (!base) base = 'field';
  if (!existing.has(base)) return base;
  for (let i = 2; i < 999; i++) {
    const cand = `${base}_${i}`;
    if (!existing.has(cand)) return cand;
  }
  return `${base}_${Date.now()}`;
}

/** Single-field default used when the operator doesn't customise the form. */
export function defaultSchema(): FormField[] {
  return [{ id: 'secret', label: 'Geheimnis', type: 'textarea' }];
}
