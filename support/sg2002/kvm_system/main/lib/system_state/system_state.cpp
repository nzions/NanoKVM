#include "config.h"
#include "system_state.h"
#include "vi_state_shared.hpp"
#include <sys/socket.h>
#include <sys/un.h>
#include <net/if.h>
#include <sys/ioctl.h>
#include <pthread.h>
#include <unistd.h>

using namespace maix;
using namespace maix::sys;

extern kvm_sys_state_t kvm_sys_state;
extern kvm_oled_state_t kvm_oled_state;

int get_nic_state(const char* interface_name)
{
	int sock;
	struct ifreq ifr;
	int ret = NIC_STATE_NO_EXIST;
	if ((sock = socket(AF_INET, SOCK_STREAM, 0)) < 0) {
		return ret;
	}
	strcpy(ifr.ifr_name, interface_name);
	if (ioctl(sock, SIOCGIFFLAGS, &ifr) < 0) {
		close(sock);
		return ret;
	}
	if (ifr.ifr_flags & IFF_UP) {
		if (ifr.ifr_flags & IFF_RUNNING) {
			ret = NIC_STATE_RUNNING;
		} else {
			ret = NIC_STATE_UP;
		}
	} else {
		ret = NIC_STATE_DOWN;
	}
	close(sock);
	return ret;
}

int get_ping_allow_state(void)
{
	if(access("/etc/kvm/stop_ping", F_OK) == 0) {
		kvm_sys_state.ping_allow = 0;
	} else {
		kvm_sys_state.ping_allow = 1;
	}
	return kvm_sys_state.ping_allow;
}

