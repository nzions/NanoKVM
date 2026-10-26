/**
 * 待解决的问题:
 * // 分辨率跟随输出
 * // 自动切换分辨率
 * // HDMI分辨率问题
 * // H264输入空图,VENC容易炸问题
 * // MJPEG/H264切换时卡退
 * // 再加一条手动清空全部内存
 * // deinit时free全部内存
 * // free 错内存时会炸的问题
 */
#include "kvm_vision.h"
#include "internal/vi_state_writer.hpp"
#include "internal/hdmi_int.h"

#include <errno.h>
#include <stdarg.h>
#include <poll.h>
#include <string.h>
#include <sys/eventfd.h>
#include <fcntl.h>
#include <stdio.h>
#include <unistd.h>
#include <sys/stat.h>

#define default_venc_chn        1

#define VENC_MJPEG              0
#define VENC_H264               1

#define KVMV_MAX_TRY_NUM	   	2
#define vi_min_width            32
#define vi_min_height           3
#define vi_max_width            1920
#define vi_max_height           1080

#define Farame_sample_size      76800   // 320*240

#define default_vi_width        1920
#define default_vi_height       1080
#define default_vpss_width      1920
#define default_vpss_height     1080
#define default_venc_type       VENC_MJPEG
#define default_mjpeg_qlty      60
#define default_h264_qlty       1000
#define default_h264_gop        30
#define fresh_frame_discard_count 5

#define kvmv_data_buffer_size   4

#define vi_width_path           "/kvmapp/kvm/width"
#define vi_height_path          "/kvmapp/kvm/height"
#define hdmi_signal_file_path   "/kvmapp/kvm/state"
#define watchdog_mode_path      "/etc/kvm/watchdog"
#define watchdog_temp_path      "/tmp/watchdog"
#define watchdog_file           "/tmp/nanokvm_wd"

#define LT6911_ADDR 	0x2B
#define LT6911_READ 	0xFF
#define LT6911_WRITE 	0x00

pthread_mutex_t vi_mutex;
pthread_mutex_t hdmi_signal_mutex = PTHREAD_MUTEX_INITIALIZER;

using namespace maix;
using namespace maix::sys;
using namespace maix::peripheral;
i2c::I2C LT6911_i2c(4, i2c::Mode::MASTER);

typedef struct {
	uint16_t vi_width = default_vi_width;
	uint16_t vi_height = default_vi_height;
	uint16_t vpss_width = default_vpss_width;
	uint16_t vpss_height = default_vpss_height;
	uint8_t venc_type;
	uint16_t qlty;
	uint8_t stream_stop;
	uint8_t frame_detact = 0;
	uint8_t display;
    uint8_t reinit_flag = 1;
    uint8_t hdmi_cable_state = 0;
    uint8_t try_exit_thread = 0;
    uint8_t thread_is_running = 0;
    uint8_t Auto_res = 0;
    uint8_t hdmi_version = 0;
    uint8_t hw_version = 0;
    uint8_t hdmi_res_type = 0;
    uint8_t hdmi_res_err = 0;
    uint8_t venc_auto_recyc = 0;
    uint8_t fresh_frame_count = 0;
} kvmv_cfg_t;

typedef struct {
	uint8_t* p_img_data = NULL;
    uint8_t img_data_type = 0;
    uint32_t img_data_size = 0;
} kvmv_data_t;

typedef struct {
    uint8_t mmf_venc_chn;
	uint8_t enc_h264_running;
	uint8_t enc_h264_init;
    mmf_venc_cfg_t kvm_venc_cfg;
} kvm_venc_t;

camera::Camera *cam = new camera::Camera(default_vpss_width, default_vpss_height, image::FMT_YVU420SP);
// camera::Camera *cam = new camera::Camera(320, 240, image::FMT_RGB888);

kvmv_cfg_t kvmv_cfg;

kvmv_data_t kvmv_data_buffer[kvmv_data_buffer_size];

uint8_t kvmv_data_buffer_index = 0;

uint8_t debug_en = 0;
uint8_t hdmi_capture_enabled = 0;
uint8_t hdmi_signal_active = 0;

void debug(const char *format, ...);

static bool write_hdmi_signal_file(uint8_t active)
{
    char temp_path[] = "/kvmapp/kvm/.state.tmp.XXXXXX";
    const char *state = active != 0 ? "1\n" : "0\n";
    const size_t state_size = 2;
    int fd = mkstemp(temp_path);
    if(fd < 0){
        debug("[hdmi] failed to create state file: %d\n", errno);
        return false;
    }

    int failure = 0;
    if(fchmod(fd, 0644) != 0){
        failure = errno;
    }

    size_t written = 0;
    while(failure == 0 && written < state_size){
        ssize_t result = write(fd, state + written, state_size - written);
        if(result > 0){
            written += (size_t)result;
        } else if(result < 0 && errno == EINTR){
            continue;
        } else {
            failure = result < 0 ? errno : EIO;
        }
    }

    if(failure == 0 && fsync(fd) != 0){
        failure = errno;
    }
    if(close(fd) != 0 && failure == 0){
        failure = errno;
    }
    if(failure == 0 && rename(temp_path, hdmi_signal_file_path) != 0){
        failure = errno;
    }
    if(failure != 0){
        unlink(temp_path);
        debug("[hdmi] failed to publish state file: %d\n", failure);
        return false;
    }
    return true;
}

static void set_hdmi_signal_state(uint8_t active)
{
    pthread_mutex_lock(&hdmi_signal_mutex);
    uint8_t enabled = __atomic_load_n(&hdmi_capture_enabled, __ATOMIC_ACQUIRE);
    uint8_t next = enabled != 0 && active != 0 ? 1 : 0;
    uint8_t previous = __atomic_exchange_n(&hdmi_signal_active, next, __ATOMIC_ACQ_REL);
    if(previous != next){
        write_hdmi_signal_file(next);
    }
    pthread_mutex_unlock(&hdmi_signal_mutex);
}

static void set_hdmi_capture_enabled(uint8_t enabled)
{
    pthread_mutex_lock(&hdmi_signal_mutex);
    __atomic_store_n(&hdmi_capture_enabled, enabled != 0 ? 1 : 0, __ATOMIC_RELEASE);
    __atomic_store_n(&hdmi_signal_active, 0, __ATOMIC_RELEASE);
    write_hdmi_signal_file(0);
    pthread_mutex_unlock(&hdmi_signal_mutex);
}

static void set_hdmi_detection_state(uint8_t active)
{
    __atomic_store_n(&kvmv_cfg.hdmi_cable_state, active != 0 ? 1 : 0, __ATOMIC_RELEASE);
    set_hdmi_signal_state(active);
}

void debug(const char *format, ...)
{
    if(debug_en){
        va_list args;
        va_start(args, format);
        vprintf(format, args);
        va_end(args);
    }
}

uint8_t to_roll(int8_t _input)
{
    if(_input <                      0) return _input + kvmv_data_buffer_size;
    if(_input >= kvmv_data_buffer_size) return _input - kvmv_data_buffer_size;
    return _input;
}

int maxmin_data(int _max, int _min, int _data)
{
	if(_data > _max) return _max;
	if(_data < _min) return _min;
	return _data;
}

