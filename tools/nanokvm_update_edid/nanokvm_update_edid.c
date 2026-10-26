#include <stdio.h>
#include <stdint.h>
#include <stdbool.h>
#include <stdlib.h>
#include <string.h>
#include <ctype.h>
#include <fcntl.h>
#include <unistd.h>
#include <errno.h>

#include <linux/i2c-dev.h>
#include <linux/i2c.h>
#include <sys/ioctl.h>
#include <sys/types.h>
#include <sys/stat.h>
#include <sys/time.h>
#include <time.h>

#include "nanokvm_update_edid.h"
#include "../../support/sg2002/additional/kvm/include/internal/kvm_gpio.h"
#include "../../support/sg2002/additional/kvm/include/internal/kvm_board.h"
#include "../../support/sg2002/additional/kvm/include/internal/lt6911.h"

// The one LT6911 handle (lt6911.c: checked I2C_RDWR, the page selected on every
// access, one lock). Opened in main, which also detects the variant.
static struct lt6911_dev lt6911;

// =======================================================================================================

int get_edid_from_file(const char *filename, uint8_t *edid_data, uint16_t *edid_size) {
    FILE *file = fopen(filename, "rb");
    if (!file) {
        perror("Failed to open EDID file");
        return -1;
    }

    size_t got = fread(edid_data, 1, EDID_BUFFER_SIZE, file);
    bool failed = ferror(file) != 0;
    // Anything past EDID_BUFFER_SIZE makes the file the wrong size.
    bool extra = !failed && fgetc(file) != EOF;
    fclose(file);
    if (failed) {
        fprintf(stderr, "Failed to read EDID file %s\n", filename);
        return -1;
    }
    if (extra) {
        fprintf(stderr, "EDID file %s is longer than %d bytes\n", filename, EDID_BUFFER_SIZE);
        return -1;
    }
    *edid_size = (uint16_t)got;

    return 0;
}

int check_edid(uint8_t *edid_data, uint16_t edid_size)
{
    uint16_t i;
    // Check if the EDID data length is valid
    if (edid_size != EDID_BUFFER_SIZE) {
        fprintf(stderr, "EDID data length is not %d bytes\n", EDID_BUFFER_SIZE);
        return -1; // EDID data length is not enough
    }

    // Check EDID header
    if (edid_data[0] != 0x00 || edid_data[1] != 0xFF ||
        edid_data[2] != 0xFF || edid_data[3] != 0xFF ||
        edid_data[4] != 0xFF || edid_data[5] != 0xFF ||
        edid_data[6] != 0xFF || edid_data[7] != 0x00) {
        fprintf(stderr, "EDID header is invalid\n");
        return -1; // EDID header is invalid
    }

    // First 128 Bytes checksum
    uint8_t checksum1 = 0;
    for (i = 0; i < 127; i++) {
        checksum1 += edid_data[i];
    }
    checksum1 = 0x100 - checksum1; // Reverse
    if (checksum1 != edid_data[127]) {
        // Checksum for first 128 bytes is incorrect
        fprintf(stderr, "Checksum for first 128 bytes is incorrect\n");
        return -1;
    }

    // Second 128 Bytes checksum
    uint8_t checksum2 = 0;
    for (i = 128; i < 255; i++) {
        checksum2 += edid_data[i];
    }
    checksum2 = 0x100 - checksum2; // Reverse
    if (checksum2 != edid_data[255]) {
        // Checksum for second 128 bytes is incorrect
        fprintf(stderr, "Checksum for second 128 bytes is incorrect\n");
        return -1;
    }

    return 0; // EDID is valid
}

// =======================================================================================================

// The reset line is the board's (kvm_board.h), owned by the kvmgpio daemon
// (S15kvmhwd); the vendor kernel (no DT line of that name) falls back to the
// board's sysfs pin. Fail closed: a reset that did not happen must not be
// followed by an EDID write as if it had.
int NanoKVM_HDMI_Reset(const struct kvm_board *board)
{
    if (!board->has_hdmi_reset) return -1;
    if (nk_gpio_set_held(board->hdmi_reset_line, board->hdmi_reset_sysfs_fallback, 0) != 0) return -1;
    usleep(KVM_HDMI_RESET_PULSE_US);
    if (nk_gpio_set_held(board->hdmi_reset_line, board->hdmi_reset_sysfs_fallback, 1) != 0) return -1;
    usleep(KVM_HDMI_RESET_PULSE_US);
    return 0;
}

// =======================================================================================================

// The sequences below name a page ("offset") and a register for every access;
// the library selects the page itself each time, so there is no page state here.
// A sequence's own write of the page register (reg 0xFF) is therefore redundant
// and the library refuses it: it is skipped.
static int i2c_report(const char *what, int err)
{
    if (err != 0) fprintf(stderr, "%s: %s\n", what, strerror(-err));
    return err == 0 ? 0 : -1;
}

int i2c_write_byte(uint8_t offset, uint8_t reg, uint8_t data)
{
    if (reg == LT6911_REG_PAGE) return 0;
    return i2c_report("Failed to write to the i2c bus", lt6911_reg_write8(&lt6911, offset, reg, data));
}

int i2c_write_bytes(uint8_t offset, uint8_t reg, const uint8_t *data, size_t len)
{
    return i2c_report("Failed to write to the i2c bus", lt6911_reg_write(&lt6911, offset, reg, data, len));
}

int i2c_read_byte(uint8_t offset, uint8_t reg, uint8_t *data)
{
    return i2c_report("Failed to read from the i2c bus", lt6911_reg_read8(&lt6911, offset, reg, data));
}

