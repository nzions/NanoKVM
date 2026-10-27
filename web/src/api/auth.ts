import { http } from '@/lib/http';

// The one account is root, always an administrator.
export type Account = {
  username: string;
  role: 'admin';
};

export function login(username: string, password: string) {
  const data = {
    username,
    password
  };
  return http.post('/api/auth/login', data);
}

export function logout() {
  return http.post('/api/auth/logout');
}

export function getAccount() {
  return http.get('/api/auth/account');
}

export function changePassword(currentPassword: string, password: string) {
  return http.post('/api/auth/password', { currentPassword, password });
}

export function isPasswordUpdated() {
  return http.get('/api/auth/password');
}

// Unauthenticated: { password_set: boolean }. False only on a device that has no
// password yet (booted without /boot/root.pass, or button-reset).
export function getAuthState() {
  return http.get('/api/auth/state');
}

// First-connection setup: accepted once, only while the password is unset; on
// success the response carries the session cookie, as a login does.
export function setupPassword(password: string) {
  return http.post('/api/auth/setup', { password });
}