kvmv_data_t* get_save_buffer()
{
    kvmv_data_buffer_index = to_roll(kvmv_data_buffer_index + 1);
    // debug("[kvmv]kvmv_data_buffer_index = %d\n", kvmv_data_buffer_index);
    // debug("[kvmv]kvmv_data_buffer.p_img_data = %d\n", kvmv_data_buffer[kvmv_data_buffer_index].p_img_data);
    if(kvmv_data_buffer[kvmv_data_buffer_index].p_img_data == NULL){
        return &kvmv_data_buffer[kvmv_data_buffer_index];
    }
    return NULL;
}

// ====HDMI RES==================================================

uint16_t hdmi_res_list[][2] = {
    {1920, 1080},
    {1600, 900},
    {1440, 1080},
    {1440, 900},
    {1280, 1024},
    {1280, 960},
    {1280, 800},
    {1280, 720},
    {1152, 864},
    {1024, 768},
    {800, 600},
	{640, 480},
};

uint16_t hdmi_unsupported_res_list[][2] = {
    {1680, 1050},
    {1440, 1050},
    {1400, 1050},
    {1368, 768},
    {1366, 768},
    {720, 576},
};

/* return 0 : normal res;
/* return 1 : new res;
 * return 2 : unsupport res;
 * return 3 : unknow res;
 */
uint8_t check_res(uint16_t _width, uint16_t _height)
{
    uint8_t i;
    uint8_t ret;

    for(i = 0; i < sizeof(hdmi_res_list)/4; i++){
        if(_width == hdmi_res_list[i][0] && _height == hdmi_res_list[i][1]) return NORMAL_RES;
    }
    for(i = 0; i < sizeof(hdmi_unsupported_res_list)/4; i++){
        if(_width == hdmi_unsupported_res_list[i][0] && _height == hdmi_unsupported_res_list[i][1]) return UNSUPPORT_RES;
    }
    return UNKNOWN_RES;
}

void write_res_to_file(uint16_t _width, uint16_t _height)
{
	char Cmd[100]={0};
    sprintf(Cmd, "echo %d > %s", _width, vi_width_path);
    system(Cmd);
    sprintf(Cmd, "echo %d > %s", _height, vi_height_path);
    system(Cmd);
    system("sync");
}

uint8_t watchdog_sf_is_open(void)
{
	if(access(watchdog_mode_path, F_OK) == 0) return 1;
    else if(access(watchdog_temp_path, F_OK) == 0) return 1;
	else return 0;
}

int vision_update_watchdog()
{
    FILE *file;

    file = fopen(watchdog_file, "w");
    if (file == NULL) {
        debug("[kvmv] watchdog open error\n");
        return -1;
    }
    // fprintf(file, "%s", 'v');
    fclose(file);
    return 1;
}

/* return :
 * 0 : error
 * 1 : out of mem
 * 2 : normal
*/
uint8_t chack_ion()
{
    // Reads the first "usage rate:" line of the ion summary and takes its
    // second whitespace-separated field, as the former
    // `grep "usage rate:" | awk '{print $2}'` pipeline printed it.
    static const char ion_summary_path[] = "/sys/kernel/debug/ion/cvi_carveout_heap_dump/summary";
    char line[256];
    char RW_Data[10] = {0};
    uint8_t ATOI_Data[3] = {0};
    uint8_t ion_usage_rate;
    FILE* fp = fopen(ion_summary_path, "r");
    if ( NULL == fp )
    {
        return 0;
    }
    while (fgets(line, sizeof(line), fp) != NULL)
    {
        if (strstr(line, "usage rate:") == NULL) continue;
        const char *p = line;
        p += strspn(p, " \t\r\n");
        p += strcspn(p, " \t\r\n");
        p += strspn(p, " \t\r\n");
        size_t n = strcspn(p, " \t\r\n");
        // awk printed the field plus a newline; the old read kept at most 7 chars of it
        snprintf(RW_Data, 8, "%.*s\n", (int)n, p);
        break;
    }
    fclose(fp);
    RW_Data[8] = 0;
    if (RW_Data[6] == '&') return 1;
    else {
        ATOI_Data[0] = RW_Data[5];
        ATOI_Data[1] = RW_Data[6];
    }
    ion_usage_rate = atoi((char*)ATOI_Data);

    if(ion_usage_rate >= 95) return 1;
    else return 2;
}

void lt6911_enable()
{
	uint8_t buf[2];

    if(kvmv_cfg.hdmi_version == 3){
        return;
    }

	buf[0] = 0xff;
	buf[1] = 0x80;
	LT6911_i2c.writeto(LT6911_ADDR, buf, 2);

	buf[0] = 0xee;
	buf[1] = 0x01;
	LT6911_i2c.writeto(LT6911_ADDR, buf, 2);

    if(kvmv_cfg.hdmi_version == 1){
        // disable watchdog
        buf[0] = 0x10;
        buf[1] = 0x00;
        LT6911_i2c.writeto(LT6911_ADDR, buf, 2);
    }
}

void lt6911_disable()
{
	uint8_t buf[2];

    if(kvmv_cfg.hdmi_version == 3){
        return;
    }

	buf[0] = 0xff;
	buf[1] = 0x80;
	LT6911_i2c.writeto(LT6911_ADDR, buf, 2);

	buf[0] = 0xee;
	buf[1] = 0x00;
	LT6911_i2c.writeto(LT6911_ADDR, buf, 2);
}

