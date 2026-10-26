/**
 * @author neucrack@sipeed, lxowalle@sipeed
 * @copyright Sipeed Ltd 2023-
 * @license Apache 2.0
 * @update 2023.9.8: Add framework, create this file.
 *
 * NanoKVM cut of MaixCDK's components/vision/src/maix_image.cpp: construction,
 * destruction and the hardware JPEG encode of a YVU420SP frame. Everything that
 * needed OpenCV or FreeType (drawing, conversion, resize, load/save, fonts) is gone.
 */

#include "maix_image.hpp"
#include "kvm_mmf.hpp"
#include <string.h>

namespace maix::image
{

    void Image::_create_image(int width, int height, image::Format format, uint8_t *data, int data_size, bool copy)
    {
        _format = format;
        _width = width;
        _height = height;
        if (width <= 0 || height <= 0)
            throw err::Exception(err::ERR_ARGS, "image width and height should > 0");

        if (_format > Format::FMT_COMPRESSED_MIN) {
            if (!data || data_size < 0)
                throw err::Exception(err::ERR_ARGS,"image data and data_size are incorrect");
            _data_size = data_size;
        } else {
            // not use data_size, uncompressed image only use fiexed size.
            int size_calc = width * height * image::fmt_size[format];
            if(data_size > 0 && data_size != size_calc)
            {
                log::error("data_size not match image content size, data_size: %d, image content size: %d\n", data_size, size_calc);
                throw err::Exception(err::ERR_ARGS, "data_size not match image content size");
            }
            _data_size = size_calc;
        }

        if (!data)
        {
            _actual_data = malloc(_data_size + 0x1000);
            if (!_actual_data)
                throw err::Exception(err::ERR_NO_MEM, "malloc image data failed");
            _data = (void *)(((uint64_t)_actual_data + 0x1000) & ~0xFFF);
            _is_malloc = true;
        }
        else
        {
            if(!copy)
            {
                _data = data;
                _actual_data = _data;
                _is_malloc = false;
            }
            else
            {
                _actual_data = malloc(_data_size + 0x1000);
                if (!_actual_data)
                    throw std::bad_alloc();
                _data = (void *)(((uint64_t)_actual_data + 0x1000) & ~0xFFF);
                memcpy(_data, data, _data_size);
                _is_malloc = true;
            }
        }
    }

    Image::Image(int width, int height, image::Format format, uint8_t *data, int data_size, bool copy)
    {
        _create_image(width, height, format, data, data_size, copy);
    }

    Image::Image(int width, int height, image::Format format)
    {
        _create_image(width, height, format, nullptr, 0, false);
    }

    Image::~Image()
    {
        if (_is_malloc)
        {
            free(_actual_data);
            _actual_data = NULL;
            _data = NULL;
        }
    }

    image::Image *Image::to_jpeg(int quality)
    {
        if (_format != image::FMT_YVU420SP) {
            log::error("to_jpeg: only FMT_YVU420SP is supported, got format %d", (int)_format);
            return nullptr;
        }

        image::Image *img = nullptr;
        if (!mmf_enc_jpg_push_with_quality(0, (uint8_t *)_data, _width, _height, mmf_invert_format_to_mmf(image::FMT_YVU420SP), quality)) {
            uint8_t *data;
            int data_size;
            if (!mmf_enc_jpg_pop(0, &data, &data_size)) {
                img = new image::Image(_width, _height, image::Format::FMT_JPEG, data, data_size, true);
                mmf_enc_jpg_free(0);
            }
        }
        return img;
    }

} // namespace maix::image