int i2c_read_bytes(uint8_t offset, uint8_t reg, uint8_t *data, size_t len)
{
    return i2c_report("Failed to read from the i2c bus", lt6911_reg_read(&lt6911, offset, reg, data, len));
}

// =======================================================================================================

int lt6911_enable(void) {
    // Enable the LT6911UXC by writing to the appropriate register
    if (i2c_write_byte(LT6911_SYS_OFFSET, 0xEE, 0x01) != 0) {
        fprintf(stderr, "Failed to enable LT6911UXC\n");
        return -1;
    }
    return 0;
}

int lt6911_disable(void) {
    // Disable the LT6911UXC by writing to the appropriate register
    if (i2c_write_byte(LT6911_SYS_OFFSET, 0xEE, 0x00) != 0) {
        fprintf(stderr, "Failed to disable LT6911UXC\n");
        return -1;
    }
    return 0;
}

// =======================================================================================================

int lt6911uxc_edid_write(uint8_t *edid_data, uint16_t edid_size)
{
    uint8_t i;
    int ret;
    uint8_t chip_data[16] = {0};
    uint8_t wr_count = edid_size / LT6911UXC_WR_SIZE + 1;
    uint8_t version_str[32] = {0};

    fprintf(stdout, "Writing EDID....\n");

    // Start
    if (i2c_write_byte(LT6911_SYS_OFFSET, 0xFF, 0x80) != 0) return -1;
    if (lt6911_enable() != 0) return -1;
    if (i2c_write_byte(LT6911_SYS_OFFSET, 0x5E, 0xDF) != 0) return -1;
    if (i2c_write_byte(LT6911_SYS_OFFSET, 0x58, 0x00) != 0) return -1;
    if (i2c_write_byte(LT6911_SYS_OFFSET, 0x59, 0x51) != 0) return -1;
    if (i2c_write_byte(LT6911_SYS_OFFSET, 0x5A, 0x10) != 0) return -1;
    if (i2c_write_byte(LT6911_SYS_OFFSET, 0x5A, 0x00) != 0) return -1;
    if (i2c_write_byte(LT6911_SYS_OFFSET, 0x58, 0x21) != 0) return -1;
    if (i2c_write_byte(LT6911_SYS_OFFSET, 0xFF, 0x80) != 0) return -1;
    if (lt6911_enable() != 0) return -1;
    if (i2c_write_byte(LT6911_SYS_OFFSET, 0x5A, 0x80) != 0) return -1;
    if (i2c_write_byte(LT6911_SYS_OFFSET, 0x5A, 0x84) != 0) return -1;
    if (i2c_write_byte(LT6911_SYS_OFFSET, 0x5A, 0x80) != 0) return -1;
    if (i2c_write_byte(LT6911_SYS_OFFSET, 0x5B, 0x01) != 0) return -1;
    if (i2c_write_byte(LT6911_SYS_OFFSET, 0x5C, 0x80) != 0) return -1;
    if (i2c_write_byte(LT6911_SYS_OFFSET, 0x5D, 0x00) != 0) return -1;
    if (i2c_write_byte(LT6911_SYS_OFFSET, 0x5A, 0x81) != 0) return -1;
    if (i2c_write_byte(LT6911_SYS_OFFSET, 0x5A, 0x80) != 0) return -1;
    // Waiting for erasure
    usleep(500000);

    if (i2c_read_byte(LT6911_SYS3_OFFSET, 0x08, chip_data) != 0) return -1;
    if (chip_data[0] != 0xEE) {
        fprintf(stderr, "Unsupported chip version\n");
        return -1;
    }
    if (i2c_write_byte(LT6911_SYS3_OFFSET, 0x08, 0xAE) != 0) return -1;
    if (i2c_write_byte(LT6911_SYS3_OFFSET, 0x08, 0xEE) != 0) return -1;
    // Write
    if (i2c_write_byte(LT6911_SYS_OFFSET, 0xFF, 0x80) != 0) return -1;
    if (lt6911_enable() != 0) return -1;
    if (i2c_write_byte(LT6911_SYS_OFFSET, 0x5A, 0x84) != 0) return -1;
    if (i2c_write_byte(LT6911_SYS_OFFSET, 0x5A, 0x80) != 0) return -1;
    if (i2c_write_byte(LT6911_SYS_OFFSET, 0x5A, 0x84) != 0) return -1;
    if (i2c_write_byte(LT6911_SYS_OFFSET, 0x5A, 0x80) != 0) return -1;
    for (i = 0; i < wr_count; i++) {
        if (i2c_write_byte(LT6911_SYS_OFFSET, 0x5E, 0xDF) != 0) return -1;
        if (i2c_write_byte(LT6911_SYS_OFFSET, 0x5A, 0x20) != 0) return -1;
        if (i2c_write_byte(LT6911_SYS_OFFSET, 0x5A, 0x00) != 0) return -1;
        if (i2c_write_byte(LT6911_SYS_OFFSET, 0x58, 0x21) != 0) return -1;
        if (i != wr_count-1) {
            if (i2c_write_bytes(LT6911_SYS_OFFSET, 0x59, edid_data+(LT6911UXC_WR_SIZE*i), LT6911UXC_WR_SIZE) != 0) return -1;
        } else {
            if (i2c_write_bytes(LT6911_SYS_OFFSET, 0x59, version_str, LT6911UXC_WR_SIZE) != 0) return -1;
        }
        if (i2c_write_byte(LT6911_SYS_OFFSET, 0x5B, 0x01) != 0) return -1;
        if (i != wr_count-1) {
            if (i2c_write_byte(LT6911_SYS_OFFSET, 0x5C, 0x80) != 0) return -1;
            if (i2c_write_byte(LT6911_SYS_OFFSET, 0x5D, 0x00+(LT6911UXC_WR_SIZE*i)) != 0) return -1;
        } else {
            if (i2c_write_byte(LT6911_SYS_OFFSET, 0x5C, 0x81) != 0) return -1;
            if (i2c_write_byte(LT6911_SYS_OFFSET, 0x5D, 0x00) != 0) return -1;
        }
        if (i2c_write_byte(LT6911_SYS_OFFSET, 0x5E, 0xC0) != 0) return -1;
        if (i2c_write_byte(LT6911_SYS_OFFSET, 0x5A, 0x90) != 0) return -1;
        if (i2c_write_byte(LT6911_SYS_OFFSET, 0x5A, 0x80) != 0) return -1;
        if (i != wr_count-1) {
            if (i2c_write_byte(LT6911_SYS_OFFSET, 0x5A, 0x84) != 0) return -1;
        } else {
            if (i2c_write_byte(LT6911_SYS_OFFSET, 0x5A, 0x88) != 0) return -1;
        }
        if (i2c_write_byte(LT6911_SYS_OFFSET, 0x5A, 0x80) != 0) return -1;
    }
    if (i2c_read_byte(LT6911_SYS3_OFFSET, 0x08, chip_data) != 0) return -1;
    if (chip_data[0] != 0xEE) return -1;
    if (i2c_write_byte(LT6911_SYS3_OFFSET, 0x08, 0xAE) != 0) return -1;
    if (i2c_write_byte(LT6911_SYS3_OFFSET, 0x08, 0xEE) != 0) return -1;

    fprintf(stdout, "EDID write completed\n");

    return 0;
}

