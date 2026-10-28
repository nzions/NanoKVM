/**
 * @author neucrack@sipeed
 * @copyright Sipeed Ltd 2023-
 * @license Apache 2.0
 * @update 2023.9.8: Add framework, create this file.
 */

#include "maix_err.hpp"
#include <exception>
#include <assert.h>
#include <vector>

namespace maix::err
{
    // Both tables are used while other translation units are still running
    // their static initialisers (an Exception thrown from a global's
    // constructor calls to_str), so neither may be a namespace-scope object
    // with a dynamic initialiser: that read an unconstructed std::vector and
    // crashed the process at start. A constexpr array of literals needs no
    // initialiser, and the string is constructed on first use.
    static constexpr const char *err_str[] = {
        "No error",
        "Invalid arguments",
        "No memory",
        "Not implemented",
        "Not ready",
        "Not initialized",
        "Not opened",
        "Not permitted",
        "Re-open",
        "Busy",
        "Read error",
        "Write error",
        "Timeout",
        "Runtime error",
        "IO error",
        "Not found",
        "Already exist",
        "Buffer full",
        "Buffer empty",
        "Cancel",
        "Overflow",
    };

    static std::string &error_str_ref()
    {
        static std::string error_str = "No error";
        return error_str;
    }

    std::string& get_error()
    {
        return error_str_ref();
    }

    void set_error(const std::string &str)
    {
        error_str_ref() = str;
    }

    void set_error(const char *str)
    {
        error_str_ref() = str;
    }

    std::string to_str(Err e)
    {
        // size of err_str must equal to ERR_MAX
        static_assert(sizeof(err_str) / sizeof(err_str[0]) == ERR_MAX, "err_str must have one entry per Err");

        if(e < ERR_NONE || e >= ERR_MAX)
        {
            return "Invalid error code";
        }
        return err_str[e];
    }

    void check_raise(Err e, const std::string &msg)
    {
        if(e != ERR_NONE)
        {
            std::string err_str = to_str(e);
            if(msg.size() > 0)
            {
                err_str += ": " + msg + "\n";
            }
            throw Exception(err_str);
        }
    }

    void check_bool_raise(bool ok, const std::string &msg)
    {
        if(!ok)
        {
            std::string err_str = "Unknown error";
            if(msg.size() > 0)
            {
                err_str += ": " + msg + "\n";
            }
            throw Exception(err_str);
        }
    }

    void check_null_raise(void *ptr, const std::string &msg)
    {
        if(!ptr)
        {
            std::string err_str = "Value is NULL";
            if(msg.size() > 0)
            {
                err_str += ": " + msg + "\n";
            }
            throw Exception(err_str);
        }
    }

    Exception::Exception(const std::string &msg, err::Err code)
    {
        if (code == err::ERR_NONE)
        {
            _msg = "";
        }
        else
        {
            _msg = to_str(code);
        }
        if(msg.size() > 0)
        {
            _msg += ": " + msg + "\n";
        }
        _code = code;
    }

    Exception::Exception(err::Err code, const std::string &msg)
    {
        if (code == err::ERR_NONE)
        {
            _msg = "";
        }
        else
        {
            _msg = to_str(code);
        }
        if(msg.size() > 0)
        {
            _msg += ": " + msg + "\n";
        }
        _code = code;
    }

    const char *Exception::what() const throw()
    {
        return _msg.data();
    }

    err::Err Exception::code() const
    {
        return _code;
    }

} // namespace maix::err
