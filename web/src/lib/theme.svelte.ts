// Theme switcher. The inline script in app.html applies the chosen theme
// before first paint (no flash); this module just keeps a reactive copy of
// the choice and persists changes back to localStorage.

export type Theme = 'light' | 'dark';

const STORAGE_KEY = 'gograb-theme';

function readInitial(): Theme {
  if (typeof document === 'undefined') return 'dark';
  const ds = document.documentElement.dataset.theme;
  if (ds === 'light' || ds === 'dark') return ds;
  return 'dark';
}

class ThemeStore {
  current = $state<Theme>(readInitial());

  set(t: Theme) {
    this.current = t;
    if (typeof document !== 'undefined') {
      document.documentElement.dataset.theme = t;
      try {
        localStorage.setItem(STORAGE_KEY, t);
      } catch {
        // private mode etc — fine to ignore
      }
    }
  }

  toggle() {
    this.set(this.current === 'dark' ? 'light' : 'dark');
  }
}

export const theme = new ThemeStore();
