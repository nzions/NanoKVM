#define _GNU_SOURCE
// kvmgpio: the owner of the NanoKVM's held GPIO outputs, by device-tree line
// NAME (hdmi-rst, oled-rst, wifi-en, ...), over the GPIO character device.
// Static, no dependencies; the lookup is libkvm's own (kvm_gpio.h).
//
// A gpio line request lives as long as its fd, and a line can be requested only
// once, so an output that must stay driven (hdmi-rst high, oled-rst high,
// wifi-en) needs a process that keeps the fd. This is it:
//
//   kvmgpio daemon [name=value ...]  start the owner (idempotent), driving the
//                                    given outputs first. Returns once the
//                                    owner listens. Any listed line missing =
//                                    nothing is held, exit 1 (fail closed).
//   kvmgpio set <name> <0|1>         drive a line through the owner (it takes
//                                    the line on first use and keeps it)
//   kvmgpio get <name>               read a line (owner's, else a one-shot)
//   kvmgpio pulse <name> <ms>        drive 1 for ms then 0, one-shot, no owner
//   kvmgpio has <name>               exit 0 the line exists, 3 no such line,
//                                    1 ambiguous / unreadable
//
// libkvm and nanokvm_update_edid change hdmi-rst through the owner's unix
// socket (nk_gpio_set_held in kvm_gpio.h); the Go server uses its own
// per-operation requests for the key lines and LEDs, which the owner never holds.
#include "../../support/sg2002/additional/kvm/include/internal/kvm_gpio.h"

#include <poll.h>
#include <signal.h>
#include <sys/stat.h>
#include <sys/time.h>
#include <time.h>

#define MAX_HELD 16

struct held {
    char name[GPIO_MAX_NAME];
    int fd;
};
static struct held table[MAX_HELD];
static int held_n;

static struct held *find_held(const char *name)
{
    for(int i = 0; i < held_n; i++)
        if(strcmp(table[i].name, name) == 0) return &table[i];
    return NULL;
}

// Drives name (taking it on first use). Returns NULL on success, else the reason.
static const char *drive(const char *name, int value)
{
    struct held *h = find_held(name);
    if(h != NULL) return nk_gpio_line_set(h->fd, value) == 0 ? NULL : "set failed";
    if(strlen(name) >= GPIO_MAX_NAME) return "name too long";
    if(held_n >= MAX_HELD) return "too many held lines";
    int fd = nk_gpio_open_named(name, "kvmgpio", NK_GPIO_V2_LINE_FLAG_OUTPUT, value);
    if(fd == NK_GPIO_NO_LINE) return "no such line";
    if(fd == NK_GPIO_AMBIGUOUS) return "ambiguous line name";
    if(fd < 0) return "request failed";
    snprintf(table[held_n].name, sizeof(table[held_n].name), "%s", name);
    table[held_n].fd = fd;
    held_n++;
    return NULL;
}

// Reads name: the owner's line when held, else a one-shot input request.
// Returns 0/1, or -1 with *why set.
static int sample(const char *name, const char **why)
{
    struct held *h = find_held(name);
    int fd = h != NULL ? h->fd : nk_gpio_open_named(name, "kvmgpio", NK_GPIO_V2_LINE_FLAG_INPUT, 0);
    if(fd == NK_GPIO_NO_LINE){ *why = "no such line"; return -1; }
    if(fd == NK_GPIO_AMBIGUOUS){ *why = "ambiguous line name"; return -1; }
    if(fd < 0){ *why = "request failed"; return -1; }
    int v = nk_gpio_line_get(fd);
    if(h == NULL) close(fd);
    if(v < 0) *why = "get failed";
    return v;
}

