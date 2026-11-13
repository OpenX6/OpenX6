/* Read-only one-file FUSE filesystem. The sole backing file is on the Mac.
 * Unprivileged file workers cannot open the transport. No path is transmitted. */
#define _GNU_SOURCE
#include <linux/fuse.h>
#include <sys/stat.h>
#include <sys/mount.h>
#include <sys/socket.h>
#include <sys/time.h>
#include <sys/uio.h>
#include <netinet/in.h>
#include <arpa/inet.h>
#include <unistd.h>
#include <fcntl.h>
#include <stdint.h>
#include <stdio.h>
#include <string.h>
#include <stdlib.h>
#include <errno.h>
#define SIZE 1734100608ULL
#define WINDOW (4U<<20)
static unsigned char cache[WINDOW], buf[256*1024];
static uint64_t pos=SIZE; static unsigned cnt;
static int stream, fuse;
static int exact(int fd,void *b,size_t n,int sending) {
    size_t done=0; while(done<n) {ssize_t m=sending?write(fd,(char*)b+done,n-done):read(fd,(char*)b+done,n-done);
        if(m<=0)return -1; done+=(size_t)m;} return 0;
}
static void reply(uint64_t u,int err,const void *data,unsigned n) {
    struct fuse_out_header h={.len=sizeof(h)+n,.error=-err,.unique=u};
    struct iovec iov[2]={{&h,sizeof(h)},{(void*)data,n}};
    if(writev(fuse,iov,n?2:1)<0 && errno!=ENOENT)exit(20);
}
static void attr(struct fuse_attr *a,uint64_t ino){
    a->ino=ino; a->mode=ino==1?S_IFDIR|0555:S_IFREG|0444; a->nlink=ino==1?2:1;
    a->size=ino==1?0:SIZE; a->blocks=(a->size+511)/512; a->blksize=4096;
}
int main(int argc,char **v){
    if(argc!=5)return 1;
    struct sockaddr_in sa={.sin_family=AF_INET,.sin_port=htons(atoi(v[3]))};
    if(!inet_aton(v[2],&sa.sin_addr))return 2;
    stream=socket(AF_INET,SOCK_STREAM,0); struct timeval timeout={.tv_sec=30};
    setsockopt(stream,SOL_SOCKET,SO_RCVTIMEO,&timeout,sizeof(timeout));
    if(connect(stream,(void*)&sa,sizeof(sa)) || exact(stream,v[4],strlen(v[4]),1))return 3;
    fuse=open("/dev/fuse",O_RDWR);if(fuse<0)return 4;
    char opts[140];snprintf(opts,sizeof opts,"fd=%d,rootmode=40000,user_id=0,group_id=0,allow_other,default_permissions,max_read=131072",fuse);
    if(mount("x6-readonly-input",v[1],"fuse",MS_RDONLY|MS_NOSUID|MS_NODEV|MS_NOEXEC,opts))return 5;
    for(;;){
        int n=read(fuse,buf,sizeof buf);if(n<0 && errno==EINTR)continue;if(n<(int)sizeof(struct fuse_in_header))break;
        struct fuse_in_header *h=(void*)buf;void *p=h+1;uint64_t u=h->unique;
        switch(h->opcode){
        case FUSE_INIT:{struct fuse_init_in *i=p;struct fuse_init_out o={.major=7,.minor=23,.max_readahead=i->max_readahead,.max_write=131072,.time_gran=1};reply(u,0,&o,sizeof o);break;}
        case FUSE_LOOKUP:{if(h->nodeid!=1 || strcmp(p,"official.bin")){reply(u,ENOENT,0,0);break;}
            struct fuse_entry_out o={.nodeid=2,.generation=1,.entry_valid=600,.attr_valid=600};attr(&o.attr,2);reply(u,0,&o,sizeof o);break;}
        case FUSE_GETATTR:{struct fuse_attr_out o={.attr_valid=600};attr(&o.attr,h->nodeid);reply(u,0,&o,sizeof o);break;}
        case FUSE_OPEN:{struct fuse_open_in *i=p;struct fuse_open_out o={.fh=1,.open_flags=FOPEN_KEEP_CACHE}; if((i->flags&O_ACCMODE)!=O_RDONLY)reply(u,EROFS,0,0);else reply(u,0,&o,sizeof o);break;}
        case FUSE_READ:{struct fuse_read_in *i=p;uint64_t off=i->offset;unsigned want=i->size;
            if(off>=SIZE){reply(u,0,0,0);break;}if(want>SIZE-off)want=SIZE-off;
            if(off<pos || off+want>pos+cnt){
                pos=off;cnt=SIZE-pos>WINDOW?WINDOW:(unsigned)(SIZE-pos);
                uint64_t request[2]={pos,cnt};
                if(exact(stream,request,sizeof request,1)||exact(stream,cache,cnt,0)){reply(u,EIO,0,0);return 6;}
            }reply(u,0,cache+off-pos,want);break;}
        case FUSE_RELEASE:case FUSE_FLUSH:case FUSE_ACCESS:reply(u,0,0,0);break;
        case FUSE_FORGET:case FUSE_BATCH_FORGET:break;
        default:reply(u,ENOSYS,0,0);}}
    return 0;
}
