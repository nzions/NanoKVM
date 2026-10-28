#include "config.h"

using namespace maix;
using namespace maix::sys;
using namespace maix::peripheral;

kvm_sys_state_t kvm_sys_state;
kvm_oled_state_t kvm_oled_state;

void build_recovery(void)
{
	uint8_t need_recovery = 0;
	if (access("/recovery", F_OK) != 0) {
		printf("unexist /recovery\n");
		mkdir("/recovery", 0777);
		need_recovery = 1;
	} else {
		printf("exist /recovery\n");

	}
}

void* thread_oled_handle(void * arg)
{
	OLED_Init();
	OLED_ColorTurn(0);		//0正常显示 1 反色显示
	OLED_DisplayTurn(0);	//0正常显示 1 屏幕翻转显示
	OLED_Clear();
	if(kvm_oled_state.ue_patch_state == 1){
		kvm_show_UE();
		while(KVM_FLAG_LOAD(kvm_sys_state.oled_thread_running) > 0){
			time::sleep_ms(100);
		}
		OLED_Clear();
	}

    while(KVM_FLAG_LOAD(kvm_sys_state.oled_thread_running) > 0)
    {
		oled_auto_sleep();
		// printf("[kvmd]thread_oled_handle - while\n");
		uint8_t page_changed = (kvm_oled_state.page == kvm_sys_state.page)? 0:1;
		uint8_t subpage_changed = (kvm_oled_state.sub_page == kvm_sys_state.sub_page)? 0:1;
		// printf("subpage_changed = %d", subpage_changed);
		// printf("kvm_oled_state.sub_page = %d", kvm_oled_state.sub_page);
		// printf("kvm_sys_state.sub_page = %d\n", kvm_sys_state.sub_page);
		kvm_oled_state.page = kvm_sys_state.page;
		kvm_oled_state.sub_page = kvm_sys_state.sub_page;


		switch(kvm_oled_state.page){
			case 0 : // main page
				kvm_main_ui_disp(page_changed, subpage_changed);
				break;
			case 1 : // wifi config page
				kvm_wifi_config_ui_disp(page_changed, subpage_changed);
				break;
			case 2 : // password page
				kvm_password_ui_disp(page_changed, subpage_changed);
				break;
			default:
				OLED_Clear();
		}
		time::sleep_ms(OLED_DELAY);
    }
	OLED_Clear();
	KVM_FLAG_STORE(kvm_sys_state.oled_thread_running, -1);
}

void* thread_key_handle(void * arg)
{
	uint64_t __attribute__((unused)) press_time;
	uint32_t press_cycle;
	int fd = open ("/dev/input/event0", O_RDONLY);
	if (fd == -1) {
		KVM_FLAG_STORE(kvm_sys_state.key_thread_running, 0);
	}
	struct input_event event;

    while(KVM_FLAG_LOAD(kvm_sys_state.key_thread_running))
    {
		read (fd, &event, sizeof (event));
		if (event.type == EV_KEY) {
			if (event.value == 1){
				// printf ("[kvmk]按键按下\n");
				press_time = time::time_ms();
			} else if (event.value == 0){
				oled_auto_sleep_time_update();
				// printf ("[kvmk]按键抬起\n");
				press_cycle = time::time_ms() - press_time;
				if(press_cycle >= KEY_LONGLONG_PRESS){
					kvm_reset_password();
				} else if (kvm_sys_state.page == 2){
					// any key closes the password page
					kvm_sys_state.page = 0;
					kvm_sys_state.sub_page = 0;
				} else if (press_cycle >= KEY_LONG_PRESS && press_cycle < KEY_LONGLONG_PRESS){
					// long
					// printf ("[kvmk]按键长按\n");
					// printf("[kvmk]wifi_state = %d\n", kvm_sys_state.wifi_state);
					if(kvm_sys_state.wifi_state == -2){
						kvm_sys_state.page = 0;
						kvm_sys_state.sub_page = 0;
						continue;
					}
					switch(kvm_sys_state.page){
						case 0:	// main page
							kvm_sys_state.page = 1;
							kvm_sys_state.sub_page = 0;
							break;
						case 1:	// wifi coonfig page
							system("/etc/init.d/S30wifi restart");
							kvm_sys_state.page = 0;
							kvm_sys_state.sub_page = 0;
							kvm_sys_state.wifi_config_process = -1;
							break;
					}
				} else {
					// short
					// printf ("[kvmk]按键短按\n");
					// printf ("[kvmk]wifi_config_process = %d\n", kvm_sys_state.wifi_config_process);
					// printf ("[kvmk]page = %d\n", kvm_sys_state.page);
					// printf ("[kvmk]sub_page = %d\n", kvm_sys_state.sub_page);
					switch(kvm_sys_state.page){
						case 0:	// main page
							if(kvm_sys_state.sub_page == 0) kvm_sys_state.sub_page = 1;
							else kvm_sys_state.sub_page = 0;
							break;
						case 1:	// wifi coonfig page
							switch(kvm_sys_state.wifi_config_process){
								case 1:	// QR1<->TEXT2
									if(kvm_sys_state.sub_page == 1) kvm_sys_state.sub_page = 2;
									else kvm_sys_state.sub_page = 1;
									// printf("[kvmk]sub_page = %d\n", kvm_sys_state.sub_page);
									break;
								case 2:
									if(kvm_sys_state.sub_page == 1) kvm_sys_state.sub_page = 2;
									else if(kvm_sys_state.sub_page == 2) kvm_sys_state.sub_page = 3;
									else if(kvm_sys_state.sub_page == 3) kvm_sys_state.sub_page = 4;
									else kvm_sys_state.sub_page = 1;
									break;
							}
							break;
					}
				}
			}
		}
		time::sleep_ms(KEY_DELAY);
    }
	kvm_sys_state.key_thread_running = 0;
}