int lt6911uxc_edid_read(uint8_t *edid_data, uint16_t edid_size)
{
    uint8_t i;
    int ret;
    uint8_t chip_data[16] = {0};
    uint8_t wr_count = edid_size / LT6911UXC_WR_SIZE;

    // Read EDID data from LT6911UXC
    fprintf(stdout, "Reading EDID...\n");
    // Read
    if (i2c_write_byte(LT6911_SYS_OFFSET, 0xFF, 0x80) != 0) return -1;
    if (lt6911_enable() != 0) return -1;
    if (i2c_write_byte(LT6911_SYS_OFFSET, 0x5A, 0x84) != 0) return -1;
    if (i2c_write_byte(LT6911_SYS_OFFSET, 0x5A, 0x80) != 0) return -1;
    for (i = 0; i < wr_count; i++) {
        if (i2c_write_byte(LT6911_SYS_OFFSET, 0x5E, 0x5F) != 0) return -1;
        if (i2c_write_byte(LT6911_SYS_OFFSET, 0x5A, 0xA0) != 0) return -1;
        if (i2c_write_byte(LT6911_SYS_OFFSET, 0x5A, 0x80) != 0) return -1;
        if (i2c_write_byte(LT6911_SYS_OFFSET, 0x5B, 0x01) != 0) return -1;
        if (i2c_write_byte(LT6911_SYS_OFFSET, 0x5C, 0x80) != 0) return -1;
        if (i2c_write_byte(LT6911_SYS_OFFSET, 0x5D, 0x00+(LT6911UXC_WR_SIZE*i)) != 0) return -1;
        if (i2c_write_byte(LT6911_SYS_OFFSET, 0x5A, 0x90) != 0) return -1;
        if (i2c_write_byte(LT6911_SYS_OFFSET, 0x5A, 0x80) != 0) return -1;
        if (i2c_write_byte(LT6911_SYS_OFFSET, 0x58, 0x21) != 0) return -1;
        if (i2c_read_bytes(LT6911_SYS_OFFSET, 0x5F, edid_data+(LT6911UXC_WR_SIZE*i), LT6911UXC_WR_SIZE) != 0) return -1;
    }

    return 0;
}

