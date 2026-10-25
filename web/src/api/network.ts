import { http } from '@/lib/http.ts';

export type DNSMode = 'manual' | 'dhcp';

// wake on lan
export function wol(mac: string) {
  const data = {
    mac
  };
  return http.post('/api/network/wol', data);
}

// get wake-on-lan macs history
export function getWolMacs() {
  return http.get('/api/network/wol/mac');
}

export function deleteWolMac(mac: string) {
  const data = {
    mac
  };
  return http.delete('/api/network/wol/mac', data);
}

// set Mac name
export function setWolMacName(mac: string, name: string) {
  return http.post('/api/network/wol/mac/name', { mac, name });
}

// get wifi information
export function getWiFi() {
  return http.get('/api/network/wifi');
}

// connect wifi without auth (only available in wifi configuration mode)
export function connectWifiNoAuth(ssid: string, password: string, apPassword?: string) {
  const data = {
    ssid,
    password
  };
  return http.post('/api/network/wifi', data, {
    headers: {
      'X-AP-Key': apPassword || ''
    }
  });
}

// verify ap login
export function verifyApLogin(apPassword: string) {
  return http.post(
    '/api/network/wifi/verify',
    {},
    {
      headers: {
        'X-AP-Key': apPassword || ''
      }
    }
  );
}

// connect wifi
export function connectWifi(ssid: string, password: string) {
  const data = {
    ssid,
    password
  };
  return http.post('/api/network/wifi/connect', data);
}

// disconnect wifi
export function disconnectWifi() {
  return http.post('/api/network/wifi/disconnect');
}

export function getDNS() {
  return http.get('/api/network/dns');
}

export function setDNS(mode: DNSMode, servers: string[]) {
  return http.post('/api/network/dns', { mode, servers });
}

export type InterfaceName = 'eth0' | 'wlan0';
export type InterfaceKind = 'ethernet' | 'wifi';
export type IPMode = 'dhcp' | 'static';

export type InterfaceCurrent = {
  addresses: string[];
  gateway: string;
  mac: string;
};

export type InterfacePending = {
  deadline_unix: number;
  now_unix: number;
  mode: IPMode;
  address: string;
  gateway: string;
};

export type NetworkInterface = {
  name: InterfaceName;
  kind: InterfaceKind;
  present: boolean;
  mode: IPMode;
  address: string;
  gateway: string;
  current: InterfaceCurrent;
  ssid?: string;
  pending: InterfacePending | null;
};

export type SetInterfaceReq = {
  mode: IPMode;
  address: string;
  gateway: string;
};

// list eth0 and wlan0 with their IP settings
export function getInterfaces() {
  return http.get('/api/network/interfaces');
}

// change one interface; it reverts after 120 s unless confirmed
export function setInterface(name: InterfaceName, req: SetInterfaceReq) {
  return http.post(`/api/network/interfaces/${name}`, req);
}

// keep the pending change
export function confirmInterface(name: InterfaceName) {
  return http.post(`/api/network/interfaces/${name}/confirm`);
}
