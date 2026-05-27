// In-memory toast notifications. Replace native alert() — non-blocking,
// auto-dismissed, stackable. Mounted globally via <Toaster /> in the admin
// layout so any code in the admin SPA can call toast.success('…') etc.

export type ToastKind = 'success' | 'error' | 'info' | 'warning';

export interface ToastItem {
  id: number;
  kind: ToastKind;
  text: string;
  /** When > 0, ms until auto-dismiss. 0 means sticky. */
  durationMs: number;
}

class Toasts {
  items = $state<ToastItem[]>([]);
  private nextId = 1;

  push(text: string, kind: ToastKind = 'info', durationMs = 4000) {
    const id = this.nextId++;
    this.items = [...this.items, { id, kind, text, durationMs }];
    if (durationMs > 0) {
      setTimeout(() => this.dismiss(id), durationMs);
    }
    return id;
  }

  dismiss(id: number) {
    this.items = this.items.filter((t) => t.id !== id);
  }

  success(text: string, durationMs = 3500) { this.push(text, 'success', durationMs); }
  error(text: string, durationMs = 6000)   { this.push(text, 'error', durationMs); }
  info(text: string, durationMs = 4000)    { this.push(text, 'info', durationMs); }
  warning(text: string, durationMs = 5000) { this.push(text, 'warning', durationMs); }
}

export const toast = new Toasts();
