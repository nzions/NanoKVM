package vm

import (
	"context"
	"sync"
	"time"

	"NanoKVM-Server/common"
	"NanoKVM-Server/proto"
	"NanoKVM-Server/service/hdmi"
	hdmistate "NanoKVM-Server/service/vm/hdmi_state"
	"NanoKVM-Server/utils"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

var (
	hdmiMutex           sync.Mutex
	hdmiCaptureReadSlot = make(chan struct{}, 1)
	hdmiIdleTimer       *time.Timer
	hdmiIdleGeneration  uint64
	hdmiDemand          = hdmistate.New()
	hdmiStoppedForIdle  bool

	// setHDMI reports what libkvm did with the capture switch. A board with
	// no reset line (the Cube) answers HdmiNoResetLine: not an error.
	setHDMI            = func(enabled bool) common.HdmiSwitch { return common.GetKvmVision().SetHDMI(enabled) }
	isHdmiDisabled     = utils.IsHdmiDisabled
	getHdmiIdleTimeout = utils.GetHDMIIdleTimeout
	hdmiHub            = hdmi.Default()
)

// hdmiSnapshot is the latest HDMI state libkvm published; the zero Snapshot
// (no signal, 0x0) until the first one.
func hdmiSnapshot() hdmi.Snapshot {
	snapshot, _ := hdmiHub.Current()
	return snapshot
}

// CurrentHdmiSnapshot is the HDMI state as the hub holds it.
func CurrentHdmiSnapshot() hdmi.Snapshot {
	return hdmiSnapshot()
}

// setHdmiLocked switches capture and, when libkvm accepted a switch to on,
// marks capture as warming: it is warm once libkvm publishes a snapshot
// newer than the one current right now that reports Streaming or NoSignal.
// The seq is read before the switch, so the answer to it cannot be missed.
func setHdmiLocked(enabled bool) {
	afterSeq := hdmiSnapshot().Seq
	result := setHDMI(enabled)
	if !enabled {
		hdmiDemand.ClearWarming()
		return
	}
	// Only a real switch makes the state machine re-read the chip, so only
	// that one can be waited on; a board without a reset line has nothing
	// switched and nothing to warm.
	if result == common.HdmiSwitched {
		hdmiDemand.MarkWarming(afterSeq)
	}
}

func (s *Service) ResetHdmi(c *gin.Context) {
	var rsp proto.Response

	utils.PersistHDMIEnabled()
	DisableHdmiCapture()
	time.Sleep(1 * time.Second)
	EnableHdmiCapture()

	rsp.OkRsp(c)
	log.Debug("reset hdmi")
}

func (s *Service) EnableHdmi(c *gin.Context) {
	var rsp proto.Response

	utils.PersistHDMIEnabled()
	EnableHdmiCapture()

	rsp.OkRsp(c)
	log.Debug("enable hdmi")
}

func (s *Service) DisableHdmi(c *gin.Context) {
	var rsp proto.Response

	utils.PersistHDMIDisabled()
	DisableHdmiCapture()

	rsp.OkRsp(c)
	log.Debug("disable hdmi")
}

func (s *Service) GetHdmiState(c *gin.Context) {
	var rsp proto.Response
	enabled := !isHdmiDisabled()

	snapshot := hdmiSnapshot()
	rsp.OkRspWithData(c, &proto.GetGetHdmiStateRsp{
		Enabled:     enabled,
		Signal:      enabled && snapshot.State == hdmi.Streaming,
		IdleTimeout: getHdmiIdleTimeout(),
		Snapshot:    snapshot,
	})

	log.Debug("get hdmi state")
}

func EnableHdmiCapture() {
	hdmiMutex.Lock()
	defer hdmiMutex.Unlock()

	setHdmiLocked(true)
	hdmiStoppedForIdle = false
	hdmiIdleGeneration++
	stopHdmiIdleTimerLocked()
	scheduleHdmiIdleTimerLocked()
}

func DisableHdmiCapture() {
	hdmiMutex.Lock()
	defer hdmiMutex.Unlock()

	setHdmiLocked(false)
	hdmiStoppedForIdle = false
	hdmiIdleGeneration++
	stopHdmiIdleTimerLocked()
}

func (s *Service) SetHdmiIdleTimeout(c *gin.Context) {
	var req proto.SetHdmiIdleTimeoutReq
	var rsp proto.Response

	if err := proto.ParseFormRequest(c, &req); err != nil {
		rsp.ErrRsp(c, -1, "invalid arguments")
		return
	}

	SetHdmiIdleTimeout(req.Minutes)
	rsp.OkRsp(c)
}

func SetHdmiIdleTimeout(minutes int) {
	if minutes < 0 || minutes > utils.MaxHDMIIdleTimeoutMinutes {
		return
	}

	utils.PersistHDMIIdleTimeout(minutes)

	hdmiMutex.Lock()
	defer hdmiMutex.Unlock()

	hdmiIdleGeneration++
	stopHdmiIdleTimerLocked()
	if hdmiStoppedForIdle && !isHdmiDisabled() {
		resumeHdmiCaptureLocked()
		hdmiStoppedForIdle = false
	}
	if !hdmiDemand.HasDemand() && !hdmiStoppedForIdle {
		scheduleHdmiIdleTimerLocked()
	}
}

func SetHdmiViewerCount(count int) {
	SetHdmiViewerCountForSource("legacy", count)
}

// SetHdmiViewerCountForSource is retained for callers that cannot provide a
// source revision. Streamers should use UpdateHdmiViewerSnapshot instead.
func SetHdmiViewerCountForSource(source string, count int) {
	hdmiMutex.Lock()
	defer hdmiMutex.Unlock()

	version := hdmiDemand.NextVersion(source)
	updateHdmiViewerSnapshotLocked(source, count, version)
}

// UpdateHdmiViewerSnapshot applies a source's authoritative client count.
// Revisions let streamers report after releasing their own client-map locks
// without allowing an older count to overwrite a newer one.
func UpdateHdmiViewerSnapshot(source string, count int, version uint64) {
	hdmiMutex.Lock()
	defer hdmiMutex.Unlock()

	updateHdmiViewerSnapshotLocked(source, count, version)
}

func updateHdmiViewerSnapshotLocked(source string, count int, version uint64) {
	if count < 0 {
		count = 0
	}

	if !hdmiDemand.UpdateViewer(source, count, version) {
		return
	}
	reconcileHdmiCaptureDemandLocked()
}

// AcquireHdmiCaptureLease prevents idle shutdown while a short-lived capture
// consumer, such as an MCP screenshot, is active. The returned release is
// safe to call more than once.
func AcquireHdmiCaptureLease() func() {
	release, _, _ := acquireHdmiCaptureLease()
	return release
}

// AcquireHdmiCaptureLeaseForRead keeps capture active and waits outside
// hdmiMutex until a capture resumed from idle has had time to produce a new
// frame. The returned claim function is consumed only by the first successful
// reader, so concurrent readers do not all discard a frame.
func AcquireHdmiCaptureLeaseForRead(ctx context.Context) (func(), func() bool, error) {
	release, warmAfterSeq, warming := acquireHdmiCaptureLease()
	if warming {
		// Capture was just switched on: wait for libkvm to report what the
		// input looks like now. No timer; the context bounds the wait.
		if _, err := hdmiHub.WaitSettled(ctx, warmAfterSeq); err != nil {
			release()
			return nil, nil, err
		}
	}
	select {
	case hdmiCaptureReadSlot <- struct{}{}:
	case <-ctx.Done():
		release()
		return nil, nil, ctx.Err()
	}

	var releaseReadOnce sync.Once
	releaseRead := func() {
		releaseReadOnce.Do(func() {
			<-hdmiCaptureReadSlot
			release()
		})
	}

	hdmiMutex.Lock()
	needsFreshFrame := hdmiDemand.NeedsFreshFrame()
	hdmiMutex.Unlock()
	if !needsFreshFrame {
		return releaseRead, nil, nil
	}
	claimFresh := func() bool {
		hdmiMutex.Lock()
		claimed := hdmiDemand.ClaimFreshFrame()
		hdmiMutex.Unlock()
		return claimed
	}
	return releaseRead, claimFresh, nil
}

// acquireHdmiCaptureLease returns the release func, and whether capture is
// still warming and the snapshot seq it is warming after.
func acquireHdmiCaptureLease() (func(), uint64, bool) {
	hdmiMutex.Lock()
	hdmiDemand.AcquireLease()
	reconcileHdmiCaptureDemandLocked()
	warmAfterSeq := hdmiDemand.WarmAfterSeq()
	// Only a hub that has published can answer; without one (no libkvm)
	// there is nothing to wait for.
	_, published := hdmiHub.Current()
	warming := hdmiDemand.NeedsFreshFrame() && published
	hdmiMutex.Unlock()

	var once sync.Once
	release := func() {
		once.Do(func() {
			hdmiMutex.Lock()
			defer hdmiMutex.Unlock()

			if !hdmiDemand.ReleaseLease() {
				return
			}
			reconcileHdmiCaptureDemandLocked()
		})
	}
	return release, warmAfterSeq, warming
}

func reconcileHdmiCaptureDemandLocked() {
	hdmiIdleGeneration++
	stopHdmiIdleTimerLocked()

	if hdmiHasCaptureDemandLocked() {
		if hdmiStoppedForIdle && !isHdmiDisabled() {
			resumeHdmiCaptureLocked()
		}
		hdmiStoppedForIdle = false
		return
	}

	scheduleHdmiIdleTimerLocked()
}

func hdmiHasCaptureDemandLocked() bool {
	return hdmiDemand.HasDemand()
}

func stopHdmiIdleTimerLocked() {
	if hdmiIdleTimer != nil {
		hdmiIdleTimer.Stop()
		hdmiIdleTimer = nil
	}
}

func scheduleHdmiIdleTimerLocked() {
	if hdmiHasCaptureDemandLocked() || hdmiStoppedForIdle || isHdmiDisabled() {
		return
	}

	minutes := getHdmiIdleTimeout()
	if minutes == 0 {
		return
	}

	generation := hdmiIdleGeneration
	hdmiIdleTimer = time.AfterFunc(time.Duration(minutes)*time.Minute, func() {
		hdmiMutex.Lock()
		defer hdmiMutex.Unlock()

		if generation != hdmiIdleGeneration || hdmiHasCaptureDemandLocked() || isHdmiDisabled() {
			return
		}

		hdmiIdleTimer = nil
		setHdmiLocked(false)
		hdmiStoppedForIdle = true
		log.Debugf("disabled hdmi capture after %d minutes without viewers", minutes)
	})
}

func resumeHdmiCaptureLocked() {
	setHdmiLocked(true)
	log.Debug("resumed hdmi capture after viewer connected")
}
