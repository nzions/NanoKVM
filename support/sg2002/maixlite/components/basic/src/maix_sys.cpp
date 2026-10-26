/**
 * @author neucrack@sipeed
 * @copyright Sipeed Ltd 2023-
 * @license Apache 2.0
 * @update 2023.9.8: Add framework, create this file.
 */
#include <sys/statvfs.h>
#include <sys/ioctl.h>
#include <fcntl.h>
#include <linux/fs.h>
#include <sys/types.h>
#include <ifaddrs.h>
#include <netinet/in.h>
#include <net/if.h>
#include <arpa/inet.h>
#include <unistd.h>
#include <netdb.h>
#include <string>
#include <vector>
#include <map>
#include <fstream>
#include <sstream>
#include <iomanip>
#include <iterator>
#include <iostream>
#include "maix_basic.hpp"

#define LIB_VERSION_FILE_PATH "/maixapp/maixcam_lib.version"

namespace maix::sys
{
    std::string os_version()
    {
        FILE *file = fopen("/boot/ver", "r");
        if (!file)
        {
            log::error("Cannot open /boot/ver");
            return "Unkonwn";
        }
        char line[128];
        if (fgets(line, sizeof(line), file))
        {
            fclose(file);
            // strip start and end space
            std::string ver = line;
            ver.erase(0, ver.find_first_not_of(" \t\n"));
            ver.erase(ver.find_last_not_of(" \t\n") + 1);
            return ver;
        }
        fclose(file);
        return "Unkonwn";
    }

    std::string _get_maixpy_version_from_file(const std::string &version_path)
    {
        std::ifstream version_file(version_path);
        // fast way, read /usr/lib/python3.11/site-packages/maix/version.py
        // find key value:
        //      version_major = 4
        //      version_minor = 4
        //      version_patch = 20
        // return $version_major.$version_minor.$version_patch
        if (!version_file.is_open())
        {
            log::warn("Failed to open version file.");
            return "";
        }

        std::string line;
        int version_major = -1, version_minor = -1, version_patch = -1;

        while (std::getline(version_file, line))
        {
            if (line.find("version_major") != std::string::npos)
            {
                std::string num = line.substr(line.find("=") + 1);
                version_major = std::stoi(num);
            }
            else if (line.find("version_minor") != std::string::npos)
            {
                std::string num = line.substr(line.find("=") + 1);
                version_minor = std::stoi(num);
            }
            else if (line.find("version_patch") != std::string::npos)
            {
                std::string num = line.substr(line.find("=") + 1);
                version_patch = std::stoi(num);
            }
            if (version_major >= 0 && version_minor >= 0 && version_patch >= 0)
                break;
        }

        version_file.close();

        if (version_major == -1 || version_minor == -1 || version_patch == -1)
        {
            log::warn("Version information incomplete or not found.");
            return "";
        }

        std::ostringstream version_stream;
        version_stream << version_major << "." << version_minor << "." << version_patch;
        return version_stream.str();
    }

    std::string maixpy_version()
    {
#if PLATFORM_MAIXCAM
        return _get_maixpy_version_from_file("/usr/lib/python3.11/site-packages/maix/version.py");
#elif PLATFORM_MAIXCAM2
        return _get_maixpy_version_from_file("/usr/local/lib/python3.13/site-packages/maix/version.py");
#else
        log::warn("maixpy_version() not implemented for this platform");
        return "";
#endif

    }

    std::string runtime_version()
    {
        fs::File *file = fs::open(LIB_VERSION_FILE_PATH, "r");
        if (!file)
        {
            return "";
        }
        std::string *version = file->readline();
        if(!version)
        {
            delete file;
            return "";
        }
        std::string curr_version = *version;

        // 去掉头尾的空字符
        const std::string whitespace = " \t\n\r";
        size_t start = curr_version.find_first_not_of(whitespace);
        size_t end = curr_version.find_last_not_of(whitespace);

        if (start != std::string::npos && end != std::string::npos)
        {
            curr_version = curr_version.substr(start, end - start + 1);
        }
        else
        {
            curr_version = ""; // 如果全是空白字符
        }

        delete version;
        file->close();
        delete file;
        return curr_version;
    }