int lt6911c_edid_write(uint8_t *edid_data, uint16_t edid_size)
{
    uint8_t i;
    int ret;
    uint8_t chip_data[16] = {0};
    uint8_t wr_count = edid_size / LT6911C_WR_SIZE;
    uint8_t version_str[32] = {0};

    fprintf(stdout, "Writing EDID....\n");

    // Start
    if (lt6911_enable() != 0) return -1;
    if (i2c_read_bytes(LT6911_SYS4_OFFSET, 0x00, chip_data, 2) != 0) return -1;
    if (chip_data[0] != 0x16 || chip_data[1] != 0x05) {
        fprintf(stderr, "Unsupported chip version\n");
        return -1;
    }
    if (lt6911_disable() != 0) return -1;

    usleep(100000);

    if (i2c_write_byte(LT6911_SYS_OFFSET, 0xFF, 0x80) != 0) return -1;
    if (lt6911_enable() != 0) return -1;
    if (i2c_read_bytes(LT6911_SYS4_OFFSET, 0x00, chip_data, 2) != 0) return -1;
    if (chip_data[0] != 0x16 || chip_data[1] != 0x05) {
        fprintf(stderr, "Unsupported chip version\n");
        return -1;
    }
    if (lt6911_disable() != 0) return -1;

    if (i2c_write_byte(LT6911_SYS_OFFSET, 0xFF, 0x80) != 0) return -1;
    if (lt6911_enable() != 0) return -1;
    if (i2c_write_byte(LT6911_SYS_OFFSET, 0x5A, 0x82) != 0) return -1;
    if (i2c_write_byte(LT6911_SYS_OFFSET, 0x5E, 0xC0) != 0) return -1;
    if (i2c_write_byte(LT6911_SYS_OFFSET, 0x58, 0x00) != 0) return -1;
    if (i2c_write_byte(LT6911_SYS_OFFSET, 0x59, 0x51) != 0) return -1;
    if (i2c_write_byte(LT6911_SYS_OFFSET, 0x5A, 0x92) != 0) return -1;
    if (i2c_write_byte(LT6911_SYS_OFFSET, 0x5A, 0x82) != 0) return -1;

    if (i2c_write_byte(LT6911_SYS_OFFSET, 0xFF, 0x80) != 0) return -1;
    if (lt6911_enable() != 0) return -1;
    if (i2c_write_byte(LT6911_SYS_OFFSET, 0x5A, 0x82) != 0) return -1;
    if (i2c_write_byte(LT6911_SYS_OFFSET, 0x5A, 0x86) != 0) return -1;
    if (i2c_write_byte(LT6911_SYS_OFFSET, 0x5A, 0x82) != 0) return -1;
    if (i2c_write_byte(LT6911_SYS_OFFSET, 0x5B, 0x01) != 0) return -1;
    if (i2c_write_byte(LT6911_SYS_OFFSET, 0x5C, 0x80) != 0) return -1;
    if (i2c_write_byte(LT6911_SYS_OFFSET, 0x5D, 0x00) != 0) return -1;
    if (i2c_write_byte(LT6911_SYS_OFFSET, 0x5A, 0x83) != 0) return -1;
    if (i2c_write_byte(LT6911_SYS_OFFSET, 0x5A, 0x82) != 0) return -1;

    // waiting to clean
    usleep(500000);
    if (i2c_read_byte(LT6911_SYS2_OFFSET, 0x02, chip_data) != 0) return -1;
    if (chip_data[0] != 0xFF) {
        fprintf(stderr, "Clean Error\n");
        return -1;
    }
    if (i2c_write_byte(LT6911_SYS2_OFFSET, 0x02, 0xDF) != 0) return -1;
    if (i2c_write_byte(LT6911_SYS2_OFFSET, 0x02, 0xFF) != 0) return -1;
    if (i2c_write_byte(LT6911_SYS_OFFSET, 0xFF, 0x80) != 0) return -1;
    if (lt6911_enable() != 0) return -1;
    if (i2c_write_byte(LT6911_SYS_OFFSET, 0x5A, 0x86) != 0) return -1;

    for (i = 0; i < wr_count; i++) {
        if (i2c_write_byte(LT6911_SYS_OFFSET, 0x5A, 0x82) != 0) return -1;
        if (i2c_write_byte(LT6911_SYS_OFFSET, 0x5A, 0x86) != 0) return -1;
        if (i2c_write_byte(LT6911_SYS_OFFSET, 0x5A, 0x82) != 0) return -1;
        if (i2c_write_byte(LT6911_SYS_OFFSET, 0x5E, 0xEF) != 0) return -1;
        if (i2c_write_byte(LT6911_SYS_OFFSET, 0x5A, 0xA2) != 0) return -1;
        if (i2c_write_byte(LT6911_SYS_OFFSET, 0x5A, 0x82) != 0) return -1;
        if (i2c_write_byte(LT6911_SYS_OFFSET, 0x58, 0x01) != 0) return -1;
        if (i2c_write_bytes(LT6911_SYS_OFFSET, 0x59, edid_data+(LT6911C_WR_SIZE*i), LT6911C_WR_SIZE) != 0) return -1;
        if (i2c_write_byte(LT6911_SYS_OFFSET, 0x5B, 0x01) != 0) return -1;
        if (i2c_write_byte(LT6911_SYS_OFFSET, 0x5C, 0x80) != 0) return -1;
        if (i2c_write_byte(LT6911_SYS_OFFSET, 0x5D, 0x00+(LT6911C_WR_SIZE*i)) != 0) return -1;
        if (i2c_write_byte(LT6911_SYS_OFFSET, 0x5E, 0xE0) != 0) return -1;
        if (i2c_write_byte(LT6911_SYS_OFFSET, 0x5A, 0x92) != 0) return -1;
    }
    if (i2c_write_byte(LT6911_SYS_OFFSET, 0x5A, 0x82) != 0) return -1;
    if (i2c_write_byte(LT6911_SYS_OFFSET, 0x5A, 0x8A) != 0) return -1;
    if (i2c_write_byte(LT6911_SYS_OFFSET, 0x5A, 0x82) != 0) return -1;
    if (i2c_read_byte(LT6911_SYS2_OFFSET, 0x02, chip_data) != 0) return -1;
    if (chip_data[0] != 0xFF) return -1;
    if (i2c_write_byte(LT6911_SYS2_OFFSET, 0x02, 0xDF) != 0) return -1;
    if (i2c_write_byte(LT6911_SYS2_OFFSET, 0x02, 0xFF) != 0) return -1;

    fprintf(stdout, "EDID write completed\n");

    return 0;
}