static void serve_one(int cfd)
{
    struct timeval tv = { .tv_sec = 2, .tv_usec = 0 };
    setsockopt(cfd, SOL_SOCKET, SO_RCVTIMEO, &tv, sizeof(tv));
    char buf[128];
    ssize_t n = read(cfd, buf, sizeof(buf) - 1);
    if(n <= 0) return;
    buf[n] = 0;
    char *nl = strchr(buf, '\n');
    if(nl != NULL) *nl = 0;
    char name[GPIO_MAX_NAME + 8];
    int value;
    char out[96];
    const char *why = NULL;
    if(strcmp(buf, "ping") == 0){
        snprintf(out, sizeof(out), "ok\n");
    } else if(sscanf(buf, "set %39s %d", name, &value) == 2 && (value == 0 || value == 1)){
        why = drive(name, value);
        if(why == NULL) snprintf(out, sizeof(out), "ok\n");
    } else if(sscanf(buf, "get %39s", name) == 1){
        int v = sample(name, &why);
        if(v >= 0) snprintf(out, sizeof(out), "ok %d\n", v);
    } else {
        why = "bad request";
    }
    if(why != NULL) snprintf(out, sizeof(out), "err %s\n", why);
    if(write(cfd, out, strlen(out)) < 0) fprintf(stderr, "[kvmgpio]reply failed: %d\n", errno);
}

static int owner_alive(void)
{
    char reply[16];
    return nk_gpio_daemon_call("ping\n", reply, sizeof(reply)) == 0 && strncmp(reply, "ok", 2) == 0;
}

static int cmd_daemon(int argc, char **argv)
{
    if(owner_alive()){
        printf("kvmgpio: owner already running\n");
        return 0;
    }
    // fork first so the caller gets the verdict: the child reports through a pipe
    // only once its lines are held and the socket listens.
    int pfd[2];
    if(pipe(pfd) < 0){ fprintf(stderr, "kvmgpio: pipe: %d\n", errno); return 1; }
    pid_t pid = fork();
    if(pid < 0){ fprintf(stderr, "kvmgpio: fork: %d\n", errno); return 1; }
    if(pid > 0){
        close(pfd[1]);
        char r = 0;
        ssize_t n;
        do { n = read(pfd[0], &r, 1); } while(n < 0 && errno == EINTR);
        if(n == 1 && r == 'R'){ printf("kvmgpio: owner running\n"); return 0; }
        fprintf(stderr, "kvmgpio: owner failed to start\n");
        return 1;
    }
    close(pfd[0]);
    signal(SIGPIPE, SIG_IGN);

    // initial holds; any miss = hold nothing and say which
    int bad = 0;
    for(int i = 2; i < argc; i++){
        char *eq = strchr(argv[i], '=');
        if(eq == NULL || (eq[1] != '0' && eq[1] != '1') || eq[2] != 0){
            fprintf(stderr, "kvmgpio: bad argument \"%s\" (want name=0|1)\n", argv[i]);
            bad = 1;
            continue;
        }
        *eq = 0;
        const char *why = drive(argv[i], eq[1] - '0');
        if(why != NULL){
            fprintf(stderr, "kvmgpio: line \"%s\": %s\n", argv[i], why);
            bad = 1;
        }
    }
    if(bad) _exit(1);

    int ls = socket(AF_UNIX, SOCK_STREAM | SOCK_CLOEXEC, 0);
    struct sockaddr_un sa;
    memset(&sa, 0, sizeof(sa));
    sa.sun_family = AF_UNIX;
    snprintf(sa.sun_path, sizeof(sa.sun_path), "%s", nk_gpio_sock_path);
    unlink(nk_gpio_sock_path);   // a dead owner's socket (owner_alive said nobody answers)
    mode_t old = umask(0177);
    int brc = ls < 0 ? -1 : bind(ls, (struct sockaddr *)&sa, sizeof(sa));
    umask(old);
    if(brc < 0 || listen(ls, 8) < 0){
        fprintf(stderr, "kvmgpio: socket %s: %d\n", nk_gpio_sock_path, errno);
        _exit(1);
    }
    if(setsid() < 0) fprintf(stderr, "kvmgpio: setsid: %d\n", errno);
    if(chdir("/") < 0) fprintf(stderr, "kvmgpio: chdir: %d\n", errno);
    int dn = open("/dev/null", O_RDWR);
    if(dn >= 0){ dup2(dn, 0); dup2(dn, 1); dup2(dn, 2); if(dn > 2) close(dn); }
    if(write(pfd[1], "R", 1) != 1) _exit(1);
    close(pfd[1]);

    for(;;){
        int cfd = accept4(ls, NULL, NULL, SOCK_CLOEXEC);
        if(cfd < 0){
            if(errno == EINTR) continue;
            _exit(1);   // the supervisor (S15kvmhwd re-init) restarts the owner
        }
        serve_one(cfd);
        close(cfd);
    }
}

