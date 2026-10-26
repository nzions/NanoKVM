export const HDMI_STATE_EVENT = 'hdmi-state';

// The libkvm HDMI state machine states; the values are the server's wire values.
export enum HdmiState {
  NoSignal = 0,
  Locking = 1,
  Streaming = 2,
  ResChange = 3
}

// One HDMI state transition. seq increases per transition on the device.
export type HdmiSnapshot = {
  state: HdmiState;
  width: number;
  height: number;
  seq: number;
};

export function parseHdmiStateMessage(message: { data: unknown }): HdmiSnapshot | null {
  if (typeof message.data !== 'string') {
    return null;
  }

  try {
    const envelope = JSON.parse(message.data) as { data?: unknown };
    if (typeof envelope.data !== 'string') {
      return null;
    }

    const snapshot = JSON.parse(envelope.data) as Partial<HdmiSnapshot>;
    if (
      typeof snapshot.state !== 'number' ||
      typeof snapshot.width !== 'number' ||
      typeof snapshot.height !== 'number' ||
      typeof snapshot.seq !== 'number' ||
      !(snapshot.state in HdmiState)
    ) {
      return null;
    }

    return {
      state: snapshot.state,
      width: snapshot.width,
      height: snapshot.height,
      seq: snapshot.seq
    };
  } catch (error) {
    console.log(error);
    return null;
  }
}