uint8_t lt6911_get_hdmi_res()
{
	uint8_t buf[2];
	uint8_t revbuf[4];
	uint16_t Vactive;
	uint16_t Hactive;


    if(kvmv_cfg.hdmi_version == 0){
        // LT6911C
        buf[0] = 0xff;
        buf[1] = 0xd2;
        LT6911_i2c.writeto(LT6911_ADDR, buf, 2);

        buf[0] = 0x83;
        buf[1] = 0x11;
        LT6911_i2c.writeto(LT6911_ADDR, buf, 2);

        time::sleep_ms(5);

        // Vactive
        buf[0] = 0x96;
        LT6911_i2c.writeto(LT6911_ADDR, buf, 1);
        maix::Bytes *dat0 = LT6911_i2c.readfrom(LT6911_ADDR, 2);

        // Hactive
        buf[0] = 0x8b;
        LT6911_i2c.writeto(LT6911_ADDR, buf, 1);
        maix::Bytes *dat1 = LT6911_i2c.readfrom(LT6911_ADDR, 2);

        revbuf[0] = (uint8_t)dat0->data[0];
        revbuf[1] = (uint8_t)dat0->data[1];
        revbuf[2] = (uint8_t)dat1->data[0];
        revbuf[3] = (uint8_t)dat1->data[1];

        Vactive = (revbuf[0] << 8)|revbuf[1];
        Hactive = (revbuf[2] << 8)|revbuf[3];
        Hactive *= 2;

        debug("[hdmi]HDMI res modification event\n");
        debug("[hdmi]new res: %d * %d\n", Hactive, Vactive);

        delete dat0;
        delete dat1;

        if (Vactive != 0 && Hactive != 0){
            return 1;
        } else {
            // system("echo 0 > %s", vi_height_path);
            // system("echo 0 > %s", vi_width_path);
            return 0;
        }
    } else if (kvmv_cfg.hdmi_version == 1){
        // LT6911UXC
        buf[0] = 0xff;
        buf[1] = 0x86;
        LT6911_i2c.writeto(LT6911_ADDR, buf, 2);

        buf[0] = 0xff;
        buf[1] = 0x86;
        LT6911_i2c.writeto(LT6911_ADDR, buf, 2);

        // HDMI signal disappear/stable
        buf[0] = 0xA3;
        LT6911_i2c.writeto(LT6911_ADDR, buf, 1);
        maix::Bytes *dat0 = LT6911_i2c.readfrom(LT6911_ADDR, 1);

        revbuf[0] = (uint8_t)dat0->data[0];
        delete dat0;

        debug("[hdmi]HDMI-UXC res modification event\n");

        if(revbuf[0] == 0x55)       return 1;
        else if(revbuf[0] == 0x88)  return 0;
        else return 0;
    } else if (kvmv_cfg.hdmi_version == 3){
        // LT6911D
        buf[0] = 0xff;
        buf[1] = 0xe0;
        LT6911_i2c.writeto(LT6911_ADDR, buf, 2);

        // Hactive
        buf[0] = 0x8c;
        LT6911_i2c.writeto(LT6911_ADDR, buf, 1);
        maix::Bytes *dat0 = LT6911_i2c.readfrom(LT6911_ADDR, 2);

        // Vactive
        buf[0] = 0x8e;
        LT6911_i2c.writeto(LT6911_ADDR, buf, 1);
        maix::Bytes *dat1 = LT6911_i2c.readfrom(LT6911_ADDR, 2);

        revbuf[0] = (uint8_t)dat0->data[0];
        revbuf[1] = (uint8_t)dat0->data[1];
        revbuf[2] = (uint8_t)dat1->data[0];
        revbuf[3] = (uint8_t)dat1->data[1];

        Hactive = ((revbuf[0] << 8)|revbuf[1]) * 2;
        Vactive = (revbuf[2] << 8)|revbuf[3];

        debug("[hdmi]HDMI-D res modification event\n");
        debug("[hdmi]new res: %d * %d\n", Hactive, Vactive);

        delete dat0;
        delete dat1;

        if(Hactive != 0 || Vactive != 0) return 1;
        else return 0;
    } else {
        return 0;
    }
}

uint8_t lt6911_get_csi_res(uint16_t *p_width, uint16_t *p_height)
{
	uint8_t buf[2];
	uint8_t revbuf[4];
    uint8_t res_type;
	uint16_t Vactive;
	uint16_t Hactive;

    if(kvmv_cfg.hdmi_version == 0){
        // LT6911C
        buf[0] = 0xff;
        buf[1] = 0xc2;
        LT6911_i2c.writeto(LT6911_ADDR, buf, 2);

        // Vactive
        buf[0] = 0x06;
        LT6911_i2c.writeto(LT6911_ADDR, buf, 1);
        maix::Bytes *dat0 = LT6911_i2c.readfrom(LT6911_ADDR, 2);

        // Hactive
        buf[0] = 0x38;
        LT6911_i2c.writeto(LT6911_ADDR, buf, 1);
        maix::Bytes *dat1 = LT6911_i2c.readfrom(LT6911_ADDR, 2);

        revbuf[0] = (uint8_t)dat0->data[0];
        revbuf[1] = (uint8_t)dat0->data[1];
        revbuf[2] = (uint8_t)dat1->data[0];
        revbuf[3] = (uint8_t)dat1->data[1];

        delete dat0;
        delete dat1;

        Vactive = (revbuf[0] << 8)|revbuf[1];
        Hactive = (revbuf[2] << 8)|revbuf[3];
    } else if(kvmv_cfg.hdmi_version == 1) {
        // LT6911UXC
        debug("[hdmi]UXC get csi res\n");
        buf[0] = 0xff;
        buf[1] = 0x85;
        LT6911_i2c.writeto(LT6911_ADDR, buf, 2);

        // Vactive
        buf[0] = 0xF0;
        LT6911_i2c.writeto(LT6911_ADDR, buf, 1);
        maix::Bytes *dat0 = LT6911_i2c.readfrom(LT6911_ADDR, 2);

        // Hactive
        buf[0] = 0xEA;
        LT6911_i2c.writeto(LT6911_ADDR, buf, 1);
        maix::Bytes *dat1 = LT6911_i2c.readfrom(LT6911_ADDR, 2);

        revbuf[0] = (uint8_t)dat0->data[0];
        revbuf[1] = (uint8_t)dat0->data[1];
        revbuf[2] = (uint8_t)dat1->data[0];
        revbuf[3] = (uint8_t)dat1->data[1];

        delete dat0;
        delete dat1;

        Vactive = (revbuf[0] << 8)|revbuf[1];
        Hactive = (revbuf[2] << 8)|revbuf[3];
    } else if(kvmv_cfg.hdmi_version == 3) {
        // LT6911D
        debug("[hdmi]D get csi res\n");
        buf[0] = 0xff;
        buf[1] = 0xe0;
        LT6911_i2c.writeto(LT6911_ADDR, buf, 2);

        // Vactive
        buf[0] = 0x8e;
        LT6911_i2c.writeto(LT6911_ADDR, buf, 1);
        maix::Bytes *dat0 = LT6911_i2c.readfrom(LT6911_ADDR, 2);

        // Hactive
        buf[0] = 0x8c;
        LT6911_i2c.writeto(LT6911_ADDR, buf, 1);
        maix::Bytes *dat1 = LT6911_i2c.readfrom(LT6911_ADDR, 2);

        revbuf[0] = (uint8_t)dat0->data[0];
        revbuf[1] = (uint8_t)dat0->data[1];
        revbuf[2] = (uint8_t)dat1->data[0];
        revbuf[3] = (uint8_t)dat1->data[1];

        delete dat0;
        delete dat1;

        Vactive = (revbuf[0] << 8)|revbuf[1];
        Hactive = ((revbuf[2] << 8)|revbuf[3]) * 2;
    } else {
        return UNKNOWN_RES;
    }

    res_type = check_res(Hactive, Vactive);
    *p_width = Hactive;
    *p_height = Vactive;

    switch (res_type)
    {
    case NORMAL_RES:
        printf("[hdmi] get res : %d * %d\n", Hactive, Vactive);
        break;
    case UNSUPPORT_RES:
        printf("[hdmi] get unsupport res : %d * %d\n", Hactive, Vactive);
        break;
    case UNKNOWN_RES:
        printf("[hdmi] get unknown res : %d * %d\n", Hactive, Vactive);
        break;
    }

	return res_type;
}

void lt6911_write_reg(uint8_t reg, uint8_t val)
{
	uint8_t buf[2];
	buf[0] = reg;
	buf[1] = val;
	LT6911_i2c.writeto(LT6911_ADDR, buf, 2);
}

void lt6911_read_reg(uint8_t reg)
{
	uint8_t buf[16];
	buf[0] = reg;
	LT6911_i2c.writeto(LT6911_ADDR, buf, 1);

	maix::Bytes *dat = LT6911_i2c.readfrom(LT6911_ADDR, 16);

	for(int i = 0; i < 16; i++){
		buf[i] = (uint8_t)dat->data[i];
	}

	delete dat;

	debug("[hdmi]%3x %3x %3x %3x %3x %3x %3x %3x |%3x %3x %3x %3x %3x %3x %3x %3x \n", \
			buf[0], buf[1], buf[2], buf[3], buf[4], buf[5], buf[6], buf[7], \
			buf[8], buf[9], buf[10], buf[11], buf[12], buf[13], buf[14], buf[15]);
}

