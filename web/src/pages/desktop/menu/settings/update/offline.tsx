import { useRef, useState } from 'react';
import { Button } from 'antd';
import { ExternalLinkIcon } from 'lucide-react';
import { useTranslation } from 'react-i18next';

import * as api from '@/api/application.ts';

interface UpdateProps {
  status: string;
  setStatus: (status: string) => void;
  setIsLocked: (isClosable: boolean) => void;
  setErrMsg: (msg: string) => void;
}

export const Offline = ({ status, setStatus, setIsLocked, setErrMsg }: UpdateProps) => {
  const { t } = useTranslation();

  const inputRef = useRef<HTMLInputElement | null>(null);
  const sigInputRef = useRef<HTMLInputElement | null>(null);
  const [signature, setSignature] = useState<File | null>(null);

  function handleClick() {
    if (!signature) {
      setStatus('failed');
      setErrMsg(t('settings.update.offline.signatureRequired'));
      return;
    }
    inputRef.current?.click();
  }

  function handleSignatureChange(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0];
    e.target.value = '';
    if (!file) {
      return;
    }
    setSignature(file);
    setErrMsg('');
  }

  function handleFileChange(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0];
    if (!file) {
      return;
    }
    e.target.value = '';

    if (!validateFilename(file.name)) {
      setStatus('failed');
      setErrMsg(t('settings.update.offline.invalidName'));
      return;
    }

    upload(file);
  }

  function upload(file: File | null) {
    if (!file) return;

    if (!signature) {
      setStatus('failed');
      setErrMsg(t('settings.update.offline.signatureRequired'));
      return;
    }

    if (signature.name !== `${file.name}.sig`) {
      setStatus('failed');
      setErrMsg(t('settings.update.offline.invalidSignatureName'));
      return;
    }

    if (!validateFilename(file.name)) {
      setStatus('failed');
      setErrMsg(t('settings.update.offline.invalidName'));
      return;
    }

    if (status === 'loading' || status === 'updating') {
      return;
    }

    setIsLocked(true);
    setStatus('updating');
    setErrMsg('');

    const formData = new FormData();
    formData.append('file', file);
    formData.append('signature', signature);

    api
      .offlineUpdate(formData)
      .then(async (rsp: Response) => {
        // The proxy may return 502 after the update stops the old server.
        if (rsp.status === 502) return;
        if (!rsp.ok) throw new Error(`HTTP error ${rsp.status}`);

        const rspj = await rsp.json();
        if (rspj.code !== 0) {
          throw new Error(rspj.msg || t('settings.update.offline.updateFailed'));
        }
      })
      .then(() => {
        setTimeout(() => {
          setIsLocked(false);
          window.location.reload();
        }, 12000);
      })
      .catch((error: unknown) => {
        setIsLocked(false);
        setStatus('failed');
        setErrMsg(
          error instanceof Error ? error.message : t('settings.update.offline.updateFailed')
        );
      });
  }

  function validateFilename(filename: string) {
    const regex: RegExp = /^nanokvm_\d+\.\d+\.\d+\.tar\.gz$/;
    return regex.test(filename);
  }

  return (
    <>
      <div className="mt-8 flex flex-col gap-3">
        <div className="flex items-center justify-between gap-4">
          <div className="flex flex-col space-y-1">
            <div className="flex items-center space-x-2">
              <span>{t('settings.update.offline.title')}</span>

              <a
                className="flex items-center text-neutral-500 hover:text-blue-500"
                href="https://github.com/sipeed/NanoKVM/releases"
                target="_blank"
              >
                <ExternalLinkIcon size={15} />
              </a>
            </div>

            <span className="text-xs text-neutral-500">{t('settings.update.offline.desc')}</span>
          </div>

          <input
            id="file-upload"
            ref={inputRef}
            type="file"
            accept=".tar.gz"
            onChange={handleFileChange}
            className="hidden"
          />
          <input
            id="signature-upload"
            ref={sigInputRef}
            type="file"
            accept=".sig"
            onChange={handleSignatureChange}
            className="hidden"
          />
          <div className="flex items-center space-x-2">
            <Button
              disabled={status === 'loading' || status === 'updating'}
              onClick={() => sigInputRef.current?.click()}
            >
              {t('settings.update.offline.signatureButton')}
            </Button>
            <Button
              type="primary"
              disabled={status === 'loading' || status === 'updating' || !signature}
              onClick={handleClick}
            >
              {t('settings.update.offline.upload')}
            </Button>
          </div>
        </div>

        <span className="text-xs text-neutral-500">
          {signature
            ? `${t('settings.update.offline.signatureSelected')}${signature.name}`
            : t('settings.update.offline.signatureRequired')}
        </span>
      </div>
    </>
  );
};
