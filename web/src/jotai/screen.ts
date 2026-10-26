import { atom } from 'jotai';

import type { HdmiSnapshot } from '@/pages/desktop/hdmi-state/model.ts';
import { ControlRegionMode, InputRegion, Resolution } from '@/types';

export const isHdmiEnabledAtom = atom(true);

// latest HDMI state the device pushed (null until the first one arrives)
export const hdmiSnapshotAtom = atom<HdmiSnapshot | null>(null);

// video mode
// direct: stream H.264 over HTTP
// h264: stream H.264 over WebRTC
// mjpeg: stream JPEG over HTTP
export const videoModeAtom = atom('');

export const videoScaleAtom = atom<number>(1.0);

// browser screen resolution
export const resolutionAtom = atom<Resolution | null>(null);

// currently effective absolute mouse input region
export const inputRegionAtom = atom<InputRegion | null>(null);
export const manualInputRegionAtom = atom<InputRegion | null>(null);
export const manualRegionsAtom = atom<InputRegion[]>([]);
export const selectedManualRegionAtom = atom<string>('');
export const selectedOriginalResolutionAtom = atom<string>('');

// device-level control region mode; disabled by default
export const controlRegionModeAtom = atom<ControlRegionMode>('off');

// show the live input-region selection overlay
export const inputRegionSelectingAtom = atom(false);