int lt6911c_edid_read(uint8_t *edid_data, uint16_t edid_size)
{
    uint8_t i;
    int ret;
    uint8_t chip_data[16] = {0};
    uint8_t wr_count = edid_size / LT6911C_WR_SIZE;

    // Read EDID data from LT6911UXC
    fprintf(stdout, "Reading EDID...\n");
    // Read

    if (i2c_write_byte(LT6911_SYS_OFFSET, 0xFF, 0x80) != 0) return -1;
    if (lt6911_enable() != 0) return -1;
    if (i2c_write_byte(LT6911_SYS_OFFSET, 0x5A, 0x86) != 0) return -1;
    if (i2c_write_byte(LT6911_SYS_OFFSET, 0x5A, 0x82) != 0) return -1;

    for (i = 0; i < wr_count; i++) {
        if (i2c_write_byte(LT6911_SYS_OFFSET, 0x5E, 0x6F) != 0) return -1;
        if (i2c_write_byte(LT6911_SYS_OFFSET, 0x5A, 0xA2) != 0) return -1;
        if (i2c_write_byte(LT6911_SYS_OFFSET, 0x5A, 0x82) != 0) return -1;
        if (i2c_write_byte(LT6911_SYS_OFFSET, 0x5B, 0x01) != 0) return -1;
        if (i2c_write_byte(LT6911_SYS_OFFSET, 0x5C, 0x80) != 0) return -1;
        if (i2c_write_byte(LT6911_SYS_OFFSET, 0x5D, 0x00+(LT6911C_WR_SIZE*i)) != 0) return -1;
        if (i2c_write_byte(LT6911_SYS_OFFSET, 0x5A, 0x92) != 0) return -1;
        if (i2c_write_byte(LT6911_SYS_OFFSET, 0x5A, 0x82) != 0) return -1;
        if (i2c_write_byte(LT6911_SYS_OFFSET, 0x58, 0x01) != 0) return -1;
        if (i2c_read_bytes(LT6911_SYS_OFFSET, 0x5F, edid_data+(LT6911C_WR_SIZE*i), LT6911C_WR_SIZE) != 0) return -1;
    }

    return 0;
}

int lt6911d_str_read(uint8_t *str)
{
    fprintf(stdout, "Reading LT6911D version data...\n");

    if (i2c_write_byte(LT6911D_MANAGE_OFFSET, 0xEE, 0x01) != 0) return -1;
    if (i2c_write_byte(LT6911D_MANAGE_OFFSET, 0x5E, 0xDF) != 0) return -1;
    if (i2c_write_byte(LT6911D_MANAGE_OFFSET, 0x58, 0x00) != 0) return -1;
    if (i2c_write_byte(LT6911D_MANAGE_OFFSET, 0x59, 0x50) != 0) return -1;
    if (i2c_write_byte(LT6911D_MANAGE_OFFSET, 0x5A, 0x10) != 0) return -1;
    usleep(1000);
    if (i2c_write_byte(LT6911D_MANAGE_OFFSET, 0x5A, 0x00) != 0) return -1;
    if (i2c_write_byte(LT6911D_MANAGE_OFFSET, 0x58, 0x21) != 0) return -1;
    if (i2c_write_byte(LT6911D_DATA_OFFSET, 0x03, 0xBF) != 0) return -1;
    usleep(1000);
    if (i2c_write_byte(LT6911D_DATA_OFFSET, 0x03, 0xFF) != 0) return -1;
    if (i2c_write_byte(LT6911D_MANAGE_OFFSET, 0x5A, 0x04) != 0) return -1;
    usleep(1000);
    if (i2c_write_byte(LT6911D_MANAGE_OFFSET, 0x5A, 0x00) != 0) return -1;
    if (i2c_write_byte(LT6911D_MANAGE_OFFSET, 0x5E, 0x5F) != 0) return -1;
    if (i2c_write_byte(LT6911D_MANAGE_OFFSET, 0x5A, 0x20) != 0) return -1;
    usleep(1000);
    if (i2c_write_byte(LT6911D_MANAGE_OFFSET, 0x5A, 0x00) != 0) return -1;
    if (i2c_write_byte(LT6911D_MANAGE_OFFSET, 0x5B, 0x00) != 0) return -1;
    if (i2c_write_byte(LT6911D_MANAGE_OFFSET, 0x5C, 0x81) != 0) return -1;
    if (i2c_write_byte(LT6911D_MANAGE_OFFSET, 0x5D, 0x00) != 0) return -1;
    if (i2c_write_byte(LT6911D_MANAGE_OFFSET, 0x5A, 0x10) != 0) return -1;
    usleep(1000);
    if (i2c_write_byte(LT6911D_MANAGE_OFFSET, 0x5A, 0x00) != 0) return -1;
    if (i2c_read_bytes(LT6911D_MANAGE_OFFSET, 0x5F, str, LT6911D_WR_SIZE) != 0) return -1;
    usleep(10000);
    if (i2c_write_byte(LT6911D_MANAGE_OFFSET, 0x5A, 0x08) != 0) return -1;
    usleep(1000);
    if (i2c_write_byte(LT6911D_MANAGE_OFFSET, 0x5A, 0x00) != 0) return -1;
    if (i2c_write_byte(LT6911D_MANAGE_OFFSET, 0x58, 0x00) != 0) return -1;

    return 0;
}

