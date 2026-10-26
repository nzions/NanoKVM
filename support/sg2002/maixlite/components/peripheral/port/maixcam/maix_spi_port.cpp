/**
 * @author neucrack@sipeed
 * @copyright Sipeed Ltd 2024-
 * @license Apache 2.0
 * @update 2025.8.7: update this file.
 */

 #include <string>
 #include <regex>
 #include <cctype>
 #include "maix_err.hpp"

 namespace maix::peripheral::spi
 {

    int maix_spi_port_get_default_hw_cs(int spi_num) {
        switch(spi_num)
        {
            case 2:
            case 4:
                return 0;
            default:
                throw err::Exception("invalid spi_num");
        }
    }

 }; // namespace maix::peripheral::gpio
