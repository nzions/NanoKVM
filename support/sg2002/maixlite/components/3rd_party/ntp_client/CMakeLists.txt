# maixlite (not upstream): MaixCDK components/3rd_party/ntp_client (C).
add_library(ntp_client STATIC ntp_client/ntp_client.c ntp_client/xtime.c)
target_include_directories(ntp_client PRIVATE "${INC_NTP}" "${CFG_DIR}")
