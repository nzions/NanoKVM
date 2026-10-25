#include "system_ctrl.h"
#include <errno.h>

using namespace maix;
using namespace maix::sys;

extern kvm_sys_state_t kvm_sys_state;
extern kvm_oled_state_t kvm_oled_state;

// no look-alike characters (0/O, 1/l/I); the AP one is lowercase for typing on a phone
#define AP_PASS_ALPHABET	"abcdefghjkmnpqrstuvwxyz23456789"
#define RESET_PASS_ALPHABET	"23456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

static uint64_t wifi_ap_start_ms = 0;
static uint8_t seed_pass_shown = 0;

// Fill out with len characters of alphabet from /dev/urandom (rejection
// sampling, no modulo bias) and terminate it. Returns 1 on success.
static uint8_t random_string(char *out, size_t len, const char *alphabet)
{
	size_t n = strlen(alphabet);
	size_t limit = 256 - (256 % n);
	size_t i = 0;
	uint8_t buf[32];
	int fd = open("/dev/urandom", O_RDONLY);
	if(fd < 0) return 0;
	while(i < len){
		ssize_t got = read(fd, buf, sizeof(buf));
		if(got < 0 && errno == EINTR) continue;
		if(got <= 0){
			close(fd);
			return 0;
		}
		for(ssize_t j = 0; j < got && i < len; j++){
			if(buf[j] < limit) out[i++] = alphabet[buf[j] % n];
		}
	}
	close(fd);
	out[len] = '\0';
	return 1;
}

// Write text to path, created 0600 (an existing file is tightened too).
static uint8_t write_secret_file(const char *path, const char *text)
{
	size_t len = strlen(text);
	int fd = open(path, O_WRONLY | O_CREAT | O_TRUNC, 0600);
	if(fd < 0) return 0;
	if(fchmod(fd, 0600) != 0){
		close(fd);
		return 0;
	}
	ssize_t written = write(fd, text, len);
	if(close(fd) != 0) return 0;
	return written == (ssize_t)len;
}

// Show the first-boot / reset password on the OLED while its file exists.
void kvm_show_seed_password(void)
{
	if(access(SEED_PASS_FILE, F_OK) != 0){
		// the owner changed the password: the file is gone, leave the page
		seed_pass_shown = 0;
		if(kvm_sys_state.page == 2){
			kvm_sys_state.page = 0;
			kvm_sys_state.sub_page = 0;
		}
		return;
	}
	if(!seed_pass_shown && kvm_sys_state.page == 0){
		seed_pass_shown = 1;
		kvm_sys_state.page = 2;
		kvm_sys_state.sub_page = 0;
	}
}

void gen_hostapd_conf(char* ap_ssid)
{
	char file_data[300] = {0};
	FILE * fp;
	sprintf(file_data, "ctrl_interface=/var/run/hostapd\nctrl_interface_group=0\nssid=NanoKVM\nhw_mode=g\nchannel=1\nbeacon_int=100\ndtim_period=2\nmax_num_sta=255\nrts_threshold=-1\nfragm_threshold=-1\nmacaddr_acl=0\nauth_algs=3\nwpa=2\nwpa_passphrase=%s\nieee80211n=1\n", ap_ssid);
	fp = fopen("/etc/hostapd.conf", "w");
	fwrite(file_data, sizeof(file_data) , 1, fp );
	fclose(fp);
}

void gen_udhcpd_conf()
{
	char file_data[300] = {0};
	FILE * fp;
	sprintf(file_data, "start 10.10.10.100\nend 10.10.10.200\ninterface wlan0\npidfile /var/run/udhcpd.wlan0.pid\nlease_file /var/lib/misc/udhcpd.wlan0.leases\noption subnet 255.255.255.0\noption lease 864000\n");
	fp = fopen("/etc/udhcpd.wlan0.conf", "w");
	fwrite(file_data, sizeof(file_data) , 1, fp );
	fclose(fp);
}

void gen_dnsmasq_conf()
{
	char file_data[300] = {0};
	FILE * fp;
	sprintf(file_data, "bind-interfaces\ninterface=wlan0\ndhcp-range=10.10.10.2,10.10.10.254\naddress=/#/10.0.0.1\n");
	fp = fopen("/etc/udhcpd.wlan0.conf", "w");
	fwrite(file_data, sizeof(file_data) , 1, fp );
	fclose(fp);
}

uint8_t sta_connect_ap(void)
{
	uint8_t RW_Data[10];	
	FILE *fp;
	fp = popen("hostapd_cli all sta | grep aid", "r");
	fgets((char*)RW_Data, 2, fp);
	pclose(fp);
	if(RW_Data[0] == 'a') return 1;
	else return 0;
}

uint8_t ssid_pass_ok(void)
{
	if(access("/kvmapp/kvm/wifi_try_connect", F_OK) == 0) return 1;
	else return 0;
}