// net_port
int get_ip_addr(ip_addr_t ip_type)
{
	switch (ip_type){
		case ETH_IP: // eth_addr
			if(strcmp(ip_address()["eth0"].c_str(), (char*)kvm_sys_state.eth_addr) != 0){
				if(*(ip_address()["eth0"].c_str()) == 0){
					printf("can`t get ip addr\r\n");
					kvm_sys_state.eth_addr[0] = 0;
					return 0;
				} 
				for(int i = 0; i <= 15; i++)
				{
					kvm_sys_state.eth_addr[i] = *(ip_address()["eth0"].c_str() + i);
					printf("%c", kvm_sys_state.eth_addr[i]);
				}
				printf("\r\n");
			}
			return 1;
		case WiFi_IP: // wifi_addr
			if(strcmp(ip_address()["wlan0"].c_str(), (char*)kvm_sys_state.wifi_addr) != 0){
				if(*(ip_address()["wlan0"].c_str()) == 0){
					printf("can`t get ip addr\r\n");
					kvm_sys_state.wifi_addr[0] = 0;
					return 0;
				} 
				for(int i = 0; i <= 15; i++)
				{
					kvm_sys_state.wifi_addr[i] = *(ip_address()["wlan0"].c_str() + i);
					printf("%c", kvm_sys_state.wifi_addr[i]);
				}
				printf("\r\n");
			}
			return 1;
		case Tailscale_IP: // tail_addr
			if(*(ip_address()["tailscale0"].c_str()) == 0){
				printf("can`t get ip addr\r\n");
				kvm_sys_state.tail_addr[0] = 0;
				return 0;
			} 
			for(int i = 0; i <= 15; i++)
			{
				kvm_sys_state.tail_addr[i] = *(ip_address()["tailscale0"].c_str() + i);
				printf("%c", kvm_sys_state.tail_addr[i]);
			}
			printf("\r\n");
			return 1;
		case RNDIS_IP: // rndis_addr
			if(*(ip_address()["usb0"].c_str()) == 0){
				printf("can`t get ip addr\r\n");
				kvm_sys_state.rndis_addr[0] = 0;
				return 0;
			} 
			for(int i = 0; i <= 15; i++)
			{
				kvm_sys_state.rndis_addr[i] = *(ip_address()["usb0"].c_str() + i);
				printf("%c", kvm_sys_state.rndis_addr[i]);
			}
			printf("\r\n");
			return 1;
		case ETH_ROUTE: // eth_route
			if(access("/etc/kvm/gateway", F_OK) != 0){
				// 不存在gateway文件
				memset( kvm_sys_state.eth_route, 0, sizeof( kvm_sys_state.eth_route ) );
				char Cmd[100]={0};
				memset( Cmd, 0, sizeof( Cmd ) );
				sprintf( Cmd,"ip route | grep -i '^default' | grep -i 'eth0' | awk '{print $3}'");
				FILE* fp = popen( Cmd, "r" );
				if ( NULL == fp )
				{
					pclose(fp);
					return 0;
				}
				memset( kvm_sys_state.eth_route, 0, sizeof( kvm_sys_state.eth_route ) );
				while ( NULL != fgets( (char*)kvm_sys_state.eth_route,sizeof( kvm_sys_state.eth_route ),fp ))
				{
					// printf("ip=%s\n",kvm_sys_state.eth_route);
					break;
				}
				if(kvm_sys_state.eth_route[0] == 0){
					// 开机时未插入ETH
					pclose(fp);
					return 0;
				}
				for(int i = 0; i < 40; i++){
					if(kvm_sys_state.eth_route[i] == 10){
						kvm_sys_state.eth_route[i] = ' ';
						break;
					}
				}
				pclose(fp);
				return 1;
			} else {
				int file_size;
				FILE *fp = fopen("/etc/kvm/gateway", "r");
				fseek(fp, 0, SEEK_END);
				file_size = ftell(fp); 
				fseek(fp, 0, SEEK_SET);
				fread(kvm_sys_state.eth_route, sizeof(char), file_size, fp);
				fclose(fp);
				return 1;
			}
		case WiFi_ROUTE: // wifi_route
			memset( kvm_sys_state.wifi_route, 0, sizeof( kvm_sys_state.wifi_route ) );
			char Cmd[100]={0};
			memset( Cmd, 0, sizeof( Cmd ) );
			sprintf( Cmd,"ip route | grep -i '^default' | grep -i 'wlan0' | awk '{print $3}'");
			FILE* fp = popen( Cmd, "r" );
			if ( NULL == fp )
			{
				pclose(fp);
				return 0;
			}
			memset( kvm_sys_state.wifi_route, 0, sizeof( kvm_sys_state.wifi_route ) );
			while ( NULL != fgets( (char*)kvm_sys_state.wifi_route,sizeof( kvm_sys_state.wifi_route ),fp ))
			{
				// printf("ip=%s\n",kvm_sys_state.wifi_route);
				break;
			}
			if(kvm_sys_state.wifi_route[0] == 0){
				// 开机时未插入ETH
				pclose(fp);
				return 0;
			}
			for(int i = 0; i < 40; i++){
				if(kvm_sys_state.wifi_route[i] == 10){
					kvm_sys_state.wifi_route[i] = ' ';
					break;
				}
			}
			pclose(fp);
			return 1;
	}
	return 0;
}

int chack_net_state(ip_addr_t use_ip_type)
{
	char Cmd[100]={0};
	if		(use_ip_type == ETH_ROUTE)  sprintf( Cmd,"ping -I eth0 -w 1 %s > /dev/null", kvm_sys_state.eth_route);
	else if	(use_ip_type == WiFi_ROUTE) sprintf( Cmd,"ping -I wlan0 -w 1 %s > /dev/null", kvm_sys_state.wifi_route);
	else return -1;	// 不支持的端口
	if(system(Cmd) == 0){	// 256：不通； = 0：通
		return 1;
	}
	return 0;
}

void patch_eth_wifi(void)
{
	// system("ip link set eth0 down");
	// system("ip link set eth0 up");
	// system("udhcpc -i eth0 &");
}

