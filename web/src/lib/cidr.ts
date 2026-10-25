export type Cidr = {
  ip: string;
  prefix: number;
};

// ipv4ToInt reads a dotted quad (each octet 0-255, no leading zeros); null when it is not one
export function ipv4ToInt(value: string): number | null {
  const parts = value.split('.');
  if (parts.length !== 4) return null;

  let result = 0;
  for (const part of parts) {
    if (!/^\d{1,3}$/.test(part)) return null;
    if (part.length > 1 && part.startsWith('0')) return null;

    const octet = Number(part);
    if (octet > 255) return null;
    result = result * 256 + octet;
  }
  return result;
}

// parseCidr reads "n.n.n.n/nn" with a prefix of 1 to 32; null when it is anything else
export function parseCidr(value: string): Cidr | null {
  const text = value.trim();
  const match = /^([^/]+)\/(\d{1,2})$/.exec(text);
  if (!match) return null;

  if (ipv4ToInt(match[1]) === null) return null;

  const prefix = Number(match[2]);
  if (prefix < 1 || prefix > 32) return null;

  return { ip: match[1], prefix };
}

// prefixToNetmask turns 24 into 255.255.255.0
export function prefixToNetmask(prefix: number): string {
  const octets: number[] = [];
  for (let i = 0; i < 4; i++) {
    const bits = Math.max(0, Math.min(8, prefix - i * 8));
    octets.push((0xff << (8 - bits)) & 0xff);
  }
  return octets.join('.');
}

// netmaskToPrefix turns 255.255.255.0 into 24; null when it is not a contiguous mask
export function netmaskToPrefix(netmask: string): number | null {
  if (ipv4ToInt(netmask) === null) return null;

  let prefix = 0;
  let seenZero = false;
  for (const part of netmask.split('.')) {
    for (let bit = 7; bit >= 0; bit--) {
      const set = (Number(part) >> bit) & 1;
      if (set && seenZero) return null;
      if (set) prefix++;
      else seenZero = true;
    }
  }
  return prefix;
}

function maskInt(prefix: number): number {
  return prefix === 0 ? 0 : (0xffffffff << (32 - prefix)) >>> 0;
}

// sameSubnet reports whether two IPv4 addresses share a network under the prefix
export function sameSubnet(a: string, b: string, prefix: number): boolean {
  const x = ipv4ToInt(a);
  const y = ipv4ToInt(b);
  if (x === null || y === null) return false;

  const mask = maskInt(prefix);
  return ((x & mask) >>> 0) === ((y & mask) >>> 0);
}

// isNetworkOrBroadcast is true for the all-zero and all-one host part; /31 and /32 have none
export function isNetworkOrBroadcast(ip: string, prefix: number): boolean {
  const value = ipv4ToInt(ip);
  if (value === null || prefix > 30) return false;

  const mask = maskInt(prefix);
  const host = (value & ~mask) >>> 0;
  return host === 0 || host === (~mask >>> 0);
}