    static std::map<std::string, std::string> _device_configs;
    std::map<std::string, std::string> device_configs(bool cache)
    {
        if (cache && !_device_configs.empty())
            return _device_configs;

        if (!fs::exists("/boot/board"))
        {
            return _device_configs;
        }
        fs::File *f = fs::open("/boot/board", "r");
        if (!f)
        {
            throw err::Exception(err::ERR_ARGS, "open /boot/board failed");
        }

        _device_configs.clear();
        while (1)
        {
            std::string line;
            int num = f->readline(line);
            if (num <= 0)
                break;

            line.erase(0, line.find_first_not_of(" \t\r\n"));
            line.erase(line.find_last_not_of(" \t\r\n") + 1);

            size_t pos = line.find('=');
            if (pos != std::string::npos)
            {
                std::string key = line.substr(0, pos);
                std::string value = line.substr(pos + 1);

                key.erase(0, key.find_first_not_of(" \t\r\n"));
                key.erase(key.find_last_not_of(" \t\r\n") + 1);
                value.erase(0, value.find_first_not_of(" \t\r\n"));
                value.erase(value.find_last_not_of(" \t\r\n") + 1);

                _device_configs[key] = value;
            }
        }
        delete f;

        // 检查是否有 id
        if (_device_configs.find("id") == _device_configs.end())
        {
            throw err::Exception(err::ERR_ARGS, "/boot/board missing 'id' item");
        }

        // 读取 /boot/boards/board.{id}
        std::string id = _device_configs["id"];
        std::string board_file_path = "/boot/boards/board." + id;
        if (fs::exists(board_file_path))
        {
            fs::File *board_file = fs::open(board_file_path, "r");
            if (!board_file)
            {
                log::error(("open " + board_file_path + " failed").c_str());
                return _device_configs;
            }

            while (1)
            {
                std::string line;
                int num = board_file->readline(line);
                if (num <= 0)
                    break;

                line.erase(0, line.find_first_not_of(" \t\r\n"));
                line.erase(line.find_last_not_of(" \t\r\n") + 1);

                size_t pos = line.find('=');
                if (pos != std::string::npos)
                {
                    std::string key = line.substr(0, pos);
                    std::string value = line.substr(pos + 1);

                    key.erase(0, key.find_first_not_of(" \t\r\n"));
                    key.erase(key.find_last_not_of(" \t\r\n") + 1);
                    value.erase(0, value.find_first_not_of(" \t\r\n"));
                    value.erase(value.find_last_not_of(" \t\r\n") + 1);

                    // 如果 /boot/board 没有该键值，则从 board.{id} 补充
                    if (_device_configs.find(key) == _device_configs.end())
                    {
                        _device_configs[key] = value;
                    }
                }
            }
            delete board_file;
        }
        else
        {
            log::warn(("Board config not found: " + board_file_path).c_str());
        }

        return _device_configs;
    }

    static std::string _device_id;
    std::string device_id(bool cache)
    {
        if(cache && !_device_id.empty())
        {
            return _device_id;
        }
        FILE *file = NULL;
        char line[128];
        std::string model = "";
        device_configs();
        file = fopen("/proc/device-tree/model", "r");
        if (file)
        {
            if (fgets(line, sizeof(line), file))
            {
                model = line;
                model.erase(0, model.find_first_not_of(" \t\n"));
                model.erase(model.find_last_not_of(" \t\n") + 1);
#if PLATFORM_MAIXCAM
                // if(model.find("MaixCAM") != std::string::npos || model.find("LicheeRv Nano") != std::string::npos)
                // 包含 maixcam 或者 licheerv nano (不区分大小写，先把 model 转换为小写)
                std::string model_lower = model;
                std::transform(model.begin(), model.end(), model_lower.begin(), ::tolower);
                if (model_lower.find("maixcam") != std::string::npos || model_lower.find("licheerv nano") != std::string::npos)
                {
                    fclose(file);
                    auto it = _device_configs.find("id");
                    if (it != _device_configs.end()) {
                        _device_id = it->second;
                    }
                    else
                        _device_id = "maixcam";
                    return _device_id;
                }
#else
                fclose(file);
                std::string model_lower = model;
                std::transform(model.begin(), model.end(), model_lower.begin(), ::tolower);
                _device_id = model_lower;
                return _device_id;
#endif
            }
            fclose(file);
        }
        std::string model_lower = model;
        std::transform(model.begin(), model.end(), model_lower.begin(), ::tolower);
        _device_id = model_lower;
        return _device_id;
    }

