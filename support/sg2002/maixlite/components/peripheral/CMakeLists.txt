# maixlite (not upstream): MaixCDK components/peripheral for platform maixcam. Source order
# is the order of MaixCDK's archive. port/maixcam/maix_i2c.cpp is NanoKVM's own
# (support/sg2002/additional/peripheral), which MaixCDK's build overlaid on the stock tree;
# upstream has no such file at the pinned commit.
add_library(peripheral STATIC
    port/linux_common/maix_gpio.cpp
    port/linux_common/maix_i2c.cpp
    port/linux_common/maix_pwm.cpp
    port/linux_common/maix_spi.cpp
    port/linux_common/maix_timer.cpp
    port/linux_common/maix_uart.cpp
    port/linux_common/maix_wdt.cpp
    port/maixcam/maix_adc.cpp
    port/maixcam/maix_gpio_port.cpp
    port/maixcam/maix_hid.cpp
    "${KVM_I2C}"
    port/maixcam/maix_key.cpp
    port/maixcam/maix_pinmap.cpp
    port/maixcam/maix_spi_port.cpp
    port/maixcam/maix_uart_port.cpp)
target_include_directories(peripheral PRIVATE
    "${INC_PERIPH}"
    "${CMAKE_CURRENT_SOURCE_DIR}/port/linux_common"
    "${CMAKE_CURRENT_SOURCE_DIR}/port/maixcam"
    "${CFG_DIR}" "${INC_BASIC}" "${INC_INI}" "${INC_YAML_SRC}" "${INC_YAML_INC}" "${INC_NTP}")