static int cmd_client(const char *cmd)
{
    char reply[96];
    if(nk_gpio_daemon_call(cmd, reply, sizeof(reply)) < 0){
        fprintf(stderr, "kvmgpio: no owner at %s: %d\n", nk_gpio_sock_path, errno);
        return 1;
    }
    fputs(reply, stdout);
    return strncmp(reply, "ok", 2) == 0 ? 0 : 1;
}

int main(int argc, char **argv)
{
    if(argc >= 2 && strcmp(argv[1], "daemon") == 0) return cmd_daemon(argc, argv);
    if(argc == 4 && strcmp(argv[1], "set") == 0){
        if(argv[3][0] != '0' && argv[3][0] != '1'){ fprintf(stderr, "kvmgpio: value must be 0 or 1\n"); return 2; }
        char cmd[96];
        snprintf(cmd, sizeof(cmd), "set %.39s %c\n", argv[2], argv[3][0]);
        return cmd_client(cmd);
    }
    if(argc == 3 && strcmp(argv[1], "get") == 0){
        char cmd[96];
        snprintf(cmd, sizeof(cmd), "get %.39s\n", argv[2]);
        char reply[96];
        if(nk_gpio_daemon_call(cmd, reply, sizeof(reply)) == 0){
            fputs(reply, stdout);
            return strncmp(reply, "ok", 2) == 0 ? 0 : 1;
        }
        const char *why = NULL;
        int v = sample(argv[2], &why);   // no owner: a one-shot read
        if(v < 0){ fprintf(stderr, "kvmgpio: get %s: %s\n", argv[2], why); return 1; }
        printf("ok %d\n", v);
        return 0;
    }
    if(argc == 4 && strcmp(argv[1], "pulse") == 0){
        int ms = atoi(argv[3]);
        if(ms <= 0){ fprintf(stderr, "kvmgpio: pulse ms must be > 0\n"); return 2; }
        int fd = nk_gpio_open_named(argv[2], "kvmgpio", NK_GPIO_V2_LINE_FLAG_OUTPUT, 1);
        if(fd < 0){ fprintf(stderr, "kvmgpio: pulse %s: line unavailable (%d)\n", argv[2], fd); return 1; }
        struct timespec ts = { .tv_sec = ms / 1000, .tv_nsec = (long)(ms % 1000) * 1000000L };
        while(nanosleep(&ts, &ts) < 0 && errno == EINTR) {}
        int rc = nk_gpio_line_set(fd, 0);
        close(fd);
        if(rc < 0){ fprintf(stderr, "kvmgpio: pulse %s: release failed: %d\n", argv[2], errno); return 1; }
        return 0;
    }
    if(argc == 3 && strcmp(argv[1], "has") == 0){
        uint32_t off = 0;
        int cfd = nk_gpio_find(argv[2], &off);
        if(cfd >= 0){ close(cfd); return 0; }
        return cfd == NK_GPIO_NO_LINE ? 3 : 1;
    }
    fprintf(stderr, "usage: kvmgpio daemon [name=0|1 ...] | set <name> <0|1> | get <name> | pulse <name> <ms> | has <name>\n");
    return 2;
}