uint8_t lt6911_read_one_reg(uint8_t reg)
{
	uint8_t ret;
	ret = reg;
	LT6911_i2c.writeto(LT6911_ADDR, &ret, 1);

	maix::Bytes *dat = LT6911_i2c.readfrom(LT6911_ADDR, 1);

	ret = (uint8_t)dat->data[0];

	delete dat;
	return ret;
}

// ==============================================================

void* watchdog_sf_feed(void * arg)
{
    while(true)
    {
        if(kvmv_cfg.try_exit_thread == 1)
            break;
        time::sleep_ms(500);
        if (watchdog_sf_is_open()){
            if (chack_ion() == 1){
                debug("[kvmv] Ion memory is full reboot now!\n");
                system("reboot");
            }
            // debug("[kvmv] watchdog_sf_feed now!\n");
            vision_update_watchdog();
        }
    }
    return NULL;
}

void get_hdmi_version()
{
	FILE *fp;
	uint8_t RW_Data[2];
    system("/kvmapp/system/init.d/S15kvmhwd get_hdmi_version");
	if(access("/etc/kvm/hdmi_version", F_OK) == 0){
        fp = fopen("/etc/kvm/hdmi_version", "r");
        fread(RW_Data, sizeof(char), 2, fp);
        fclose(fp);
        if(RW_Data[0] == 'u'){
            // 6911uxc
            if(RW_Data[1] == 'e'){
                kvmv_cfg.hdmi_version = 2;
                debug("[hdmi]HDMI-UE exist!\n");
            } else if(RW_Data[1] == 'x') {
                kvmv_cfg.hdmi_version = 1;
                debug("[hdmi]HDMI-UX exist!\n");
            } else {
                kvmv_cfg.hdmi_version = 1;
                debug("[hdmi]Incomplete version number, set to 'ux'\n");
            }
        } else if(RW_Data[0] == 'd'){
            // 6911d
            kvmv_cfg.hdmi_version = 3;
            debug("[hdmi]HDMI-D exist!\n");
        } else {
            // 6911c
            kvmv_cfg.hdmi_version = 0;
            debug("[hdmi]HDMI-C exist!\n");
        }
        RW_Data[0] = 0;
    } else {
        kvmv_cfg.hdmi_version = 0;
    }
}

// ====HDMI STATE MACHINE========================================

extern kvm_venc_t kvm_venc;
//
// One thread owns the HDMI input path. It is the only writer of hdmi_state
// and the only code that tears down / re-inits the video chain, which it
// does strictly in order: VENC (h264 channel, jpeg encoder), then VPSS and
// VI/CSI (cam->restart). All of that runs under vi_mutex, the same mutex
// kvmv_read_img holds for the whole of a read, so a read never touches a
// channel that is being torn down and a restart never runs mid-read.
//
//   NO_SIGNAL  --probe: signal--------> LOCKING   (new resolution to init)
//   NO_SIGNAL  --probe: signal--------> STREAMING (VI already at that res)
//   LOCKING    --chain re-inited------> STREAMING
//   STREAMING  --probe: no signal-----> NO_SIGNAL
//   STREAMING  --probe: new res-------> RES_CHANGE --chain re-inited--> STREAMING
//   RES_CHANGE --probe: no signal-----> NO_SIGNAL
//
// The thread blocks in poll(). Its wake-ups are: the wake eventfd (resync
// request from kvmv_hdmi_control, or exit) and the LT6911 INT line, a GPIO
// character device v2 line-event fd requested for both edges. If that line
// cannot be opened the thread logs it and ends: nothing probes the chip and
// the cable state is never set to connected.

typedef enum {
    HDMI_NO_SIGNAL = 0,
    HDMI_LOCKING,
    HDMI_STREAMING,
    HDMI_RES_CHANGE,
} hdmi_state_t;

typedef struct {
    bool present;
    uint8_t res_type;
    uint16_t width;
    uint16_t height;
} hdmi_probe_t;

// Written only under vi_mutex by the state machine thread.
static hdmi_state_t hdmi_state = HDMI_NO_SIGNAL;
// Resolution the VI/CSI chain was last initialised at (0 = never).
static uint16_t vi_inited_width = 0;
static uint16_t vi_inited_height = 0;

// Held for the whole of an LT6911 register sequence, and by
// kvmv_hdmi_control around hdmi_stopped (chip held in reset: do not read).
static pthread_mutex_t lt_mutex = PTHREAD_MUTEX_INITIALIZER;
static uint8_t hdmi_stopped = 0;

static pthread_t hdmi_thread;
static uint8_t hdmi_thread_started = 0;
static int hdmi_wake_fd = -1;
static uint8_t hdmi_resync = 0;

static void hdmi_wake(void)
{
    uint64_t one = 1;
    if(hdmi_wake_fd >= 0){
        ssize_t r = write(hdmi_wake_fd, &one, sizeof(one));
        (void)r;
    }
}

static void hdmi_post_resync(void)
{
    __atomic_store_n(&hdmi_resync, 1, __ATOMIC_RELEASE);
    hdmi_wake();
}

// Caller holds lt_mutex.
static hdmi_probe_t lt6911_probe(void)
{
    hdmi_probe_t p = {false, UNKNOWN_RES, 0, 0};
    lt6911_enable();
    if(lt6911_get_hdmi_res()){
        p.present = true;
        p.res_type = lt6911_get_csi_res(&p.width, &p.height);
    }
    lt6911_disable();
    return p;
}

// Caller holds vi_mutex. VENC first, then VPSS and VI/CSI.
static void vi_chain_reinit_locked(uint16_t _width, uint16_t _height)
{
    mmf_del_venc_channel(kvm_venc.mmf_venc_chn);
    kvm_venc.enc_h264_init = 0;
    mmf_enc_jpg_deinit(0);

    kvmv_cfg.vi_width = _width;
    kvmv_cfg.vi_height = _height;
    write_res_to_file(_width, _height);
    cam->restart(default_vpss_width, default_vpss_height, image::FMT_YVU420SP);
    vi_inited_width = _width;
    vi_inited_height = _height;
}

static void hdmi_apply(const hdmi_probe_t &p)
{
    if(!p.present){
        set_hdmi_detection_state(0);
        pthread_mutex_lock(&vi_mutex);
        hdmi_state = HDMI_NO_SIGNAL;
        pthread_mutex_unlock(&vi_mutex);
        return;
    }
    set_hdmi_detection_state(1);

    uint8_t res_type = p.res_type;
    uint8_t res_err = NORMAL_RES;
    if(res_type == UNKNOWN_RES){
        // Not on either list: try it as reported when the VI can hold it.
        if(p.width >= vi_min_width && p.width <= vi_max_width &&
           p.height >= vi_min_height && p.height <= vi_max_height){
            res_type = NORMAL_RES;
        } else {
            res_err = ERROR_RES;
        }
    }

    pthread_mutex_lock(&vi_mutex);
    kvmv_cfg.hdmi_res_type = res_type == UNSUPPORT_RES ? UNSUPPORT_RES : NORMAL_RES;
    kvmv_cfg.hdmi_res_err = res_err;
    if(res_type == UNSUPPORT_RES || res_err != NORMAL_RES){
        // Reads report -6 / -7 from the two fields above; the chain is left alone.
        pthread_mutex_unlock(&vi_mutex);
        return;
    }
    if(p.width != vi_inited_width || p.height != vi_inited_height){
        hdmi_state = hdmi_state == HDMI_NO_SIGNAL ? HDMI_LOCKING : HDMI_RES_CHANGE;
        debug("[hdmi] state %s: re-init chain at %d * %d\n",
              hdmi_state == HDMI_LOCKING ? "LOCKING" : "RES_CHANGE", p.width, p.height);
        vi_chain_reinit_locked(p.width, p.height);
    }
    hdmi_state = HDMI_STREAMING;
    pthread_mutex_unlock(&vi_mutex);
}

