/* Limit the codec's raw input slots only for this FreeFrame recording profile.
 * The output-buffer ioctl interposer is compiled from the installed layer's
 * original source. Use the kernel directly for these read-only format queries. */
#define _GNU_SOURCE
#include <dlfcn.h>
#include <errno.h>
#include <linux/videodev2.h>
#include <stdbool.h>
#include <stdint.h>
#include <stdio.h>
#include <string.h>
#include <sys/syscall.h>
#include <unistd.h>

#define GET_MAX "_ZN3qc213QC2V4l2Driver14getMaxBufCountENS_4v4l28PortTypeERj"
#define CONFIG_INSTANCE "_ZZN3ins4base9SingletonINS_6mediad13ConfigManagerEE11GetInstanceEvE8instance"
#define CONFIG_GUARD "_ZGVZN3ins4base9SingletonINS_6mediad13ConfigManagerEE11GetInstanceEvE8instance"
#define PROFILE_GETTER "_ZN3ins6mediad13ConfigManager20GetUserProfileConfigEv"

static const uint64_t sat_freeframe = ((uint64_t)6 << 32) | 2;
static const uint8_t *config_guard;
static const uint64_t *profile_key;

__attribute__((constructor))
static void find_profile(void) {
    const uint32_t *getter = dlsym(RTLD_DEFAULT, PROFILE_GETTER);
    const char *manager = dlsym(RTLD_DEFAULT, CONFIG_INSTANCE);
    /* Exact load in the ABI-checked GetUserProfileConfig. Read the aligned
     * pair in one instruction, without re-entering the configuration mutex
     * from a codec worker during camera setup. */
    if (manager && getter && getter[0x40/4] == 0xf9456a73) {
        profile_key = (const uint64_t *)(manager + 0xad0);
        config_guard = dlsym(RTLD_DEFAULT, CONFIG_GUARD);
    }
}

static bool square_freeframe(uint64_t profile, int port, unsigned previous,
                            const struct v4l2_format *raw, const struct v4l2_format *coded) {
    return profile == sat_freeframe && port == V4L2_BUF_TYPE_VIDEO_OUTPUT_MPLANE &&
           previous == 16 && raw->fmt.pix_mp.width == 7680 && raw->fmt.pix_mp.height == 7680 &&
           coded->fmt.pix_mp.width == 7680 && coded->fmt.pix_mp.height == 7680 &&
           coded->fmt.pix_mp.pixelformat == V4L2_PIX_FMT_HEVC;
}

typedef int (*vendor_count)(void *, int, unsigned *);
int freeframe_input_count(void *, int, unsigned *) __asm__(GET_MAX);
int freeframe_input_count(void *driver, int port, unsigned *count) {
    void *lib = dlopen("/usr/lib/libqcodec2_v4l2codec.so", RTLD_NOW | RTLD_NOLOAD);
    vendor_count original = lib ? (vendor_count)dlsym(lib, GET_MAX) : NULL;
    if (!original) original = (vendor_count)dlsym(RTLD_NEXT, GET_MAX);
    if (!original || original == freeframe_input_count) {
        if (lib) dlclose(lib);
        return ENOSYS;
    }
    int status = original(driver, port, count);
    int saved_errno = errno;
    if (status == 0 && driver && count && *count == 16 && port == V4L2_BUF_TYPE_VIDEO_OUTPUT_MPLANE &&
        config_guard && (__atomic_load_n(config_guard, __ATOMIC_ACQUIRE) & 1)) {
        uint64_t profile = __atomic_load_n(profile_key, __ATOMIC_ACQUIRE);
        if (profile == sat_freeframe) {
            int fd;
            memcpy(&fd, (char *)driver + 0xcc, sizeof(fd));
            struct v4l2_format raw = {.type = V4L2_BUF_TYPE_VIDEO_OUTPUT_MPLANE};
            struct v4l2_format coded = {.type = V4L2_BUF_TYPE_VIDEO_CAPTURE_MPLANE};
            struct v4l2_control minimum = {.id = V4L2_CID_MIN_BUFFERS_FOR_OUTPUT};
            if (fd >= 0 && syscall(SYS_ioctl, fd, VIDIOC_G_FMT, &raw) == 0 &&
                syscall(SYS_ioctl, fd, VIDIOC_G_FMT, &coded) == 0 &&
                square_freeframe(profile, port, *count, &raw, &coded) &&
                syscall(SYS_ioctl, fd, VIDIOC_G_CTRL, &minimum) == 0 && minimum.value == 7) {
                *count = 8;
                fprintf(stderr, "X6 FreeFrame 7680 HEVC input 16 -> 8 (driver minimum 7)\n");
            }
        }
    }
    if (lib) dlclose(lib);
    errno = saved_errno;
    return status;
}
