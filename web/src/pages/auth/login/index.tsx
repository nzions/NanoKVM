import { ReactElement, useEffect, useState } from 'react';
import { LockOutlined } from '@ant-design/icons';
import { Button, Form, Input } from 'antd';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';

import * as api from '@/api/auth.ts';
import { Head } from '@/components/head.tsx';

import { Tips } from './tips.tsx';

// Codes of POST /api/auth/login and POST /api/auth/setup (server/proto/auth.go).
const passwordUnsetCode = -6;
const setupDoneCode = -7;

export const Login = (): ReactElement => {
  const navigate = useNavigate();
  const { t } = useTranslation();

  const [isLoading, setIsloading] = useState(false);
  const [msg, setMsg] = useState('');
  // true on a device with no password yet: the first connection sets it
  const [isUnset, setIsUnset] = useState(false);

  useEffect(() => {
    api
      .getAuthState()
      .then((rsp: any) => {
        if (rsp.code === 0 && rsp.data?.password_set === false) {
          setIsUnset(true);
        }
      })
      .catch(() => {});
  }, []);

  useEffect(() => {
    api
      .getAccount()
      .then((rsp) => {
        if (rsp.code === 0 && rsp.data?.username) {
          navigate('/', { replace: true });
        }
      })
      .catch(() => {});
  }, [navigate]);

  useEffect(() => {
    if (msg) {
      setTimeout(() => setMsg(''), 3000);
    }
  }, [msg]);

  function login(values: any) {
    if (isLoading) return;
    setIsloading(true);

    // the one account is root; its password is root's Linux password
    api
      .login('root', values.password)
      .then((rsp: any) => {
        if (rsp.code === passwordUnsetCode) {
          setIsUnset(true);
          return;
        }
        if (rsp.code !== 0) {
          let errorMsg = t('auth.error');
          if (rsp.code === -2) errorMsg = t('auth.invalidUser');
          else if (rsp.code === -5) errorMsg = t('auth.locked');
          else if (rsp.code === -4) errorMsg = t('auth.globalLocked');

          setMsg(errorMsg);
          return;
        }

        setMsg('');
        navigate('/', { replace: true });
      })
      .catch(() => {
        setMsg(t('auth.error'));
      })
      .finally(() => {
        setIsloading(false);
      });
  }

  function setup(values: any) {
    if (isLoading) return;
    if (values.password !== values.password2) {
      setMsg(t('auth.differentPassword'));
      return;
    }
    setIsloading(true);

    api
      .setupPassword(values.password)
      .then((rsp: any) => {
        if (rsp.code === setupDoneCode) {
          // someone set it first: back to the login form
          setIsUnset(false);
          setMsg(t('auth.setupDone'));
          return;
        }
        if (rsp.code !== 0) {
          setMsg(rsp.msg || t('auth.error'));
          return;
        }

        setMsg('');
        navigate('/', { replace: true });
      })
      .catch(() => {
        setMsg(t('auth.error'));
      })
      .finally(() => {
        setIsloading(false);
      });
  }

  return (
    <>
      <Head title={t('head.login')} />

      <div className="flex h-screen w-screen flex-col items-center justify-center">
        <Form
          style={{ minWidth: 300, maxWidth: 500 }}
          initialValues={{ remember: true }}
          onFinish={isUnset ? setup : login}
        >
          <div className="flex flex-col items-center justify-center pb-4">
            <img
              id="logo"
              src="/sipeed.ico"
              alt="Sipeed"
              onClick={(evt) => {
                evt.preventDefault();
                (evt.target as HTMLImageElement).classList.add('animate-spin');
                setTimeout(() => {
                  (evt.target as HTMLImageElement).classList.remove('animate-spin');
                }, 1000);
              }}
            />
          </div>
          <div className="pb-4 text-center text-neutral-300">
            {isUnset ? t('auth.setupTitle') : 'root'}
          </div>
          {isUnset && (
            <div className="pb-4 text-center text-sm text-neutral-400">{t('auth.setupDesc')}</div>
          )}

          <Form.Item
            name="password"
            rules={[{ required: true, message: t('auth.noEmptyPassword'), min: 1 }]}
          >
            <Input
              prefix={<LockOutlined />}
              type="password"
              autoComplete={isUnset ? 'new-password' : 'current-password'}
              placeholder={t('auth.placeholderPassword')}
            />
          </Form.Item>

          {isUnset && (
            <Form.Item
              name="password2"
              rules={[{ required: true, message: t('auth.noEmptyPassword'), min: 1 }]}
            >
              <Input
                prefix={<LockOutlined />}
                type="password"
                autoComplete="new-password"
                placeholder={t('auth.placeholderPassword2')}
              />
            </Form.Item>
          )}

          <div className="pb-1 text-red-500">{msg}</div>

          <Form.Item>
            <Button type="primary" htmlType="submit" className="w-full" loading={isLoading}>
              {isUnset ? t('auth.setupButtonText') : t('auth.loginButtonText')}
            </Button>
          </Form.Item>

          {!isUnset && (
            <div className="flex justify-end pb-4 text-sm">
              <Tips />
            </div>
          )}
        </Form>
      </div>
    </>
  );
};