int kvm_wifi_exist()
{
	if (get_nic_state("wlan0") == NIC_STATE_NO_EXIST) return 0;
	else return 1;
}

void kvm_update_usb_state()
{
	// usb_state, hid_state, rndis_state, udisk_state
	FILE *fp;
	int file_size;
	uint8_t RW_Data[10];		
	fp = fopen("/sys/class/udc/4340000.usb/state", "r");
	fseek(fp, 0, SEEK_END);
	file_size = ftell(fp); 
	fseek(fp, 0, SEEK_SET);
	fread(RW_Data, sizeof(char), file_size, fp);
	fclose(fp);
	if(RW_Data[0] == 'n') kvm_sys_state.usb_state = 0;
	else if(RW_Data[0] == 'c') kvm_sys_state.usb_state = 1;
	else kvm_sys_state.usb_state = -1;
	// hid_state & udisk_state (rndis_state单独处理)
	if(kvm_sys_state.usb_state == 1){
		if(access("/sys/kernel/config/usb_gadget/g0/configs/c.1/hid.GS*", F_OK) == 0) 
			kvm_sys_state.hid_state = 1;
		if(access("/sys/kernel/config/usb_gadget/g0/configs/c.1/mass_storage.disk0", F_OK) == 0) 
			kvm_sys_state.udisk_state = 1;
	} else {
		kvm_sys_state.hid_state = 0;
		kvm_sys_state.udisk_state = 0;
	}
}

void kvm_update_hdmi_state()
{
	const uint32_t shared_state_ttl_ms = 30000U;
	const uint32_t fallback_state_ttl_ms = 10000U;
	static uint8_t check_times = 4;
	static vi_state_shared::State fallback_state = {};
	static uint32_t fallback_updated_ms = 0;
	static uint32_t fallback_attempted_ms = 0;
	static bool fallback_valid = false;
	static bool fallback_attempted = false;
	static bool shared_failure_logged = false;
	static bool fallback_failure_logged = false;
	if(++check_times > 5){
		check_times = 0;
		vi_state_shared::State state = {};
		vi_state_shared::ReadStatus status = vi_state_shared::read_state(&state, shared_state_ttl_ms);
		if (status == vi_state_shared::READ_OK) {
			kvm_sys_state.hdmi_state = state.fps == 0 ? 0 : 1;
			if (shared_failure_logged) {
				fprintf(stderr, "[kvm_system] VI shared state recovered\n");
			}
			shared_failure_logged = false;
			fallback_failure_logged = false;
			return;
		}

		if (!shared_failure_logged) {
			fprintf(stderr, "[kvm_system] VI shared state %s; using direct fallback\n",
				vi_state_shared::read_status_name(status));
			shared_failure_logged = true;
		}

		uint32_t now = vi_state_shared::monotonic_ms();
		if (!fallback_attempted || now - fallback_attempted_ms >= fallback_state_ttl_ms) {
			fallback_attempted_ms = now;
			fallback_attempted = true;
			uint32_t fields = vi_state_shared::FIELD_NONE;
			vi_state_shared::State direct_state = {};
			vi_state_shared::ProcReadStatus direct_status =
				vi_state_shared::read_proc_state(&direct_state, &fields);
			if (direct_status == vi_state_shared::PROC_READ_OK &&
				(fields & vi_state_shared::FIELD_FPS) != 0U) {
				fallback_state = direct_state;
				fallback_updated_ms = now;
				fallback_valid = true;
				if (fallback_failure_logged) {
					fprintf(stderr, "[kvm_system] direct VI fallback recovered\n");
				}
				fallback_failure_logged = false;
			} else {
				fallback_valid = false;
				if (!fallback_failure_logged) {
					fprintf(stderr, "[kvm_system] direct VI fallback unavailable\n");
					fallback_failure_logged = true;
				}
			}
		}

		if (fallback_valid && now - fallback_updated_ms <= fallback_state_ttl_ms) {
			kvm_sys_state.hdmi_state = fallback_state.fps == 0 ? 0 : 1;
		} else {
			kvm_sys_state.hdmi_state = -1;
		}
	}
}