void* hdmi_state_machine(void * arg)
{
    kvmv_cfg.thread_is_running = 1;
    get_hdmi_version();

    if(hdmi_wake_fd < 0){
        fprintf(stderr, "[hdmi]state machine: no wake eventfd: %d\n", errno);
        kvmv_cfg.thread_is_running = 0;
        return NULL;
    }
    int int_fd = hdmi_int_open();
    if(int_fd < 0){
        fprintf(stderr, "[hdmi]FATAL: LT6911 INT line unavailable, HDMI state is UNKNOWN (not probing, not reporting connected)\n");
        kvmv_cfg.thread_is_running = 0;
        return NULL;
    }
    int fl = fcntl(int_fd, F_GETFL);
    if(fl < 0 || fcntl(int_fd, F_SETFL, fl | O_NONBLOCK) < 0){
        fprintf(stderr, "[hdmi]FATAL: INT line O_NONBLOCK failed: %d\n", errno);
        close(int_fd);
        kvmv_cfg.thread_is_running = 0;
        return NULL;
    }
    hdmi_int_edge_seen(int_fd, kvmv_cfg.hdmi_version);     // drop edges from before we started
    hdmi_post_resync();             // initial read of the chip

    struct pollfd fds[2];
    fds[0].fd = hdmi_wake_fd;
    fds[0].events = POLLIN;
    fds[1].fd = int_fd;
    fds[1].events = POLLIN;

    while(__atomic_load_n(&kvmv_cfg.try_exit_thread, __ATOMIC_ACQUIRE) == 0)
    {
        fds[0].revents = 0;
        fds[1].revents = 0;
        if(poll(fds, 2, -1) < 0){
            if(errno == EINTR) continue;
            debug("[hdmi]poll failed: %d\n", errno);
            break;
        }
        bool probe_now = false;
        if(fds[0].revents & POLLIN){
            uint64_t v;
            ssize_t r = read(hdmi_wake_fd, &v, sizeof(v));
            (void)r;
        }
        if(__atomic_load_n(&kvmv_cfg.try_exit_thread, __ATOMIC_ACQUIRE) != 0) break;
        if(__atomic_exchange_n(&hdmi_resync, 0, __ATOMIC_ACQ_REL) != 0) probe_now = true;
        if(fds[1].revents & POLLIN){
            if(hdmi_int_edge_seen(int_fd, kvmv_cfg.hdmi_version)) probe_now = true;
        }

        if(probe_now){
            if(kvmv_cfg.hdmi_version == 2){
                // No register read for this chip: assume the default mode.
                hdmi_probe_t assumed = {true, NORMAL_RES, default_vi_width, default_vi_height};
                hdmi_apply(assumed);
            } else {
                pthread_mutex_lock(&lt_mutex);
                bool stopped = hdmi_stopped != 0;
                hdmi_probe_t p = {false, UNKNOWN_RES, 0, 0};
                if(!stopped) p = lt6911_probe();
                pthread_mutex_unlock(&lt_mutex);
                // Held in reset: wait for kvmv_hdmi_control(1) to resync.
                if(!stopped) hdmi_apply(p);
            }
        }
    }
    close(int_fd);
    kvmv_cfg.thread_is_running = 0;
    return NULL;
}

uint8_t frame_changed(image::Image *raw)
{
    static int raw_size = 0;
    static uint8_t Farame_sample[Farame_sample_size] = {0};
    uint8_t ret = 0;
    uint8_t sample_byte;
    if(raw->data_size() != raw_size){
        raw_size = raw->data_size();
        ret = 1;
    }
    int Detection_Pixel_Interval = raw_size/(Farame_sample_size*1.5);
    for(int i = 0; i < Farame_sample_size; i ++){
        // printf("[kvmv] i = %d\n", i);
        if(i >= raw_size){
            ret = 0;
        }
        sample_byte = *(uint8_t*)(raw->data()+(i*Detection_Pixel_Interval));
        if(sample_byte != Farame_sample[i]){
            Farame_sample[i] = sample_byte;
            ret = 1;
        }
    }
    return ret;
}

bool jpg_dump(kvmv_data_t* dump_to, image::Image *raw)
{
    if(dump_to == NULL || raw == NULL || raw->data() == NULL || raw->data_size() == 0){
        return false;
    }
    dump_to->p_img_data = (uint8_t *)malloc(raw->data_size());
    if(dump_to->p_img_data == NULL){
        dump_to->img_data_size = 0;
        dump_to->img_data_type = 0;
        return false;
    }
    dump_to->img_data_size = raw->data_size();
    dump_to->img_data_type = VENC_MJPEG;
    memcpy(dump_to->p_img_data, (uint8_t *)raw->data(), raw->data_size());
    return true;
}

uint8_t kvmvenc_gop = default_h264_gop;
kvm_venc_t kvm_venc;
mmf_venc_cfg_t cfg;
void init_venc_h264(uint16_t _width, uint16_t _height, uint16_t _qlty)
{
    cfg.type = 2; //1, h265, 2, h264
    cfg.w = _width;
    cfg.h = _height;
    cfg.fmt = mmf_invert_format_to_mmf(image::Format::FMT_YVU420SP);
    cfg.jpg_quality = 0;       // unused
    cfg.gop = kvmvenc_gop;
    cfg.intput_fps = 60;
    cfg.output_fps = 60;
    cfg.bitrate = _qlty;  // 码率

    kvm_venc.mmf_venc_chn = default_venc_chn;
    kvm_venc.enc_h264_init = 0;
    kvm_venc.kvm_venc_cfg = cfg;

	// if(mmf_vdec_is_used(kvm_venc.mmf_venc_chn)){
		mmf_del_venc_channel(kvm_venc.mmf_venc_chn);
	// }
    if (0 != mmf_add_venc_channel(kvm_venc.mmf_venc_chn, &kvm_venc.kvm_venc_cfg)) {
        err::check_raise(err::ERR_RUNTIME, "mmf venc init failed!");
    }
    kvm_venc.enc_h264_init = 1;

	// init_kvm_h264_stream(&kvm_h264_stream, mmf_stream_buf);
	// init_h264_stream_struct(&kvm_h264_stream);
}