int lt6911d_edid_write(uint8_t *edid_data, uint16_t edid_size)
{
    uint8_t i;
    uint8_t wr_count = edid_size / LT6911D_WR_SIZE + 1;
    uint8_t version_str[LT6911D_WR_SIZE] = {0};

    if (lt6911d_str_read(version_str) != 0) {
        fprintf(stderr, "Failed to read LT6911D version data\n");
        return -1;
    }

    fprintf(stdout, "Writing EDID....\n");

    if (i2c_write_byte(LT6911D_MANAGE_OFFSET, 0xEE, 0x01) != 0) return -1;
    if (i2c_write_byte(LT6911D_MANAGE_OFFSET, 0x5E, 0xDF) != 0) return -1;
    if (i2c_write_byte(LT6911D_MANAGE_OFFSET, 0x58, 0x00) != 0) return -1;
    if (i2c_write_byte(LT6911D_MANAGE_OFFSET, 0x59, 0x50) != 0) return -1;
    if (i2c_write_byte(LT6911D_MANAGE_OFFSET, 0x5A, 0x10) != 0) return -1;
    usleep(1000);
    if (i2c_write_byte(LT6911D_MANAGE_OFFSET, 0x5A, 0x00) != 0) return -1;
    if (i2c_write_byte(LT6911D_MANAGE_OFFSET, 0x58, 0x21) != 0) return -1;
    if (i2c_write_byte(LT6911D_MANAGE_OFFSET, 0x5A, 0x04) != 0) return -1;
    usleep(1000);
    if (i2c_write_byte(LT6911D_MANAGE_OFFSET, 0x5A, 0x00) != 0) return -1;
    if (i2c_write_byte(LT6911D_MANAGE_OFFSET, 0x5B, 0x00) != 0) return -1;
    if (i2c_write_byte(LT6911D_MANAGE_OFFSET, 0x5C, 0x80) != 0) return -1;
    if (i2c_write_byte(LT6911D_MANAGE_OFFSET, 0x5D, 0x00) != 0) return -1;
    if (i2c_write_byte(LT6911D_MANAGE_OFFSET, 0x5A, 0x01) != 0) return -1;
    usleep(1000);
    if (i2c_write_byte(LT6911D_MANAGE_OFFSET, 0x5A, 0x00) != 0) return -1;
    usleep(600000);

    for (i = 0; i < wr_count; i++) {
        if (i2c_write_byte(LT6911D_DATA_OFFSET, 0x03, 0xBF) != 0) return -1;
        usleep(1000);
        if (i2c_write_byte(LT6911D_DATA_OFFSET, 0x03, 0xFF) != 0) return -1;
        if (i2c_write_byte(LT6911D_MANAGE_OFFSET, 0x5A, 0x04) != 0) return -1;
        usleep(1000);
        if (i2c_write_byte(LT6911D_MANAGE_OFFSET, 0x5A, 0x00) != 0) return -1;
        if (i2c_write_byte(LT6911D_MANAGE_OFFSET, 0x5E, 0xDF) != 0) return -1;
        if (i2c_write_byte(LT6911D_MANAGE_OFFSET, 0x5A, 0x20) != 0) return -1;
        usleep(1000);
        if (i2c_write_byte(LT6911D_MANAGE_OFFSET, 0x5A, 0x00) != 0) return -1;
        if (i2c_write_byte(LT6911D_MANAGE_OFFSET, 0x58, 0x21) != 0) return -1;
        if (i != wr_count - 1) {
            if (i2c_write_bytes(LT6911D_MANAGE_OFFSET, 0x59, edid_data + (LT6911D_WR_SIZE * i), LT6911D_WR_SIZE) != 0) return -1;
        } else {
            if (i2c_write_bytes(LT6911D_MANAGE_OFFSET, 0x59, version_str, LT6911D_WR_SIZE) != 0) return -1;
        }
        if (i2c_write_byte(LT6911D_MANAGE_OFFSET, 0x5B, 0x00) != 0) return -1;
        if (i != wr_count - 1) {
            if (i2c_write_byte(LT6911D_MANAGE_OFFSET, 0x5C, 0x80) != 0) return -1;
        } else {
            if (i2c_write_byte(LT6911D_MANAGE_OFFSET, 0x5C, 0x81) != 0) return -1;
        }
        if (i2c_write_byte(LT6911D_MANAGE_OFFSET, 0x5D, 0x00 + (LT6911D_WR_SIZE * i)) != 0) return -1;
        if (i2c_write_byte(LT6911D_MANAGE_OFFSET, 0x5A, 0x10) != 0) return -1;
        usleep(5000);
        if (i2c_write_byte(LT6911D_MANAGE_OFFSET, 0x5A, 0x00) != 0) return -1;
        usleep(10000);
    }
    if (i2c_write_byte(LT6911D_MANAGE_OFFSET, 0x5A, 0x08) != 0) return -1;
    usleep(1000);
    if (i2c_write_byte(LT6911D_MANAGE_OFFSET, 0x5A, 0x00) != 0) return -1;
    if (i2c_write_byte(LT6911D_MANAGE_OFFSET, 0x58, 0x00) != 0) return -1;

    fprintf(stdout, "EDID write completed\n");

    return 0;
}

