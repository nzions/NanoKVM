// The one LT6911 driver. See internal/lt6911.h.
//
// History: this replaces four copies of the same register knowledge: the
// variant code in kvm_vision.cpp (tripled across enable/disable, hdmi_res and
// csi_res), kvm_system's dead hdmi.cpp (removed in 2.5.17), the edid tool's own
// I2C helpers with their own page cache, and the Sophgo lt6911_cmos.c that is
// prebuilt into libkvm_mmf (a vendor blob that cannot be removed here).
// Relative, so libkvm (include/ on its path) and the EDID tool (compiled with
// this file by path) find the same header.
#include "../include/internal/lt6911.h"

#include <ctype.h>
#include <errno.h>
#include <fcntl.h>
#include <limits.h>
#include <linux/i2c-dev.h>
#include <linux/i2c.h>
#include <pthread.h>
#include <stdio.h>
#include <string.h>
#include <sys/file.h>
#include <sys/inotify.h>
#include <sys/ioctl.h>
#include <time.h>
#include <unistd.h>

// ---- register map, per variant ---------------------------------------------

// Pages. The enable/disable page and register are the same on C and UXC.
#define LT6911_PAGE_SYS          0x80
#define LT6911_REG_ENABLE        0xEE
#define LT6911_ENABLE_ON         0x01
#define LT6911_ENABLE_OFF        0x00
// UXC only: register 0x10 on the SYS page holds the chip's watchdog; enable
// writes 0 to stop it.
#define LT6911UXC_REG_WATCHDOG   0x10
#define LT6911UXC_WATCHDOG_OFF   0x00

// C: the receive side (HDMI) is read after the chip is told which block to latch.
#define LT6911C_PAGE_RX          0xD2
#define LT6911C_REG_RX_LATCH     0x83
#define LT6911C_RX_LATCH_VALUE   0x11
#define LT6911C_RX_SETTLE_US     5000
#define LT6911C_PAGE_CSI         0xC2

// UXC: HDMI signal state byte and its two values.
#define LT6911UXC_PAGE_HDMI      0x86
#define LT6911UXC_REG_SIGNAL     0xA3
#define LT6911UXC_SIGNAL_STABLE  0x55
#define LT6911UXC_SIGNAL_GONE    0x88
#define LT6911UXC_PAGE_CSI       0x85

#define LT6911D_PAGE_RES         0xE0

#define LT6911_REG_PAIR          2   // a 16-bit big-endian value is two registers

// Resolution registers. h_scale reconciles the old "C doubles Hactive" quirk:
//
//  * C receive side (page D2, 0x8B): the width counts pairs of pixels. The old
//    code multiplied it by 2 (but only to log it: presence was V != 0 && H !=
//    0, which the scale does not change).
//  * C CSI side (page C2, 0x38): the old code did NOT scale, and its result
//    was used as the VI size, so these are real pixels.
//  * D (page E0, 0x8C): the old code multiplied by 2 in BOTH the presence read
//    and the CSI read; it is one register pair, so one scale.
//  * UXC CSI (page 85, 0xEA): never scaled.
// Not one chip behaving two ways: different registers with different units.
// Every use of a register keeps the scale the old code gave it, so the sizes
// handed to the VI are unchanged.
static const struct lt6911_res_regs lt6911c_rx_regs  = { LT6911C_PAGE_RX,  0x96, 0x8B, 2 };
static const struct lt6911_res_regs lt6911c_csi_regs = { LT6911C_PAGE_CSI, 0x06, 0x38, 1 };
static const struct lt6911_res_regs lt6911uxc_csi_regs = { LT6911UXC_PAGE_CSI, 0xF0, 0xEA, 1 };
static const struct lt6911_res_regs lt6911d_regs     = { LT6911D_PAGE_RES, 0x8E, 0x8C, 2 };

// ---- the lock ---------------------------------------------------------------