void kvm_update_stream_fps(void)
{
	FILE *fp;
	int file_size;
	uint8_t RW_Data[10];

	// FPS
	fp = fopen("/kvmapp/kvm/now_fps", "r");
    fseek(fp, 0, SEEK_END);
    file_size = ftell(fp); 
    fseek(fp, 0, SEEK_SET);
    fread(RW_Data, sizeof(char), file_size, fp);
	fclose(fp);
	RW_Data[file_size] = 0;
	kvm_sys_state.now_fps = atoi((char*)RW_Data);
}

void kvm_update_stream_type(void)
{
	FILE *fp;
	int file_size;
	uint8_t RW_Data[10];

	// type
	fp = fopen("/kvmapp/kvm/type", "r");
    fseek(fp, 0, SEEK_END);
    file_size = ftell(fp); 
    fseek(fp, 0, SEEK_SET);
    fread(RW_Data, sizeof(char), file_size, fp);
	fclose(fp);
	if(RW_Data[0] == 'm') 		kvm_sys_state.type = KVM_TYPE_MJPG;
	else if(RW_Data[0] == 'h') 	kvm_sys_state.type = KVM_TYPE_H264;
	else 						kvm_sys_state.type = KVM_TYPE_none;
}

void kvm_update_stream_qlty(void)
{
	FILE *fp;
	int file_size;
	uint8_t RW_Data[10];
	uint16_t tmp16;

	// QLTY
	fp = fopen("/kvmapp/kvm/qlty", "r");
    fseek(fp, 0, SEEK_END);
    file_size = ftell(fp); 
    fseek(fp, 0, SEEK_SET);
    fread(RW_Data, sizeof(char), file_size, fp);
	fclose(fp);
	RW_Data[file_size] = 0;
	tmp16 = atoi((char*)RW_Data);
	if(kvm_sys_state.type == KVM_TYPE_MJPG){
		if(tmp16 < 60) 						 	kvm_sys_state.qlty = 1;
		else if(tmp16 >= 60 && tmp16 < 75) 	 	kvm_sys_state.qlty = 2;
		else if(tmp16 >= 75 && tmp16 < 90) 	 	kvm_sys_state.qlty = 3;
		else if(tmp16 >= 90 && tmp16 <= 100) 	kvm_sys_state.qlty = 4;
		else 									kvm_sys_state.qlty = 4;
	} else {
		if(tmp16 < 1500) 						kvm_sys_state.qlty = 1;
		else if(tmp16 >= 1500 && tmp16 < 2500) 	kvm_sys_state.qlty = 2;
		else if(tmp16 >= 2500 && tmp16 < 3500) 	kvm_sys_state.qlty = 3;
		else if(tmp16 >= 3500 && tmp16 <= 5000) kvm_sys_state.qlty = 4;
		else 									kvm_sys_state.qlty = 4;
	}
}

// ---- HDMI state pushed by NanoKVM-Server -------------------------------------
// The server writes one JSON line per HDMI transition to a unix socket:
//   {"state":2,"width":1920,"height":1080,"seq":7}
// (server/service/hdmi Snapshot). The current state is the first line of every
// connection. A thread blocks in read; nothing is polled and no file is read.

#define HDMI_SOCKET_PATH		"/var/run/kvmhdmi.sock"
#define HDMI_SOCKET_RETRY_US	1000000		// pause before reconnecting; not a state

enum hdmi_snapshot_state_t {
	HDMI_SNAPSHOT_NO_SIGNAL = 0,
	HDMI_SNAPSHOT_LOCKING = 1,
	HDMI_SNAPSHOT_STREAMING = 2,
	HDMI_SNAPSHOT_RES_CHANGE = 3,
};