int lt6911d_edid_read(uint8_t *edid_data, uint16_t edid_size)
{
    uint8_t i;
    uint8_t wr_count = edid_size / LT6911D_WR_SIZE;

    fprintf(stdout, "Reading EDID...\n");

    if (i2c_write_byte(LT6911D_MANAGE_OFFSET, 0xEE, 0x01) != 0) return -1;
    if (i2c_write_byte(LT6911D_MANAGE_OFFSET, 0x5E, 0xDF) != 0) return -1;
    if (i2c_write_byte(LT6911D_MANAGE_OFFSET, 0x58, 0x00) != 0) return -1;
    if (i2c_write_byte(LT6911D_MANAGE_OFFSET, 0x59, 0x50) != 0) return -1;
    if (i2c_write_byte(LT6911D_MANAGE_OFFSET, 0x5A, 0x10) != 0) return -1;
    usleep(1000);
    if (i2c_write_byte(LT6911D_MANAGE_OFFSET, 0x5A, 0x00) != 0) return -1;
    if (i2c_write_byte(LT6911D_MANAGE_OFFSET, 0x58, 0x21) != 0) return -1;
    for (i = 0; i < wr_count; i++) {
        if (i2c_write_byte(LT6911D_DATA_OFFSET, 0x03, 0xBF) != 0) return -1;
        usleep(1000);
        if (i2c_write_byte(LT6911D_DATA_OFFSET, 0x03, 0xFF) != 0) return -1;
        if (i2c_write_byte(LT6911D_MANAGE_OFFSET, 0x5A, 0x04) != 0) return -1;
        usleep(1000);
        if (i2c_write_byte(LT6911D_MANAGE_OFFSET, 0x5A, 0x00) != 0) return -1;
        if (i2c_write_byte(LT6911D_MANAGE_OFFSET, 0x5E, 0x5F) != 0) return -1;
        if (i2c_write_byte(LT6911D_MANAGE_OFFSET, 0x5A, 0x20) != 0) return -1;
        usleep(1000);
        if (i2c_write_byte(LT6911D_MANAGE_OFFSET, 0x5A, 0x00) != 0) return -1;
        if (i2c_write_byte(LT6911D_MANAGE_OFFSET, 0x5B, 0x00) != 0) return -1;
        if (i2c_write_byte(LT6911D_MANAGE_OFFSET, 0x5C, 0x80) != 0) return -1;
        if (i2c_write_byte(LT6911D_MANAGE_OFFSET, 0x5D, 0x00 + (LT6911D_WR_SIZE * i)) != 0) return -1;
        if (i2c_write_byte(LT6911D_MANAGE_OFFSET, 0x5A, 0x10) != 0) return -1;
        usleep(1000);
        if (i2c_write_byte(LT6911D_MANAGE_OFFSET, 0x5A, 0x00) != 0) return -1;
        if (i2c_read_bytes(LT6911D_MANAGE_OFFSET, 0x5F, edid_data + (LT6911D_WR_SIZE * i), LT6911D_WR_SIZE) != 0) return -1;
        usleep(10000);
    }
    if (i2c_write_byte(LT6911D_MANAGE_OFFSET, 0x5A, 0x08) != 0) return -1;
    usleep(1000);
    if (i2c_write_byte(LT6911D_MANAGE_OFFSET, 0x5A, 0x00) != 0) return -1;
    if (i2c_write_byte(LT6911D_MANAGE_OFFSET, 0x58, 0x00) != 0) return -1;

    return 0;
}

// =======================================================================================================

// The EDID programming sequences, per variant. UXE has none (its EDID cannot
// be updated) and neither has an unknown chip.
struct edid_ops {
    int (*write)(uint8_t *edid_data, uint16_t edid_size);
    int (*read)(uint8_t *edid_data, uint16_t edid_size);
};

static const struct edid_ops *edid_ops_for(enum lt6911_variant variant)
{
    static const struct edid_ops uxc = { lt6911uxc_edid_write, lt6911uxc_edid_read };
    static const struct edid_ops c   = { lt6911c_edid_write,   lt6911c_edid_read };
    static const struct edid_ops d   = { lt6911d_edid_write,   lt6911d_edid_read };
    switch (variant) {
    case LT6911_UXC: return &uxc;
    case LT6911_C:   return &c;
    case LT6911_D:   return &d;
    default:         return NULL;
    }
}

// The chip is already open (main); this writes the EDID and reads it back.
int lt6911_edid_config(uint8_t *edid_data, uint16_t edid_size)
{
    const struct edid_ops *ops = edid_ops_for(lt6911.variant);
    if (ops == NULL)
    {
        fprintf(stderr, "Unknown chip version\n");
        return -1;
    }
    const char *name = lt6911_variant_name(lt6911.variant);

    // edid write
    if (ops->write(edid_data, edid_size) != 0) {
        fprintf(stderr, "Failed to write EDID data to %s\n", name);
        return -1;
    }
    // sleep 1s
    sleep(1);

    // read edid data back
    uint8_t edid_read_data[EDID_BUFFER_SIZE] = {0};
    if (ops->read(edid_read_data, EDID_BUFFER_SIZE) != 0) {
        fprintf(stderr, "Failed to read EDID data from %s\n", name);
        return -1;
    }

    // check if the read data matches the written data
    if (memcmp(edid_data, edid_read_data, EDID_BUFFER_SIZE) != 0) {
        fprintf(stderr, "EDID data mismatch after write/read cycle\n");
        // print edid with 16*16 hex format
        fprintf(stderr, "Written EDID data:\n");
        for (int i = 0; i < EDID_BUFFER_SIZE; i++) {
            fprintf(stderr, "%02X ", edid_data[i]);
            if ((i + 1) % 16 == 0) {
                fprintf(stderr, "\n");
            }
        }
        fprintf(stderr, "\nRead EDID data:\n");
        for (int i = 0; i < EDID_BUFFER_SIZE; i++) {
            fprintf(stderr, "%02X ", edid_read_data[i]);
            if ((i + 1) % 16 == 0) {
                fprintf(stderr, "\n");
            }
        }
        return -1;
    } else {
        fprintf(stdout, "EDID data verified successfully\n");
    }

    return 0;
}

// =======================================================================================================

