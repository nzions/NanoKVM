//go:build !nokvm

package common

/*
	#cgo CFLAGS: -I../include
	#cgo LDFLAGS: -L../dl_lib -lkvm
	#include "kvm_vision.h"
*/
import "C"
import (
	"sync"
	"unsafe"

	"NanoKVM-Server/service/hdmi"

	log "github.com/sirupsen/logrus"
)

var (
	kvmVision     *KvmVision
	kvmVisionOnce sync.Once
)

type KvmVision struct {
	mutex  sync.RWMutex
	closed bool
}

func GetKvmVision() *KvmVision {
	kvmVisionOnce.Do(func() {
		kvmVision = &KvmVision{}

		logLevel := C.uint8_t(0)
		C.kvmv_init(logLevel)
		log.Debugf("kvm vision initialized")
	})

	return kvmVision
}

func (k *KvmVision) ReadMjpeg(width uint16, height uint16, quality uint16) (data []byte, result int) {
	k.mutex.RLock()
	defer k.mutex.RUnlock()
	if k.closed {
		return nil, -1
	}

	var (
		kvmData  *C.uint8_t
		dataSize C.uint32_t
	)

	result = int(C.kvmv_read_img(
		C.uint16_t(width),
		C.uint16_t(height),
		C.uint8_t(0),
		C.uint16_t(quality),
		&kvmData,
		&dataSize,
	))
	if result < 0 {
		log.Errorf("failed to read kvm image: %v", result)
		return
	}
	defer C.free_kvmv_data(&kvmData)

	data = C.GoBytes(unsafe.Pointer(kvmData), C.int(dataSize))
	return
}

func (k *KvmVision) ReadH264(width uint16, height uint16, bitRate uint16) (data []byte, result int) {
	k.mutex.RLock()
	defer k.mutex.RUnlock()
	if k.closed {
		return nil, -1
	}

	var (
		kvmData  *C.uint8_t
		dataSize C.uint32_t
	)

	result = int(C.kvmv_read_img(
		C.uint16_t(width),
		C.uint16_t(height),
		C.uint8_t(1),
		C.uint16_t(bitRate),
		&kvmData,
		&dataSize,
	))
	if result < 0 {
		log.Errorf("failed to read kvm image: %v", result)
		return
	}
	defer C.free_kvmv_data(&kvmData)

	data = C.GoBytes(unsafe.Pointer(kvmData), C.int(dataSize))
	return
}

func (k *KvmVision) SetHDMI(enable bool) int {
	k.mutex.RLock()
	defer k.mutex.RUnlock()
	if k.closed {
		return -1
	}

	hdmiEnable := C.uint8_t(0)
	if enable {
		hdmiEnable = C.uint8_t(1)
	}

	result := int(C.kvmv_hdmi_control(hdmiEnable))
	if result < 0 {
		log.Errorf("failed to set hdmi to %t", enable)
		return result
	}

	return result
}

// hdmiWaitIntervalMs bounds one blocking wait. It is not a poll period: the
// wait returns the moment libkvm publishes a snapshot, and a timeout only
// means nothing changed.
const hdmiWaitIntervalMs = 10000

// WaitHDMI blocks until libkvm publishes an HDMI snapshot newer than lastSeq.
// It does not hold k.mutex while blocked: Close must be able to run, and its
// kvmv_deinit is what releases the wait with WaitClosed.
func (k *KvmVision) WaitHDMI(lastSeq uint64) (hdmi.Snapshot, hdmi.WaitResult) {
	k.mutex.RLock()
	closed := k.closed
	k.mutex.RUnlock()
	if closed {
		return hdmi.Snapshot{}, hdmi.WaitClosed
	}

	var snap C.struct_kvm_hdmi_snapshot
	switch C.kvmv_hdmi_wait(C.uint64_t(lastSeq), &snap, C.int(hdmiWaitIntervalMs)) {
	case 1:
		return hdmi.Snapshot{
			State:  hdmi.State(snap.state),
			Width:  uint16(snap.res.width),
			Height: uint16(snap.res.height),
			Seq:    uint64(snap.seq),
		}, hdmi.WaitChanged
	case 0:
		return hdmi.Snapshot{}, hdmi.WaitTimeout
	default:
		return hdmi.Snapshot{}, hdmi.WaitClosed
	}
}

func (k *KvmVision) SetGop(gop uint8) {
	k.mutex.RLock()
	defer k.mutex.RUnlock()
	if k.closed {
		return
	}

	_gop := C.uint8_t(gop)
	C.set_h264_gop(_gop)
}

func (k *KvmVision) SetFrameDetect(frame uint8) {
	k.mutex.RLock()
	defer k.mutex.RUnlock()
	if k.closed {
		return
	}

	_frame := C.uint8_t(frame)
	C.set_frame_detact(_frame)
}

func (k *KvmVision) Close() {
	k.mutex.Lock()
	defer k.mutex.Unlock()
	if k.closed {
		return
	}

	k.closed = true
	C.kvmv_deinit()
	log.Debugf("stop kvm vision...")
}
