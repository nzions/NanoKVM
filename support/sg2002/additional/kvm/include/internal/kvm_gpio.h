// GPIO lines by device-tree line NAME, over the GPIO character device (v2
// uABI). Header-only plain C so libkvm (kvm_vision.cpp, hdmi_int.h), the
// nanokvm_update_edid tool and the line owner (tools/kvmgpio) run the very
// same lookup.
//
// The vendor kernel numbers lines globally (sysfs: 451, 502, ...); that number
// is a vendor-kernel artefact and differs on the mainline kernel (dynamic
// bases). Nothing here knows a number: a line is found by the gpio-line-names
// entry the board DT provides (hdmi-int, hdmi-rst, oled-rst, pwr-key, pwr-led,
// rst-key, wifi-en). A name found on no chip is NK_GPIO_NO_LINE (the caller may
// then fall back to the vendor sysfs path, only because there is no named
// line); a name found twice or any I/O failure is an error, never a guess.
//
// A line request lives as long as its fd, so an output that must stay driven
// (hdmi-rst high, oled-rst high, wifi-en) has an owner process: tools/kvmgpio
// "daemon" (started by S15kvmhwd). Others change a held line through its unix
// socket (nk_gpio_daemon_set) because a line can only be requested once.
//
// Structs are declared here (layout of linux/gpio.h v2) so the build does not
// depend on the toolchain's kernel headers being 5.10 or newer.
#ifndef KVM_GPIO_H
#define KVM_GPIO_H

#include <dirent.h>
#include <errno.h>
#include <fcntl.h>
#include <stdbool.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/ioctl.h>
#include <sys/socket.h>
#include <sys/un.h>
#include <unistd.h>

#define GPIO_IOCTL_TYPE         0xB4
#define GPIO_MAX_NAME           32

#define NK_GPIO_NO_LINE         (-2)   // no chip carries the name
#define NK_GPIO_AMBIGUOUS       (-3)   // two lines carry the name

#define nk_gpio_sock_path       "/var/run/kvmgpio.sock"

struct nk_gpiochip_info {
    char name[GPIO_MAX_NAME];
    char label[GPIO_MAX_NAME];
    uint32_t lines;
};

struct nk_gpio_v2_line_attribute {
    uint32_t id;
    uint32_t padding;
    uint64_t value;
};

struct nk_gpio_v2_line_config_attribute {
    struct nk_gpio_v2_line_attribute attr;
    uint64_t mask;
};

struct nk_gpio_v2_line_config {
    uint64_t flags;
    uint32_t num_attrs;
    uint32_t padding[5];
    struct nk_gpio_v2_line_config_attribute attrs[10];
};

struct nk_gpio_v2_line_request {
    uint32_t offsets[64];
    char consumer[GPIO_MAX_NAME];
    struct nk_gpio_v2_line_config config;
    uint32_t num_lines;
    uint32_t event_buffer_size;
    uint32_t padding[5];
    int32_t fd;
};

struct nk_gpio_v2_line_info {
    char name[GPIO_MAX_NAME];
    char consumer[GPIO_MAX_NAME];
    uint32_t offset;
    uint32_t num_attrs;
    uint64_t flags;
    struct nk_gpio_v2_line_attribute attrs[10];
    uint32_t padding[4];
};

struct nk_gpio_v2_line_values {
    uint64_t bits;
    uint64_t mask;
};

struct nk_gpio_v2_line_event {
    uint64_t timestamp_ns;
    uint32_t id;
    uint32_t offset;
    uint32_t seqno;
    uint32_t line_seqno;
    uint32_t padding[6];
};

#define NK_GPIO_GET_CHIPINFO_IOCTL       _IOR(GPIO_IOCTL_TYPE, 0x01, struct nk_gpiochip_info)
#define NK_GPIO_V2_GET_LINEINFO_IOCTL    _IOWR(GPIO_IOCTL_TYPE, 0x05, struct nk_gpio_v2_line_info)
#define NK_GPIO_V2_GET_LINE_IOCTL        _IOWR(GPIO_IOCTL_TYPE, 0x07, struct nk_gpio_v2_line_request)
#define NK_GPIO_V2_LINE_GET_VALUES_IOCTL _IOWR(GPIO_IOCTL_TYPE, 0x0E, struct nk_gpio_v2_line_values)
#define NK_GPIO_V2_LINE_SET_VALUES_IOCTL _IOWR(GPIO_IOCTL_TYPE, 0x0F, struct nk_gpio_v2_line_values)

#define NK_GPIO_V2_LINE_FLAG_INPUT         (1ULL << 2)
#define NK_GPIO_V2_LINE_FLAG_OUTPUT        (1ULL << 3)
#define NK_GPIO_V2_LINE_FLAG_EDGE_RISING   (1ULL << 4)
#define NK_GPIO_V2_LINE_FLAG_EDGE_FALLING  (1ULL << 5)
#define NK_GPIO_V2_LINE_ATTR_ID_OUTPUT_VALUES 2U
#define NK_GPIO_V2_LINE_EVENT_RISING_EDGE  1U
#define NK_GPIO_V2_LINE_EVENT_FALLING_EDGE 2U

