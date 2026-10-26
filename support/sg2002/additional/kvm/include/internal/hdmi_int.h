// LT6911 INT line as a GPIO character device v2 line-event fd. Header-only and
// plain C so libkvm (kvm_vision.cpp) and the emulator harness
// (tools/hdmiint) run the very same code.
#ifndef HDMI_INT_H
#define HDMI_INT_H

#include <dirent.h>
#include <errno.h>
#include <fcntl.h>
#include <stdbool.h>
#include <stdint.h>
#include <stdio.h>
#include <string.h>
#include <sys/ioctl.h>
#include <unistd.h>

// ---- LT6911 INT line: GPIO character device (v2 uABI) ----------------------
//
// The LT6911 INT pin is vendor GPIO 353. The vendor kernel numbers the SG2002
// banks downward in 32-line blocks (GPIOA 480.., GPIOB 448.., GPIOC 416..,
// GPIOD 384.., then the RTC-domain bank 352..), so 353 is bank "RTC" offset 1.
// The number is a vendor-kernel artefact and is NOT used here: the line is found by
// its device-tree line NAME (hdmi_int_line_name, a gpio-line-names entry the
// board DT must provide), or by an explicit "<gpiochip name> <offset>" pair in
// hdmi_int_override_path. Neither found = no INT source = fail closed.
//
// Structs are declared here (layout of linux/gpio.h v2) so the build does not
// depend on the toolchain's kernel headers being 5.10 or newer.

#define hdmi_int_line_name      "hdmi-int"
#define hdmi_int_override_path  "/etc/kvm/hdmi_int_line"

#define GPIO_IOCTL_TYPE         0xB4
#define GPIO_MAX_NAME           32

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

struct nk_gpio_v2_line_event {
    uint64_t timestamp_ns;
    uint32_t id;
    uint32_t offset;
    uint32_t seqno;
    uint32_t line_seqno;
    uint32_t padding[6];
};

#define NK_GPIO_GET_CHIPINFO_IOCTL     _IOR(GPIO_IOCTL_TYPE, 0x01, struct nk_gpiochip_info)
#define NK_GPIO_V2_GET_LINEINFO_IOCTL  _IOWR(GPIO_IOCTL_TYPE, 0x05, struct nk_gpio_v2_line_info)
#define NK_GPIO_V2_GET_LINE_IOCTL      _IOWR(GPIO_IOCTL_TYPE, 0x07, struct nk_gpio_v2_line_request)

#define NK_GPIO_V2_LINE_FLAG_INPUT         (1ULL << 2)
#define NK_GPIO_V2_LINE_FLAG_EDGE_RISING   (1ULL << 4)
#define NK_GPIO_V2_LINE_FLAG_EDGE_FALLING  (1ULL << 5)
#define NK_GPIO_V2_LINE_EVENT_RISING_EDGE  1U
#define NK_GPIO_V2_LINE_EVENT_FALLING_EDGE 2U

// Requests chip/offset for both edges. Returns the line event fd or -1.
static inline int hdmi_int_request(int chip_fd, uint32_t offset)
{
    struct nk_gpio_v2_line_request req;
    memset(&req, 0, sizeof(req));
    req.offsets[0] = offset;
    snprintf(req.consumer, sizeof(req.consumer), "kvm-hdmi-int");
    req.num_lines = 1;
    req.config.flags = NK_GPIO_V2_LINE_FLAG_INPUT | NK_GPIO_V2_LINE_FLAG_EDGE_RISING |
                       NK_GPIO_V2_LINE_FLAG_EDGE_FALLING;
    if(ioctl(chip_fd, NK_GPIO_V2_GET_LINE_IOCTL, &req) < 0) return -1;
    return req.fd;
}