void* thread_sys_handle(void * arg)
{
	get_ping_allow_state();
    while(KVM_FLAG_LOAD(kvm_sys_state.sys_thread_running))
    {
		kvm_show_seed_password();
		// net
		if(kvm_sys_state.page == 0){
			kvm_update_eth_state();
			kvm_update_wifi_state();
			// kvm_update_rndis_state();
			// kvm_update_tailscale_state();
			// sys_state
			kvm_update_usb_state();
			kvm_update_hdmi_state();
			kvm_update_stream_fps();
			kvm_update_stream_type();
			kvm_update_stream_qlty();
			kvm_wifi_web_config_process();

		} else if(kvm_sys_state.page == 1){
			kvm_wifi_config_process();
		}

		time::sleep_ms(STATE_DELAY);
		auto_remove_temp_watchdog();
    }
	KVM_FLAG_STORE(kvm_sys_state.sys_thread_running, 0);
}


// Thread bodies run behind a guard: an exception that escapes one is logged
// with its cause and the thread ends, instead of std::terminate killing the app.
template <void *(*Body)(void *)>
static void *kvm_guarded_thread(void *arg, const char *name)
{
	try {
		return Body(arg);
	} catch (const std::exception &e) {
		fprintf(stderr, "[kvm_system] %s thread died: %s\n", name, e.what());
	} catch (...) {
		fprintf(stderr, "[kvm_system] %s thread died: unknown exception\n", name);
	}
	return NULL;
}
static void *kvm_guarded_sys(void *arg)  { return kvm_guarded_thread<thread_sys_handle>(arg, "sys"); }
static void *kvm_guarded_oled(void *arg) { return kvm_guarded_thread<thread_oled_handle>(arg, "oled"); }
static void *kvm_guarded_key(void *arg)  { return kvm_guarded_thread<thread_key_handle>(arg, "key"); }
// Runs one start-up step. A step that throws is logged (what failed and why)
// and skipped, so one failing subsystem does not take the whole app down.
template <typename Fn>
static bool kvm_step(const char *what, Fn fn)
{
	try {
		fn();
		return true;
	} catch (const std::exception &e) {
		fprintf(stderr, "[kvm_system] %s failed: %s\n", what, e.what());
	} catch (...) {
		fprintf(stderr, "[kvm_system] %s failed: unknown exception\n", what);
	}
	return false;
}

// Reached by an exception nothing caught (a thread body, a noexcept path) or
// an explicit abort: say what it was before dying. The supervisor restarts us.
static void kvm_terminate_handler(void)
{
	fprintf(stderr, "[kvm_system] terminate called\n");
	std::exception_ptr ep = std::current_exception();
	if(ep){
		try {
			std::rethrow_exception(ep);
		} catch (const std::exception &e) {
			fprintf(stderr, "[kvm_system] uncaught exception: %s\n", e.what());
		} catch (...) {
			fprintf(stderr, "[kvm_system] uncaught exception: unknown type\n");
		}
	} else {
		fprintf(stderr, "[kvm_system] no active exception\n");
	}
	fflush(stderr);
	_exit(1);
}

