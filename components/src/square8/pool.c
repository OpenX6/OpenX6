/* Request larger real DMABUFs from the codec2 client on the two supported large square HEVC outputs.
 * Refuse undersized imports. Never enlarge a queued plane beyond its actual fd. */
#define _GNU_SOURCE
#include <errno.h>
#include <linux/videodev2.h>
#include <stdarg.h>
#include <stdatomic.h>
#include <stdio.h>
#include <sys/ioctl.h>
#include <sys/stat.h>
#include <sys/syscall.h>
#include <unistd.h>
static _Atomic unsigned extent_by_fd[4096];
static _Atomic unsigned logged_extent[4096];
static unsigned required_extent(unsigned width, unsigned height, unsigned format) {
    if (width != height || format != V4L2_PIX_FMT_HEVC) return 0;
    if (width == 7296) return 24u * 1024 * 1024;
    if (width == 7680) return 28u * 1024 * 1024;
    return 0;
}
int ioctl(int fd, unsigned long cmd, ...) {
    va_list args;
    va_start(args,cmd);
    void *p=va_arg(args,void*);
    va_end(args);
    if (fd<0 || fd>=4096 || !p) return syscall(SYS_ioctl,fd,cmd,p);
    unsigned extent=atomic_load(&extent_by_fd[fd]);
    if (cmd==VIDIOC_QBUF && extent) {
        struct v4l2_buffer *b=p;
        if (b->type==V4L2_BUF_TYPE_VIDEO_CAPTURE_MPLANE) {
            if (b->memory!=V4L2_MEMORY_DMABUF || !b->m.planes || !b->length) {
                fprintf(stderr,"SQUAREBUF unsupported memory %u\n",b->memory);
                errno=EINVAL;return -1;
            }
            struct v4l2_plane *plane=b->m.planes;
            struct stat info;
            if (fstat(plane->m.fd,&info) || info.st_size<extent ||
                plane->length<extent || plane->length>(unsigned long long)info.st_size) {
                fprintf(stderr,"SQUAREBUF refusing plane %u fd %d\n",plane->length,plane->m.fd);
                errno=EINVAL;return -1;
            }
            if (atomic_exchange(&logged_extent[fd],extent)!=extent)
                fprintf(stderr,"SQUAREBUF verified first import extent=%lld plane=%u\n",(long long)info.st_size,plane->length);
        }
    }
    int rc=syscall(SYS_ioctl,fd,cmd,p);
    if (!rc && (cmd==VIDIOC_S_FMT || cmd==VIDIOC_G_FMT || cmd==VIDIOC_TRY_FMT)) {
        struct v4l2_format *f=p;
        if (f->type==V4L2_BUF_TYPE_VIDEO_CAPTURE_MPLANE) {
            struct v4l2_pix_format_mplane *fmt=&f->fmt.pix_mp;
            unsigned need=required_extent(fmt->width,fmt->height,fmt->pixelformat);
            if (cmd!=VIDIOC_TRY_FMT) {
                if (atomic_exchange(&extent_by_fd[fd],need)!=need || cmd==VIDIOC_S_FMT)
                    atomic_store(&logged_extent[fd],0);
            }
            if (need && fmt->plane_fmt[0].sizeimage < need) {
                if (extent!=need)
                    fprintf(stderr,"SQUAREBUF %ux%u format %u -> %u\n",
                            fmt->width,fmt->height,fmt->plane_fmt[0].sizeimage,need);
                fmt->plane_fmt[0].sizeimage=need;
            }
        }
    }
    return rc;
}
