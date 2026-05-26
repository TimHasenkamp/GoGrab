// Cryptographically random password generator. Uses crypto.getRandomValues
// with rejection sampling to avoid modulo bias on the byte → charset mapping.

const LOWER = 'abcdefghijkmnpqrstuvwxyz';        // omit 'l' and 'o' for legibility
const UPPER = 'ABCDEFGHJKLMNPQRSTUVWXYZ';        // omit 'I' and 'O'
const DIGITS = '23456789';                        // omit 0 and 1
const SYMBOLS = '!@#$%^&*-_=+';                   // keyboard-easy, no quotes/backticks

export interface PwOptions {
  length: number;
  symbols: boolean;
}

export function buildCharset(opts: PwOptions): string {
  let s = LOWER + UPPER + DIGITS;
  if (opts.symbols) s += SYMBOLS;
  return s;
}

export function generate(opts: PwOptions): string {
  const charset = buildCharset(opts);
  const setLen = charset.length;
  const maxValid = Math.floor(256 / setLen) * setLen;
  const out: string[] = [];

  while (out.length < opts.length) {
    // Generate twice the needed bytes to reduce reroll loops.
    const buf = new Uint8Array(Math.max(16, (opts.length - out.length) * 2));
    crypto.getRandomValues(buf);
    for (const b of buf) {
      if (b < maxValid) {
        out.push(charset[b % setLen]!);
        if (out.length >= opts.length) break;
      }
    }
  }
  return out.join('');
}

/** Approximate entropy in bits for the given options. */
export function entropyBits(opts: PwOptions): number {
  const setLen = buildCharset(opts).length;
  return Math.round(Math.log2(setLen) * opts.length);
}

export type Strength = 'schwach' | 'ok' | 'stark' | 'sehr stark';

export function strengthLabel(bits: number): Strength {
  if (bits < 60) return 'schwach';
  if (bits < 90) return 'ok';
  if (bits < 128) return 'stark';
  return 'sehr stark';
}

export function strengthColor(bits: number): string {
  if (bits < 60) return '#dc2626';
  if (bits < 90) return '#d97706';
  if (bits < 128) return '#16a34a';
  return '#059669';
}
