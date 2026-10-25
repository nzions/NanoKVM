import { Select } from 'antd';
import { useTranslation } from 'react-i18next';

import type { ThemeMode } from '@/lib/localstorage.ts';
import { changeThemeMode, useThemeMode } from '@/lib/theme.ts';

export const Theme = () => {
  const { t } = useTranslation();
  const mode = useThemeMode();

  const options: { value: ThemeMode; label: string }[] = [
    { value: 'dark', label: t('settings.appearance.theme.dark') },
    { value: 'light', label: t('settings.appearance.theme.light') },
    { value: 'system', label: t('settings.appearance.theme.system') }
  ];

  return (
    <div className="mt-5 flex items-center justify-between space-x-5">
      <div className="flex flex-col space-y-1">
        <span>{t('settings.appearance.theme.title')}</span>
        <span className="text-xs text-neutral-500">{t('settings.appearance.theme.desc')}</span>
      </div>

      <div>
        <Select
          value={mode}
          style={{ width: 180 }}
          options={options}
          onSelect={(value: ThemeMode) => changeThemeMode(value)}
        />
      </div>
    </div>
  );
};