// Opens the line event fd for the LT6911 INT pin. Every failure is logged to
// stderr (not debug()) with its reason; returns -1 then.
static inline int hdmi_int_open(void)
{
    // 1. explicit "<gpiochip name> <offset>" override
    FILE *fp = fopen(hdmi_int_override_path, "r");
    if(fp != NULL){
        char chip[GPIO_MAX_NAME + 8] = {0};
        unsigned offset = 0;
        int n = fscanf(fp, "%39s %u", chip, &offset);
        fclose(fp);
        if(n != 2){
            fprintf(stderr, "[hdmi]%s must hold \"<gpiochip name> <offset>\"\n", hdmi_int_override_path);
            return -1;
        }
        char path[64];
        snprintf(path, sizeof(path), "/dev/%s", chip);
        int cfd = open(path, O_RDONLY | O_CLOEXEC);
        if(cfd < 0){
            fprintf(stderr, "[hdmi]open %s failed: %d\n", path, errno);
            return -1;
        }
        int lfd = hdmi_int_request(cfd, offset);
        int err = errno;
        close(cfd);
        if(lfd < 0) fprintf(stderr, "[hdmi]INT line %s %u request failed: %d\n", chip, offset, err);
        else fprintf(stderr, "[hdmi]INT line %s offset %u (override)\n", chip, offset);
        return lfd;
    }

    // 2. the DT line name, searched over every gpiochip
    DIR *dir = opendir("/dev");
    if(dir == NULL){
        fprintf(stderr, "[hdmi]opendir /dev failed: %d\n", errno);
        return -1;
    }
    int result = -1;
    int scanned = 0;
    struct dirent *de;
    while((de = readdir(dir)) != NULL){
        if(strncmp(de->d_name, "gpiochip", 8) != 0) continue;
        char path[300];
        snprintf(path, sizeof(path), "/dev/%s", de->d_name);
        int cfd = open(path, O_RDONLY | O_CLOEXEC);
        if(cfd < 0){
            fprintf(stderr, "[hdmi]open %s failed: %d\n", path, errno);
            continue;
        }
        struct nk_gpiochip_info ci;
        memset(&ci, 0, sizeof(ci));
        if(ioctl(cfd, NK_GPIO_GET_CHIPINFO_IOCTL, &ci) < 0){
            fprintf(stderr, "[hdmi]chip info %s failed: %d\n", path, errno);
            close(cfd);
            continue;
        }
        scanned++;
        for(uint32_t line = 0; line < ci.lines; line++){
            struct nk_gpio_v2_line_info li;
            memset(&li, 0, sizeof(li));
            li.offset = line;
            if(ioctl(cfd, NK_GPIO_V2_GET_LINEINFO_IOCTL, &li) < 0) break;
            if(strncmp(li.name, hdmi_int_line_name, GPIO_MAX_NAME) != 0) continue;
            result = hdmi_int_request(cfd, line);
            if(result < 0) fprintf(stderr, "[hdmi]INT line \"%s\" (%s offset %u) request failed: %d\n",
                                   hdmi_int_line_name, ci.name, line, errno);
            else fprintf(stderr, "[hdmi]INT line \"%s\" = %s offset %u\n", hdmi_int_line_name, ci.name, line);
            break;
        }
        close(cfd);
        if(result >= 0) break;
    }
    closedir(dir);
    if(result < 0 && scanned == 0)
        fprintf(stderr, "[hdmi]no gpiochip readable under /dev\n");
    return result;
}

// Drains the line event fd (non-blocking). Returns true when the variant's
// edge (C: rising, UXC/D: falling) was seen.
static inline bool hdmi_int_edge_seen(int fd, uint8_t hdmi_version)
{
    struct nk_gpio_v2_line_event ev[16];
    bool seen = false;
    for(;;){
        ssize_t n = read(fd, ev, sizeof(ev));
        if(n < 0){
            if(errno == EINTR) continue;
            if(errno != EAGAIN) fprintf(stderr, "[hdmi]line event read failed: %d\n", errno);
            return seen;
        }
        if(n == 0 || n % (ssize_t)sizeof(ev[0]) != 0) return seen;
        for(size_t i = 0; i < (size_t)n / sizeof(ev[0]); i++){
            if(hdmi_version == 0 && ev[i].id == NK_GPIO_V2_LINE_EVENT_RISING_EDGE) seen = true;
            if((hdmi_version == 1 || hdmi_version == 3) &&
               ev[i].id == NK_GPIO_V2_LINE_EVENT_FALLING_EDGE) seen = true;
        }
    }
}

#endif
