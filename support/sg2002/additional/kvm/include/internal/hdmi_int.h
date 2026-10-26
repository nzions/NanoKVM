// LT6911 INT line as a GPIO character device v2 line-event fd. Header-only and
// plain C so libkvm (kvm_vision.cpp) and the emulator harness
// (tools/hdmiint) run the very same code. The by-name line lookup itself is
// kvm_gpio.h's (shared with every other GPIO user).
#ifndef HDMI_INT_H
#define HDMI_INT_H

#include "kvm_gpio.h"
#include "lt6911.h"

// ---- LT6911 INT line: GPIO character device (v2 uABI) ----------------------
//
// The LT6911 INT pin is vendor GPIO 353. The vendor kernel numbers the SG2002
// banks downward in 32-line blocks (GPIOA 480.., GPIOB 448.., GPIOC 416..,
// GPIOD 384.., then the RTC-domain bank 352..), so 353 is bank "RTC" offset 1.
// The number is a vendor-kernel artefact and is NOT used here: the line is found by
// its device-tree line NAME (hdmi_int_line_name, a gpio-line-names entry the
// board DT must provide), or by an explicit "<gpiochip name> <offset>" pair in
// hdmi_int_override_path. Neither found = no INT source = fail closed.

#define hdmi_int_line_name      "hdmi-int"
#define hdmi_int_override_path  "/etc/kvm/hdmi_int_line"

// Requests chip/offset for both edges. Returns the line event fd or -1.
static inline int hdmi_int_request(int chip_fd, uint32_t offset)
{
    return nk_gpio_request(chip_fd, offset, "kvm-hdmi-int",
                           NK_GPIO_V2_LINE_FLAG_INPUT | NK_GPIO_V2_LINE_FLAG_EDGE_RISING |
                           NK_GPIO_V2_LINE_FLAG_EDGE_FALLING, 0);
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
    uint32_t line = 0;
    int cfd = nk_gpio_find(hdmi_int_line_name, &line);
    if(cfd == NK_GPIO_NO_LINE){
        fprintf(stderr, "[hdmi]no gpio line named \"%s\"\n", hdmi_int_line_name);
        return -1;
    }
    if(cfd < 0) return -1;
    int result = hdmi_int_request(cfd, line);
    int err = errno;
    close(cfd);
    if(result < 0) fprintf(stderr, "[hdmi]INT line \"%s\" (offset %u) request failed: %d\n",
                           hdmi_int_line_name, line, err);
    else fprintf(stderr, "[hdmi]INT line \"%s\" = offset %u\n", hdmi_int_line_name, line);
    return result;
}

// Drains the line event fd (non-blocking). Returns true when `edge` (the
// variant's lt6911_ops.int_edge: C rising, UXC/D falling) was seen. LT6911_INT_NONE
// sees nothing.
static inline bool hdmi_int_edge_seen(int fd, enum lt6911_int_edge edge)
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
            if(edge == LT6911_INT_RISING && ev[i].id == NK_GPIO_V2_LINE_EVENT_RISING_EDGE) seen = true;
            if(edge == LT6911_INT_FALLING && ev[i].id == NK_GPIO_V2_LINE_EVENT_FALLING_EDGE) seen = true;
        }
    }
}

#endif
