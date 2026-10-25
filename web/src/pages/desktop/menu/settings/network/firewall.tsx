import { useEffect, useState } from 'react';
import { message, Modal, Select, Tooltip } from 'antd';
import { CircleAlertIcon } from 'lucide-react';
import { useTranslation } from 'react-i18next';

import * as api from '@/api/vm.ts';

export const Firewall = () => {
  const { t } = useTranslation();
  const [messageApi, messageHolder] = message.useMessage();
  const [modal, modalHolder] = Modal.useModal();

  const [mode, setMode] = useState<api.FirewallMode | null>(null);
  const [isLoading, setIsLoading] = useState(true);

  useEffect(() => {
    api
      .getFirewall()
      .then((rsp) => {
        const m = rsp.data?.mode;
        if (rsp.code === 0 && (m === 'open' || m === 'drop')) {
          setMode(m);
        }
      })
      .catch((err) => console.log(err))
      .finally(() => setIsLoading(false));
  }, []);

  async function apply(next: api.FirewallMode) {
    setIsLoading(true);
    try {
      const rsp = await api.setFirewall(next);
      if (rsp.code === 0 && rsp.data?.mode === next) {
        setMode(next);
        messageApi.success(t('settings.network.firewall.applied'));
      } else {
        messageApi.error(t('settings.network.firewall.failed'));
      }
    } catch (err) {
      console.log(err);
      messageApi.error(t('settings.network.firewall.failed'));
    } finally {
      setIsLoading(false);
    }
  }

  function change(next: api.FirewallMode) {
    if (next === mode || isLoading) return;

    if (next === 'drop') {
      modal.confirm({
        title: t('settings.network.firewall.confirmTitle'),
        content: t('settings.network.firewall.confirmContent'),
        okText: t('settings.network.firewall.confirmOk'),
        cancelText: t('settings.network.firewall.confirmCancel'),
        okButtonProps: { danger: true },
        onOk: () => apply(next)
      });
      return;
    }

    apply(next);
  }

  return (
    <div className="flex items-center justify-between space-x-5">
      {messageHolder}
      {modalHolder}

      <div className="flex flex-col space-y-1">
        <div className="flex items-center space-x-2">
          <span>{t('settings.network.firewall.title')}</span>

          <Tooltip
            title={t('settings.network.firewall.tip')}
            className="cursor-pointer"
            placement="right"
            styles={{ root: { maxWidth: '400px' } }}
          >
            <CircleAlertIcon className="text-neutral-500" size={14} />
          </Tooltip>
        </div>
        <span className="text-xs text-neutral-500">
          {t('settings.network.firewall.description')}
        </span>
      </div>

      <Select
        value={mode}
        loading={isLoading}
        disabled={isLoading || mode === null}
        style={{ width: 180 }}
        options={[
          { value: 'open', label: t('settings.network.firewall.open') },
          { value: 'drop', label: t('settings.network.firewall.drop') }
        ]}
        onSelect={change}
      />
    </div>
  );
};