typedef struct {
	hdmi_snapshot_state_t state;
	uint16_t width;
	uint16_t height;
	uint64_t seq;
} hdmi_snapshot_t;

static bool parse_hdmi_snapshot(const char *line, hdmi_snapshot_t *out)
{
	unsigned int state, width, height;
	unsigned long long seq;
	if (sscanf(line, "{\"state\":%u,\"width\":%u,\"height\":%u,\"seq\":%llu}",
			&state, &width, &height, &seq) != 4) {
		return false;
	}
	if (state > HDMI_SNAPSHOT_RES_CHANGE || width > 0xFFFF || height > 0xFFFF) {
		return false;
	}
	out->state = (hdmi_snapshot_state_t)state;
	out->width = (uint16_t)width;
	out->height = (uint16_t)height;
	out->seq = (uint64_t)seq;
	return true;
}

static void* thread_hdmi_snapshots(void *arg)
{
	while (KVM_FLAG_LOAD(kvm_sys_state.sys_thread_running)) {
		int fd = socket(AF_UNIX, SOCK_STREAM, 0);
		if (fd < 0) {
			usleep(HDMI_SOCKET_RETRY_US);
			continue;
		}
		struct sockaddr_un addr;
		memset(&addr, 0, sizeof(addr));
		addr.sun_family = AF_UNIX;
		strncpy(addr.sun_path, HDMI_SOCKET_PATH, sizeof(addr.sun_path) - 1);
		if (connect(fd, (struct sockaddr*)&addr, sizeof(addr)) != 0) {
			close(fd);
			usleep(HDMI_SOCKET_RETRY_US);	// the server is not up (yet)
			continue;
		}

		FILE *fp = fdopen(fd, "r");
		if (fp == NULL) {
			close(fd);
			usleep(HDMI_SOCKET_RETRY_US);
			continue;
		}
		char line[128];
		while (fgets(line, sizeof(line), fp) != NULL) {
			hdmi_snapshot_t snapshot;
			if (!parse_hdmi_snapshot(line, &snapshot)) {
				fprintf(stderr, "[kvm_system] malformed hdmi snapshot line: %s", line);
				continue;
			}
			// The resolution is the last one the VI chain was set to; it stays
			// shown when the signal drops.
			kvm_sys_state.hdmi_width = snapshot.width;
			kvm_sys_state.hdmi_height = snapshot.height;
		}
		fclose(fp);		// the server closed the socket: connect again
	}
	return NULL;
}

void kvm_start_hdmi_client(void)
{
	pthread_t thread;
	if (0 != pthread_create(&thread, NULL, thread_hdmi_snapshots, NULL)) {
		fprintf(stderr, "[kvm_system] cannot start the hdmi snapshot thread\n");
		return;
	}
	pthread_detach(thread);
}

void kvm_update_eth_state(void)
{	
	static uint8_t nic_state = 0;
	nic_state = get_nic_state("eth0");

	if(nic_state == NIC_STATE_RUNNING){
		// Get IP
		if(strcmp(ip_address()["eth0"].c_str(), (char*)kvm_sys_state.eth_addr) != 0){
			if(get_ip_addr(ETH_IP)){
				kvm_sys_state.eth_state = 2;
			} else {
				kvm_sys_state.eth_state = 1;
				return;
			}
		}
		if(kvm_sys_state.ping_allow){
			// ping route
			if(kvm_sys_state.eth_route[0] == 0){
				get_ip_addr(ETH_ROUTE);
			} else {
				if(chack_net_state(ETH_ROUTE)){
					// Ping successful
					kvm_sys_state.eth_state = 3;
				} else {
					kvm_sys_state.eth_state = 2;
				}
			}
		} else {
			// Consider the network to be connected
			kvm_sys_state.eth_state = 3;
		}

	} else {
		kvm_sys_state.eth_state = 0;
		patch_eth_wifi();
	}
}