// Two locks, always taken together (see internal/lt6911.h): a mutex for the
// threads of this process and an flock on LT6911_LOCK_PATH for the other
// processes. The *_locked functions below require both. A thread that holds
// them through a session sets tls_in_session, and the public functions then run
// without taking them again.
static pthread_mutex_t lt6911_lock = PTHREAD_MUTEX_INITIALIZER;
static int lt6911_lock_fd = -1;                 // opened once, under lt6911_lock
static __thread bool tls_in_session = false;

#define LT6911_LOCK_RETRY_US     5000
#define MS_PER_S                 1000
#define NS_PER_MS                1000000

static int64_t now_ms(void)
{
    struct timespec ts;
    if(clock_gettime(CLOCK_MONOTONIC, &ts) < 0) return -1;
    return (int64_t)ts.tv_sec * MS_PER_S + ts.tv_nsec / NS_PER_MS;
}

// Caller holds lt6911_lock.
static int lock_file_open_locked(void)
{
    if(lt6911_lock_fd >= 0) return 0;
    int fd = open(LT6911_LOCK_PATH, O_RDWR | O_CREAT | O_CLOEXEC, 0600);
    if(fd < 0){
        int err = errno;
        fprintf(stderr, "[lt6911]open %s failed: %d\n", LT6911_LOCK_PATH, err);
        return -err;
    }
    lt6911_lock_fd = fd;
    return 0;
}

// flock(LOCK_EX), polled with LOCK_NB until the deadline. Caller holds lt6911_lock
// and has the lock file open.
static int flock_bounded(void)
{
    int64_t deadline = now_ms();
    if(deadline < 0) return -errno;
    deadline += LT6911_LOCK_TIMEOUT_MS;
    for(;;){
        if(flock(lt6911_lock_fd, LOCK_EX | LOCK_NB) == 0) return 0;
        if(errno != EWOULDBLOCK && errno != EINTR){
            int err = errno;
            fprintf(stderr, "[lt6911]flock %s failed: %d\n", LT6911_LOCK_PATH, err);
            return -err;
        }
        int64_t now = now_ms();
        if(now < 0) return -errno;
        if(now >= deadline){
            fprintf(stderr, "[lt6911]bus busy: another process holds %s after %d ms\n",
                    LT6911_LOCK_PATH, LT6911_LOCK_TIMEOUT_MS);
            return -ETIMEDOUT;
        }
        usleep(LT6911_LOCK_RETRY_US);
    }
}

static int lock_acquire(void)
{
    int r = pthread_mutex_lock(&lt6911_lock);
    if(r != 0) return -r;
    int err = lock_file_open_locked();
    if(err == 0) err = flock_bounded();
    if(err != 0){
        int u = pthread_mutex_unlock(&lt6911_lock);
        if(u != 0) fprintf(stderr, "[lt6911]mutex unlock failed: %d\n", u);
    }
    return err;
}

static int lock_release(void)
{
    int err = 0;
    if(flock(lt6911_lock_fd, LOCK_UN) < 0){
        err = -errno;
        fprintf(stderr, "[lt6911]funlock %s failed: %d\n", LT6911_LOCK_PATH, -err);
    }
    int u = pthread_mutex_unlock(&lt6911_lock);
    if(u != 0){
        fprintf(stderr, "[lt6911]mutex unlock failed: %d\n", u);
        if(err == 0) err = -u;
    }
    return err;
}

// ---- checked transfers -------------------------------------------------------

static int xfer(struct lt6911_dev *dev, struct i2c_msg *msgs, size_t n)
{
    struct i2c_rdwr_ioctl_data data;
    data.msgs = msgs;
    data.nmsgs = (uint32_t)n;
    int r = ioctl(dev->fd, I2C_RDWR, &data);
    if(r < 0) return -errno;
    if((size_t)r != n) return -EIO;
    return 0;
}

// Selects `page` with a write of its own (its own STOP), so the chip has the
// page before the register access begins.
static int select_page_locked(struct lt6911_dev *dev, uint8_t page)
{
    uint8_t buf[2] = { LT6911_REG_PAGE, page };
    struct i2c_msg msg = { dev->ops->i2c_addr, 0, sizeof(buf), buf };
    return xfer(dev, &msg, 1);
}

