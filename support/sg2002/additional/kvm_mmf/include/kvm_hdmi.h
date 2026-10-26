#ifndef KVM_HDMI_H_
#define KVM_HDMI_H_

/*
 * Types shared by libkvm (kvm_vision.cpp), the sensor code that sets the VI
 * size (sample_common_sensor.c) and the Go server (cgo). The copy under
 * server/include/ must stay byte-identical to this one.
 */

#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

/* The resolution the VI/CSI chain is initialised at. */
struct kvm_resolution {
    uint16_t width;
    uint16_t height;
};

enum kvm_hdmi_state {
    KVM_HDMI_NO_SIGNAL = 0,
    KVM_HDMI_LOCKING = 1,
    KVM_HDMI_STREAMING = 2,
    KVM_HDMI_RES_CHANGE = 3,
};

/*
 * One HDMI state transition, immutable once published. seq increases by one
 * per published snapshot; res is the resolution the chain is (being)
 * initialised at, 0x0 until a signal was ever locked.
 */
struct kvm_hdmi_snapshot {
    uint8_t state; /* enum kvm_hdmi_state */
    struct kvm_resolution res;
    uint64_t seq;
};

/*
 * Hands the resolution the next VI init will use to the sensor code
 * (PIC_CUSTOMIZE of LONTIUM_LT6911). The vendor VI init path above it has
 * fixed signatures, so the struct is stored here by value, atomically, and
 * read by SAMPLE_COMM_SNS_GetPicSize. Call it before the VI chain restart.
 */
void SAMPLE_COMM_SNS_SetCustomSize(const struct kvm_resolution *res);

#ifdef __cplusplus
}
#endif

#endif /* KVM_HDMI_H_ */