    static std::string _device_name;
    std::string device_name(bool cache)
    {
        if(cache && !_device_name.empty())
        {
            return _device_name;
        }
        FILE *file = NULL;
        char line[128];
        std::string model = "unknown";
        file = fopen("/proc/device-tree/model", "r");
        if (file)
        {
            if (fgets(line, sizeof(line), file))
            {
                model = line;
                model.erase(0, model.find_first_not_of(" \t\n"));
                model.erase(model.find_last_not_of(" \t\n") + 1);
#if PLATFORM_MAIXCAM || PLATFORM_MAIXCAM2
                // if(model.find("MaixCAM") != std::string::npos || model.find("LicheeRv Nano") != std::string::npos)
                // 包含 maixcam 或者 licheerv nano (不区分大小写，先把 model 转换为小写)
                std::string model_lower = model;
                std::transform(model.begin(), model.end(), model_lower.begin(), ::tolower);
                if (model_lower.find("maixcam2") != std::string::npos)
                {
                    fclose(file);
                    auto it = _device_configs.find("name");
                    if (it != _device_configs.end()) {
                        _device_name = it->second;
                    }
                    else
                        _device_name = "MaixCAM2";
                    return _device_name;
                }
                else if (model_lower.find("maixcam") != std::string::npos || model_lower.find("licheerv nano") != std::string::npos)
                {
                    fclose(file);
                    auto it = _device_configs.find("name");
                    if (it != _device_configs.end()) {
                        _device_name = it->second;
                    }
                    else
                        _device_name = "MaixCAM";
                    return _device_name;
                }
#else
                fclose(file);
                return model;
#endif
            }
            fclose(file);
        }
        _device_name = model;
        return model;
    }

    std::string host_name()
    {
        char hostname[256];
        if (gethostname(hostname, sizeof(hostname)) == 0)
        {
            return std::string(hostname);
        }
        return "";
    }

    std::string host_domain()
    {
        std::string host = host_name();
        if (host.empty())
            return "";
        return host + ".local";
    }

    std::map<std::string, std::string> ip_address()
    {
        // get all ip addresses of all network interfaces
        std::map<std::string, std::string> ips;
        struct ifaddrs *ifaddr, *ifa;
        int s;
        char host[NI_MAXHOST];

        if (getifaddrs(&ifaddr) == -1)
        {
            log::error("getifaddrs failed");
            return ips;
        }

        for (ifa = ifaddr; ifa != NULL; ifa = ifa->ifa_next)
        {
            if (ifa->ifa_addr == NULL)
                continue;

            s = getnameinfo(ifa->ifa_addr, sizeof(struct sockaddr_in), host, NI_MAXHOST, NULL, 0, NI_NUMERICHOST);

            if ((strcmp(ifa->ifa_name, "lo") != 0) && (ifa->ifa_addr->sa_family == AF_INET))
            {
                if (s != 0)
                {
                    log::error("getnameinfo() failed: %s", gai_strerror(s));
                    freeifaddrs(ifaddr);
                    return ips;
                }
                ips[ifa->ifa_name] = host;
            }
        }
        freeifaddrs(ifaddr);
        return ips;
    }

    std::map<std::string, std::string> mac_address()
    {
        struct ifaddrs *ifaddr, *ifa;
        std::map<std::string, std::string> result;

        if (getifaddrs(&ifaddr) == -1)
        {
            log::error("getifaddrs failed");
            exit(EXIT_FAILURE);
        }

        for (ifa = ifaddr; ifa != NULL; ifa = ifa->ifa_next)
        {
            if (ifa->ifa_addr == NULL)
                continue;

            if ((strcmp(ifa->ifa_name, "lo") != 0) && (ifa->ifa_addr->sa_family == AF_INET))
            {
                int fd = socket(AF_INET, SOCK_DGRAM, 0);
                struct ifreq ifr;
                ifr.ifr_addr.sa_family = AF_INET;
                strncpy(ifr.ifr_name, ifa->ifa_name, IFNAMSIZ - 1);

                if (ioctl(fd, SIOCGIFHWADDR, &ifr) != -1)
                {
                    char mac[18];
                    unsigned char *hwaddr = (unsigned char *)ifr.ifr_hwaddr.sa_data;
                    snprintf(mac, sizeof(mac), "%02x:%02x:%02x:%02x:%02x:%02x", hwaddr[0], hwaddr[1], hwaddr[2], hwaddr[3], hwaddr[4], hwaddr[5]);
                    result[ifa->ifa_name] = mac;
                }

                close(fd);
            }
        }
        freeifaddrs(ifaddr);
        return result;
    }

