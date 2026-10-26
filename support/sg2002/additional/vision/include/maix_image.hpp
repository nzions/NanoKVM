/**
 * @author neucrack@sipeed, lxowalle@sipeed
 * @copyright Sipeed Ltd 2023-
 * @license Apache 2.0
 * @update 2023.9.8: Add framework, create this file.
 *
 * NanoKVM cut of MaixCDK's components/vision/include/maix_image.hpp: only the
 * frame holder the KVM capture path uses is kept (constructors, data access,
 * to_jpeg). Drawing, filtering, file I/O and fonts are gone with their OpenCV
 * and FreeType dependencies; libkvm.so links no OpenCV.
 */

#pragma once

#include "maix_tensor.hpp"
#include "maix_log.hpp"
#include "maix_err.hpp"
#include "maix_fs.hpp"
#include "maix_image_def.hpp"
#include "maix_type.hpp"
#include <stdlib.h>

namespace maix::image
{

    /**
     * Image class: one captured frame
     */
    class Image
    {
    public:
        /**
         * @param width image width, should > 0
         * @param height image height, should > 0
         * @param format image format @see image::Format
         */
        Image(int width, int height, image::Format format = image::Format::FMT_RGB888);

        /**
         * @param data image data, if data is nullptr, will malloc memory for image data
         * If the image is in jpeg format, data must be filled in.
         * @param data_size image data size, only for compressed format like jpeg, data_size must be filled in, or should be -1, default is -1.
         * @param copy if true and data is not nullptr, will copy data to new buffer, else will use data directly.
         */
        Image(int width, int height, image::Format format, uint8_t *data, int data_size, bool copy);

        ~Image();

        image::Format format() { return this->_format; }

        /** Get image's data size */
        int data_size() { return _data_size; }

        int width() { return _width; }

        int height() { return _height; }

        void *data(){ return _data; }

        /**
         * Encode to a JPEG image with the hardware encoder. Only the FMT_YVU420SP
         * frames the KVM camera produces are supported.
         * @param quality JPEG quality
         * @return JPEG Image, nullptr if the format is unsupported or the encode failed
         */
        image::Image *to_jpeg(int quality = 95);

    private:
        void *_actual_data = nullptr;
        void *_data = nullptr;
        int _width = 0;
        int _height = 0;
        int _data_size = 0;
        Format _format = image::Format::FMT_INVALID;
        bool _is_malloc = false;

        void _create_image(int width, int height, image::Format format, uint8_t *data, int data_size, bool copy);
    }; // class Image
} // namespace maix::image
