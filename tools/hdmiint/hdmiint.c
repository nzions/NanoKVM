// hdmiint runs libkvm's real INT-line code (include/internal/hdmi_int.h, the
// same header kvm_vision.cpp includes) against the emulator's gpio-sim chip:
// hdmi_int_open() finds the line by its name or the /etc/kvm/hdmi_int_line
// override, then the loop is the state machine's wait: poll() on the line fd
// and hdmi_int_edge_seen() for the given HDMI variant.
//
//   hdmiint <hdmi_version> <seconds>
//
// Prints one line per poll wake-up and "PROBE" where the state machine would
// probe the LT6911. Exit 2 when the INT line cannot be opened (fail closed).
#include "../../support/sg2002/additional/kvm/include/internal/hdmi_int.h"

#include <poll.h>
#include <stdlib.h>
#include <time.h>

static long ms_now(void)
{
    struct timespec ts;
    clock_gettime(CLOCK_MONOTONIC, &ts);
    return ts.tv_sec * 1000L + ts.tv_nsec / 1000000L;
}

int main(int argc, char **argv)
{
    if(argc != 3){
        fprintf(stderr, "usage: hdmiint <hdmi_version> <seconds>\n");
        return 2;
    }
    uint8_t version = (uint8_t)atoi(argv[1]);
    long deadline = ms_now() + atol(argv[2]) * 1000L;
    int fd = hdmi_int_open();
    if(fd < 0){
        fprintf(stderr, "FAIL: INT line unavailable, state UNKNOWN\n");
        return 2;
    }
    int fl = fcntl(fd, F_GETFL);
    if(fl < 0 || fcntl(fd, F_SETFL, fl | O_NONBLOCK) < 0){
        fprintf(stderr, "FAIL: O_NONBLOCK: %d\n", errno);
        return 2;
    }
    hdmi_int_edge_seen(fd, version);
    printf("ready: hdmi_version %u\n", version);
    fflush(stdout);
    int probes = 0, wakes = 0;
    for(;;){
        long left = deadline - ms_now();
        if(left <= 0) break;
        struct pollfd p = {fd, POLLIN, 0};
        int n = poll(&p, 1, (int)left);
        if(n < 0){
            if(errno == EINTR) continue;
            fprintf(stderr, "FAIL: poll: %d\n", errno);
            return 1;
        }
        if(n == 0) break;
        wakes++;
        bool probe = hdmi_int_edge_seen(fd, version);
        printf("wake %d: %s\n", wakes, probe ? "PROBE (variant edge seen)" : "no probe (other edge)");
        fflush(stdout);
        if(probe) probes++;
    }
    printf("DONE: %d wakes, %d probes\n", wakes, probes);
    return 0;
}