// Requests one line of an open chip. flags: INPUT[|edges] or OUTPUT; for an
// output, value is its initial level. Returns the line fd or -1 (errno set).
static inline int nk_gpio_request(int chip_fd, uint32_t offset, const char *consumer,
                                  uint64_t flags, int value)
{
    struct nk_gpio_v2_line_request req;
    memset(&req, 0, sizeof(req));
    req.offsets[0] = offset;
    snprintf(req.consumer, sizeof(req.consumer), "%s", consumer);
    req.num_lines = 1;
    req.config.flags = flags;
    if(flags & NK_GPIO_V2_LINE_FLAG_OUTPUT){
        req.config.num_attrs = 1;
        req.config.attrs[0].attr.id = NK_GPIO_V2_LINE_ATTR_ID_OUTPUT_VALUES;
        req.config.attrs[0].attr.value = value ? 1 : 0;
        req.config.attrs[0].mask = 1;
    }
    if(ioctl(chip_fd, NK_GPIO_V2_GET_LINE_IOCTL, &req) < 0) return -1;
    return req.fd;
}

// Drives a requested output line. 0 or -1 (errno set).
static inline int nk_gpio_line_set(int line_fd, int value)
{
    struct nk_gpio_v2_line_values v = { value ? 1ULL : 0ULL, 1ULL };
    return ioctl(line_fd, NK_GPIO_V2_LINE_SET_VALUES_IOCTL, &v) < 0 ? -1 : 0;
}

// Reads a requested line. 0, 1 or -1 (errno set).
static inline int nk_gpio_line_get(int line_fd)
{
    struct nk_gpio_v2_line_values v = { 0ULL, 1ULL };
    if(ioctl(line_fd, NK_GPIO_V2_LINE_GET_VALUES_IOCTL, &v) < 0) return -1;
    return (int)(v.bits & 1);
}

// Finds the line called name over every /dev/gpiochip*. On success returns the
// open chip fd (caller closes) with *offset set. NK_GPIO_NO_LINE: every chip was
// read and none carries the name. NK_GPIO_AMBIGUOUS: two do. -1: a chip could
// not be read (logged to stderr with the reason; errno set).
static inline int nk_gpio_find(const char *name, uint32_t *offset)
{
    DIR *dir = opendir("/dev");
    if(dir == NULL){
        fprintf(stderr, "[gpio]opendir /dev failed: %d\n", errno);
        return -1;
    }
    int found = -1, matches = 0, failed = 0, scanned = 0;
    struct dirent *de;
    while((de = readdir(dir)) != NULL){
        if(strncmp(de->d_name, "gpiochip", 8) != 0) continue;
        char path[300];
        snprintf(path, sizeof(path), "/dev/%s", de->d_name);
        int cfd = open(path, O_RDWR | O_CLOEXEC);
        if(cfd < 0){
            fprintf(stderr, "[gpio]open %s failed: %d\n", path, errno);
            failed = 1;
            continue;
        }
        struct nk_gpiochip_info ci;
        memset(&ci, 0, sizeof(ci));
        if(ioctl(cfd, NK_GPIO_GET_CHIPINFO_IOCTL, &ci) < 0){
            fprintf(stderr, "[gpio]chip info %s failed: %d\n", path, errno);
            failed = 1;
            close(cfd);
            continue;
        }
        scanned++;
        bool keep = false;
        for(uint32_t line = 0; line < ci.lines; line++){
            struct nk_gpio_v2_line_info li;
            memset(&li, 0, sizeof(li));
            li.offset = line;
            if(ioctl(cfd, NK_GPIO_V2_GET_LINEINFO_IOCTL, &li) < 0){
                fprintf(stderr, "[gpio]line info %s %u failed: %d\n", path, line, errno);
                failed = 1;
                break;
            }
            if(strncmp(li.name, name, GPIO_MAX_NAME) != 0) continue;
            matches++;
            if(!keep && found < 0){
                found = cfd;
                *offset = line;
                keep = true;
            }
        }
        if(!keep) close(cfd);
    }
    closedir(dir);
    if(matches > 1){
        fprintf(stderr, "[gpio]line \"%s\" is named %d times: refusing to pick one\n", name, matches);
        if(found >= 0) close(found);
        return NK_GPIO_AMBIGUOUS;
    }
    if(matches == 1) return found;
    if(failed){
        errno = EIO;
        return -1;
    }
    if(scanned == 0) fprintf(stderr, "[gpio]no gpiochip readable under /dev\n");
    return NK_GPIO_NO_LINE;
}

