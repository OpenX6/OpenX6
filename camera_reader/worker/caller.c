#define _GNU_SOURCE
#include <dlfcn.h>
#include <elf.h>
#include <errno.h>
#include <fcntl.h>
#include <link.h>
#include <linux/audit.h>
#include <linux/filter.h>
#include <linux/seccomp.h>
#include <stddef.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/prctl.h>
#include <sys/syscall.h>
#include <unistd.h>

static const char *test;

static uintptr_t base;
static int find_main(struct dl_phdr_info *i, size_t n, void *arg) {
    if (!i->dlpi_name[0]) { base = i->dlpi_addr; return 1; }
    return 0;
}
static void die(const char *msg) { perror(msg); _exit(120); }
static int named(const char *s) { return strcmp(test,s)==0; }
int InsBaseLogShouldLog(int p) { return 0; }
// GNU libstdc++ C++11 short-string object, arguments kept alive for the entire call.
struct sso { char *data; size_t len; char local[16]; };
static void string_arg(struct sso *s, const char *v) {
    s->len=strlen(v); if(s->len>=16) die("short string");
    s->data=s->local; memcpy(s->local,v,s->len+1);
}
#define PERMIT(name) BPF_JUMP(BPF_JMP|BPF_JEQ|BPF_K,__NR_##name,0,1), BPF_STMT(BPF_RET|BPF_K,SECCOMP_RET_ALLOW)
static void restrict_calls(void) {
    struct sock_filter filter[]={
      BPF_STMT(BPF_LD|BPF_W|BPF_ABS,offsetof(struct seccomp_data,arch)),
      BPF_JUMP(BPF_JMP|BPF_JEQ|BPF_K,AUDIT_ARCH_AARCH64,1,0),
      BPF_STMT(BPF_RET|BPF_K,SECCOMP_RET_KILL_PROCESS),
      BPF_STMT(BPF_LD|BPF_W|BPF_ABS,offsetof(struct seccomp_data,nr)),
      /* Only thread creation, for the vendor QSEE stack. All exec calls remain denied. */
      BPF_JUMP(BPF_JMP|BPF_JEQ|BPF_K,__NR_clone,0,4),
      BPF_STMT(BPF_LD|BPF_W|BPF_ABS,offsetof(struct seccomp_data,args[0])),
      BPF_JUMP(BPF_JMP|BPF_JSET|BPF_K,0x10000,0,1),
      BPF_STMT(BPF_RET|BPF_K,SECCOMP_RET_ALLOW),
      BPF_STMT(BPF_RET|BPF_K,SECCOMP_RET_ERRNO|EPERM),
      PERMIT(ioctl),PERMIT(faccessat),PERMIT(getdents64),PERMIT(rt_sigaction),
      PERMIT(rt_sigprocmask),PERMIT(gettimeofday),PERMIT(nanosleep),PERMIT(clock_nanosleep),
      PERMIT(set_robust_list),PERMIT(rseq),PERMIT(sched_getaffinity),PERMIT(getegid),
      PERMIT(getgid),PERMIT(getppid),PERMIT(uname),PERMIT(dup3),PERMIT(prlimit64),
      PERMIT(readlinkat),PERMIT(dup),PERMIT(openat),PERMIT(close),PERMIT(read),PERMIT(write),PERMIT(lseek),
      PERMIT(pread64),PERMIT(pwrite64),PERMIT(readv),PERMIT(writev),PERMIT(fsync),
      PERMIT(fdatasync),PERMIT(ftruncate),PERMIT(fstat),PERMIT(newfstatat),
      PERMIT(statx),PERMIT(getdents64),PERMIT(unlinkat),PERMIT(renameat),
      PERMIT(renameat2),PERMIT(brk),PERMIT(mmap),PERMIT(mprotect),PERMIT(munmap),
      PERMIT(mremap),PERMIT(madvise),PERMIT(futex),PERMIT(clock_gettime),
      PERMIT(getrandom),PERMIT(getpid),PERMIT(gettid),PERMIT(getuid),PERMIT(geteuid),
      PERMIT(fcntl),PERMIT(rt_sigreturn),PERMIT(exit_group),PERMIT(exit),
      BPF_STMT(BPF_RET|BPF_K,SECCOMP_RET_ERRNO|EPERM)};
    struct sock_fprog p={sizeof(filter)/sizeof(*filter),filter};
    if(prctl(PR_SET_NO_NEW_PRIVS,1,0,0,0)||prctl(PR_SET_SECCOMP,SECCOMP_MODE_FILTER,&p)) die("seccomp");
}
#include <signal.h>
#include <ucontext.h>
static void badcall(int sig,siginfo_t *info,void *u) {
    /* Record only a syscall number, never pointers, data or registers. */
    char msg[48];int n=snprintf(msg,sizeof msg,"BLOCKED %d\n",info->si_syscall);
    write(3,msg,n);
    ((ucontext_t*)u)->uc_mcontext.regs[0] = -EPERM;
}
static int entry(int argc, char **argv, char **env) {
    setvbuf(stdout,NULL,_IONBF,0); setvbuf(stderr,NULL,_IONBF,0);
    test=argv[1]; if (!test || !argv[2] || getuid()!=1000) die("isolation");
    dl_iterate_phdr(find_main,0); if (!base) die("missing PIE");
    struct sigaction sa={.sa_sigaction=badcall,.sa_flags=SA_SIGINFO};
    if(prctl(PR_SET_DUMPABLE,0) || sigaction(SIGSYS,&sa,0)) _exit(118); restrict_calls();
    if(named("guard")){
        int socket_denied=syscall(__NR_socket,2,1,0)<0;
        int exec_denied=syscall(__NR_execve,"/worker",argv,env)<0 && errno==EPERM;
        int input_ro=open("/in/official.bin",O_RDWR)<0;
        int no_disk=open("/dev/sda",O_RDWR)<0 && open("/data/test",O_CREAT|O_WRONLY,0600)<0;
        dprintf(3,"GUARD %d %d %d %d\n",socket_denied,exec_denied,input_ro,no_disk);
        _exit(!(socket_denied&&exec_denied&&input_ro&&no_disk));
    }
    if(named("public")){
        int (*initq)(const char*)=dlsym(RTLD_DEFAULT,"ins_qsee_init");
        int (*getpub)(void*,unsigned*)=dlsym(RTLD_DEFAULT,"ins_qsee_get_rsa_public_key");
        unsigned char der[1024];unsigned len=sizeof der;
        int i=initq(NULL);int r=i? -999:getpub(der,&len);
        dprintf(3,"PUBLIC init=%d rc=%d length=%u\n",i,r,len);_exit(i||r||len!=294);
    }
    // Seed the c9 configuration normally set by the service during startup.
    struct sso *mgr=*(struct sso**)(base+0x35e258);
    **(uint64_t**)(base+0x35ef98)=1;string_arg(mgr,"c9");
    ((unsigned char*)mgr)[0x38]=1;
    unsigned char meta[0x1200] __attribute__((aligned(16)))={0};
    struct sso source,dir;string_arg(&source,argv[2]);string_arg(&dir,"/out");
    int (*prepare)(void*,void*)=(void*)(base+0x183870);
    int (*split)(void*,void*,void*,void*)=(void*)(base+0x199460);
    unsigned char cancel=named("cancel");
    int prep=prepare(&source,meta);
    int rc=prep ? prep : split(&source,&dir,meta,&cancel);
    dprintf(3,"RESULT prep=%d rc=%d\n",prep,rc);
    fflush(stdout);
    _exit(rc ? 42 : 0); // one worker per case; no daemon teardown
}
typedef int (*start_main)(void*,int,char**,void*,void*,void*,void*);
int __libc_start_main(void *ignored,int argc,char **argv,void *init,void *fini,void *rt,void *sp) {
    start_main real=(start_main)dlsym(RTLD_NEXT,"__libc_start_main");
    if(!real) die("libc start");
    return real(entry,argc,argv,init,fini,rt,sp);
}
