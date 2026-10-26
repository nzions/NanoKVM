// The one LT6911 driver. libkvm (the HDMI state machine) and
// tools/nanokvm_update_edid are built from the very same lt6911.c; nobody else
// opens /dev/i2c-4 or writes the chip's registers.
//
// Plain C. Every I2C transfer is a checked I2C_RDWR ioctl and every function
// returns 0 or a negative errno; no caller dereferences a result it has not
// checked. The chip's page register (0xFF) is shared state, so no caller holds
// it: lt6911_reg_read / lt6911_reg_write always select the page themselves,
// inside one process-wide critical section, and lt6911_probe holds that section
// across a whole enable -> query -> disable sequence.
//
// Variants. The chip on a board is one of C, UXC, D or UXE. It is detected ONCE,
// by lt6911_open, from /etc/kvm/hdmi_version, which S15kvmhwd writes after
// probing the bus ("c", "ux", "d", "ue"). A missing, unreadable or unrecognised
// marker is an error (LT6911_UNKNOWN is never a working device): the variant
// is never guessed. Each variant has one `struct lt6911_ops` row.
//
// Hactive: the chip reports the width of the receive side (C) and of D in units
// of two pixels, so the pixel width is the register times a per-register scale
// (struct lt6911_res_regs.h_scale). See lt6911.c for the table and the reasons.
#ifndef LT6911_H
#define LT6911_H

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

#define LT6911_I2C_BUS_PATH      "/dev/i2c-4"
#define LT6911_VARIANT_FILE      "/etc/kvm/hdmi_version"

// The 7-bit address every register access in this tree uses, for every
// variant (kvm_vision and nanokvm_update_edid both always used it, C included).
#define LT6911_I2C_ADDR          0x2B
// S15kvmhwd's detection treats an ACK at this address as "LT6911C". Detection
// only: no code in this tree talks to the chip there. See lt6911.c.
#define LT6911C_DETECT_ADDR      0x2C

// Register 0xFF of every page selects the page. Only the library writes it.
#define LT6911_REG_PAGE          0xFF

// The widest block one transaction moves (the EDID tool writes 32 bytes).
#define LT6911_MAX_BLOCK         32

enum lt6911_variant {
    LT6911_UNKNOWN = 0,
    LT6911_C,
    LT6911_UXC,
    LT6911_D,
    LT6911_UXE,     // no register access at all; its mode is assumed
};

// The INT-line edge on which a variant announces a change.
enum lt6911_int_edge {
    LT6911_INT_NONE = 0,    // no INT usable
    LT6911_INT_RISING,
    LT6911_INT_FALLING,
};

struct lt6911_res {
    uint16_t width;
    uint16_t height;
};

// What one probe saw. csi is valid only when present.
struct lt6911_probe_result {
    bool present;
    struct lt6911_res csi;
};

// Where a resolution lives: one page, a 16-bit big-endian register each for the
// height and the width, and the multiplier that turns the width register into
// pixels.
struct lt6911_res_regs {
    uint8_t page;
    uint8_t v_reg;
    uint8_t h_reg;
    uint8_t h_scale;
};

struct lt6911_dev;

// Per-variant behaviour. A NULL op means the variant does not have it.
struct lt6911_ops {
    const char *name;
    uint8_t i2c_addr;
    enum lt6911_int_edge int_edge;
    // Wake the chip's register interface / put it back. Called by lt6911_probe
    // with the library lock held.
    int (*enable)(struct lt6911_dev *dev);
    int (*disable)(struct lt6911_dev *dev);
    // Is a source locked on the HDMI input (with its size, where the chip says)?
    int (*signal_present)(struct lt6911_dev *dev, bool *present);
    // The size the chip is sending down the CSI link.
    int (*csi_res)(struct lt6911_dev *dev, struct lt6911_res *res);
};

struct lt6911_dev {
    int fd;
    enum lt6911_variant variant;
    const struct lt6911_ops *ops;
};

// Opens LT6911_I2C_BUS_PATH, checks it can do plain I2C, detects the variant
// once. 0 on success; negative errno otherwise (-ENODEV: marker missing or
// not a known variant) and the device is not open.
int lt6911_open(struct lt6911_dev *dev);
void lt6911_close(struct lt6911_dev *dev);

// Maps the marker text (trailing whitespace removed) to a variant.
enum lt6911_variant lt6911_variant_parse(const char *text);
const char *lt6911_variant_name(enum lt6911_variant variant);

// Registers. Each call selects `page` itself. reg == LT6911_REG_PAGE is
// refused (-EINVAL). len is 1..LT6911_MAX_BLOCK.
int lt6911_reg_read(struct lt6911_dev *dev, uint8_t page, uint8_t reg,
                    uint8_t *out, size_t len);
int lt6911_reg_write(struct lt6911_dev *dev, uint8_t page, uint8_t reg,
                     const uint8_t *data, size_t len);
int lt6911_reg_read8(struct lt6911_dev *dev, uint8_t page, uint8_t reg, uint8_t *out);
int lt6911_reg_write8(struct lt6911_dev *dev, uint8_t page, uint8_t reg, uint8_t val);

// enable -> signal_present -> (csi_res when present) -> disable, as one
// critical section. The disable runs even when a query failed; the first
// error is the one returned. -ENOTSUP for a variant with no register access.
int lt6911_probe(struct lt6911_dev *dev, struct lt6911_probe_result *out);

#ifdef __cplusplus
}
#endif

#endif