static int read_locked(struct lt6911_dev *dev, uint8_t page, uint8_t reg,
                       uint8_t *out, size_t len)
{
    if(len == 0 || len > LT6911_MAX_BLOCK) return -EINVAL;
    int r = select_page_locked(dev, page);
    if(r != 0) return r;
    uint8_t addr = reg;
    struct i2c_msg msgs[2] = {
        { dev->ops->i2c_addr, 0,        1,           &addr },
        { dev->ops->i2c_addr, I2C_M_RD, (uint16_t)len, out },
    };
    return xfer(dev, msgs, 2);
}

static int write_locked(struct lt6911_dev *dev, uint8_t page, uint8_t reg,
                        const uint8_t *data, size_t len)
{
    if(len == 0 || len > LT6911_MAX_BLOCK) return -EINVAL;
    int r = select_page_locked(dev, page);
    if(r != 0) return r;
    uint8_t buf[1 + LT6911_MAX_BLOCK];
    buf[0] = reg;
    memcpy(buf + 1, data, len);
    struct i2c_msg msg = { dev->ops->i2c_addr, 0, (uint16_t)(1 + len), buf };
    return xfer(dev, &msg, 1);
}

static int write8_locked(struct lt6911_dev *dev, uint8_t page, uint8_t reg, uint8_t val)
{
    return write_locked(dev, page, reg, &val, 1);
}

static int read8_locked(struct lt6911_dev *dev, uint8_t page, uint8_t reg, uint8_t *out)
{
    return read_locked(dev, page, reg, out, 1);
}

// Reads a height/width pair at `regs`; width in pixels (register * h_scale).
static int read_res_locked(struct lt6911_dev *dev, const struct lt6911_res_regs *regs,
                           struct lt6911_res *res)
{
    uint8_t v[LT6911_REG_PAIR], h[LT6911_REG_PAIR];
    int r = read_locked(dev, regs->page, regs->v_reg, v, sizeof(v));
    if(r != 0) return r;
    r = read_locked(dev, regs->page, regs->h_reg, h, sizeof(h));
    if(r != 0) return r;
    uint32_t width = (uint32_t)((h[0] << 8) | h[1]) * regs->h_scale;
    if(width > UINT16_MAX) return -ERANGE;
    res->height = (uint16_t)((v[0] << 8) | v[1]);
    res->width = (uint16_t)width;
    return 0;
}

// ---- C ----------------------------------------------------------------------

static int c_enable(struct lt6911_dev *dev)
{
    int r = write8_locked(dev, LT6911_PAGE_SYS, LT6911_REG_ENABLE, LT6911_ENABLE_ON);
    return r;
}

static int c_disable(struct lt6911_dev *dev)
{
    return write8_locked(dev, LT6911_PAGE_SYS, LT6911_REG_ENABLE, LT6911_ENABLE_OFF);
}

static int c_signal_present(struct lt6911_dev *dev, bool *present)
{
    int r = write8_locked(dev, LT6911C_PAGE_RX, LT6911C_REG_RX_LATCH, LT6911C_RX_LATCH_VALUE);
    if(r != 0) return r;
    usleep(LT6911C_RX_SETTLE_US);
    struct lt6911_res rx;
    r = read_res_locked(dev, &lt6911c_rx_regs, &rx);
    if(r != 0) return r;
    // A size with a zero side is not a signal.
    *present = rx.width != 0 && rx.height != 0;
    return 0;
}

static int c_csi_res(struct lt6911_dev *dev, struct lt6911_res *res)
{
    return read_res_locked(dev, &lt6911c_csi_regs, res);
}

// ---- UXC --------------------------------------------------------------------

static int uxc_enable(struct lt6911_dev *dev)
{
    int r = c_enable(dev);
    if(r != 0) return r;
    return write8_locked(dev, LT6911_PAGE_SYS, LT6911UXC_REG_WATCHDOG, LT6911UXC_WATCHDOG_OFF);
}