int h264_stream_dump(kvmv_data_t* dump_to, mmf_stream_t* dump_from)
{
    static int8_t I_Frame_index = -1;
    // debug("[kvmv]dump_from->count = %d\n", dump_from->count);
    if (dump_from->count == 3) {

        dump_to->p_img_data = (uint8_t *)malloc(dump_from->data_size[0]+dump_from->data_size[1]+dump_from->data_size[2]);
        if (dump_to->p_img_data == NULL) {
            dump_to->img_data_size = 0;
            return IMG_VENC_ERROR;
        }
        dump_to->img_data_size = dump_from->data_size[0]+dump_from->data_size[1]+dump_from->data_size[2];
        dump_to->img_data_type = IMG_H264_TYPE_IF;
        memcpy(dump_to->p_img_data, dump_from->data[0], dump_from->data_size[0]);
        memcpy(dump_to->p_img_data+dump_from->data_size[0], dump_from->data[1], dump_from->data_size[1]);
        memcpy(dump_to->p_img_data+dump_from->data_size[0]+dump_from->data_size[1], dump_from->data[2], dump_from->data_size[2]);

        debug("[kvmv]SPS size = %d\n", dump_from->data_size[0]);
        debug("[kvmv]PPS size = %d\n", dump_from->data_size[1]);
        debug("[kvmv]I-Frame size = %d\n", dump_from->data_size[2]);

        return IMG_H264_TYPE_IF;

    } else if (dump_from->count == 1) {
        // debug("[kvmv]dump P-Frame\r\n");
        I_Frame_index = -1;
        dump_to->p_img_data = (uint8_t *)malloc(dump_from->data_size[0]);
        if (dump_to->p_img_data == NULL) {
            dump_to->img_data_size = 0;
            return IMG_VENC_ERROR;
        }
        dump_to->img_data_size = dump_from->data_size[0];
        dump_to->img_data_type = IMG_H264_TYPE_PF;
        memcpy(dump_to->p_img_data, dump_from->data[0], dump_from->data_size[0]);
        return IMG_H264_TYPE_PF;
    } else {
        debug("[kvmv]venc error!\r\n");
        return IMG_VENC_ERROR;
    }
}

void set_h264_gop(uint8_t _gop)
{
    kvm_venc.enc_h264_init = 0; // call
    kvmvenc_gop = maxmin_data(100, 1, (int)_gop);
    debug("[kvmv] set_h264_gop = %d\n", kvmvenc_gop);
}

void set_frame_detact(uint8_t _frame_detact)
{
    uint8_t frame_detact = maxmin_data(100, 0, (int)_frame_detact);
    kvmv_cfg.frame_detact = frame_detact;
    debug("[kvmv] set_frame_detact = %d\n", kvmv_cfg.frame_detact);
    kvmv_cfg.stream_stop = 0;
}

int8_t raw_to_h264(image::Image *raw, kvmv_data_t* ret_stream, uint16_t _qlty)
{
	uint64_t __attribute__((unused)) start_time;
	uint64_t __attribute__((unused)) frame_time;
	int8_t ret = 0;
	static uint8_t P_Frame_Count = 0;
		// start_time = time::time_ms();
		// log::info("getimg: %d \r\n", (int)(time::time_ms()));
    mmf_stream_t _stream = {0};
    if(kvm_venc.enc_h264_init != 1 || raw->width() != kvm_venc.kvm_venc_cfg.w || raw->height() != kvm_venc.kvm_venc_cfg.h || _qlty != kvm_venc.kvm_venc_cfg.bitrate){
		debug("[kvmv]init_venc_h264	enc_h264_init = %d; raw->width() = %d | %d raw->height() = %d | %d \n",
				kvm_venc.enc_h264_init,
				raw->width(), kvm_venc.kvm_venc_cfg.w,
				raw->height(), kvm_venc.kvm_venc_cfg.h);

		init_venc_h264(raw->width(), raw->height(), _qlty);
        debug("[kvmv]init_venc_h264 finish enc_h264_init = %d; raw->width() = %d | %d raw->height() = %d | %d \n",
				kvm_venc.enc_h264_init,
				raw->width(), kvm_venc.kvm_venc_cfg.w,
				raw->height(), kvm_venc.kvm_venc_cfg.h);
        // if(kvm_venc.enc_h264_init == 1){
		// 	init_venc_h264(raw->width(), raw->height(), _qlty);
		// } else {
		// 	init_venc_h264(default_vpss_width, default_vpss_height, default_h264_qlty);
		// }
    }
	// log::info("init(): %d \r\n", (int)(time::time_ms() - start_time));
    if (mmf_venc_push(kvm_venc.mmf_venc_chn, (uint8_t *)raw->data(), raw->width(), raw->height(), mmf_invert_format_to_mmf(raw->format()))) {
        mmf_del_venc_channel(kvm_venc.mmf_venc_chn);
        kvm_venc.enc_h264_init = 0;
        // rtmp->unlock();
		debug("[kvmv]mmf venc push failed!\n");
        // err::check_raise(err::ERR_RUNTIME, "mmf venc push failed!\r\n");
        return -1;
    }
	// log::info("push(): %d \r\n", (int)(time::time_ms() - start_time));
    if (mmf_venc_pop(kvm_venc.mmf_venc_chn, &_stream)) {
        // log::error("mmf_venc_pop failed\n");
        mmf_venc_free(kvm_venc.mmf_venc_chn);
        mmf_del_venc_channel(kvm_venc.mmf_venc_chn);
        kvm_venc.enc_h264_init = 0;
		debug("[kvmv]mmf venc push failed!\n");
        // rtmp->unlock();
        return -1;
    }
	// log::info("pop(): %d \r\n", (int)(time::time_ms() - start_time));
    ret = h264_stream_dump(ret_stream, &_stream);
    mmf_venc_free(kvm_venc.mmf_venc_chn);
	// log::info("dump(): %d \r\n", (int)(time::time_ms() - start_time));

	// debug("[kvmv]_stream.data[0][4] = %d;\n", _stream.data[0][4]);
	debug("[kvmv]Frame size = %d;\n", ret_stream->img_data_size);

    if(ret == IMG_H264_TYPE_IF){
		debug("[kvmv]================ GOP = %d ================\n", kvm_venc.kvm_venc_cfg.gop);
		debug("[kvmv]SPS; PPS; I-Frame, I-Frame size = %d\n", ret_stream->img_data_size);
		P_Frame_Count = 0;

    } else if(ret == IMG_H264_TYPE_PF){
		debug("[kvmv]P-Frame size = %d, P-count = %d\n", ret_stream->img_data_size, P_Frame_Count);
		P_Frame_Count++;
    }
    debug("[kvmv]dump ret = %d\n", ret);
    return ret;
}

void kvmv_init(uint8_t _debug_info_en)
{
    pthread_t thread;
    pthread_mutex_init(&vi_mutex, NULL);
    if(hdmi_wake_fd < 0) hdmi_wake_fd = eventfd(0, EFD_CLOEXEC);
    if(_debug_info_en == 0) debug_en = 0;
    else                    debug_en = 1;

    // debug("[kvmv]kvmv_init - 1\r\n");

    cam->hmirror(1);
    cam->vflip(1);
    pthread_mutex_lock(&vi_mutex);
    cam->restart(default_vpss_width, default_vpss_height, image::FMT_YVU420SP);
    pthread_mutex_unlock(&vi_mutex);
    for(int i = 0; i < kvmv_data_buffer_size; i++){
        kvmv_data_buffer[i].p_img_data = NULL;
    }

    kvmv_cfg.try_exit_thread = 0;
    // debug("[kvmv]kvmv_init - 2\r\n");

    if(kvmv_cfg.thread_is_running == 1){
        debug("[kvmv]thread is running!\r\n");
    } else {
        if (0 != pthread_create(&hdmi_thread, NULL, hdmi_state_machine, NULL)) {
            debug("[kvmv]create hdmi_state_machine thread failed!\r\n");
            // return -1;
        } else {
            hdmi_thread_started = 1;
        }

        if (0 != pthread_create(&thread, NULL, watchdog_sf_feed, NULL)) {
            debug("[kvmv]create watchdog_sf_feed thread failed!\r\n");
            // return -1;
        }
    }
    // debug("[kvmv]kvmv_init - 3\r\n");
}