    std::string device_key()
    {
        FILE *file = fopen("/device_key", "r");
        if (!file)
        {
            log::error("Cannot open /device_key");
            return "";
        }
        char line[128];
        if (fgets(line, sizeof(line), file))
        {
            fclose(file);
            // strip start and end space
            std::string key = line;
            key.erase(0, key.find_first_not_of(" \t\n"));
            key.erase(key.find_last_not_of(" \t\n") + 1);
            return key;
        }
        fclose(file);
        return "";
    }

    std::map<std::string, int64_t> memory_info()
    {
        std::map<std::string, int64_t> res;
        FILE *file = fopen("/proc/meminfo", "r");
        if (!file)
        {
            log::error("Cannot open /proc/meminfo");
            return res;
        }

        unsigned long total_memory = 0;
        unsigned long free_memory = 0;
        char line[256];

        while (fgets(line, sizeof(line), file))
        {
            if (sscanf(line, "MemTotal: %lu kB", &total_memory) == 1)
            {
                // printf("Total memory: %lu kB\n", total_memory);
            }
            if (sscanf(line, "MemAvailable: %lu kB", &free_memory) == 1)
            {
                // printf("MemAvailable memory: %lu kB\n", free_memory);
                break;
            }
        }

        fclose(file);

        res["cmm_total"] = 0; // Board or Chip contiguous memory management area. For MaixCAM is IOA, for MaixCAM2 is CMM.
        res["cmm_used"] = 0;
        res["cma_total"] = 0;    // Contiguous Memory Allocator (Linux CMA standard)
        res["cma_used"] = 0;
        res["used"] = (total_memory - free_memory) * 1024;
        res["total"] = total_memory * 1024;
#if PLATFORM_MAIXCAM
        res["hw_total"] = 256 * 1024 * 1024;
        file = fopen("/sys/kernel/debug/ion/cvi_carveout_heap_dump/alloc_mem", "r");
        if (!file)
        {
            log::warn("Cannot open /sys/kernel/debug/ion/cvi_carveout_heap_dump/alloc_mem");
        }
        else
        {
            unsigned long used_cmm_memory = 0;
            while (fgets(line, sizeof(line), file))
            {
                int i = 0;
                while(1)
                {
                    if(line[i] != ' ' && line[i] != '\t')
                        break;
                    ++i;
                }
                if (sscanf(line + i, "%ld", &used_cmm_memory) == 1)
                {
                    break;
                }
            }

            fclose(file);
            res["cmm_used"] = used_cmm_memory;
        }
        file = fopen("/sys/kernel/debug/ion/cvi_carveout_heap_dump/total_mem", "r");
        if (!file)
        {
            log::warn("Cannot open /sys/kernel/debug/ion/cvi_carveout_heap_dump/total_mem");
        }
        else
        {
            unsigned long total_cmm_memory = 0;
            while (fgets(line, sizeof(line), file))
            {
                int i = 0;
                while(1)
                {
                    if(line[i] != ' ' && line[i] != '\t')
                        break;
                    ++i;
                }
                if (sscanf(line + i, "%ld", &total_cmm_memory) == 1)
                {
                    break;
                }
            }

            fclose(file);
            res["cmm_total"] = total_cmm_memory;
        }
#elif PLATFORM_MAIXCAM2
        file = fopen("/proc/ax_proc/mem_cmm_info", "r");
        if (!file)
        {
            log::error("Cannot open /proc/ax_proc/mem_cmm_info");
            return res;
        }

        unsigned long total_cmm_memory = 0;
        unsigned long used_cmm_memory = 0;
        while (fgets(line, sizeof(line), file))
        {
            int i = 0;
            while(1)
            {
                if(line[i] != ' ' && line[i] != '\t')
                    break;
                ++i;
            }
            if (sscanf(line + i, "total size=%ldKB%*[^u]used=%ldKB", &total_cmm_memory, &used_cmm_memory) == 2)
            {
                break;
            }
        }

        fclose(file);
        res["cmm_total"] = total_cmm_memory * 1024;
        res["cmm_used"] = used_cmm_memory * 1024;
        res["hw_total"] = res["total"] + total_cmm_memory * 1024;
        if (res["hw_total"] > (int64_t)3.9 * 1024 * 1024 * 1024 && res["hw_total"] < (int64_t)4.1 * 1024 * 1024 * 1024)
            res["hw_total"] = (int64_t)4 * 1024 * 1024 * 1024; // 4G version
        else if (res["hw_total"] > 0.9 * 1024 * 1024 * 1024 && res["hw_total"] < 1.1 * 1024 * 1024 * 1024)
            res["hw_total"] = 1 * 1024 * 1024 * 1024; // 1G version
#else
        res["hw_total"] = res["total"];
#endif
        return res;
    }

