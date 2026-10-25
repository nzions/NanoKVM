import { useSyncExternalStore } from 'react';

import { getThemeMode, setThemeMode, type ThemeMode } from './localstorage.ts';

export type ResolvedTheme = 'dark' | 'light';

const query = window.matchMedia('(prefers-color-scheme: dark)');
const listeners = new Set<() => void>();

let mode: ThemeMode = getThemeMode();

function resolve(m: ThemeMode): ResolvedTheme {
  if (m === 'system') return query.matches ? 'dark' : 'light';
  return m;
}

// Colour variables (index.css) follow the `light` class on <html>; the dark
// palette is the default.
function applyToDocument() {
  const root = document.documentElement;
  const resolved = resolve(mode);
  root.classList.toggle('light', resolved === 'light');
  root.classList.toggle('dark', resolved === 'dark');
  root.style.colorScheme = resolved;
}

function emit() {
  applyToDocument();
  listeners.forEach((l) => l());
}

query.addEventListener('change', () => {
  if (mode === 'system') emit();
});

applyToDocument();

function subscribe(listener: () => void) {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

export function changeThemeMode(next: ThemeMode) {
  mode = next;
  setThemeMode(next);
  emit();
}

export function useThemeMode(): ThemeMode {
  return useSyncExternalStore(subscribe, () => mode);
}

export function useResolvedTheme(): ResolvedTheme {
  return useSyncExternalStore(subscribe, () => resolve(mode));
}