static int uxc_signal_present(struct lt6911_dev *dev, bool *present)
{
    uint8_t state;
    int r = read8_locked(dev, LT6911UXC_PAGE_HDMI, LT6911UXC_REG_SIGNAL, &state);
    if(r != 0) return r;
    // 0x55 = a stable signal; 0x88 = gone; anything else is not a lock either.
    *present = state == LT6911UXC_SIGNAL_STABLE;
    return 0;
}

static int uxc_csi_res(struct lt6911_dev *dev, struct lt6911_res *res)
{
    return read_res_locked(dev, &lt6911uxc_csi_regs, res);
}

// ---- D ----------------------------------------------------------------------

static int d_signal_present(struct lt6911_dev *dev, bool *present)
{
    struct lt6911_res res;
    int r = read_res_locked(dev, &lt6911d_regs, &res);
    if(r != 0) return r;
    // The old D code said present if EITHER side was non-zero while C said both.
    // A size with a zero side cannot be streamed, so both is the rule for all.
    *present = res.width != 0 && res.height != 0;
    return 0;
}

static int d_csi_res(struct lt6911_dev *dev, struct lt6911_res *res)
{
    return read_res_locked(dev, &lt6911d_regs, res);
}

// ---- the table ---------------------------------------------------------------

// Address: every row is LT6911_I2C_ADDR. S15kvmhwd's detection calls an ACK at
// 0x2C "LT6911C" and an ACK at 0x2B (and none at 0x2C) UXC or D, which reads as
// if C sat at a different address; yet kvm_vision and the EDID tool have always
// talked to a C at 0x2B (the EDID tool even reads the C's chip id 0x1605 there).
// Both facts are in the tree and neither settles which address a C answers at
// (it may answer at both). Behaviour is kept: 0x2B everywhere. OPEN QUESTION,
// to be answered on a C board with i2cdetect.
static const struct lt6911_ops lt6911_ops_table[] = {
    [LT6911_UNKNOWN] = { "unknown", LT6911_I2C_ADDR, LT6911_INT_NONE,
                         NULL, NULL, NULL, NULL },
    [LT6911_C]   = { "LT6911C",   LT6911_I2C_ADDR, LT6911_INT_RISING,
                     c_enable,   c_disable, c_signal_present,   c_csi_res },
    [LT6911_UXC] = { "LT6911UXC", LT6911_I2C_ADDR, LT6911_INT_FALLING,
                     uxc_enable, c_disable, uxc_signal_present, uxc_csi_res },
    // D has no enable/disable: its registers are always reachable.
    [LT6911_D]   = { "LT6911D",   LT6911_I2C_ADDR, LT6911_INT_FALLING,
                     NULL,       NULL,      d_signal_present,   d_csi_res },
    // UXE: nothing to read; the caller assumes the mode.
    [LT6911_UXE] = { "LT6911UXE", LT6911_I2C_ADDR, LT6911_INT_NONE,
                     NULL, NULL, NULL, NULL },
};

#define LT6911_VARIANT_COUNT (sizeof(lt6911_ops_table) / sizeof(lt6911_ops_table[0]))

// ---- variant -----------------------------------------------------------------

enum lt6911_variant lt6911_variant_parse(const char *text)
{
    // The marker S15kvmhwd writes (kvm_hdmi_version_detect).
    if(strcmp(text, "c") == 0)  return LT6911_C;
    if(strcmp(text, "ux") == 0) return LT6911_UXC;
    if(strcmp(text, "d") == 0)  return LT6911_D;
    if(strcmp(text, "ue") == 0) return LT6911_UXE;
    return LT6911_UNKNOWN;
}

const char *lt6911_variant_name(enum lt6911_variant variant)
{
    if((size_t)variant >= LT6911_VARIANT_COUNT) return lt6911_ops_table[LT6911_UNKNOWN].name;
    return lt6911_ops_table[variant].name;
}