// Finds name and requests it. Returns the line fd, NK_GPIO_NO_LINE,
// NK_GPIO_AMBIGUOUS or -1 (logged).
static inline int nk_gpio_open_named(const char *name, const char *consumer,
                                     uint64_t flags, int value)
{
    uint32_t offset = 0;
    int cfd = nk_gpio_find(name, &offset);
    if(cfd < 0) return cfd;
    int lfd = nk_gpio_request(cfd, offset, consumer, flags, value);
    int err = errno;
    close(cfd);
    if(lfd < 0){
        fprintf(stderr, "[gpio]request \"%s\" (offset %u) failed: %d\n", name, offset, err);
        errno = err;
    }
    return lfd;
}

// ---- the owner's socket ----------------------------------------------------
// One request per connection: "set <name> <0|1>\n", "get <name>\n", "ping\n".
// Reply: "ok[ <value>]\n" or "err <reason>\n".

// Sends cmd to the owner. 0: reply in reply[] (NUL terminated). -1: no owner
// reachable (errno set: ENOENT / ECONNREFUSED / ...).
static inline int nk_gpio_daemon_call(const char *cmd, char *reply, size_t reply_len)
{
    int fd = socket(AF_UNIX, SOCK_STREAM | SOCK_CLOEXEC, 0);
    if(fd < 0) return -1;
    struct sockaddr_un sa;
    memset(&sa, 0, sizeof(sa));
    sa.sun_family = AF_UNIX;
    snprintf(sa.sun_path, sizeof(sa.sun_path), "%s", nk_gpio_sock_path);
    if(connect(fd, (struct sockaddr *)&sa, sizeof(sa)) < 0){
        int err = errno;
        close(fd);
        errno = err;
        return -1;
    }
    size_t n = strlen(cmd);
    if(write(fd, cmd, n) != (ssize_t)n){
        int err = errno;
        close(fd);
        errno = err;
        return -1;
    }
    size_t got = 0;
    while(got + 1 < reply_len){
        ssize_t r = read(fd, reply + got, reply_len - 1 - got);
        if(r < 0 && errno == EINTR) continue;
        if(r <= 0) break;
        got += (size_t)r;
        if(reply[got - 1] == '\n') break;
    }
    reply[got] = 0;
    close(fd);
    return 0;
}

// Drives a line the owner holds. 0 done; -1 no owner reachable (errno set);
// -2 the owner refused (reason on stderr).
static inline int nk_gpio_daemon_set(const char *name, int value)
{
    char cmd[96], reply[96];
    snprintf(cmd, sizeof(cmd), "set %s %d\n", name, value ? 1 : 0);
    if(nk_gpio_daemon_call(cmd, reply, sizeof(reply)) < 0) return -1;
    if(strncmp(reply, "ok", 2) == 0) return 0;
    fprintf(stderr, "[gpio]owner refused \"%s\": %s", cmd, reply);
    return -2;
}


// ---- vendor-kernel fallback ---------------------------------------------------
// Only when NO line carries the name (the vendor kernel's DT has no
// gpio-line-names) is the vendor sysfs number used. A line that exists but has
// no owner, an ambiguous name or a failing chip is an error, never a guess.

static inline int nk_gpio_sysfs_write(int pin, const char *file, const char *text)
{
    char path[64];
    snprintf(path, sizeof(path), "/sys/class/gpio/gpio%d/%s", pin, file);
    FILE *fp = fopen(path, "w");
    if(fp == NULL){
        fprintf(stderr, "[gpio]open %s failed: %d\n", path, errno);
        return -1;
    }
    int rc = (fputs(text, fp) < 0) ? -1 : 0;
    if(fclose(fp) != 0) rc = -1;
    if(rc != 0) fprintf(stderr, "[gpio]write %s failed: %d\n", path, errno);
    return rc;
}

// Drives the held output "name" through its owner (tools/kvmgpio daemon); on a
// vendor kernel (no such named line) drives sysfs gpio<pin> instead.
// 0 done, -1 failed (logged).
static inline int nk_gpio_set_held(const char *name, int sysfs_pin, int value)
{
    int r = nk_gpio_daemon_set(name, value);
    if(r == 0) return 0;
    if(r == -2) return -1;
    uint32_t offset = 0;
    int cfd = nk_gpio_find(name, &offset);
    if(cfd >= 0){
        close(cfd);
        fprintf(stderr, "[gpio]line \"%s\" exists but nothing owns it (%s not running)\n", name, nk_gpio_sock_path);
        return -1;
    }
    if(cfd != NK_GPIO_NO_LINE) return -1;
    char path[64];
    snprintf(path, sizeof(path), "/sys/class/gpio/gpio%d", sysfs_pin);
    if(access(path, F_OK) != 0){
        FILE *fp = fopen("/sys/class/gpio/export", "w");
        if(fp == NULL){
            fprintf(stderr, "[gpio]no line \"%s\" and no sysfs gpio: %d\n", name, errno);
            return -1;
        }
        fprintf(fp, "%d", sysfs_pin);
        fclose(fp);
        if(nk_gpio_sysfs_write(sysfs_pin, "direction", "out") != 0) return -1;
    }
    return nk_gpio_sysfs_write(sysfs_pin, "value", value ? "1" : "0");
}

#endif
