# maixlite (not upstream): MaixCDK components/3rd_party/ini.
add_library(ini STATIC inifile2/src/inifile.cpp)
target_include_directories(ini PRIVATE "${INC_INI}" "${CFG_DIR}")