static enum lt6911_variant variant_read(void)
{
    FILE *fp = fopen(LT6911_VARIANT_FILE, "r");
    if(fp == NULL){
        fprintf(stderr, "[lt6911]open %s failed: %d\n", LT6911_VARIANT_FILE, errno);
        return LT6911_UNKNOWN;
    }
    char text[16] = {0};
    char *got = fgets(text, sizeof(text), fp);
    int read_err = ferror(fp) ? errno : 0;
    fclose(fp);
    if(got == NULL){
        fprintf(stderr, "[lt6911]read %s failed: %d\n", LT6911_VARIANT_FILE, read_err);
        return LT6911_UNKNOWN;
    }
    size_t n = strlen(text);
    while(n > 0 && isspace((unsigned char)text[n - 1])) text[--n] = '\0';
    enum lt6911_variant v = lt6911_variant_parse(text);
    if(v == LT6911_UNKNOWN)
        fprintf(stderr, "[lt6911]%s holds \"%s\": not a known variant\n", LT6911_VARIANT_FILE, text);
    return v;
}

// ---- public ------------------------------------------------------------------

int lt6911_open(struct lt6911_dev *dev)
{
    dev->fd = -1;
    dev->variant = LT6911_UNKNOWN;
    dev->ops = &lt6911_ops_table[LT6911_UNKNOWN];

    enum lt6911_variant v = variant_read();
    if(v == LT6911_UNKNOWN) return -ENODEV;

    int fd = open(LT6911_I2C_BUS_PATH, O_RDWR | O_CLOEXEC);
    if(fd < 0){
        int err = errno;
        fprintf(stderr, "[lt6911]open %s failed: %d\n", LT6911_I2C_BUS_PATH, err);
        return -err;
    }
    unsigned long funcs = 0;
    if(ioctl(fd, I2C_FUNCS, &funcs) < 0){
        int err = errno;
        fprintf(stderr, "[lt6911]I2C_FUNCS on %s failed: %d\n", LT6911_I2C_BUS_PATH, err);
        close(fd);
        return -err;
    }
    if(!(funcs & I2C_FUNC_I2C)){
        fprintf(stderr, "[lt6911]%s cannot do plain I2C\n", LT6911_I2C_BUS_PATH);
        close(fd);
        return -ENOTSUP;
    }
    dev->fd = fd;
    dev->variant = v;
    dev->ops = &lt6911_ops_table[v];
    fprintf(stderr, "[lt6911]%s on %s\n", dev->ops->name, LT6911_I2C_BUS_PATH);
    return 0;
}

void lt6911_close(struct lt6911_dev *dev)
{
    if(dev->fd >= 0) close(dev->fd);
    dev->fd = -1;
    dev->variant = LT6911_UNKNOWN;
    dev->ops = &lt6911_ops_table[LT6911_UNKNOWN];
}

static bool usable(const struct lt6911_dev *dev)
{
    return dev->fd >= 0 && dev->variant != LT6911_UNKNOWN;
}

int lt6911_reg_read(struct lt6911_dev *dev, uint8_t page, uint8_t reg,
                    uint8_t *out, size_t len)
{
    if(!usable(dev) || reg == LT6911_REG_PAGE) return -EINVAL;
    if(tls_in_session) return read_locked(dev, page, reg, out, len);
    int r = lock_acquire();
    if(r != 0) return r;
    r = read_locked(dev, page, reg, out, len);
    int u = lock_release();
    return r != 0 ? r : u;
}

int lt6911_reg_write(struct lt6911_dev *dev, uint8_t page, uint8_t reg,
                     const uint8_t *data, size_t len)
{
    if(!usable(dev) || reg == LT6911_REG_PAGE) return -EINVAL;
    if(tls_in_session) return write_locked(dev, page, reg, data, len);
    int r = lock_acquire();
    if(r != 0) return r;
    r = write_locked(dev, page, reg, data, len);
    int u = lock_release();
    return r != 0 ? r : u;
}