void print_warning(const struct kvm_board *board) {
    printf("\n=========================================================\n");
    printf("Incorrect EDID may cause issues such as \n");
    printf("inability to display images, please modify with caution\n");
    printf("=========================================================\n\n");

    if (!board->has_hdmi_reset) {
        printf("\n==========================================================\n");
        printf("⚠️  WARNING: Hardware version detected as Cube/Lite!\n");
        printf("==========================================================\n");
        printf("After flashing, you MUST manually power cycle the device!\n");
        printf("Please ensure you can physically disconnect its power,\n");
        printf("NOT just remotely reboot it!!\n");
        printf("==========================================================\n\n");
    }
}

void print_success(const struct kvm_board *board) {
    printf("\n=========================================================\n");
    printf("✅  EDID update successful!\n");
    if (!board->has_hdmi_reset) {
        printf("Please manually power cycle the device to apply changes.\n");
    }
    printf("=========================================================\n\n");
}

int get_user_confirmation() {
    char input[256];
    printf("Do you want to continue? (Y/N): \n");

    while (1) {

        if (fgets(input, sizeof(input), stdin) == NULL) {
            printf("\nInput error. Exiting.\n");
            return 0;
        }

        // 去除换行符
        input[strcspn(input, "\n")] = '\0';

        if (strlen(input) == 0) {
            continue; // 空输入，重新提示
        }

        // 转换为小写方便比较
        char choice = tolower(input[0]);

        if (choice == 'y') {
            return 1;
        } else if (choice == 'n') {
            return 0;
        } else {
            printf("Invalid input. Please enter Y or N.\n");
        }
    }
}

// =======================================================================================================

int main(int argc, char *argv[]) {

    uint8_t counter = 0;

    // Open the bus and detect the variant (once, in the library). A missing,
    // unreadable or unknown marker is an error, never a guess.
    int open_err = lt6911_open(&lt6911);
    if (open_err != 0) {
        fprintf(stderr, "Chip Version Error: cannot open the LT6911 (%s)\n", strerror(-open_err));
        return 1;
    }
    if (lt6911.variant == LT6911_UXE) {
        fprintf(stderr, "Chip Version Error: UE version's edid can't be updated\n");
        lt6911_close(&lt6911);
        return 1;
    }
    fprintf(stdout, "Chip Version: %s\n", lt6911_variant_name(lt6911.variant));

    const struct kvm_board *board = kvm_board_get();
    if (board->kind == KVM_BOARD_UNKNOWN) {
        fprintf(stderr, "Product Version Error: Unknown version\n");
        return 1;
    }
    fprintf(stdout, "Product Version: %s\n", board->name);

    print_warning(board);
    if (!board->has_hdmi_reset) {
        if (get_user_confirmation() == 0) {
            return EXIT_FAILURE;
        }
    }

    // check command line arguments
    if (argc != 2) {
        fprintf(stderr, "Please enter the location of the EDID file using \"%s /path/to/edid.bin\"\n", argv[0]);
        return EXIT_FAILURE;
    }

    uint16_t edid_size = 0;
    uint8_t edid_data[EDID_BUFFER_SIZE] = {0};
    if (get_edid_from_file(argv[1], edid_data, &edid_size) != 0) {
        fprintf(stderr, "Failed to read EDID data from file %s\n", argv[1]);
        return EXIT_FAILURE;
    }
    if (check_edid(edid_data, edid_size) != 0) {
        fprintf(stderr, "EDID data is invalid\n");
        return EXIT_FAILURE;
    }
    fprintf(stdout, "EDID data loaded successfully from %s\n", argv[1]);

    // The server's HDMI state machine (libkvm, another process) uses this bus
    // and this chip. One session holds the cross-process lock from the first
    // reset to the last, so nothing interleaves with the flash sequence; it
    // ends "changed", which wakes the state machine to re-read the chip. The
    // wait for the lock is bounded: a busy bus is an error, not a hang.
    int sess_err = lt6911_session_begin(&lt6911);
    if (sess_err != 0) {
        fprintf(stderr, "Cannot take the LT6911 bus (%s)\n", strerror(-sess_err));
        lt6911_close(&lt6911);
        return EXIT_FAILURE;
    }

    int status = EXIT_SUCCESS;
    if (board->has_hdmi_reset && NanoKVM_HDMI_Reset(board) != 0) {
        fprintf(stderr, "Failed to reset the LT6911 (%s)\n", board->hdmi_reset_line);
        status = EXIT_FAILURE;
    }
    // From here the chip's flash may be touched, so the session ends "changed"
    // whatever happens: a half-written EDID is a state the server must see.
    bool chip_touched = false;
    if (status == EXIT_SUCCESS) {
        chip_touched = true;
        // configure lt6911 edid
        if (lt6911_edid_config(edid_data, edid_size) != 0) {
            fprintf(stderr, "Failed to configure LT6911 EDID\n");
            status = EXIT_FAILURE;
        }
    }
    if (status == EXIT_SUCCESS && board->has_hdmi_reset && NanoKVM_HDMI_Reset(board) != 0) {
        fprintf(stderr, "Failed to reset the LT6911 (%s)\n", board->hdmi_reset_line);
        status = EXIT_FAILURE;
    }

    sess_err = lt6911_session_end(&lt6911, chip_touched);
    if (sess_err != 0) {
        fprintf(stderr, "Failed to release the LT6911 bus or tell the server (%s)\n", strerror(-sess_err));
        status = EXIT_FAILURE;
    }
    lt6911_close(&lt6911);
    if (status != EXIT_SUCCESS) return status;

    print_success(board);

    return EXIT_SUCCESS;
}