    std::string bytes_to_human(unsigned long long bytes, int precision, int base, const std::vector<std::string> &units, const std::string &sep)
    {
        static const std::vector<std::string> default_units = {
            "B", "KiB", "MiB", "GiB", "TiB", "PiB", "EiB", "ZiB", "YiB"
        };
        const std::vector<std::string>& actual_units = units.empty() ? default_units : units;
        size_t unit_index = 0;
        double bytes_double = static_cast<double>(bytes);

        while (bytes_double >= base && unit_index < actual_units.size())
        {
            bytes_double /= base;
            ++unit_index;
        }

        std::ostringstream out;
        out << std::fixed << std::setprecision(precision) << bytes_double << sep << actual_units[unit_index];
        return out.str();
    }

    std::map<std::string, unsigned long> cpu_freq()
    {
        static unsigned long last_freq = 0;
        std::map<std::string, unsigned long> res;
        res["cpu0"] = last_freq;
#if PLATFORM_MAIXCAM
        /* format:
                                 enable  prepare  protect                                duty
        clock                          count    count    count        rate   accuracy phase  cycle
        ---------------------------------------------------------------------------------------------
        audio_clock                          0        0        0    24576000          0     0  50000
        i2s_mclk                             2        2        0    24576000          0     0  50000
        eth_ptpclk                           1        1        0    50000000          0     0  50000
        */
        FILE *file = fopen("/sys/kernel/debug/clk/clk_summary", "r");
        if (!file)
        {
            perror("Cannot open /sys/kernel/debug/clk/clk_summary");
            return res;
        }
        // read lines, find clk_c906_0 and clk_tpu
        char line[256];
        while (fgets(line, sizeof(line), file))
        {
            char *idx = strstr(line, "clk_c906_0");
            if (idx)
            {
                unsigned long freq;
                if (sscanf(idx, "clk_c906_0 %*d %*d %*d %lu", &freq) == 1)
                {
                    fclose(file);
                    res["cpu0"] = freq;
                    return res;
                }
            }
        }
        fclose(file);
#elif PLATFORM_MAIXCAM2
        int count = 0;
        while (!app::need_exit()) {
            std::string freq;
            auto path = "/sys/devices/system/cpu/cpu" + std::to_string(count) + "/cpufreq/cpuinfo_cur_freq";
            if (std::getline(std::ifstream(path), freq)) {
                res["cpu" + std::to_string(count)] = atoi(freq.c_str()) * 1000;
            } else {
                break;
            }
            ++count;
        }
#else
        std::ifstream cpuinfo("/proc/cpuinfo");
        std::string line;
        int count = 0;
        while (std::getline(cpuinfo, line))
        {
            if (line.substr(0, 9) == "processor")
            {
                ++count;
            }
            else if (line.substr(0, 7) == "cpu MHz")
            {
                size_t pos = line.find(':');
                if (pos != std::string::npos)
                {
                    unsigned long freq = std::stoul(line.substr(pos + 1)) * 1000000; // MHz to Hz
                    res["cpu" + std::to_string(count)] = freq;
                }
            }
        }
#endif
        last_freq = res["cpu0"];
        return res;
    }

