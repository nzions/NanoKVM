//go:build nokvm

package common

// The nokvm build has no capture hardware: it lets the server build and run off
// the device (amd64/arm64, a VM or a container) so everything but video, HDMI
// and encoding can be exercised. Every read fails, HDMI is never active.

import (
	"sync"

	log "github.com/sirupsen/logrus"
)

var (
	kvmVision     *KvmVision
	kvmVisionOnce sync.Once
)

type KvmVision struct{}

func GetKvmVision() *KvmVision {
	kvmVisionOnce.Do(func() {
		kvmVision = &KvmVision{}
		log.Warnf("kvm vision: nokvm build, no capture hardware")
	})

	return kvmVision
}

func (k *KvmVision) ReadMjpeg(width uint16, height uint16, quality uint16) (data []byte, result int) {
	return nil, -1
}

func (k *KvmVision) ReadH264(width uint16, height uint16, bitRate uint16) (data []byte, result int) {
	return nil, -1
}

func (k *KvmVision) SetHDMI(enable bool) int {
	return -1
}

func (k *KvmVision) HasHDMISignal() bool {
	return false
}

func (k *KvmVision) SetGop(gop uint8) {}

func (k *KvmVision) SetFrameDetect(frame uint8) {}

func (k *KvmVision) Close() {}
