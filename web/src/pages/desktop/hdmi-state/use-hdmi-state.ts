import { useEffect } from 'react';
import { useSetAtom } from 'jotai';

import { hdmiSnapshotAtom } from '@/jotai/screen.ts';
import { client } from '@/lib/websocket.ts';

import { HDMI_STATE_EVENT, parseHdmiStateMessage } from './model';

// Keeps hdmiSnapshotAtom on the device's HDMI state. The server pushes the
// current snapshot when the websocket connects and one per transition after
// that; one connection delivers them in order.
export function useHdmiState() {
  const setSnapshot = useSetAtom(hdmiSnapshotAtom);

  useEffect(() => {
    return client.on(HDMI_STATE_EVENT, (message) => {
      const snapshot = parseHdmiStateMessage(message);
      if (snapshot) {
        setSnapshot(snapshot);
      }
    });
  }, [setSnapshot]);
}