uint8_t check_kvmv(uint8_t _try_num)
{
    if(__atomic_load_n(&kvmv_cfg.hdmi_cable_state, __ATOMIC_ACQUIRE) == 0){
        debug("[kvmv]HDMI Cable not exist!\n");
        return 0;
    }
    if(_try_num >= KVMV_MAX_TRY_NUM){
        debug("[kvmv]try_num >= KVMV_MAX_TRY_NUM!\n");
        return 0;
    }
    debug("[kvmv]just try again\n");
    return 1;
}

void set_venc_auto_recyc(uint8_t _enable)
{
    if(_enable) kvmv_cfg.venc_auto_recyc = 1;
    else kvmv_cfg.venc_auto_recyc = 0;
}

/**********************************************************************************
 * @name    kvmv_read_img
 * @author  Sipeed BuGu
 * @date    2024/10/25
 * @version R1.0
 * @brief   Acquire the encoded image with auto init
 * @param	_width				@input: 	Output image width
 * @param	_height				@input: 	Output image height
 * @param	_type				@input: 	Encode type
 * @param	_qlty				@input: 	MJPEG: (50-100) | H264:  (500-10000)
 * @param	_pp_kvm_data		@output: 	Encode data
 * @param	_p_kvmv_data_size	@output: 	Encode data size
 * @return
        -7: HDMI INPUT RES ERROR
        -6: Unsupported resolution, please modify it in the host settings.
        -5: Retrieving image, please wait
        -4: Modifying image resolution, please wait
        -3: img buffer full
        -2: VENC Errorl
        -1: No images were acquired
         0: Acquire MJPEG encoded images
         1: Acquire H264 encoded images(SPS)[Deprecated]
         2: Acquire H264 encoded images(PPS)[Deprecated]
         3: Acquire H264 encoded images(I)
         4: Acquire H264 encoded images(P)
         5: IMG not changed
 **********************************************************************************/
int kvmv_read_img(uint16_t _width, uint16_t _height, uint8_t _type, uint16_t _qlty, uint8_t** _pp_kvm_data, uint32_t* _p_kvmv_data_size)
{
    *_pp_kvm_data = NULL;
    *_p_kvmv_data_size = 0;
    static uint8_t frame_undetact_count = 0;
	// uint64_t __attribute__((unused)) start_time = time::time_ms();
    debug("[kvmv]kvmv_read_img type = %d...\n", _type);
    struct timespec ts;
    clock_gettime(CLOCK_REALTIME, &ts);
    ts.tv_sec += 1;
    // pthread_mutex_lock(&vi_mutex);         // Add lock
    int mutex_res = pthread_mutex_timedlock(&vi_mutex, &ts);
    if(mutex_res != 0){
        return -5;
    }
    if (kvmv_cfg.hdmi_res_err == ERROR_RES){
        pthread_mutex_unlock(&vi_mutex);
        return -7;
    }
    if (kvmv_cfg.hdmi_res_type == UNSUPPORT_RES){
        pthread_mutex_unlock(&vi_mutex);
        return -6;
    }
    // hdmi_state is written only under vi_mutex, which we hold: the VI
    // channel cannot be torn down while this read uses it.
    if (hdmi_state == HDMI_NO_SIGNAL){
        pthread_mutex_unlock(&vi_mutex);
        return IMG_NOT_EXIST;
    }
    if (hdmi_state != HDMI_STREAMING){
        pthread_mutex_unlock(&vi_mutex);
        return -4;
    }
    uint8_t try_num = 0;
    do {
        if(kvmv_cfg.vpss_width != _width || kvmv_cfg.vpss_height != _height){
            if(_width == 0 || _height == 0){
                // Follow the HDMI output
                kvmv_cfg.vpss_width = kvmv_cfg.vi_width;
                kvmv_cfg.vpss_height = kvmv_cfg.vi_height;

                if(kvmv_cfg.Auto_res == 0){
                    kvmv_cfg.Auto_res = 1;
                    cam->set_resolution(kvmv_cfg.vpss_width, kvmv_cfg.vpss_height);
                    kvmv_cfg.reinit_flag = 1;
                }

            } else {
                kvmv_cfg.Auto_res = 0;
                // Set the output
                kvmv_cfg.vpss_width = _width;
                kvmv_cfg.vpss_height = _height;

                cam->set_resolution(kvmv_cfg.vpss_width, kvmv_cfg.vpss_height);
                kvmv_cfg.reinit_flag = 1;
            }
        }

        //
        if (kvmv_cfg.reinit_flag == 1) {
            cam->hmirror(1);
            cam->vflip(1);
			kvmv_cfg.reinit_flag = 0;
        }
        // debug("[kvmv]befor read img: %d \r\n", (int)(time::time_ms() - start_time));
        image::Image *img = cam->read();
        // debug("[kvmv]read img: %d \r\n", (int)(time::time_ms() - start_time));

        // Do not restart VI after HDMI idle. Reopening the MMF
        // channel can exhaust the carveout heap when the detector
        // thread is also transitioning. Consume queued frames
        // instead; the camera buffer contains at most three frames.
        // Discard in its own bounded loop (fresh_frame_count only ever
        // decrements) so that it does not spend the try_num budget.
        while(img != NULL && kvmv_cfg.fresh_frame_count != 0){
            kvmv_cfg.fresh_frame_count--;
            delete img;
            img = cam->read();
        }

        if(img != NULL){
            // frame detect
            if(_type == VENC_MJPEG && kvmv_cfg.frame_detact != 0){
                if(kvmv_cfg.stream_stop == 0){
                    frame_undetact_count++;
                } else {
                    frame_undetact_count = kvmv_cfg.frame_detact;
                }

                if(frame_undetact_count == kvmv_cfg.frame_detact){
                    frame_undetact_count = 0;

                    if(frame_changed(img) == 0){
                        debug("[kvmv]frame not changed...\n");
                        kvmv_cfg.stream_stop = 1;
                        delete img;
                        pthread_mutex_unlock(&vi_mutex);
                        return 5;
                    } else {
                        kvmv_cfg.stream_stop = 0;
                    }
                }
            }
        } else {
            delete img;
            debug("[kvmv]can`t get img...\n");
            continue;
            // pthread_mutex_unlock(&vi_mutex);
            // return IMG_NOT_EXIST;
        }
        // debug("[kvmv]cheak img null?: %d \r\n", (int)(time::time_ms() - start_time));

        // img exist
        // Encode
        if(kvmv_cfg.venc_type == VENC_MJPEG && kvmv_cfg.venc_type != _type){
            if(kvmv_cfg.venc_auto_recyc == 1){
                mmf_enc_jpg_deinit(0);
            }
            kvm_venc.enc_h264_init = 1;
        }
        if(kvmv_cfg.venc_type == VENC_H264 && kvmv_cfg.venc_type != _type){
            if(kvmv_cfg.venc_auto_recyc == 1){
                mmf_del_venc_channel(kvm_venc.mmf_venc_chn);
            }
            kvm_venc.enc_h264_init = 0;
        }

        kvmv_cfg.venc_type = _type;

        if(kvmv_cfg.venc_type == VENC_MJPEG){
            kvmv_data_t* p_kvmv_data = get_save_buffer();
            if(p_kvmv_data == NULL){
                // buffer full
			    delete img;
                debug("[kvmv]jpg buffer full\n");
                pthread_mutex_unlock(&vi_mutex);
                return IMG_BUFFER_FULL;
            }
            image::Image *jpg = img->to_jpeg(maxmin_data(99, 51, (int)_qlty));
            if(jpg == NULL || !jpg_dump(p_kvmv_data, jpg)){
                delete jpg;
			    delete img;
                debug("[kvmv]failed to allocate jpg buffer\n");
                pthread_mutex_unlock(&vi_mutex);
                return IMG_BUFFER_FULL;
            }
            delete jpg;
			delete img;
            *_pp_kvm_data = p_kvmv_data->p_img_data;
            *_p_kvmv_data_size = p_kvmv_data->img_data_size;
            pthread_mutex_unlock(&vi_mutex);
            return IMG_MJPEG_TYPE;
        } else if (kvmv_cfg.venc_type == VENC_H264){
            int ret;
            kvmv_data_t* p_kvmv_data = get_save_buffer();
            if(p_kvmv_data == NULL){
                // buffer full
			    delete img;
                *_pp_kvm_data = NULL;
                pthread_mutex_unlock(&vi_mutex);
                return IMG_BUFFER_FULL;
            }
            // debug("[kvmv]get_save_buffer: %d \r\n", (int)(time::time_ms() - start_time));
            ret = raw_to_h264(img, p_kvmv_data, maxmin_data(10000, 500, (int)_qlty));
            // debug("[kvmv]venc raw_to_h264: %d \r\n", (int)(time::time_ms() - start_time));
			delete img;
            *_pp_kvm_data = p_kvmv_data->p_img_data;
            *_p_kvmv_data_size = p_kvmv_data->img_data_size;
            pthread_mutex_unlock(&vi_mutex);
            return ret;
        }
    } while (check_kvmv(try_num++));
    // debug("[kvmv]return: %d \r\n", (int)(time::time_ms() - start_time));
    *_pp_kvm_data = NULL;
    pthread_mutex_unlock(&vi_mutex);
    return IMG_NOT_EXIST;
}

