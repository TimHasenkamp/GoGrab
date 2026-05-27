// Promise-based confirm dialog — replaces native confirm() so we don't lock
// the whole tab and can style the prompt to match the app.
//
//   const ok = await confirmStore.ask({
//     title: '...',
//     body: '...',
//     confirmLabel: 'Löschen',
//     destructive: true,
//   });
//   if (!ok) return;

interface ConfirmState {
  title: string;
  body: string;
  confirmLabel: string;
  cancelLabel: string;
  destructive: boolean;
  resolve: (value: boolean) => void;
}

class ConfirmStore {
  current = $state<ConfirmState | null>(null);

  ask(opts: {
    title: string;
    body: string;
    confirmLabel?: string;
    cancelLabel?: string;
    destructive?: boolean;
  }): Promise<boolean> {
    return new Promise((resolve) => {
      this.current = {
        title: opts.title,
        body: opts.body,
        confirmLabel: opts.confirmLabel ?? 'Bestätigen',
        cancelLabel: opts.cancelLabel ?? 'Abbrechen',
        destructive: opts.destructive ?? false,
        resolve
      };
    });
  }

  resolve(value: boolean) {
    const c = this.current;
    if (!c) return;
    this.current = null;
    c.resolve(value);
  }
}

export const confirmStore = new ConfirmStore();
