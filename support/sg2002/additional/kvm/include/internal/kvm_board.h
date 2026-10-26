// The board this device is, read ONCE from /etc/kvm/hw (written last by
// S15kvmhwd's detection: "alpha", "beta" or "pcie"). Header-only plain C so
// libkvm, kvm_system (a separate process) and nanokvm_update_edid run the very
// same parser; nobody else opens the file.
//
// An unreadable, empty or unrecognised file is reported on stderr and the board
// is KVM_BOARD_UNKNOWN, which has no reset line: a board that cannot be named
// is never guessed to be one that can be reset.
#ifndef KVM_BOARD_H
#define KVM_BOARD_H

#include <errno.h>
#include <pthread.h>
#include <stdbool.h>
#include <stdio.h>
#include <string.h>

#define KVM_BOARD_FILE "/etc/kvm/hw"

// How long the LT6911 reset is held low, and how long the chip is given to
// come back after the release. One value for every user of the line.
#define KVM_HDMI_RESET_PULSE_US 100000

enum kvm_board_kind {
    KVM_BOARD_UNKNOWN = 0,   // file missing / unreadable / not recognised
    KVM_BOARD_CUBE_ALPHA,    // "alpha"
    KVM_BOARD_CUBE_BETA,     // "beta"
    KVM_BOARD_PCIE,          // "pcie"
};

struct kvm_board {
    enum kvm_board_kind kind;
    const char *name;                  // for logs
    bool has_hdmi_reset;               // a software-driven LT6911 reset line exists
    const char *hdmi_reset_line;       // DT gpio-line-names entry; NULL without a reset line
    int hdmi_reset_sysfs_fallback;     // vendor-kernel sysfs pin, used only when no DT line carries the name
};

static const struct kvm_board kvm_boards[] = {
    { KVM_BOARD_UNKNOWN,    "unknown",    false, NULL,       -1  },
    { KVM_BOARD_CUBE_ALPHA, "cube-alpha", false, NULL,       -1  },
    { KVM_BOARD_CUBE_BETA,  "cube-beta",  false, NULL,       -1  },
    { KVM_BOARD_PCIE,       "pcie",       true,  "hdmi-rst", 451 },
};

// Maps the file's text (trailing whitespace removed) to a kind. The legacy
// first-letter forms ("a", "b", "p") that libkvm used to accept stay valid.
static inline enum kvm_board_kind kvm_board_parse(const char *text)
{
    if(strcmp(text, "alpha") == 0 || strcmp(text, "a") == 0) return KVM_BOARD_CUBE_ALPHA;
    if(strcmp(text, "beta") == 0 || strcmp(text, "b") == 0) return KVM_BOARD_CUBE_BETA;
    if(strcmp(text, "pcie") == 0 || strcmp(text, "p") == 0) return KVM_BOARD_PCIE;
    return KVM_BOARD_UNKNOWN;
}

static inline enum kvm_board_kind kvm_board_read(void)
{
    FILE *fp = fopen(KVM_BOARD_FILE, "r");
    if(fp == NULL){
        fprintf(stderr, "[board]open %s failed: %d\n", KVM_BOARD_FILE, errno);
        return KVM_BOARD_UNKNOWN;
    }
    char buf[32];
    memset(buf, 0, sizeof(buf));
    size_t n = fread(buf, 1, sizeof(buf) - 1, fp);
    bool failed = ferror(fp) != 0;
    fclose(fp);
    if(failed){
        fprintf(stderr, "[board]read %s failed\n", KVM_BOARD_FILE);
        return KVM_BOARD_UNKNOWN;
    }
    while(n > 0 && (buf[n - 1] == '\n' || buf[n - 1] == '\r' || buf[n - 1] == ' ' || buf[n - 1] == '\t')){
        buf[--n] = 0;
    }
    enum kvm_board_kind kind = kvm_board_parse(buf);
    if(kind == KVM_BOARD_UNKNOWN){
        fprintf(stderr, "[board]%s holds \"%s\": not a known board\n", KVM_BOARD_FILE, buf);
    }
    return kind;
}

static const struct kvm_board *kvm_board_cached;
static pthread_once_t kvm_board_once = PTHREAD_ONCE_INIT;

static inline void kvm_board_detect(void)
{
    enum kvm_board_kind kind = kvm_board_read();
    for(size_t i = 0; i < sizeof(kvm_boards) / sizeof(kvm_boards[0]); i++){
        if(kvm_boards[i].kind == kind){
            kvm_board_cached = &kvm_boards[i];
            return;
        }
    }
    kvm_board_cached = &kvm_boards[0];
}

// The board, detected on the first call and never again. Never NULL.
static inline const struct kvm_board *kvm_board_get(void)
{
    pthread_once(&kvm_board_once, kvm_board_detect);
    return kvm_board_cached;
}

#endif