int free_kvmv_data(uint8_t ** _pp_kvm_data)
{
        // debug("[kvmv]free_kvmv_data - 1\r\n");
    for(int i = 0; i < kvmv_data_buffer_size; i++){
        if(*_pp_kvm_data == kvmv_data_buffer[i].p_img_data){
            // debug("[kvmv]free buffer : %d\n", *_pp_kvm_data);
            if (*_pp_kvm_data != NULL){
        // debug("[kvmv]free_kvmv_data - 2\r\n");
                free(*_pp_kvm_data);
        // debug("[kvmv]free_kvmv_data - 3\r\n");
                kvmv_data_buffer[i].p_img_data = NULL;
                uint8_t _type = kvmv_data_buffer[i].img_data_type;
                return _type;
            } else {
                return IMG_NOT_EXIST;
            }
        }
    }
    return IMG_NOT_EXIST;
}

void free_all_kvmv_data()
{
    for(int i = 0; i < kvmv_data_buffer_size; i++){
        if(kvmv_data_buffer[i].p_img_data != NULL){
            free(kvmv_data_buffer[i].p_img_data);
            kvmv_data_buffer[i].p_img_data = NULL;
        }
    }
}

void kvmv_deinit()
{
    set_hdmi_capture_enabled(0);
    // Stop the state machine first: it is the only other user of vi_mutex
    // and of the VI channel.
    __atomic_store_n(&kvmv_cfg.try_exit_thread, 1, __ATOMIC_RELEASE);
    hdmi_wake();
    if(hdmi_thread_started){
        pthread_join(hdmi_thread, NULL);
        hdmi_thread_started = 0;
    }
    pthread_mutex_destroy(&vi_mutex);
    cam->close();
    mmf_deinit();
    free_all_kvmv_data();
}

uint8_t kvmv_hdmi_control(uint8_t _en)
{
    if(_en == 0){
        set_hdmi_capture_enabled(0);
    }

    if(kvmv_cfg.hw_version == 0){

        FILE *fp;
	    uint8_t RW_Data[2];
        if(access("/etc/kvm/hw", F_OK) == 0){
            fp = fopen("/etc/kvm/hw", "r");
            fread(RW_Data, sizeof(char), 1, fp);
            fclose(fp);
            switch(RW_Data[0]){
                case 'a':
                case 'b':
                case 'p':
                    kvmv_cfg.hw_version = RW_Data[0];
                    break;
                default :
                    kvmv_cfg.hw_version = 'a';
            }
        }
    }
    if(kvmv_cfg.hw_version != 'p'){
        debug("[kvmv]Hardware not support!\n");
        if(_en != 0){
            set_hdmi_capture_enabled(1);
        }
        return -1;
    }
    // The LT6911 reset line is "hdmi-rst" (DT line name), owned by the
    // kvmgpio daemon S15kvmhwd started; vendor sysfs gpio451 only when no
    // line carries the name. A failure leaves the state machine untouched.
    if(_en == 0){
        // lt_mutex is held by the state machine for the whole of an LT6911
        // read, so taking it here waits for a read in flight and keeps the
        // machine off the bus while the chip is held in reset.
        pthread_mutex_lock(&lt_mutex);
        hdmi_stopped = 1;
        pthread_mutex_unlock(&lt_mutex);
        if(nk_gpio_set_held("hdmi-rst", 451, 0) != 0){
            fprintf(stderr, "[kvmv]hdmi-rst: cannot hold the LT6911 in reset\n");
            return -1;
        }
        return 0;
    } else {
        if(nk_gpio_set_held("hdmi-rst", 451, 1) != 0){
            fprintf(stderr, "[kvmv]hdmi-rst: cannot release the LT6911 reset\n");
            return -1;
        }
        pthread_mutex_lock(&lt_mutex);
        hdmi_stopped = 0;
        pthread_mutex_unlock(&lt_mutex);

        // Keep the existing VI channel and consume queued frames after idle.
        // Reopening it here or from kvmv_read_img can exhaust the carveout
        // heap while the HDMI detector is transitioning.
        kvmv_cfg.fresh_frame_count = fresh_frame_discard_count;
        set_hdmi_capture_enabled(1);
        // The chip came out of reset: have the state machine re-read it.
        hdmi_post_resync();
        return 0;
    }
    return -1;
}

uint8_t kvmv_hdmi_signal_active()
{
    uint8_t enabled = __atomic_load_n(&hdmi_capture_enabled, __ATOMIC_ACQUIRE);
    uint8_t active = __atomic_load_n(&hdmi_signal_active, __ATOMIC_ACQUIRE);
    return enabled != 0 && active != 0 ? 1 : 0;
}