int lt6911_session_begin(struct lt6911_dev *dev)
{
    if(!usable(dev)) return -EINVAL;
    if(tls_in_session) return -EBUSY;
    int r = lock_acquire();
    if(r != 0) return r;
    tls_in_session = true;
    return 0;
}

int lt6911_session_end(struct lt6911_dev *dev, bool changed)
{
    (void)dev;
    if(!tls_in_session) return -EINVAL;
    tls_in_session = false;
    int r = lock_release();
    if(!changed) return r;
    // After the unlock, so the state machine's probe does not queue behind us.
    // Any write to the file is an IN_MODIFY for lt6911_watch fds; the content
    // is only a changing stamp.
    int64_t stamp = now_ms();
    if(stamp < 0) return r != 0 ? r : -errno;
    ssize_t w = pwrite(lt6911_lock_fd, &stamp, sizeof(stamp), 0);
    int err = 0;
    if(w < 0) err = -errno;
    else if((size_t)w != sizeof(stamp)) err = -EIO;
    if(err != 0) fprintf(stderr, "[lt6911]change notice on %s failed: %d\n", LT6911_LOCK_PATH, -err);
    return r != 0 ? r : err;
}

int lt6911_watch_open(void)
{
    int r = pthread_mutex_lock(&lt6911_lock);
    if(r != 0) return -r;
    r = lock_file_open_locked();
    int u = pthread_mutex_unlock(&lt6911_lock);
    if(r != 0) return r;
    if(u != 0) return -u;
    int fd = inotify_init1(IN_NONBLOCK | IN_CLOEXEC);
    if(fd < 0) return -errno;
    if(inotify_add_watch(fd, LT6911_LOCK_PATH, IN_MODIFY) < 0){
        int err = errno;
        close(fd);
        return -err;
    }
    return fd;
}

int lt6911_watch_changed(int fd)
{
    int seen = 0;
    for(;;){
        char buf[sizeof(struct inotify_event) + NAME_MAX + 1];
        ssize_t n = read(fd, buf, sizeof(buf));
        if(n > 0){
            seen = 1;
            continue;
        }
        if(n == 0) return seen;
        if(errno == EINTR) continue;
        if(errno == EAGAIN || errno == EWOULDBLOCK) return seen;
        return -errno;
    }
}

int lt6911_reg_read8(struct lt6911_dev *dev, uint8_t page, uint8_t reg, uint8_t *out)
{
    return lt6911_reg_read(dev, page, reg, out, 1);
}

int lt6911_reg_write8(struct lt6911_dev *dev, uint8_t page, uint8_t reg, uint8_t val)
{
    return lt6911_reg_write(dev, page, reg, &val, 1);
}

int lt6911_probe(struct lt6911_dev *dev, struct lt6911_probe_result *out)
{
    out->present = false;
    out->csi.width = 0;
    out->csi.height = 0;
    if(!usable(dev)) return -EINVAL;
    const struct lt6911_ops *ops = dev->ops;
    if(ops->signal_present == NULL || ops->csi_res == NULL) return -ENOTSUP;

    bool own_lock = !tls_in_session;
    int result = 0;
    if(own_lock) result = lock_acquire();
    if(result != 0){
        fprintf(stderr, "[lt6911]%s probe failed: %d\n", ops->name, result);
        return result;
    }
    if(ops->enable != NULL) result = ops->enable(dev);
    if(result == 0){
        bool present = false;
        result = ops->signal_present(dev, &present);
        if(result == 0 && present){
            struct lt6911_res csi = {0, 0};
            result = ops->csi_res(dev, &csi);
            if(result == 0){
                out->present = true;
                out->csi = csi;
            }
        }
    }
    // The chip is put back even when a query failed; the first error stands.
    if(ops->disable != NULL){
        int d = ops->disable(dev);
        if(result == 0) result = d;
    }
    if(own_lock){
        int u = lock_release();
        if(result == 0) result = u;
    }
    if(result != 0){
        out->present = false;
        fprintf(stderr, "[lt6911]%s probe failed: %d\n", ops->name, result);
    }
    return result;
}