void kvm_update_wifi_state(void)
{	
	// No WiFi module (check for existence?) -> Module exists & not connected (check if connected) ->
	if(kvm_sys_state.wifi_state == -2) return;
	switch (kvm_sys_state.wifi_state){
		case -1:
		// Initial default value.
			if (kvm_wifi_exist()) {
				kvm_sys_state.wifi_state = 0;
				system("touch /etc/kvm/wifi_exist");
			}
			else {
				kvm_sys_state.wifi_state = -2; // WiFi module does not exist, exiting directly.
				system("rm /etc/kvm/wifi_exist");
				return;
			}
			// break;	// Start checking the connection directly.
		case 0:
		// WiFi is available but not connected.
			system("echo 0 > /kvmapp/kvm/wifi_state");
			if (get_ip_addr(WiFi_IP) && get_ip_addr(WiFi_ROUTE)){
				// IP+Route has been acquired
				if(kvm_sys_state.ping_allow){
					if (chack_net_state(WiFi_ROUTE)){
						// Ping successful
						kvm_sys_state.wifi_state = 1;
					}
				} else {
					// Consider the network to be connected
					kvm_sys_state.wifi_state = 1;
				}
			}
			break;
		case 1:
		// Connected to the network & continuously checking if it can ping successfully.
			system("echo 1 > /kvmapp/kvm/wifi_state");
			get_ip_addr(WiFi_IP);
			if(kvm_sys_state.ping_allow){
				if (kvm_sys_state.wifi_route[0] != 0){
					if (chack_net_state(WiFi_ROUTE) == 0){
						// Ping successful
						kvm_sys_state.wifi_state = 0;
					}
				}
			}
		// default:
		// 	kvm_sys_state.wifi_state = -1;
	}
}

void kvm_update_rndis_state(void)
{
	if (get_nic_state("usb0") == NIC_STATE_RUNNING) {
		if(kvm_sys_state.rndis_state != 1) {
			if (get_ip_addr(RNDIS_IP)) {
				kvm_sys_state.rndis_state = 1;
			}
		}
	}
	else kvm_sys_state.rndis_state = 0;
}

void kvm_update_tailscale_state(void)
{
	if (get_nic_state("tailscale0") == NIC_STATE_RUNNING) {
		if(kvm_sys_state.tail_state != 1){
			if (get_ip_addr(Tailscale_IP)) {
				kvm_sys_state.tail_state = 1;
			}
		}
	}
	else kvm_sys_state.tail_state = 0;
}

//============================================================================

uint8_t ion_free_space(void)
{
	//cat /sys/kernel/debug/ion/cvi_carveout_heap_dump/summary | grep "usage rate:" | awk '{print $2}'

	return 0;
}

int create_temp_watchdog(void) 
{
    FILE *file;

    file = fopen(watchdog_temp_path, "w");
    if (file == NULL) {
        printf("[kvmv] Temp watchdog create error\n");
        return -1;
    }
    // fprintf(file, "%s", 'v');
    fclose(file);
    return 1;
}

void rm_temp_watchdog(void)
{
	if(access(watchdog_temp_path, F_OK) == 0) {
		remove(watchdog_temp_path);
	}
}

void auto_remove_temp_watchdog(void)
{
	static uint8_t run_times = 0;
	static uint8_t temp_watchdog_removed = 0;
	if(temp_watchdog_removed) return;
	if(run_times++ >= RM_Watchdog_times){
		run_times = 0;
		temp_watchdog_removed = 1;
		rm_temp_watchdog();
	}
}

uint8_t watchdog_sf_is_open(void)
{
	if(access(watchdog_mode_path, F_OK) == 0) return 1;
	if(access(watchdog_temp_path, F_OK) == 0) return 1;
	else return 0;
}

int check_watchdog() 
{
	if(access(watchdog_file, F_OK) == 0) {
		if (remove(watchdog_file) == 0) {
			return 1;
		} else {
			return -1;
		}
	} else return 0;
}
