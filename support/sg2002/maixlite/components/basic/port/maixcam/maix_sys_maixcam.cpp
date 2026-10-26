#include "maix_sys.hpp"
#include "maix_app.hpp"
#include "signal.h"

namespace maix::sys
{
    static void signal_handle(int signal)
    {
        switch (signal) {
        case SIGINT:
            maix::app::set_exit_flag(true);
#if CONFIG_BUILD_WITH_MAIXPY
            raise(SIGINT);
#endif
        break;
        default: break;
        }
    }

    void poweroff()
    {
        int ret = system("poweroff");
        if (ret != 0)
        {
            throw err::Exception(err::Err::ERR_RUNTIME, "power off failed");
        }
    }

    void register_default_signal_handle() {
        signal(SIGINT, signal_handle);
    }

    bool is_support(sys::Feature feature)
    {
        switch(feature)
        {
            case Feature::AI_ISP:
                return false;
            default:
                return true;
        }
    }
}