static int kvm_system_run(int argc, char* argv[])
{
    // Catch SIGINT signal(e.g. Ctrl + C), and set exit flag to true.
    signal(SIGINT, [](int sig){ 
	KVM_FLAG_STORE(kvm_sys_state.oled_thread_running, 0);
	KVM_FLAG_STORE(kvm_sys_state.key_thread_running, 0);
	KVM_FLAG_STORE(kvm_sys_state.sys_thread_running, 0);
	app::set_exit_flag(true);
	log::info("[kvms]Prepare to exit\n");
	});	

	pthread_t sys_state_thread;
	pthread_t display_thread;
	pthread_t key_thread;

	// Execute only when it's a new image
	if(access("/kvmapp/kvm_new_img", F_OK) == 0){
		kvm_step("new image init", new_img_init);
	}

	// Execute only when it's a new app
	if(access("/kvmapp/kvm_new_app", F_OK) == 0){
		kvm_step("new app init", new_app_init);
	}

	// only for production testing
	if (access("/tmp/S49kvmtest", F_OK) == 0){
		printf("Production testing patch\n");
		kvm_step("production testing patch", Production_testing_patch);
	}

	system("sync");

	// oled_exist logs why itself when the bus cannot be opened; a Cube has none.
	OLED_state = 0;
	kvm_step("OLED probe", [&]{ OLED_state = oled_exist(); });
	if(OLED_state){
		printf("oled exist\r\n");
		system("touch /etc/kvm/oled_exist");
	} else {
		printf("oled not exist\r\n");
		system("rm /etc/kvm/oled_exist");
	}

	if(kvm_sys_state.sys_thread_running == 0){
		kvm_sys_state.sys_thread_running = 1;
		if (0 != pthread_create(&sys_state_thread, NULL, kvm_guarded_sys, NULL)) {
			kvm_sys_state.sys_thread_running = 0;
			printf("[kvms]create thread failed!\r\n");
		}
	}
	if(kvm_sys_state.sys_thread_running == 1){
		kvm_step("HDMI client start", kvm_start_hdmi_client);
	}
	if(OLED_state == 1 && kvm_sys_state.oled_thread_running == 0){
		kvm_sys_state.oled_thread_running = 1;
		if (0 != pthread_create(&display_thread, NULL, kvm_guarded_oled, NULL)) {
			kvm_sys_state.oled_thread_running = 0;
			printf("[kvms]create thread failed!\r\n");
		}
	}
	if(kvm_sys_state.key_thread_running == 0){
		kvm_sys_state.key_thread_running = 1;
		if (0 != pthread_create(&key_thread, NULL, kvm_guarded_key, NULL)) {
			kvm_sys_state.key_thread_running = 0;
			printf("[kvms]create thread failed!\r\n");
		}
	}

	// while(!app::need_exit()){
	uint8_t kvm_wd_count = 0;
	int kvm_wd_state = 0;
	while(KVM_FLAG_LOAD(kvm_sys_state.sys_thread_running)){
		time::sleep_ms(1000);
		if(watchdog_sf_is_open()){
			kvm_wd_state = check_watchdog();
			if(kvm_wd_state == 1) kvm_wd_count = 0;
			else if(kvm_wd_state == 0) {
				kvm_wd_count++;
				printf("Vision service unresponsive : %d\n", kvm_wd_count);
			}
			if(kvm_wd_count > KVM_WD_COUNT_MAX){
				printf("Vision service unresponsive, restart now\n");
				system("reboot");
			}
		} else {
			kvm_wd_count = 0;
		}
	}
	KVM_FLAG_STORE(kvm_sys_state.sys_thread_running, 0);
	KVM_FLAG_STORE(kvm_sys_state.oled_thread_running, 0);
	KVM_FLAG_STORE(kvm_sys_state.key_thread_running, 0);
	// Bounded: the OLED thread acknowledges with -1, but a thread that never
	// started (or is stuck on the bus) must not hang the shutdown.
	for(int waited = 0; KVM_FLAG_LOAD(kvm_sys_state.oled_thread_running) != -1; waited += 100){
		if(waited >= KVM_OLED_STOP_WAIT_MS){
			printf("[kvms]OLED thread did not stop in %d ms\n", KVM_OLED_STOP_WAIT_MS);
			break;
		}
		time::sleep_ms(100);
	}
	return 0;
}

int main(int argc, char* argv[])
{
	std::set_terminate(kvm_terminate_handler);
	try {
		return kvm_system_run(argc, argv);
	} catch (const std::exception &e) {
		fprintf(stderr, "[kvm_system] fatal: %s\n", e.what());
	} catch (...) {
		fprintf(stderr, "[kvm_system] fatal: unknown exception\n");
	}
	return 1;
}