    std::map<std::string, float> cpu_temp()
    {
        static unsigned long last_temp = 0;
        std::map<std::string, float> res;
        res["cpu"] = last_temp;
        FILE *file = fopen("/sys/class/thermal/thermal_zone0/temp", "r");
        if (!file)
        {
            perror("Cannot open /sys/class/thermal/thermal_zone0/temp");
            return res;
        }
        int temp = 0;
        if (fscanf(file, "%d", &temp) == 1)
        {
            // printf("CPU temp: %d\n", temp);
        }
        fclose(file);
        res["cpu"] = temp / 1000.0;
        last_temp = res["cpu"];
        return res;
    }

    std::map<std::string, float> cpu_usage()
    {
        struct CpuTimes
        {
            long total;
            long idle;
        };

        static std::map<std::string, CpuTimes> prev; // 记住上一次的数据
        std::map<std::string, float> usage;
        bool first_time = prev.empty();

        // 读取 /proc/stat
        std::ifstream proc_stat("/proc/stat");
        std::string line;
        while (std::getline(proc_stat, line))
        {
            std::istringstream iss(line);
            std::vector<std::string> words((std::istream_iterator<std::string>(iss)),
                                           std::istream_iterator<std::string>());

            if (words[0].substr(0, 3) == "cpu")
            {
                long total_time = 0;
                for (size_t i = 1; i < words.size(); i++)
                    total_time += std::stol(words[i]);

                long idle_time = std::stol(words[4]) + std::stol(words[5]); // idle + iowait

                // 如果有上一次数据，就计算
                if (!first_time)
                {
                    long total_diff = total_time - prev[words[0]].total;
                    long idle_diff  = idle_time - prev[words[0]].idle;

                    if (total_diff > 0)
                    {
                        float cpu_percent = 100.0f * (total_diff - idle_diff) / total_diff;
                        usage[words[0]] = cpu_percent;
                    }
                    else
                    {
                        usage[words[0]] = 0.0f;
                    }
                }

                // 更新记录
                prev[words[0]] = {total_time, idle_time};
            }
        }

        if(first_time)
            return cpu_usage();
        return usage;
    }

    std::map<std::string, unsigned long> npu_freq()
    {
        static unsigned long last_freq = 0;
        std::map<std::string, unsigned long> res;
        res["npu0"] = last_freq;
#if PLATFORM_MAIXCAM
        /* format:
                                 enable  prepare  protect                                duty
        clock                          count    count    count        rate   accuracy phase  cycle
        ---------------------------------------------------------------------------------------------
        audio_clock                          0        0        0    24576000          0     0  50000
        i2s_mclk                             2        2        0    24576000          0     0  50000
        eth_ptpclk                           1        1        0    50000000          0     0  50000
        */
        FILE *file = fopen("/sys/kernel/debug/clk/clk_summary", "r");
        if (!file)
        {
            perror("Cannot open /sys/kernel/debug/clk/clk_summary");
            return res;
        }
        // read lines, find clk_c906_0 and clk_tpu
        char line[256];
        while (fgets(line, sizeof(line), file))
        {
            char *idx = strstr(line, "clk_tpu");
            if (idx)
            {
                unsigned long freq;
                if (sscanf(idx, "clk_tpu %*d %*d %*d %lu", &freq) == 1)
                {
                    fclose(file);
                    res["npu0"] = freq;
                    return res;
                }
            }
        }
        fclose(file);
#elif PLATFORM_MAIXCAM2
        std::string freq;
        auto path = "/proc/ax_proc/npu/clk";
        if (std::getline(std::ifstream(path), freq)) {
            freq.pop_back(); // remove 'M'
            res["npu0"] = atoi(freq.c_str()) * 1000000;
        }
#endif
        last_freq = res["npu0"];
        return res;
    }

#if PLATFORM_MAIXCAM2
    static void ensure_enable_npu_top()
    {
        std::ifstream fin("/proc/ax_proc/npu/enable");
        std::string content;
        if(fin.is_open())
        {
            std::getline(fin, content);
            fin.close();

            // 去除空白
            content.erase(content.begin(), std::find_if(content.begin(), content.end(), [](int ch){ return !std::isspace(ch); }));
            content.erase(std::find_if(content.rbegin(), content.rend(), [](int ch){ return !std::isspace(ch); }).base(), content.end());

            if(content == "1")
                return; // 已是1，无需写入
        }

        // 写入 "1"
        std::ofstream fout("/proc/ax_proc/npu/enable");
        if(fout.is_open())
        {
            fout << "1" << std::endl;
            fout.close();
        }
        else
        {
            log::error("Failed to open /proc/ax_proc/npu/enable for writing");
        }
    }
#endif