uint8_t wifi_connected(void)
{
	uint8_t RW_Data[20];	
	FILE *fp;
	fp = popen("wpa_cli -i wlan0 status | grep \"wpa_state\"", "r");
	fgets((char*)RW_Data, 15, fp);
	pclose(fp);
	if(RW_Data[10] == 'S') return 0;
	else if(RW_Data[10] == 'C') return 1;
	else return 0;
}

void kvm_start_wifi_config_process(void)
{
	if(kvm_sys_state.wifi_config_process == -1){
		// printf("[kvmw]开始配置wifi\n");
		kvm_sys_state.wifi_config_process = 0;
		kvm_sys_state.sub_page = 0;
	}
}

void kvm_wifi_web_config_process()
{
	if(ssid_pass_ok()){
		system("rm /kvmapp/kvm/wifi_try_connect");
		system("/etc/init.d/S30wifi restart");
		kvm_sys_state.wifi_config_process = 3;
		// 尝试连接wifi
		if(kvm_sys_state.wifi_config_process == 3){
			// time::sleep_ms(WIFI_CONNECTION_DELAY);
			if(wifi_connected()){
				// 连上了
				kvm_sys_state.wifi_config_process = -1;
			} else {
				// 没连上
				kvm_sys_state.wifi_config_process = 0;
			}
		}
	}
}

void kvm_wifi_config_process()
{	
	if((kvm_sys_state.wifi_config_process == 1 || kvm_sys_state.wifi_config_process == 2) &&
		time::time_ms() - wifi_ap_start_ms >= WIFI_AP_TIMEOUT_MS){
		// nobody configured the device in time: stop the AP, back to idle
		system("/etc/init.d/S30wifi restart");
		unlink("/kvmapp/kvm/ap.pass");
		memset(kvm_sys_state.wifi_ap_pass, 0, sizeof(kvm_sys_state.wifi_ap_pass));
		kvm_sys_state.wifi_config_process = -1;
		kvm_sys_state.page = 0;
		kvm_sys_state.sub_page = 0;
		return;
	}
	switch(kvm_sys_state.wifi_config_process){
		case -1:
			// printf("[kvms]Not in the process of configuring WiFi\n");
			break;
		case 0: // 启动wifi
			if(!random_string(kvm_sys_state.wifi_ap_pass, WIFI_AP_PASS_LEN, AP_PASS_ALPHABET) ||
				!write_secret_file("/kvmapp/kvm/ap.pass", kvm_sys_state.wifi_ap_pass)){
				// no password, no AP
				memset(kvm_sys_state.wifi_ap_pass, 0, sizeof(kvm_sys_state.wifi_ap_pass));
				kvm_sys_state.wifi_config_process = -1;
				kvm_sys_state.page = 0;
				kvm_sys_state.sub_page = 0;
				break;
			}

			system("echo NanoKVM > /kvmapp/kvm/ap.ssid");
			system("/etc/init.d/S30wifi ap");
			wifi_ap_start_ms = time::time_ms();

			kvm_sys_state.wifi_config_process = 1;
			kvm_sys_state.sub_page = 1;
			break;
		case 1: // 等待设备连接
			if(sta_connect_ap()){
				kvm_sys_state.wifi_config_process = 2;
				kvm_sys_state.sub_page = 3;
			}
			break;
		case 2: // 等待输入帐号密码
			if(ssid_pass_ok()){
				system("rm /kvmapp/kvm/wifi_try_connect");
				system("/etc/init.d/S30wifi restart");
				kvm_sys_state.wifi_config_process = 3;
				kvm_sys_state.sub_page = 5;
			}
			break;
		case 3: // 尝试连接wifi
			time::sleep_ms(WIFI_CONNECTION_DELAY);
			if(wifi_connected()){
				// 连上了
				kvm_sys_state.wifi_config_process = -1;
				kvm_sys_state.page = 0;
				kvm_sys_state.sub_page = 0;
			} else {
				// 没连上
				kvm_sys_state.wifi_config_process = 0;
				kvm_sys_state.sub_page = 0;
			}
			break;
	}
}

// Set a fresh random root password, make the web store reseed from it, and show
// it on the OLED. Returns 0 on success.
uint8_t kvm_reset_password(void)
{
	char pass[RESET_PASS_LEN + 1] = {0};
	char line[RESET_PASS_LEN + 8];

	if(!random_string(pass, RESET_PASS_LEN, RESET_PASS_ALPHABET)){
		perror("urandom");
		return 1;
	}

	FILE *fp = popen("chpasswd", "w");
	if (fp == NULL) {
		perror("popen");
		return 1;
	}
	fprintf(fp, "root:%s\n", pass);
	if (pclose(fp) != 0) {
		perror("pclose");
		return 1;
	}

	// the web store seeds the admin account from this file once /etc/kvm/pwd is gone
	snprintf(line, sizeof(line), "%s\n", pass);
	if(!write_secret_file(SEED_PASS_FILE, line)){
		perror("write seed password");
		return 1;
	}
	unlink("/etc/kvm/pwd");
	sync();

	seed_pass_shown = 1;
	kvm_sys_state.page = 2;
	kvm_sys_state.sub_page = 0;
	return 0;
}