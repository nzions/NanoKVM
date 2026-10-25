import { useEffect, useState } from 'react';
import { Button, Input, Modal, Segmented } from 'antd';
import { useTranslation } from 'react-i18next';

import * as api from '@/api/network.ts';
import type { IPMode, NetworkInterface } from '@/api/network.ts';

import { ipv4ToInt, isNetworkOrBroadcast, parseCidr, sameSubnet } from '@/lib/cidr.ts';

import { Wifi } from './wifi.tsx';

const maxPrefix = 31;

function hostOf(address: string) {
  return address.split('/')[0];
}

type PanelProps = {
  info: NetworkInterface;
  onChanged: () => void;
};

const InterfacePanel = ({ info, onChanged }: PanelProps) => {
  const { t } = useTranslation();

  const [mode, setMode] = useState<IPMode>(info.mode);
  const [address, setAddress] = useState('');
  const [gateway, setGateway] = useState('');
  const [error, setError] = useState('');
  const [isSaving, setIsSaving] = useState(false);

  const [pending, setPending] = useState<api.InterfacePending | null>(info.pending);
  // device clock minus browser clock when the deadline arrived
  const [skew, setSkew] = useState(0);
  const [now, setNow] = useState(Date.now());

  const label = t(`settings.network.interfaces.${info.kind}`);
  const currentAddress = info.current.addresses[0] || '';

  useEffect(() => {
    // a saved static config wins; otherwise open on what the interface has now
    const saved = info.mode === 'static' && !!info.address;

    setMode(info.mode);
    setAddress(saved ? info.address : currentAddress);
    setGateway((saved && info.gateway) || info.current.gateway || '');
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [info.mode, info.address, info.gateway, currentAddress, info.current.gateway]);

  useEffect(() => {
    if (info.pending) {
      showPending(info.pending);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [info.pending]);

  useEffect(() => {
    if (!pending) return;

    const timer = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(timer);
  }, [pending]);

  function showPending(value: api.InterfacePending) {
    setSkew(value.now_unix * 1000 - Date.now());
    setNow(Date.now());
    setPending(value);
  }

  // seconds left, from the server's deadline and the device clock
  const remaining = pending
    ? Math.max(0, pending.deadline_unix - Math.floor((now + skew) / 1000))
    : 0;

  function validate(): string {
    if (mode === 'dhcp') return '';

    // the server accepts /1 to /31, a /31 being point-to-point (server/service/network/interfaces.go)
    const cidr = parseCidr(address);
    if (!cidr || cidr.prefix > maxPrefix) return t('settings.network.interfaces.invalidAddress');
    if (isNetworkOrBroadcast(cidr.ip, cidr.prefix)) {
      return t('settings.network.interfaces.hostNotAllowed');
    }

    const gw = gateway.trim();
    if (gw) {
      if (ipv4ToInt(gw) === null) return t('settings.network.interfaces.invalidGateway');
      if (!sameSubnet(gw, cidr.ip, cidr.prefix)) {
        return t('settings.network.interfaces.gatewayOutsideSubnet');
      }
    }

    return '';
  }

  async function save() {
    const problem = validate();
    if (problem) {
      setError(problem);
      return;
    }
    setError('');

    const cidr = parseCidr(address);
    const req: api.SetInterfaceReq = {
      mode,
      address: mode === 'static' && cidr ? `${cidr.ip}/${cidr.prefix}` : '',
      gateway: mode === 'static' ? gateway.trim() : ''
    };

    setIsSaving(true);
    try {
      const rsp = await api.setInterface(info.name, req);
      if (rsp.code !== 0) {
        setError(rsp.msg || t('settings.network.interfaces.saveFailed'));
        return;
      }

      showPending(rsp.data as api.InterfacePending);
    } catch {
      // the connection may drop when the address changes under it
      setError(t('settings.network.interfaces.saveFailed'));
    } finally {
      setIsSaving(false);
    }
  }

  async function confirm() {
    try {
      const rsp = await api.confirmInterface(info.name);
      if (rsp.code !== 0) {
        setError(rsp.msg || t('settings.network.interfaces.confirmFailed'));
        return;
      }

      setPending(null);
      setError('');
      onChanged();
    } catch {
      setError(t('settings.network.interfaces.confirmFailed'));
    }
  }

  function close() {
    setPending(null);
    onChanged();
  }

  const newHost = pending && pending.mode === 'static' ? hostOf(pending.address) : '';
  const addressChanged = !!newHost && !info.current.addresses.some((a) => hostOf(a) === newHost);
  const newUrl = `https://${newHost}/`;

  const hasChanges =
    mode !== info.mode ||
    (mode === 'static' &&
      (address.trim() !== info.address ||
        gateway.trim() !== info.gateway));

  return (
    <>
      <div className="flex flex-col space-y-3">
        <div className="flex items-center justify-between">
          <div className="flex flex-col space-y-1">
            <span>{label}</span>
            <span className="text-xs text-neutral-500">
              {[
                info.ssid,
                currentAddress,
                info.current.gateway &&
                  `${t('settings.network.interfaces.gateway')} ${info.current.gateway}`,
                info.current.mac
              ]
                .filter(Boolean)
                .join(' · ') || t('settings.network.interfaces.noAddress')}
            </span>
          </div>

          <Segmented
            disabled={isSaving || !!pending}
            value={mode}
            onChange={(val) => {
              setMode(val as IPMode);
              setError('');
            }}
            options={[
              { label: t('settings.network.interfaces.dhcp'), value: 'dhcp' },
              { label: t('settings.network.interfaces.static'), value: 'static' }
            ]}
          />
        </div>

        {mode === 'static' && (
          <div className="flex flex-col space-y-2">
            <Input
              addonBefore={t('settings.network.interfaces.address')}
              placeholder="192.168.1.20/24"
              value={address}
              disabled={isSaving || !!pending}
              status={address && !parseCidr(address) ? 'error' : undefined}
              onChange={(e) => setAddress(e.target.value)}
            />
            <Input
              addonBefore={t('settings.network.interfaces.gateway')}
              placeholder={t('settings.network.interfaces.optional')}
              value={gateway}
              disabled={isSaving || !!pending}
              status={gateway && ipv4ToInt(gateway.trim()) === null ? 'error' : undefined}
              onChange={(e) => setGateway(e.target.value)}
            />
          </div>
        )}

        <div className="flex items-center justify-between">
          <span className="text-xs text-red-400">{error}</span>
          <Button
            type="primary"
            size="small"
            loading={isSaving}
            disabled={!hasChanges || !!pending}
            onClick={save}
          >
            {t('settings.network.interfaces.save')}
          </Button>
        </div>
      </div>

      <Modal
        closable={false}
        maskClosable={false}
        open={!!pending}
        centered={true}
        title={t('settings.network.interfaces.changedTitle')}
        footer={
          <div className="flex justify-end space-x-2">
            <Button onClick={close}>{t('settings.network.interfaces.close')}</Button>
            <Button type="primary" disabled={remaining === 0} onClick={confirm}>
              {t('settings.network.interfaces.confirm')}
            </Button>
          </div>
        }
      >
        <div className="flex flex-col space-y-3 py-2">
          <span>{t('settings.network.interfaces.changedDesc')}</span>

          <span className="text-lg font-bold">
            {remaining > 0
              ? t('settings.network.interfaces.countdown', { count: remaining })
              : t('settings.network.interfaces.reverted')}
          </span>

          {addressChanged && (
            <div className="flex flex-col space-y-1">
              <span>{t('settings.network.interfaces.newAddress')}</span>
              <a href={newUrl} target="_blank" rel="noreferrer">
                {newUrl}
              </a>
              <span className="text-xs text-neutral-500">
                {t('settings.network.interfaces.confirmFromNew')}
              </span>
            </div>
          )}

          {error && <span className="text-sm text-red-400">{error}</span>}
        </div>
      </Modal>
    </>
  );
};

// Interfaces lists the wired and the Wi-Fi interface; hardware that is not
// there gets one muted line and no settings.
export const Interfaces = () => {
  const { t } = useTranslation();

  const [interfaces, setInterfaces] = useState<NetworkInterface[]>([]);

  useEffect(() => {
    load();
  }, []);

  async function load() {
    try {
      const rsp = await api.getInterfaces();
      if (rsp.code !== 0) {
        console.log(rsp.msg);
        return;
      }

      setInterfaces((rsp.data?.interfaces as NetworkInterface[]) || []);
    } catch {
      /* empty */
    }
  }

  return (
    <>
      {interfaces.map((info) => {
        if (!info.present) {
          return (
            <span key={info.name} className="text-xs text-neutral-500">
              {t('settings.network.interfaces.notPresent', {
                name: t(`settings.network.interfaces.${info.kind}`)
              })}
            </span>
          );
        }

        return (
          <div key={info.name} className="flex flex-col space-y-8">
            {info.kind === 'wifi' && <Wifi />}
            <InterfacePanel info={info} onChanged={load} />
          </div>
        );
      })}
    </>
  );
};