    std::map<std::string, float> npu_usage()
    {
        std::map<std::string, float> usage;
        usage["npu"] = -1;
#if PLATFORM_MAIXCAM2
        // ensure /proc/ax_proc/npu/enable content is 1
        ensure_enable_npu_top();
        // read /proc/ax_proc/npu/top, format is:
        /*  core:vnpu_1
            time:140
            period:1000000
            utilization:95%

            core:vnpu_2
            time:0
            period:1000000
            utilization:0%
            */
        // content can be "nputop info not updated over 127727 ms!"
        // if get_sys_config_kv(npu, ai_isp, 0) is 1, only read vnpu_1's utilization as npu and npu0 result.
        // else read vnpu_1 as npu0, vnpu_2 as npu1, average npu1 and npu0 as npu usage.
        bool ai_isp_enabled = (app::get_sys_config_kv("npu", "ai_isp", "0") == "1");
        std::ifstream fin("/proc/ax_proc/npu/top");
        if(!fin.is_open())
        {
            return usage;
        }

        std::string line;
        float vnpu_1_util = 0.0f;
        float vnpu_2_util = 0.0f;
        std::string current_core;

        while(std::getline(fin, line))
        {
            line.erase(line.begin(), std::find_if(line.begin(), line.end(), [](int ch) { return !std::isspace(ch); }));
            line.erase(std::find_if(line.rbegin(), line.rend(), [](int ch) { return !std::isspace(ch); }).base(), line.end());

            if(line.empty()) continue;

            if(line.find("core:") == 0)
            {
                size_t pos = line.find("vnpu");
                if(pos != std::string::npos)
                {
                    current_core = line.substr(pos);
                }
            }
            else if(line.find("utilization:") == 0)
            {
                std::string util_str = line.substr(12);
                if(!util_str.empty() && util_str.back() == '%')
                    util_str.pop_back();
                try
                {
                    float util_val = std::stof(util_str);
                    if(current_core == "vnpu_1")
                    {
                        vnpu_1_util = util_val;
                    }
                    else if(current_core == "vnpu_2")
                    {
                        vnpu_2_util = util_val;
                    }
                }
                catch(...)
                {
                    // 转换失败忽略
                }
            }
        }

        fin.close();

        if(!ai_isp_enabled)
        {
            usage["npu"] = vnpu_2_util > vnpu_1_util ? vnpu_2_util : vnpu_1_util ;
            usage["npu0"] = usage["npu"];
        }
        else
        {
            usage["npu0"] = vnpu_1_util;
            usage["npu1"] = vnpu_2_util;
            usage["npu"] = (vnpu_1_util + vnpu_2_util) / 2.0f;
        }
#endif
        return usage;
    }

    std::map<std::string, unsigned long long> disk_usage(const std::string &path)
    {
        std::map<std::string, unsigned long long> usage;
        struct statvfs stat;

        if (statvfs(path.c_str(), &stat) != 0)
        {
            // error happens, just return an empty map
            return usage;
        }
        usage["total"] = stat.f_blocks * stat.f_bsize;
        unsigned long long free = stat.f_bfree * stat.f_bsize;
        usage["used"] = usage["total"] - free;
        return usage;
    }

    std::vector<std::map<std::string, std::string>> disk_partitions(bool only_disk)
    {
        std::vector<std::map<std::string, std::string>> partitions;

        std::ifstream mounts("/proc/mounts");
        std::string line;
        while (std::getline(mounts, line))
        {
            std::istringstream iss(line);
            std::vector<std::string> words((std::istream_iterator<std::string>(iss)), std::istream_iterator<std::string>());

            // If only_disk is true, only add entries where device starts with "/dev/"
            if (only_disk && words[0].substr(0, 5) != "/dev/")
            {
                continue;
            }

            std::map<std::string, std::string> partition;
            partition["device"] = words[0];
            partition["mountpoint"] = words[1];
            partition["fstype"] = words[2];

            partitions.push_back(partition);
        }

        return partitions;
    }


    void reboot()
    {
        int ret = system("reboot");
        if (ret != 0)
        {
            log::error("reboot failed, ret: %d", ret);
            throw err::Exception(err::Err::ERR_RUNTIME, "reboot failed");
        }
    }

} // namespace maix::sys
